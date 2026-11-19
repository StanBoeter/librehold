package game

import (
	"encoding/json"
	"fmt"
)

// The bots' playbook: openings, role and troop targets, mistakes and plays.

type VillageRole string

const (
	RoleMixed VillageRole = "mixed"
	RoleOff   VillageRole = "off"
	RoleDef   VillageRole = "def"
)

// OpeningStep is one opening build step; it marshals as a [building, level] tuple.
type OpeningStep struct {
	Building BuildingID
	Level    int
}

func (s OpeningStep) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{s.Building, s.Level})
}

func (s *OpeningStep) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("opening step: want 2 elements, got %d", len(raw))
	}
	if err := json.Unmarshal(raw[0], &s.Building); err != nil {
		return err
	}
	return json.Unmarshal(raw[1], &s.Level)
}

var Opening = []OpeningStep{
	{BWood, 1}, {BClay, 1}, {BIron, 1}, {BWood, 2}, {BClay, 2}, {BMain, 2}, {BMain, 3}, {BBarracks, 1},
	{BWood, 3}, {BClay, 3}, {BBarracks, 2}, {BStorage, 2}, {BIron, 2}, {BStorage, 3}, {BBarracks, 3},
	{BStatue, 1}, {BFarm, 2}, {BIron, 3}, {BWood, 4}, {BClay, 4}, {BMain, 5}, {BSmithy, 1}, {BWall, 1},
	{BHide, 3}, {BWood, 5}, {BClay, 5}, {BMarket, 1}, {BMain, 10}, {BSmithy, 5}, {BStable, 3}, {BStorage, 7},
}

var offTargets = map[BuildingID]int{
	BMain: 20, BBarracks: 25, BStable: 20, BWorkshop: 10, BSmithy: 20, BMarket: 10, BFarm: 30, BStorage: 28,
	BWood: 30, BClay: 30, BIron: 30, BWall: 20, BHide: 10,
}

func defTargets() map[BuildingID]int {
	out := map[BuildingID]int{}
	for k, v := range offTargets {
		if k != BWorkshop {
			out[k] = v
		}
	}
	out[BStable] = 10
	return out
}

var RoleTargets = map[VillageRole]map[BuildingID]int{
	RoleOff: offTargets,
	RoleDef: defTargets(),
	// Mixed builds the same farmstead as the military roles: its army is sized for it, and a small farm beside a real
	// army is a village permanently clamped. The workshop is there so it can field rams and catapults at all.
	RoleMixed: {BMain: 15, BBarracks: 15, BStable: 10, BWorkshop: 5, BSmithy: 10, BFarm: 30, BStorage: 25, BWood: 25,
		BClay: 25, BIron: 25, BWall: 15, BHide: 10},
}

// TroopTargets is the army each role builds towards when it has no variant of its own, and the only place those
// numbers live. PlanRecruitment trains towards them and stops when they are met, so a target is a cap, not a floor: it
// has to be the army the village is supposed to have. Every village of a role picking from TroopVariants, so this
// table is what a role falls back on: it is variant 0 of the role, and the two have to stay equal.
//
// Scouts sit in every plan because information is what lets a bot act at all, and a scouting mission that is caught
// reports nothing. The sizes follow from the scouting rule itself: a party gets through only when it beats
// ScoutDefenseFactor per defending scout, so an offence village's scout line has to clear twice a defence village's
// home guard, with something left over for the reserve that lets it look twice.
//
//   - off keeps 700: the scouting line the baseline party is sized from, and a full party is what a village with no
//     variant of its own has to be able to send.
//   - def keeps 250: its job is holding a wall, where a scout defends as little as it attacks, so a home guard.
//   - mixed keeps 300: a home guard too. It is deliberately the largest line a 700 party can still see through, because
//     a mixed village is the shape most bot villages have and a bot that cannot look inside its neighbour never attacks.
var TroopTargets = map[VillageRole]map[UnitID]int{
	RoleOff: {UAxe: 4750, ULightCav: 2300, URam: 250, UCatapult: 50, UScout: 700},
	RoleDef: {USpear: 7300, ULightCav: 400, UHeavyCav: 1515, UScout: 250},
	RoleMixed: {USpear: 2500, USword: 2500, UAxe: 2500, UArcher: 3000, UScout: 300, ULightCav: 1300,
		UMountedArcher: 400, URam: 100},
}

// TroopVariants are the armies a role may field, each one a coherent force built around a different unit: an axe
// horde fights differently from a cavalry wing, which fights differently from a siege train. A village picks one and
// trains towards it, so two offence villages are not the same fight to attack and scouting one tells a raider very
// little about its neighbour.
//
// Every variant of a role costs the same population. That is the point of the list rather than scaling one plan by a
// fraction: a village should differ in what it fields, not in how strong it is, or the militia of a role would range
// from a soft target to a hard one for no reason a player could see. TestTroopVariantsKeepTheirStrength holds that,
// and TestAScoutLineIsInEveryVariant keeps reconnaissance working whichever variant a village rolls.
//
// The scout line varies between variants, and trades against the rest of the army. It has to stay at or above
// minScoutParty, or the village can never field a party and the scouts are dead weight — but within that, one offence
// village is quick and blind (650) and another is slow and watchful (1,200). That matters beyond the bot: an identical
// line in every variant made reconnaissance the one part of a village a player could read off the map.
//
// Equal population is not on its own equal strength, because the unit lines defend at very different values. Measured
// through ResolveBattle, the offence variants' attack ranges 403,800–518,500 and their defence against infantry
// 112,400–137,500; the defence variants' average defence across the three attacker classes ranges 436,800–527,700. A
// band that wide is the point — a role should be a different fight, not a softer or harder one — but a variant far
// outside it is a balance bug, not variety. TestVariantStrengthStaysInBand holds that.
var TroopVariants = map[VillageRole][]map[UnitID]int{
	RoleOff: {
		// The axe horde: nothing but the cheapest attacker there is, and the most of it. It is the strongest army here in
		// the open and the one with the least answer to a wall.
		{UAxe: 4750, ULightCav: 2300, URam: 250, UCatapult: 50, UScout: 700},
		// A cavalry wing, and the eyes of the army: it trades its axes for horses, so it arrives sooner and carries more
		// home. It keeps the largest scout line of any army here — 1,400, against 800–1,200 elsewhere — because
		// reconnaissance is what the variant is for. It cannot go much higher: the offence role's share of its army in
		// scouts is 17%, and 1,400 is already 16.5% of the plan.
		{UAxe: 550, ULightCav: 2600, UMountedArcher: 420, URam: 230, UScout: 1400},
		// A siege train: rams and catapults enough to open a walled village and wreck what is behind it, paid for out of
		// the axe line. At 420 rams it razes a level-20 wall with more to spare than it needs; the wrecking crew below is
		// the variant built around doing exactly that, and this one keeps its escorts.
		{UAxe: 3300, ULightCav: 1900, URam: 420, UCatapult: 300, UScout: 800},
		// Horse archers: the archer line on horseback. It grinds infantry down from a distance and comes home cheap.
		{UAxe: 200, ULightCav: 1600, UMountedArcher: 1450, URam: 230, UScout: 1000},
		// The wrecking crew: 240 rams open a level-20 wall in one battle — 220 is the exact threshold, so this carries a
		// little spare for the luck that goes against it — and 350 catapults then level what was behind the wall. It is the
		// army you send at a fortress, and the worst one here to meet an army in the open: the siege lines are 3,000
		// population the battle line does not get, so it carries the least cavalry of the role while still being able to
		// leave.
		{UAxe: 4000, ULightCav: 1925, URam: 240, UCatapult: 350, UScout: 650},
		// Skirmishers: light cavalry with a token siege line, the fastest army of the role and the best at reaching a
		// village before its support arrives.
		{UAxe: 500, ULightCav: 3200, UMountedArcher: 200, URam: 100, UScout: 1100},
		// A mounted host with barely a foot soldier: the extreme of the cavalry wing.
		{UAxe: 200, ULightCav: 2600, UMountedArcher: 600, URam: 200, UScout: 1200},
		// Light riders: no siege worth the name and no bows — three and a half thousand horsemen and the axes behind them.
		// It is the cheapest army to build and the worst to send at a wall.
		{UAxe: 250, ULightCav: 3550, URam: 150, UScout: 900},
	},
	RoleDef: {
		// The wall of spears: the cheapest defence there is, plus heavy cavalry to punish whatever reaches it, and the
		// smallest line of any role. Its 250 scouts are a home guard that can never look at anything — a party has to
		// beat 500 to see it, and the party it sends is sized from the 250 it holds, so it stays home. That is the trade
		// for the cheapest army here.
		{USpear: 7300, ULightCav: 400, UHeavyCav: 1515, UScout: 250},
		// An infantry wall: spears and swords together. A swordsman defends at 50 against infantry where a spear defends
		// at 15, which is what stops an axe horde; the spears are there because they cost nothing and hold more troops.
		{USpear: 7140, USword: 7330, ULightCav: 400, UHeavyCav: 270, UScout: 400},
		// Mixed arms: spears in front, swords behind, bows over their heads and heavy cavalry to counter.
		{USpear: 5490, USword: 2620, UArcher: 2270, ULightCav: 400, UHeavyCav: 985, UScout: 300},
		// The archer wall: an archer defends at 50 against infantry and 40 against cavalry, behind spears defending at 45
		// against cavalry. Five thousand bows held by eleven thousand spears is the strongest answer to a cavalry army
		// here, and the weakest to a ram fleet.
		{USpear: 10990, UArcher: 5300, ULightCav: 400, UScout: 300},
		// The sword wall: ten thousand swordsmen. The most defence per person a village can stand behind, and the
		// slowest army here to send anywhere.
		{USpear: 4990, USword: 10000, ULightCav: 400, UHeavyCav: 200, UScout: 350},
		// A cavalry reserve: fewer troops, each of them heavy cavalry. A heavy cavalryman defends at 200 against infantry
		// and 80 against archers, so this is the role's counter-attacking shape — worse than a wall against bows, the
		// best there is against an axe horde.
		//
		// It is also the role's one garrison with eyes. 600 is set by what the scouting formula allows in both
		// directions:
		//
		//   - it sends a party of 600, so it can look into anything keeping under 300 — the wall of spears, and the
		//     mixed-arms, archer-wall and shield-and-bow garrisons;
		//   - it is itself seen by a party of over 1,200, which only the offence cavalry wing (1,400) keeps. So it can be
		//     scouted, which matters: a garrison nothing can look at is a garrison bots can never choose to attack.
		//
		// It cannot go higher. At 800 it would see more, but no offence variant could then see it, which is why the two
		// numbers are pinned together rather than tuned for the best scout. It is paid for out of the spear front, so it
		// holds a line worse than the wall of spears.
		{USpear: 3090, ULightCav: 400, UHeavyCav: 2100, UScout: 600},
		// A militia levy: as many spearmen as the farm holds, with two thousand archers behind them. It is what a village
		// that spent its iron elsewhere looks like, and it is here so that not every defence village is a fortress.
		{USpear: 13690, UArcher: 2000, ULightCav: 500, UScout: 400},
		// Shield and bow for an army standing in the open: swords to hold, bows to kill, and no heavy cavalry at all.
		{USpear: 9290, USword: 3000, UArcher: 4000, ULightCav: 400, UScout: 300},
	},
	RoleMixed: {
		// A field army rather than a garrison: spears and swords to hold, axes to break, bows behind them and cavalry to
		// flank. It is the flexible role — it has an answer to everything and is the best at nothing. One variant, because
		// it is the shape a bot holds every village with.
		//
		// Its scout line stays at 300 and cannot be raised. A party has to beat ScoutDefenseFactor per defending scout, so
		// 400 would put every mixed village — most bot villages on the map — beyond the 700 baseline party, and a bot
		// that cannot look inside its neighbour stops taking villages at all: measured on the instant world, raising this
		// to 400 took seed 2 from eight villages changing hands in the hour to none, with the player's own conquest
		// stalled. The guard is a home guard, not a scouting line.
		{USpear: 2500, USword: 2500, UAxe: 2500, UArcher: 3000, UScout: 300, ULightCav: 1300, UMountedArcher: 400,
			URam: 100},
	},
}

// scoutShare is the most of an army's population a role may put into scouts. It is not a separate design goal but the
// arithmetic of the scouting rule: a party must be bigger than twice the guard it means to beat, and a village has to
// hold more than half a party back to look again, so a role that can both look and look again needs a fleet whatever
// the rest of its plan costs.
//
// Because the scout line is fixed while the rest of the plan is capped by the farm, this share is a consequence of the
// plan's size rather than a number chosen on its own: it rises whenever the plan has to shrink to fit. Defence sits at
// 7% because one of its variants carries a party of its own (600 scouts, 1,200 people) instead of a home guard; the rest
// of the role stays at a guard, so a defence village is usually read as a fortress and occasionally as an eye.
func scoutShare(role VillageRole) float64 {
	switch role {
	case RoleOff:
		return 0.17
	case RoleDef:
		return 0.07
	case RoleMixed:
		return 0.05
	}
	return 0.05
}

// Mistakes are the chances that a bot makes a given mistake (strong / weak; linear in skill between).
var Mistakes = struct {
	HybridVillage         StrongWeak `json:"hybridVillage"`
	EcoTo30BeforeMilitary StrongWeak `json:"ecoTo30BeforeMilitary"`
	IdleResources         StrongWeak `json:"idleResources"`
	ShortQueues           StrongWeak `json:"shortQueues"`
	BarracksDayOne        StrongWeak `json:"barracksDayOne"`
}{StrongWeak{0.02, 0.5}, StrongWeak{0, 0.6}, StrongWeak{0.05, 0.9}, StrongWeak{0.05, 0.9}, StrongWeak{0, 0.3}}

var Plays = struct {
	LongBuildBeforeSleep StrongWeak `json:"longBuildBeforeSleep"`
}{StrongWeak{0.9, 0.05}}

// CatapultsPerLevel is the catapults needed to knock one level off a building (index = level).
var CatapultsPerLevel = []int{
	0, 2, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16,
}

// EconomyPlay is how a bot tier builds up on a normal world: the mines first, iron behind wood and clay, no idle
// build queue, a build plan for the break, a farming army and the auto-farm. The zero play is the classic one,
// which short worlds always play.
type EconomyPlay struct {
	MinesOpening bool    `json:"minesOpening"` // MinesOpening instead of Opening
	MinesFirstTo int     `json:"minesFirstTo"` // after the opening only economy buildings until the mines reach it
	IronBehind   int     `json:"ironBehind"`   // iron waits until it is this many levels behind wood and clay
	FillQueue    bool    `json:"fillQueue"`    // an unaffordable pick gives way to an affordable wanted building
	PlanSteps    int     `json:"planSteps"`    // the build plan left for the break
	FarmAt       float64 `json:"farmAt"`       // the farm grows when less than this share is free, queued levels counted
	StorageHours float64 `json:"storageHours"` // the storage grows while it holds less than this much top income
	HideCap      int     `json:"hideCap"`      // unless farmed hard
	WallCap      int     `json:"wallCap"`      // unless farmed hard
	RushDays     float64 `json:"rushDays"`     // the start check-ins (BotConfig.StartRush) last at least this long
	CavalryAt    int     `json:"cavalryAt"`    // CavalryPath once wood and clay reach this level; 0 = never
	ArmyShare    float64 `json:"armyShare"`    // troops recruited first, from half the purse, up to this share of the farm
	AutoFarm     bool    `json:"autoFarm"`     // raids on the auto-farm tick once unlocked, as humans' auto-farm
}

// TierPlays: within a tier, farming styles keep an army from the start (light cavalry early, over half the farm in
// troops) and the other styles put the mines first with a small army.
type TierPlays struct {
	Economy EconomyPlay `json:"economy"`
	Farming EconomyPlay `json:"farming"`
}

// CavalryPath: barracks 5, smithy 5 and stable 3 for light cavalry to farm with.
var CavalryPath = []OpeningStep{{BMain, 5}, {BBarracks, 3}, {BSmithy, 1}, {BBarracks, 5}, {BSmithy, 3}, {BSmithy, 5},
	{BStable, 1}, {BStable, 2}, {BStable, 3}}

// EconomyPlays: strong bots keep the economy first to mines 18, the farming ones with the stable from mines 8; medium
// ones turn to troops at 15 and farm a little.
var EconomyPlays = map[BotTier]TierPlays{
	TierStrong: {
		Economy: strongPlay(14, 0.1),
		Farming: strongPlay(8, 0.55),
	},
	TierMedium: {Economy: mediumPlay, Farming: mediumPlay},
}

func strongPlay(cavalryAt int, armyShare float64) EconomyPlay {
	return EconomyPlay{MinesOpening: true, MinesFirstTo: 18, IronBehind: 3, FillQueue: true, PlanSteps: 24, FarmAt: 0.25,
		StorageHours: 12, HideCap: 3, WallCap: 5, RushDays: 4, CavalryAt: cavalryAt, ArmyShare: armyShare, AutoFarm: true}
}

var mediumPlay = EconomyPlay{MinesOpening: true, MinesFirstTo: 15, IronBehind: 3, FillQueue: true, PlanSteps: 12,
	FarmAt: 0.25, StorageHours: 12, HideCap: 3, WallCap: 5, RushDays: 2, CavalryAt: 12, ArmyShare: 0.3, AutoFarm: true}

// MinesOpening takes wood and clay to 10 and iron to 8, with storage and main alongside, then a barracks. The farm
// follows EconomyPlay.FarmAt.
var MinesOpening = minesOpening()

func minesOpening() []OpeningStep {
	var out []OpeningStep
	for lvl := 1; lvl <= 10; lvl++ {
		out = append(out, OpeningStep{BWood, lvl}, OpeningStep{BClay, lvl})
		if lvl <= 3 {
			out = append(out, OpeningStep{BIron, lvl})
		} else if lvl%4 != 0 {
			out = append(out, OpeningStep{BIron, lvl - lvl/4})
		}
		switch {
		case lvl == 2:
			out = append(out, OpeningStep{BStorage, 2})
		case lvl == 4:
			out = append(out, OpeningStep{BStorage, 3}, OpeningStep{BMain, 2}, OpeningStep{BBarracks, 1})
		case lvl >= 5 && lvl%2 == 1:
			out = append(out, OpeningStep{BStorage, lvl - 1}, OpeningStep{BMain, lvl / 2})
		}
	}
	return append(out, OpeningStep{BStorage, 10}, OpeningStep{BMain, 7}, OpeningStep{BBarracks, 3}, OpeningStep{BSmithy, 1})
}
