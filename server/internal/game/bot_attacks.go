package game

import (
	"math"
	"sort"
)

// Bot attacks: choosing a target, sizing the army, timing the send.

func init() { launchConquest = LaunchConquest }

type Purpose string

func rngFor(w World, botID PlayerID, what, id string) func() float64 {
	return Mulberry32(HiddenSeed(w, what, botID, id, w.Now()))
}

func villagesByDistance(w World, botID PlayerID, x, y int) []*Village {
	vs := PlayerVillages(w, botID)
	sort.SliceStable(vs, func(i, j int) bool { return Distance(vs[i].X, vs[i].Y, x, y) < Distance(vs[j].X, vs[j].Y, x, y) })
	return vs
}

// noblesReady counts the bot's nobles at home in all villages (a train gathers from several).
func noblesReady(w World, botID PlayerID) int {
	n := 0
	for _, v := range PlayerVillages(w, botID) {
		n += v.Units[UNoble]
	}
	return n
}

// ChoosePurpose: conquest, demolition, farming the player or harassment — drawn with style weights, not always
// the best.
func ChoosePurpose(w World, botID PlayerID, k *KnownVillage) Purpose {
	b := brainOf(w, botID)
	v := w.Village(k.ID)
	// "Can this army take the place?" A bot only sends a train at a defence it has actually counted, and only if its
	// army is comfortably bigger. Not knowing is a reason to look first, not a reason to give up on the idea for ever.
	// Judged on the line that would actually march, not on the bot's strongest garrison: a defence village's wall of
	// spears and heavy cavalry is not an attacking army, so measuring the target against it offers clears this bot
	// could never carry out and hides the ones it could win.
	clearable := Clearable(w, botID, k)
	payback := 1.0
	if k.OwnerID != "" && PlayerID(k.OwnerID) == HardFarmer(w, botID) {
		payback = 3
	}
	harass := 1.0
	if b.Style == StyleAggressor {
		harass = 2
	}
	// Harassment is what a bot does with a band of scouts to spend. A bot that cannot field one is not ready to poke a
	// player and should be building instead — without this gate harass is always on the table and swallows every draw.
	var options []Option[Purpose]
	if harassReady(w, botID) {
		options = append(options, Option[Purpose]{"harass", harass * payback})
	}
	// A player is taken only with a full train; a ready train on a clear road is the best option. Without a look inside
	// the village there is no "clear road" to judge, so the train waits for the scouts — but a bot whose scouts keep
	// being caught must still go eventually, or it never attacks at all.
	if noblesReady(w, botID) >= NoblesNeededFor(w.Config(), true, ConquestBalance.MaxLoyalty) && v != nil &&
		CanBeConquered(w, v) && (clearable || intelHopeless(w, k)) {
		score := 3.0
		if b.Style == StyleAggressor {
			score = 4
		}
		options = append(options, Option[Purpose]{"conquer", score})
	}
	// A bot that has counted a garrison it can beat marches its offensive line at that village to break it. This is the
	// answer to a neighbour who is out-growing the bot: looting him takes what is in his stores, but his army is still
	// there tomorrow, and a bot that only ever farms a player feeds a rival instead of slowing him down.
	//
	// It needs numbers for the same reason a hetman train does, but it is not a train: a clear is what a bot does when it
	// has an army and no hetmen, which is most bots on most days.
	if clearable && clearReady(w, botID) {
		score := 1.5
		if b.Style == StyleAggressor {
			score = 2.5
		}
		options = append(options, Option[Purpose]{"clear", score * payback})
	}
	// A bot with a real army and no numbers to go on sometimes just throws it: people attack without scouting when the
	// target looks weak enough, and a bot that never takes that risk is not playing like one. It is a gamble on points,
	// not on a count, and it is the only path here that does not require knowing the garrison.
	if !clearable && allInReady(w, botID) {
		score := 0.6
		if b.Style == StyleAggressor {
			score = 1.2
		}
		options = append(options, Option[Purpose]{"allIn", score * payback})
	}
	cats := 0
	for _, x := range PlayerVillages(w, botID) {
		cats = max(cats, x.Units[UCatapult])
	}
	if cats >= 50 && (k.Buildings[BFarm] >= 10 || k.Buildings[BSmithy] >= 10) {
		options = append(options, Option[Purpose]{"demolish", 1})
	}
	if k.OwnerID != "" && float64(PlayerPoints(w, PlayerID(k.OwnerID))) < 0.5*float64(PlayerPoints(w, botID)) {
		score := 1.0
		if b.Style == StyleFarmer {
			score = 2
		}
		options = append(options, Option[Purpose]{"farmPlayer", score * payback})
	}
	// Nothing worth doing here yet: the bot is not ready to poke this player. The caller acts on the purpose, so an
	// empty-handed answer has to be a real one rather than a panic in the weighted draw.
	if len(options) == 0 {
		return "hold"
	}
	return WeightedPick(rngFor(w, botID, "purpose", k.ID), options, b.Skill)
}

// allInReady: the bot has an army worth gambling. Below this it is not a gamble, it is a gift.
func allInReady(w World, botID PlayerID) bool {
	for _, v := range PlayerVillages(w, botID) {
		if allInArmy(v).Total() >= BotConfig.AllInMin {
			return true
		}
	}
	return false
}

// clearReady: one of the bot's villages holds a line worth marching at a village. The same bar as a gamble, because it
// is the same army: below it a clear is not a clear, it is a donation, and the soldiers lost are the ones that would have
// defended the village they came from.
func clearReady(w World, botID PlayerID) bool {
	for _, v := range PlayerVillages(w, botID) {
		if clearArmy(v).Total() >= ClearMin {
			return true
		}
	}
	return false
}

// harassReady: the bot has a scouting party to spend on probing. One party is the bar, not four: LaunchHarass sends up
// to four when it can, but a bot that could not probe with a single party would never scout anybody, and the party size
// and this gate must not move together — raising the party would then make harassing harder, not easier.
func harassReady(w World, botID PlayerID) bool {
	have := 0
	for _, v := range PlayerVillages(w, botID) {
		have += v.Units[UScout]
	}
	return have >= ScoutParty
}

// nightLanding is the first instant ≥ earliest in the target's local night (00:00–06:00) or offline window.
func nightLanding(w World, owner PlayerRef, earliest float64, rng func() float64) float64 {
	start, end := 0, 6*60
	if owner != "" {
		if win, ok := LikelyOfflineWindow(w, PlayerID(owner)); ok {
			start, end = win.StartMin, win.EndMin
		}
	}
	l := ((end - start + 1440) % 1440)
	if l == 0 {
		l = 1440
	}
	minNow := LocalMinuteOfDay(w, int64(earliest))
	offset := (((start - minNow) % 1440) + 1440) % 1440
	return earliest + (float64(offset)+rng()*float64(l))*float64(MsPerMinute)
}

// pastTheBell: in a short world, a time after the game's end, when nothing a bot plans matters any more.
func pastTheBell(w World, at int64) bool {
	cfg := w.Config()
	return IsShort(cfg) && cfg.DurationMin > 0 && at > int64(cfg.DurationMin)*MsPerMinute
}

func botSchedule(w World, botID PlayerID, in ScheduleSendInput) {
	if pastTheBell(w, in.At) {
		return
	}
	actSoft(w, botID, "schedule-send", in, func() error { _, err := ScheduleSend(w, botID, in); return err })
}

// LaunchHarass: 4 probes from different villages landing within a second, to see what is standing at home and to
// spoil a defender's day; every fourth lands at the target's night. A probe is a scouting party, not a token force:
// the fog of war means a probe that dies reports nothing, so one spear would tell the bot nothing at all.
func LaunchHarass(w World, botID PlayerID, k *KnownVillage) {
	rng := rngFor(w, botID, "harass", k.ID)
	sources := villagesByDistance(w, botID, k.X, k.Y)
	if len(sources) == 0 {
		return
	}
	type pick struct {
		v     *Village
		units UnitCounts
	}
	var picks []pick
	used := map[*Village]int{}
	for i := range 4 {
		v := sources[i%len(sources)]
		left := v.Units[UScout] - used[v]
		if left < ScoutParty {
			continue
		}
		used[v] += ScoutParty
		picks = append(picks, pick{v, UnitCounts{UScout: ScoutParty}})
	}
	if len(picks) == 0 {
		return
	}
	latest := int64(0)
	for _, p := range picks {
		latest = max(latest, TravelTimeForUnits(w, p.v, k.X, k.Y, p.units))
	}
	T := float64(w.Now()+latest+BotDur(w, 5*MsPerMinute)) + float64(BotDur(w, int64(rng()*3*float64(MsPerHour))))
	landed := false
	if k.OwnerID != "" {
		if at, ok := LandAfterDodge(w, botID, PlayerID(k.OwnerID), w.Now()+latest+BotDur(w, 5*MsPerMinute), rng); ok {
			T, landed = float64(at), true
		}
	}
	if !landed && rng() < 0.25 && !IsShort(w.Config()) {
		T = nightLanding(w, k.OwnerID, T, rng)
	}
	at := int64(math.Round(math.Min(T, float64(w.Now()+latest+BotDur(w, MsPerDay)))))
	for i, p := range picks {
		botSchedule(w, botID, ScheduleSendInput{OriginID: p.v.ID, X: k.X, Y: k.Y, Units: p.units, Type: "attack",
			Mode: "arriveAt", At: at + int64(i*1000/4)})
	}
}

// LaunchFarmPlayer: an ordinary raid sized by simulation (repeated in later sessions while the hauls are good).
func LaunchFarmPlayer(w World, botID PlayerID, k *KnownVillage) {
	b := brainOf(w, botID)
	for _, v := range villagesByDistance(w, botID, k.X, k.Y) {
		units, ok := SizeRaid(w, v, k, b.Skill)
		if !ok {
			continue
		}
		botSchedule(w, botID, ScheduleSendInput{OriginID: v.ID, X: k.X, Y: k.Y, Units: units, Type: "attack",
			Mode: "sendAt", At: w.Now() + SendJitter(w, botID, k.ID)})
		return
	}
}

// CatapultsFor is the catapults to bring a building to zero: the table's sum over each level.
func CatapultsFor(level int) int {
	n := 0
	for l := 1; l <= level; l++ {
		n += CatapultsPerLevel[min(l, len(CatapultsPerLevel)-1)]
	}
	return n
}

// LaunchDemolish: catapults by the building's level, escorted by axes.
func LaunchDemolish(w World, botID PlayerID, k *KnownVillage, building BuildingID) {
	level, ok := k.Buildings[building]
	if !ok {
		level = 10
	}
	need := CatapultsFor(level)
	var from *Village
	for _, v := range villagesByDistance(w, botID, k.X, k.Y) {
		if v.Units[UCatapult] >= min(need, 50) && v.Units[UAxe] >= 100 {
			from = v
			break
		}
	}
	if from == nil {
		return
	}
	cats := min(from.Units[UCatapult], need)
	units := UnitCounts{UCatapult: cats, UAxe: min(from.Units[UAxe], max(100, cats*20))}
	b := building
	botSchedule(w, botID, ScheduleSendInput{OriginID: from.ID, X: k.X, Y: k.Y, Units: units, Type: "attack",
		CatapultTarget: &b, Mode: "sendAt", At: w.Now() + SendJitter(w, botID, k.ID)})
}

// decoys: 2–5 fakes on the owner's other villages landing ±30 min from the main hit (strong bots split defence).
func decoys(w World, botID PlayerID, owner PlayerID, exceptID string, T int64, rng func() float64) {
	var others []*Village
	for _, v := range PlayerVillages(w, owner) {
		if v.ID != exceptID {
			others = append(others, v)
		}
	}
	n := min(len(others), 2+int(math.Floor(rng()*4)))
	sources := PlayerVillages(w, botID)
	for i := 0; i < n && len(sources) > 0; i++ {
		t := others[i]
		v := sources[i%len(sources)]
		if v.Units[UAxe] < 1 {
			continue
		}
		at := int64(math.Round(float64(T) + (rng()*2-1)*30*float64(MsPerMinute)))
		botSchedule(w, botID, ScheduleSendInput{OriginID: v.ID, X: t.X, Y: t.Y, Units: UnitCounts{UAxe: 1}, Type: "attack", Mode: "arriveAt", At: at})
	}
}

// clearArmy is the line a bot takes a village's troops apart with: axes to break the wall's infantry and light cavalry
// for the reach and the haul. Mounted archers go along when a village fields them, since they hit infantry from outside
// its reach. Heavy cavalry stays home — it is the wall's counter-attack — and so do swords, spears, archers and scouts:
// clearing is what the offensive line is for.
func clearArmy(v *Village) UnitCounts {
	units := UnitCounts{}
	for _, u := range []UnitID{UAxe, ULightCav, UMountedArcher, URam} {
		if n := v.Units[u]; n > 0 {
			units[u] = n
		}
	}
	return units
}

// ClearMin is the smallest force worth marching at another player to hurt him. Below it a bot is not clearing a village,
// it is donating an army: a village with a handful of axes cannot beat a garrison, and the soldiers it loses are the ones
// it needed to defend itself. Farm raids have their own, smaller floor — this one is about taking a village apart.
const ClearMin = 500

// LaunchClear: the whole offensive line of the bot's strongest village at another player's village, to destroy what is
// behind the wall. Taking the village is a hetman's job (LaunchConquest); this is the step before it, and the answer to a
// neighbour who is out-growing the bot: a player whose army is broken cannot farm, cannot defend and cannot expand, so a
// bot that only ever looted him was feeding a rival.
//
// It goes only at a garrison the bot has counted. "I can beat what I know" is true of every unscouted village, since
// under the fog of war an unscouted one has an empty garrison in the bot's memory — the risk of attacking blind is what
// the all-in is for, not the plan. Numbers that have aged past IntelStaleHours are not counted either: a player who
// rebuilt since then is exactly the one this must not walk into.
func LaunchClear(w World, botID PlayerID, k *KnownVillage) bool {
	if k == nil || k.OwnerID == "" || PlayerID(k.OwnerID) == botID {
		return false
	}
	if _, known := KnownDefence(k); !known || intelStale(w, k) {
		return false
	}
	from, army := offensiveVillage(w, botID)
	if from == nil || army.Total() < ClearMin {
		return false
	}
	if !beats(from, k, army) {
		return false
	}
	if len(CheckSend(w, botID, from.ID, k.X, k.Y, army, "attack").Blockers) > 0 {
		return false
	}
	at := w.Now() + SendJitter(w, botID, "clear:"+k.ID)
	if pastTheBell(w, at) {
		return false
	}
	// A clear is loud, so it goes when the owner is least likely to be watching: the troops that would meet it are the
	// ones standing at home, and there is no reason to let him pull them out of the way.
	if night, ok := LandAfterDodge(w, botID, PlayerID(k.OwnerID), at, rngFor(w, botID, "clear", k.ID)); ok {
		at = night
	} else if !IsShort(w.Config()) && rngFor(w, botID, "clearnight", k.ID)() < 0.5 {
		at = int64(nightLanding(w, k.OwnerID, float64(at), rngFor(w, botID, "clearland", k.ID)))
	}
	in := ScheduleSendInput{OriginID: from.ID, X: k.X, Y: k.Y, Units: army, Type: "attack", Mode: "arriveAt", At: at}
	return actSoft(w, botID, "schedule-send", in, func() error {
		_, err := ScheduleSend(w, botID, in)
		return err
	})
}

// LaunchConquest: a clearing nuke at T, up to 4 nobles (from several villages) right behind it (strong: every
// 100 ms, weak: randomly up to 5 s), the first escorted by 300 axes, the others by 100. A strong bot aims at the
// owner's offline window when it knows it, and scatters decoys.
func LaunchConquest(w World, botID PlayerID, k *KnownVillage) {
	b := brainOf(w, botID)
	for _, s := range w.ScheduledSendsOf(botID) {
		if s.Units[UNoble] > 0 {
			return
		}
	}
	rng := rngFor(w, botID, "conquest", k.ID)
	strong := b.Skill >= 0.7
	var sources []*Village
	for _, v := range villagesByDistance(w, botID, k.X, k.Y) {
		if v.Units[UNoble] > 0 && Distance(v.X, v.Y, k.X, k.Y) <= ConquestBalance.NobleMaxDistance {
			sources = append(sources, v)
		}
	}
	var nobleFrom []*Village
	for _, v := range sources {
		for i := 0; i < v.Units[UNoble] && len(nobleFrom) < 4; i++ {
			nobleFrom = append(nobleFrom, v)
		}
	}
	if len(nobleFrom) == 0 {
		return
	}
	candidates := append(append([]*Village{}, sources...), villagesByDistance(w, botID, k.X, k.Y)...)
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Units[UAxe] > candidates[j].Units[UAxe] })
	nukeVillage := candidates[0]
	escorts := make([]int, len(nobleFrom))
	for i := range escorts {
		escorts[i] = 100
	}
	escorts[0] = 300
	reserved := func(v *Village) int {
		n := 0
		for i, x := range nobleFrom {
			if x == v {
				n += escorts[i]
			}
		}
		return min(n, v.Units[UAxe]/2)
	}
	var nuke UnitCounts
	if left := max(0, nukeVillage.Units[UAxe]-reserved(nukeVillage)); left > 0 {
		nuke[UAxe] = left
	}
	nuke[ULightCav] = nukeVillage.Units[ULightCav]
	nuke[URam] = nukeVillage.Units[URam]
	slowest := TravelTimeForUnits(w, nukeVillage, k.X, k.Y, nuke)
	for _, v := range nobleFrom {
		slowest = max(slowest, TravelTimeForUnits(w, v, k.X, k.Y, UnitCounts{UNoble: 1}))
	}
	T := float64(w.Now() + slowest + BotDur(w, 10*MsPerMinute))
	if strong && k.OwnerID != "" {
		if at, ok := LandAfterDodge(w, botID, PlayerID(k.OwnerID), int64(T), rng); ok {
			T = float64(at)
		} else if _, ok := LikelyOfflineWindow(w, PlayerID(k.OwnerID)); ok && !IsShort(w.Config()) {
			T = nightLanding(w, k.OwnerID, T, rng)
		}
	}
	at := int64(math.Round(math.Min(T, float64(w.Now()+slowest+BotDur(w, MsPerDay)))))
	if nuke.Total() > 0 {
		botSchedule(w, botID, ScheduleSendInput{OriginID: nukeVillage.ID, X: k.X, Y: k.Y, Units: nuke, Type: "attack", Mode: "arriveAt", At: at})
	}
	offset := int64(0)
	used := map[*Village]int{}
	for i, v := range nobleFrom {
		if strong {
			offset += 100
		} else {
			offset += 100 + int64(math.Floor(rng()*1100))
		}
		units := UnitCounts{UNoble: 1}
		spare := v.Units[UAxe] - used[v]
		if v == nukeVillage {
			spare -= nuke[UAxe]
		}
		if escort := min(escorts[i], max(0, spare)); escort > 0 {
			units[UAxe] = escort
			used[v] += escort
		}
		botSchedule(w, botID, ScheduleSendInput{OriginID: v.ID, X: k.X, Y: k.Y, Units: units, Type: "attack", Mode: "arriveAt", At: at + offset})
	}
	if strong && k.OwnerID != "" {
		decoys(w, botID, PlayerID(k.OwnerID), k.ID, at, rng)
	}
}

// allInArmy is what a bot throws at a village it has decided to gamble on: the whole offensive army of its strongest
// village, minus nothing. A gamble is only worth the name if the stake is real — a token force that loses teaches the
// bot no more than a scout would, and wastes the army it was meant to risk.
func allInArmy(v *Village) UnitCounts {
	units := UnitCounts{}
	for _, u := range UnitIDs {
		switch u {
		case UAxe, ULightCav, UMountedArcher, UHeavyCav, URam, UCatapult:
			units[u] = v.Units[u]
		}
	}
	return units
}

// LaunchAllIn: the whole army at one village, on the strength of its points rather than a count of its garrison. People
// attack without scouting when they think the target looks weak enough, and lose; a bot that never does the same is not
// playing like a person. It is rare on purpose — this is a gamble that can cost a bot its army, so it must not become
// its habit.
func LaunchAllIn(w World, botID PlayerID, k *KnownVillage) bool {
	if k == nil {
		return false
	}
	b := brainOf(w, botID)
	rng := rngFor(w, botID, "allin", k.ID)
	if rng() >= BotConfig.AllInChance {
		return false
	}
	var from *Village
	var army UnitCounts
	for _, v := range villagesByDistance(w, botID, k.X, k.Y) {
		units := allInArmy(v)
		if units.Total() > army.Total() {
			from, army = v, units
		}
	}
	if from == nil || army.Total() < BotConfig.AllInMin {
		return false
	}
	if len(CheckSend(w, botID, from.ID, k.X, k.Y, army, "attack").Blockers) > 0 {
		return false
	}
	at := w.Now() + SendJitter(w, botID, "allin:"+k.ID)
	if pastTheBell(w, at) {
		return false
	}
	landed := false
	if k.OwnerID != "" && b.Skill >= 0.7 {
		// Even a gamble is landed when the owner is least likely to be watching, if the bot has noticed when that is.
		if night, ok := LandAfterDodge(w, botID, PlayerID(k.OwnerID), at, rng); ok {
			at, landed = night, true
		}
	}
	if !landed {
		at += BotDur(w, int64(rng()*30*float64(MsPerMinute)))
	}
	in := ScheduleSendInput{OriginID: from.ID, X: k.X, Y: k.Y, Units: army, Type: "attack", Mode: "arriveAt", At: at}
	return actSoft(w, botID, "schedule-send", in, func() error {
		_, err := ScheduleSend(w, botID, in)
		return err
	})
}

// LaunchOperation runs an operation on the campaign target, with a purpose fitted to the situation.
func LaunchOperation(w World, botID PlayerID, k *KnownVillage) Purpose {
	purpose := ChoosePurpose(w, botID, k)
	switch purpose {
	case "conquer":
		LaunchConquest(w, botID, k)
	case "demolish":
		b := BSmithy
		if k.Buildings[BFarm] >= k.Buildings[BSmithy] {
			b = BFarm
		}
		LaunchDemolish(w, botID, k, b)
	case "farmPlayer":
		LaunchFarmPlayer(w, botID, k)
	case "clear":
		if !LaunchClear(w, botID, k) {
			// Nothing to clear with after all: fall back to what the village can actually do rather than spending the
			// session on nothing.
			LaunchFarmPlayer(w, botID, k)
		}
	case "allIn":
		if !LaunchAllIn(w, botID, k) {
			// The gamble did not go out after all (the roll, or nothing worth sending): fall back to a probe rather
			// than spending the session on nothing.
			LaunchHarass(w, botID, k)
		}
	case "hold":
		// Nothing worth sending: the bot waits and builds instead of spending troops it cannot spare.
	default:
		LaunchHarass(w, botID, k)
	}
	return purpose
}
