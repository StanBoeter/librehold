package game

import (
	"math"
	"sort"
)

// Bot farming: raids on barbarian and inactive villages.

var raidSpy func(botID PlayerID, c *Command, night bool, nextSessionAt int64)

// SpyRaids observes every bot raid (night = sent before leaving the game). Tests only; returns a restore func.
func SpyRaids(fn func(botID PlayerID, c *Command, night bool, nextSessionAt int64)) (restore func()) {
	prev := raidSpy
	raidSpy = fn
	return func() { raidSpy = prev }
}

func farmRadius(skill float64) int {
	return int(math.Round(BotConfig.FarmRadius.Weak + (BotConfig.FarmRadius.Strong-BotConfig.FarmRadius.Weak)*skill))
}

func isAvailable(w World, k *KnownVillage) bool {
	if k.LastAttackAt != nil && w.Now()-*k.LastAttackAt < BotDur(w, int64(BotConfig.FarmMinIntervalMin*float64(MsPerMinute))) {
		return false
	}
	if k.DeadSince != nil && w.Now()-*k.DeadSince < BotDur(w, int64(BotConfig.DeadFarmDays*float64(MsPerDay))) {
		return false
	}
	return true
}

// FarmCandidates are barbarians from the map, abandoned players, and active players half as strong if they may
// be attacked. radius 0 = the bot's usual farm radius.
func FarmCandidates(w World, botID PlayerID, from *Village, radius int) []*KnownVillage {
	return farmCandidates(w, botID, from, radius, true)
}

// farmCandidates: without players, only barbarians and abandoned villages (the auto-farm's farms).
func farmCandidates(w World, botID PlayerID, from *Village, radius int, players bool) []*KnownVillage {
	brain := w.Brain(botID)
	if brain == nil {
		return nil
	}
	r := radius
	if r == 0 {
		r = farmRadius(brain.Skill)
	}
	stored := map[string]VillageRef{}
	for _, ref := range w.VillagesIn(from.X-r, from.Y-r, from.X+r, from.Y+r) {
		stored[ref.ID] = ref
	}
	var out []*KnownVillage
	for _, t := range GeneratedVillagesNear(w.Seed(), w.Config(), from.X, from.Y, r) {
		s, isStored := stored[t.ID]
		if isStored && s.OwnerID != "" {
			continue
		}
		k := brain.Memory.Known[t.ID]
		if k == nil {
			points := t.Points
			if isStored {
				points = s.Points
			}
			k = &KnownVillage{ID: t.ID, X: t.X, Y: t.Y, Points: points, Hauls: []float64{}}
		}
		if isAvailable(w, k) {
			out = append(out, k)
		}
	}
	own := float64(PlayerPoints(w, botID))
	for _, k := range sortedKnown(brain) {
		if k.OwnerID == "" || k.OwnerID == Ref(botID) {
			continue
		}
		owner := w.Player(PlayerID(k.OwnerID))
		// Abandoned players are farms; active ones only when much weaker and when allowed (world setting).
		inactive := owner != nil && owner.Status == StatusQuit
		if !inactive && (!players || float64(k.Points) >= 0.5*own || !MayTarget(w, botID, k.OwnerID)) {
			continue
		}
		if Distance(from.X, from.Y, k.X, k.Y) > float64(r) || !isAvailable(w, k) {
			continue
		}
		out = append(out, k)
	}
	return out
}

// winsSafely: the group wins at the worst luck (−25 %) against what the bot knows of the target, without losses.
func winsSafely(from *Village, k *KnownVillage, units UnitCounts) bool {
	r := predictedBattle(from, k, units)
	return r.Outcome == "attackerWon" && r.AttackerSurvivors.Total() > 0 && r.AttackerLosses.Total() == 0
}

// beats: the army wins against what the bot counted, at the worst luck, whatever it costs. A clear is expected to lose
// men — that is what clearing a village is — so it asks this rather than winsSafely, which is the raid rule that a farm
// run must come home without losses to pay for itself.
func beats(from *Village, k *KnownVillage, units UnitCounts) bool {
	r := predictedBattle(from, k, units)
	return r.Outcome == "attackerWon" && r.AttackerSurvivors.Total() > 0
}

// predictedBattle is the battle a raid or a clear would fight against what the bot remembers, at the worst luck it can
// draw and with no die roll of its own: the two planners only need to know which way it goes.
func predictedBattle(from *Village, k *KnownVillage, units UnitCounts) BattleResult {
	var def UnitCounts
	if k.Units != nil {
		def = *k.Units
	}
	wall := 0
	if k.Wall != nil {
		wall = *k.Wall
	}
	var b Buildings
	b[BWall] = wall
	return ResolveBattle(BattleInput{
		Attacker: BattleSide{Units: units, Research: UnitCounts(from.Research)},
		Defender: BattleDefender{BattleSide: BattleSide{Units: def}, Wall: wall, Buildings: b},
		Luck:     -CombatBalance.LuckRange, Morale: CombatBalance.MoraleVsBarbarian, Rng: func() float64 { return 0.5 },
		LossMode: "normal",
	})
}

// raiders are the units a bot may loot with, best first. Axes and light cavalry are the attackers: an axe hits for 40
// and light cavalry is fast and carries the most.
//
// Heavy cavalry and mounted archers are here too, and this is the only thing they are ever sent out for. Both carry well
// and neither is worth keeping home from a farm run that they can win alone — but neither is ever an attacking line, and
// clearing a village is LaunchClear's job, which builds its force from axes, light cavalry and rams and never asks this
// table.
//
// Spears come last and are not a plan but a fallback. Carrying 25 they are worse raiders than cavalry, and their job is
// holding a wall — but a village with spears and nothing else that will not loot simply does not grow, and early on that
// is most villages. Swords and archers never leave: they carry 15 and 10, and they are the wall's infantry.
var raiders = []UnitID{ULightCav, UHeavyCav, UMountedArcher, UAxe, USpear}

// raiderGroup is n of one raider: a UnitCounts for a unit chosen at run time.
func raiderGroup(u UnitID, n int) UnitCounts {
	c := UnitCounts{}
	c[u] = n
	return c
}

// raiderOrder is the raiding units a village has, best first. Cavalry leads when there is cavalry to spare, because it
// is fast and carries the most; without it an axe leads, and a spear brings up the rear.
func raiderOrder(home Units) []UnitID {
	var out []UnitID
	if home[ULightCav] >= 4 {
		out = []UnitID{ULightCav, UHeavyCav, UMountedArcher, UAxe, USpear}
	} else {
		out = []UnitID{UAxe, USpear, UHeavyCav, UMountedArcher, ULightCav}
	}
	have := out[:0]
	for _, u := range out {
		if home[u] > 0 {
			have = append(have, u)
		}
	}
	return have
}

// PlayerRaidFloor is the smallest column a bot sends at another player's village, and the haul it is sized to fill.
//
// A barbarian is an optimiser: raid it the moment it has refilled, with whatever four horsemen can carry, because
// nobody reads those reports. A person does. A village with a real army in it holds far more than a hut, so a party
// sized by the smallest group that wins against an empty garrison is a patrol — it takes a pittance, tells the player
// a bot is farming them, and reads as a broken attack rather than a raid.
//
// So a player's village is raided by a column instead: enough raiders to clear the garrison the bot last saw and to
// carry what a farm run is worth, and at least this many units so there is something to read in the report. Below it the
// bot keeps farming barbarians until it has an army worth marching.
const PlayerRaidFloor = 1000

// PlayerRaidShare is how much of a village's raiders go on a raid against a person: a column, so that the stores he is
// sitting on actually move, rather than a patrol that wins on paper and brings back a fraction of a load. It is not all
// of them — the village keeps a reserve to defend itself and to raid again — but it is a share a player will notice,
// which is the point: farming a person is meant to cost him something.
const PlayerRaidShare = 0.75

// SizeRaid is the smallest group that wins without losses, built from the raiders the village actually has.
//
// It reads the village rather than assuming light cavalry: a village with a handful of cavalry and four hundred axes
// raids with the axes. Anything else leaves troops at home that nothing else a bot does will ever spend, and they sit
// in the farm forever taking up room it cannot free.
//
// A player's village may be farmed, but only once the bot has looked inside it: the raid is sized by simulating against
// the garrison the bot remembers, and with no memory of the village that garrison is empty, so the raid would "win" on
// paper and the troops would be lost for nothing. Given a real count it is safe to farm a weak player — that is the point
// of scouting them. A barbarian needs no look, because empty is what a barbarian is. So does another bot's village: a bot
// puts its army out to farm, so an unscouted bot village is empty far more often than not, and that is why bots farm each
// other on sight.
//
// Wall 1–2 without defenders → 2 rams per level + 20 axes (strong bots only); otherwise none.
//
// An unknown wall is opened up with a small group, which is how a bot finds out what a barbarian is made of — but only
// against a village nobody is standing in. A player's village may hold an army, and under the fog of war a group that
// dies reports nothing: learning what stands there is a scouting mission's job (campaignScout), not a raid's.
func SizeRaid(w World, from *Village, k *KnownVillage, skill float64) (UnitCounts, bool) {
	human := IsHumanID(PlayerID(k.OwnerID))
	if human && (k.Units == nil || k.UnitsSeenAt == nil) {
		return UnitCounts{}, false
	}
	if human {
		return sizeColumn(w, from, k)
	}
	units, ok := sizeRaid(w, from, k, skill)
	if !ok {
		return units, false
	}
	// Another bot's village is farmed like a barbarian's: it is away from home with its army most of the time, nobody
	// reads those reports, and a small party that wins is a party worth sending. Only a person's village needs a column.
	if k.OwnerID != "" && !human && units.Total() < BotRaidFloor {
		grown, ok := growRaid(from, units, BotRaidFloor)
		if !ok {
			return UnitCounts{}, false
		}
		units = grown
	}
	return units, true
}

// BotRaidFloor is the smallest party a bot sends at another bot's village. It keeps a bot from dribbling a horseman or
// two at a neighbour, which is not a raid and does not teach anything, without giving another bot the column a person
// gets: a bot's village is empty most of the time, and a party sized for that is the normal case.
const BotRaidFloor = 25

// needsColumn: the target is a person's village this one has no column for, so refusing it says nothing about whether
// there is a farm further down the list.
func needsColumn(k *KnownVillage, v *Village) bool {
	if !IsHumanID(PlayerID(k.OwnerID)) {
		return false
	}
	raiders := 0
	for _, u := range raiderOrder(v.Units) {
		raiders += v.Units[u]
	}
	return raiders < PlayerRaidFloor
}

// sizeColumn is the raid on a player's village: a column, not a patrol.
//
// A barbarian is looted by the smallest group that wins, because the only thing that matters there is the trip and
// nobody reads the report. A person is different twice over. His stores are far bigger, so a party sized for a hut takes
// a fraction of what is lying there. And he can count: a handful of horsemen a dozen times a day reads as a broken
// attack, not a raid.
//
// So the column is a share of the raiders a village actually has — a real army, not a detail — and the garrison the bot
// counted decides only whether it is safe to go. A village that cannot field one at all leaves players alone and keeps
// farming barbarians until it can.
func sizeColumn(w World, from *Village, k *KnownVillage) (UnitCounts, bool) {
	order := raiderOrder(from.Units)
	if len(order) == 0 {
		return UnitCounts{}, false
	}
	group := func(n int) UnitCounts {
		out, left := UnitCounts{}, n
		for _, u := range order {
			if left <= 0 {
				break
			}
			take := min(left, from.Units[u])
			if take > 0 {
				out[u] = take
				left -= take
			}
		}
		return out
	}
	all := 0
	for _, u := range order {
		all += from.Units[u]
	}
	if all < PlayerRaidFloor {
		return UnitCounts{}, false
	}
	want := max(PlayerRaidFloor, int(math.Round(PlayerRaidShare*float64(all))))
	want = min(want, all)
	// Only the garrison stops it: a column that cannot beat the troops the bot counted does not go.
	if !winsSafely(from, k, group(want)) {
		return UnitCounts{}, false
	}
	return group(want), true
}

// growRaid tops a party up to want, filling the raiders the village has in the rounds the party was built in. A party
// rewritten to one unit instead is a different army: the mix that simulated a win is what makes the raid safe.
func growRaid(from *Village, units UnitCounts, want int) (UnitCounts, bool) {
	out := UnitCounts{}
	for u, n := range units {
		out[u] = n
	}
	short := want - units.Total()
	for _, u := range raiderOrder(from.Units) {
		if short <= 0 {
			break
		}
		room := from.Units[u] - out[u]
		if room <= 0 {
			continue
		}
		take := min(short, room)
		out[u] += take
		short -= take
	}
	if out.Total() < want {
		return UnitCounts{}, false
	}
	return out, true
}

func sizeRaid(w World, from *Village, k *KnownVillage, skill float64) (UnitCounts, bool) {
	home := from.Units
	if k.Wall == nil {
		if IsHumanID(PlayerID(k.OwnerID)) {
			return UnitCounts{}, false
		}
		for _, u := range raiders {
			if home[u] >= 4 {
				probe := raiderGroup(u, 4)
				if home[UScout] > 0 {
					probe[UScout] = 1
				}
				return probe, true
			}
		}
		if home[USpear] >= 15 {
			return UnitCounts{USpear: 15}, true
		}
		return UnitCounts{}, false
	}
	if *k.Wall > 0 {
		noDefenders := k.Units == nil || k.Units.Total() == 0
		if *k.Wall <= 2 && noDefenders && skill >= 0.7 && home[URam] >= 2**k.Wall && home[UAxe] >= 20 {
			return UnitCounts{URam: 2 * *k.Wall, UAxe: 20}, true
		}
		return UnitCounts{}, false
	}
	// A party is built from the raiders the village has, in order, until it wins — rather than sizing one unit as if the
	// others did not exist. A garrison of spears shrugs off four axes and folds in front of twice as many.
	order := raiderOrder(home)
	if len(order) == 0 {
		return UnitCounts{}, false
	}
	group := func(n int) UnitCounts {
		out, left := UnitCounts{}, n
		for _, u := range order {
			if left <= 0 {
				break
			}
			take := min(left, home[u])
			if take > 0 {
				out[u] = take
				left -= take
			}
		}
		return out
	}
	maxN := 0
	for _, u := range order {
		maxN += home[u]
	}
	if maxN == 0 {
		return UnitCounts{}, false
	}
	minimum := 10
	if home[ULightCav] >= 4 {
		minimum = min(maxN, cavalryForLoot(w, k))
	}
	if maxN < minimum || !winsSafely(from, k, group(maxN)) {
		return UnitCounts{}, false
	}
	lo, hi := minimum, maxN
	if winsSafely(from, k, group(lo)) {
		return group(lo), true
	}
	for hi-lo > 1 {
		mid := (lo + hi) >> 1
		if winsSafely(from, k, group(mid)) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return group(hi), true
}

func expectedHaulRatio(k *KnownVillage) float64 {
	if len(k.Hauls) == 0 {
		return 1
	}
	s := 0.0
	for _, h := range k.Hauls {
		s += h
	}
	return s / float64(len(k.Hauls))
}

// cavalryForLoot: light cavalry for the expected loot — the farm rests BotConfig.FarmRefillHours, and full past
// hauls make the group bigger.
func cavalryForLoot(w World, k *KnownVillage) int {
	restedH := BotConfig.FarmRefillHours
	if k.LastAttackAt != nil {
		restedH = float64(w.Now()-*k.LastAttackAt) / float64(MsPerHour)
	}
	loot := BotConfig.FarmLoot * math.Min(1, restedH/BotConfig.FarmRefillHours) * (0.5 + expectedHaulRatio(k))
	return max(4, int(math.Ceil(loot/UnitDefs[ULightCav].Haul)))
}

func sendRaid(w World, botID PlayerID, from *Village, k *KnownVillage, units UnitCounts, night bool, nextSessionAt int64) bool {
	if len(CheckSend(w, botID, from.ID, k.X, k.Y, units, "attack").Blockers) > 0 {
		return false
	}
	var c *Command
	if !act(w, botID, "send", sendArgs(from.ID, k.X, k.Y, units, "attack"), func() error {
		var err error
		c, err = SendCommand(w, botID, from.ID, k.X, k.Y, units, "attack", SendOpts{})
		return err
	}) {
		return false
	}
	brain := brainOf(w, botID)
	entry := brain.Memory.Known[k.ID]
	if entry == nil {
		cp := *k
		cp.Hauls = append([]float64{}, k.Hauls...)
		entry = &cp
		brain.Memory.Known[k.ID] = entry
	}
	entry.LastAttackAt = ptrTo(w.Now())
	if raidSpy != nil {
		raidSpy(botID, c, night, nextSessionAt)
	}
	return true
}

// PlanFarming, in a session: from each village, groups sized by simulation at targets by expected haul ÷
// distance, until the troops run out.
func PlanFarming(w World, botID PlayerID) {
	if w.Brain(botID) == nil {
		return
	}
	for _, v := range PlayerVillages(w, botID) {
		scoutStaleBeforeFarming(w, botID, v)
		farmFrom(w, botID, v, BotConfig.MaxRaidsPerThink, true)
	}
}

// scoutStaleBeforeFarming sends a proper scouting party at a target whose garrison it no longer trusts, instead of
// discovering the change with a raid that dies on it. One per village per session keeps the scouts at home mostly.
//
// Note this does not consult isAvailable: that is the farm's cooldown, and it asks whether the village has refilled
// enough to be worth raiding again. It has nothing to do with whether the bot has looked inside it. Sharing it meant a
// village the bot was actively farming was never scouted at all — the raid kept resetting the cooldown the scout was
// waiting on — so a player could be looted all day long and never looked into once.
// scoutsPerSession is how many stale villages one of a bot's villages looks into per session. Looking only at the
// nearest means the bot never learns about the one behind it, however empty that one turns out to be — and a party that
// comes back reporting an undefended village two tiles further out is worth as much as one reporting the neighbour.
const scoutsPerSession = 3

// scoutReadyAtHome is how many scouts a village needs at home before it looks into a target. It used to be a full
// party, which meant a village could not look until its line reached 600 — and most roles plan fewer than that, so most
// villages never looked at all. It is now only enough to be worth sending: any scouts at all, so a village with 300 goes
// and accepts that it may be caught. scoutPartyFor decides how many of them go.
const scoutReadyAtHome = 1

// scoutHumansAhead keeps a bot from only ever scouting its nearest neighbours. Bots outnumber people on a full map by
// about nine to one, so the nearest few stale villages are almost always bot villages and a player may never be looked
// into at all. Each session therefore spends its looks on the nearest player village first when there is one, and only
// then on the nearest of anything else.
const scoutHumansAhead = true

func scoutStaleBeforeFarming(w World, botID PlayerID, v *Village) {
	if v.Units[UScout] < scoutReadyAtHome {
		return
	}
	type candidate struct {
		k     *KnownVillage
		d     float64
		human bool
	}
	var stale []candidate
	for _, k := range sortedKnown(brainOf(w, botID)) {
		if k.OwnerID == "" || k.OwnerID == Ref(botID) || !MayTarget(w, botID, k.OwnerID) {
			continue
		}
		if !intelStale(w, k) {
			continue
		}
		stale = append(stale, candidate{k, Distance(v.X, v.Y, k.X, k.Y), IsHumanID(PlayerID(k.OwnerID))})
	}
	// Nearest first within each group; people before bots. A look costs the party's travel either way, but a bot can
	// scout its neighbours any session, while a player who is never nearest is never scouted.
	sort.SliceStable(stale, func(i, j int) bool {
		if scoutHumansAhead && stale[i].human != stale[j].human {
			return stale[i].human
		}
		return stale[i].d < stale[j].d
	})
	committed := 0
	for i, c := range stale {
		if i >= scoutsPerSession || v.Units[UScout]-committed < scoutReadyAtHome {
			break
		}
		party := scoutPartyFor(v.Units[UScout]-committed, c.k)
		if party <= 0 {
			break
		}
		if campaignScout(w, botID, c.k.X, c.k.Y) {
			committed += party
		}
	}
}

// farmFrom sends up to maxRaids sized raids from v, best expected haul ÷ distance first, and returns how many went;
// without players only at barbarians and abandoned villages.
func farmFrom(w World, botID PlayerID, v *Village, maxRaids int, players bool) int {
	if UnitCounts(v.Units).Total() == 0 {
		return 0 // everyone is out: no need to list the farms
	}
	type scored struct {
		k     *KnownVillage
		score float64
	}
	var targets []scored
	for _, k := range farmCandidates(w, botID, v, 0, players) {
		targets = append(targets, scored{k, expectedHaulRatio(k) / math.Max(1, Distance(v.X, v.Y, k.X, k.Y))})
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].score > targets[j].score })
	misses, sent := 0, 0
	for _, t := range targets {
		if UnitCounts(v.Units).Total() == 0 || misses > 20 || sent >= maxRaids {
			break
		}
		units, ok := SizeRaid(w, v, t.k, brainOf(w, botID).Skill)
		if !ok {
			// A player's village a village cannot field a column for is not a miss in the same sense as a farm that has
			// dried up: every one of them is refused for the same reason, and counting them all against the miss budget
			// spends it before the barbarians further down the list are ever looked at. Leaving a player alone is a
			// decision, not a failure to find a farm.
			if !needsColumn(t.k, v) {
				misses++
			}
			continue
		}
		if sendRaid(w, botID, v, t.k, units, false, 0) {
			sent++
		} else {
			misses++
		}
	}
	return sent
}

const nightReach = 60

// PlanNightRuns, before sleep: the remaining light cavalry on raids that come back ±60 min from the next session
// (a strong bot picks the distance, a weak one sends anywhere in its usual range). At night it reaches further.
func PlanNightRuns(w World, botID PlayerID, nextSessionAt int64) {
	brain := w.Brain(botID)
	if brain == nil {
		return
	}
	rng := Mulberry32(HiddenSeed(w, "night", botID, w.Now()))
	strong := brain.Skill >= 0.7 || rng() < brain.Skill
	for _, v := range PlayerVillages(w, botID) {
		if v.Units[ULightCav] < 4 {
			continue
		}
		reach := 0
		if strong {
			reach = nightReach
		}
		var pool []*KnownVillage
		for _, k := range FarmCandidates(w, botID, v, reach) {
			if (k.Wall == nil || *k.Wall == 0) && (k.Units == nil || k.Units.Total() == 0) {
				pool = append(pool, k)
			}
		}
		if strong {
			kept := pool[:0]
			for _, k := range pool {
				back := 2 * TravelTimeForUnits(w, v, k.X, k.Y, UnitCounts{ULightCav: 1})
				if abs64(w.Now()+back-nextSessionAt) <= 60*MsPerMinute {
					kept = append(kept, k)
				}
			}
			pool = kept
		} else {
			for i := len(pool) - 1; i > 0; i-- {
				j := int(math.Floor(rng() * float64(i+1)))
				pool[i], pool[j] = pool[j], pool[i]
			}
		}
		sent := 0
		for _, k := range pool {
			if v.Units[ULightCav] < 4 || sent >= BotConfig.MaxNightRuns {
				break
			}
			// A night is many hours of rest for the farm — a group as for a full farm.
			n := min(v.Units[ULightCav], max(4, int(math.Ceil(BotConfig.FarmLoot*(0.5+expectedHaulRatio(k))/UnitDefs[ULightCav].Haul))))
			if sendRaid(w, botID, v, k, UnitCounts{ULightCav: n}, true, nextSessionAt) {
				sent++
			}
		}
	}
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
