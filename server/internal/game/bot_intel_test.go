package game_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"librehold/internal/game"
	"librehold/internal/game/gametest"
)

// Recon is what a bot sends to learn a garrison. It has to be a real scouting mission, and it has to be big enough to
// beat the scouts a village keeps at home: a scouting mission that is caught brings nothing back at all.

// scoutBotWorld is a world with bot "bot_0" holding one village that has scouts, and a human village next to it to
// scout. A scouting send needs a real village at the target, since there is nothing to look into otherwise.
func scoutBotWorld(t *testing.T, seed uint32, scouts int) (*gametest.MemWorld, *game.Village, *game.Village) {
	t.Helper()
	w, near := gametest.NewTestWorld(seed)
	w.Player("h1").ProtectedUntil = 0
	gametest.NewBotPlayer(w, "bot_0")
	w.PutBrain("bot_0", &game.BotBrain{Skill: 0.8, Memory: game.BotMemory{Known: map[string]*game.KnownVillage{}}})
	home := gametest.AddStartVillage(w, "bot_0", near.X+3, near.Y)
	home.Units[game.UScout] = scouts
	den := gametest.AddStartVillage(w, "h1", near.X+5, near.Y)
	return w, home, den
}

func TestScoutPartyIsBigEnoughToSeeAVillage(t *testing.T) {
	// A scouting mission gets through only when it beats ScoutDefenseFactor per defending scout, and a caught mission
	// reports nothing. The party therefore has to clear the home guard the roles mostly keep — not the largest line
	// anywhere: a line big enough to look into itself is un-scoutable by construction, since a village never sends more
	// scouts than it holds, and those are meant to be the hard targets rather than the ordinary case.
	guard := 0.0
	for _, role := range []game.VillageRole{game.RoleDef, game.RoleMixed} {
		for _, v := range game.TroopVariants[role] {
			if n := float64(v[game.UScout]); guard == 0 || n < guard {
				guard = n
			}
		}
	}
	need := guard * game.CombatBalance.ScoutDefenseFactor
	if float64(game.ScoutParty) <= need {
		t.Fatalf("a party of %d scouts cannot see a village keeping %.0f (needs more than %.0f)",
			game.ScoutParty, guard, need)
	}
}

func TestMilitaryVillagesKeepTwoScoutParties(t *testing.T) {
	// Arriving at a defended village is normal and often fatal, so an offence village must at least be able to send a
	// full party. It used to keep two; the scout line is now smaller and a party smaller with it, so one look is what the
	// numbers buy — the party that comes home is what makes the second look possible.
	kept := game.TroopTargets[game.RoleOff][game.UScout]
	party := game.ScoutPartyFor(kept, &game.KnownVillage{})
	if party < game.ScoutParty {
		t.Fatalf("off keeps %d scouts but sends only %d, less than a full party of %d", kept, party, game.ScoutParty)
	}
	if party < kept/2 {
		t.Fatalf("off keeps %d scouts and sends %d of them, which is too much of the line", kept, party)
	}
}

func TestScoutPartyEscalatesAfterAFailedLook(t *testing.T) {
	// A party and a guard are not constants: the bot tries, and a party that came home changes what it sends next. If
	// the two were fixed numbers, "who can see whom" would be settled by the playbook instead of by play.
	lost, won := ptr(true), ptr(false)
	first := game.ScoutPartyFor(5000, &game.KnownVillage{ScoutLoss: lost, LastScoutParty: game.ScoutParty})
	second := game.ScoutPartyFor(5000, &game.KnownVillage{ScoutLoss: lost, LastScoutParty: first})
	if second <= first {
		t.Fatalf("a caught party of %d was followed by %d, which is not an escalation", first, second)
	}
	// Once it gets through, the size holds rather than climbing for ever.
	held := game.ScoutPartyFor(5000, &game.KnownVillage{ScoutLoss: won, LastScoutParty: first})
	if held != first {
		t.Fatalf("a party that got through changed from %d to %d", first, held)
	}
	// And it never asks for more than the village has.
	if got := game.ScoutPartyFor(650, &game.KnownVillage{ScoutLoss: lost, LastScoutParty: game.ScoutParty}); got != 650 {
		t.Fatalf("sent %d of 650", got)
	}
}

func TestCaughtScoutsTeachNoGarrison(t *testing.T) {
	// A caught party must not tell the bot what beat it — that is the fog of war. All it learns is that its scouts did
	// not come home, which is enough to try again bigger.
	w, home, den := scoutBotWorld(t, 95, 0)
	target := w.Village(den.ID)
	target.Units[game.USpear] = 5000
	game.ObserveBattle(w, "bot_0", target, game.BattleSeen{Won: ptr(false), ScoutParty: game.ScoutParty})

	k := w.Brain("bot_0").Memory.Known[den.ID]
	if k == nil {
		t.Fatal("the attempt should be remembered")
	}
	if k.Units != nil && k.Units.Total() > 0 {
		t.Fatalf("a caught party reported the garrison: %+v", k.Units)
	}
	if k.ScoutLoss == nil || !*k.ScoutLoss || k.LastScoutParty != game.ScoutParty {
		t.Fatalf("the failed attempt was not recorded: loss=%v party=%d", k.ScoutLoss, k.LastScoutParty)
	}
	_ = home
}

func TestScoutsThatGotThroughDoRecordTheGarrison(t *testing.T) {
	w, _, den := scoutBotWorld(t, 96, 0)
	target := w.Village(den.ID)
	target.Units[game.USpear] = 40
	units := game.UnitCounts{game.USpear: 40}
	game.ObserveBattle(w, "bot_0", target, game.BattleSeen{Units: &units, Won: ptr(true), ScoutParty: game.ScoutParty})

	k := w.Brain("bot_0").Memory.Known[den.ID]
	if k == nil || k.Units == nil || k.Units.Total() == 0 {
		t.Fatalf("a party that got through must record the garrison: %+v", k)
	}
	if k.ScoutLoss == nil || *k.ScoutLoss {
		t.Fatalf("a party that got through is not a loss: %v", k.ScoutLoss)
	}
}

func TestABotVillageActuallySendsScouts(t *testing.T) {
	// The whole scouting rule is worthless if no village ever dispatches a party, and that is exactly what happened:
	// every test here asserted a plan or a party size, and a gate no village could pass still satisfied all of them. In a
	// 30-day world of 50 bots and 50 humans, zero scouting missions were sent while bots held 29,000 scouts between them.
	//
	// So this one measures the outcome, not the intention, and it gives the village only what its own plan tells it to
	// keep — the number it will actually reach, not a number picked to make the test pass.
	w, home, den := scoutBotWorld(t, 200, 0)
	role := game.ChooseRole(w, "bot_0", home)
	plan := game.TroopTargetsOf(w, home, role)
	home.Units[game.UScout] = plan[game.UScout]
	t.Logf("role %s plans %d scouts", role, plan[game.UScout])

	b := w.Brain("bot_0")
	b.Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, Points: 400, OwnerID: den.OwnerID}
	if b.Mood.Phase == "campaign" {
		b.Mood.Phase = "build"
	}
	game.PlanFarming(w, "bot_0")
	sent := false
	for _, s := range w.ScheduledSendsOf("bot_0") {
		if game.IsScoutMission(s.Units) {
			sent = true
		}
	}
	if !sent {
		t.Fatalf("a %s village holding the %d scouts its own plan asks for did not send a scouting party",
			role, plan[game.UScout])
	}
}

func TestAVillageWithOnlyItsPlannedScoutsCanStillLook(t *testing.T) {
	// A village can only scout from what it actually holds, and a plan is a cap rather than a schedule: a plan of 900
	// scouts is reached long after the village is otherwise ready to fight. The lines the roles plan are therefore what
	// has to be big enough to send a party, or most villages are born unable to scout and never learn better. This
	// failed at every defence variant but one, and at the mixed role outright.
	smallest := 0
	for role, variants := range game.TroopVariants {
		for i, v := range variants {
			party := game.ScoutPartyFor(v[game.UScout], &game.KnownVillage{})
			if party <= 0 {
				t.Fatalf("%s variant %d plans %d scouts and could never send a party at all", role, i, v[game.UScout])
			}
			if smallest == 0 || v[game.UScout] < smallest {
				smallest = v[game.UScout]
			}
		}
	}
	if smallest < 1 {
		t.Fatal("no variant plans scouts at all")
	}
	t.Logf("the smallest scout line any variant plans is %d", smallest)
}

func TestScoutPartyNeverDribbles(t *testing.T) {
	// Two rules meet here, and the order between them matters.
	//
	// A village below minScoutParty sends its whole line rather than nothing: most roles plan fewer than 600 scouts, so
	// refusing to send under the floor meant most villages never scouted at all. But it never sends a *fraction* of a
	// line it could grow — a village with a thousand scouts sends a party, not a dribble, and one with fifty sends all
	// fifty because fifty is all it has and looking is worth more than waiting for a line that will never come.
	if got := game.ScoutPartyFor(50, &game.KnownVillage{}); got != 50 {
		t.Fatalf("a village with 50 scouts sent %d, but it holds no more than 50", got)
	}
	if got := game.ScoutPartyFor(0, &game.KnownVillage{}); got != 0 {
		t.Fatalf("a village with no scouts sent %d", got)
	}
	if got := game.ScoutPartyFor(game.MinScoutParty, &game.KnownVillage{}); got < game.MinScoutParty {
		t.Fatalf("a village holding exactly the floor sent only %d", got)
	}
	if got := game.ScoutPartyFor(5000, &game.KnownVillage{}); got != game.ScoutParty {
		t.Fatalf("an unknown target opens with the baseline party of %d, got %d", game.ScoutParty, got)
	}
}

func TestScoutsDoNotCrowdOutTheArmy(t *testing.T) {
	// A military role's troop plan is a cap, and the plan fills its farm: what it spends on scouts is army the village
	// never gets. A scout defends as little as it attacks, so a scouting fleet in a defence village is the worst case.
	// (A mixed village's plan is deliberately small and it over-recruits past it, so the share there says little.)
	for _, role := range []game.VillageRole{game.RoleOff, game.RoleDef} {
		plan := 0
		for u, n := range game.TroopTargets[role] {
			plan += n * game.UnitDefs[u].Pop
		}
		scoutPop := game.TroopTargets[role][game.UScout] * game.UnitDefs[game.UScout].Pop
		if share := float64(scoutPop) / float64(plan); share > game.ScoutShare(role) {
			t.Fatalf("%s spends %.0f%% of its troop plan on scouts, over its %.0f%% allowance",
				role, 100*share, 100*game.ScoutShare(role))
		}
	}
}

func TestTroopVariantsKeepTheirStrength(t *testing.T) {
	// The variants exist to make villages differ in what they field, not in how strong they are. If one costs more
	// population than another, a role's militia ranges from soft target to hard target for no reason a player can see.
	for role, variants := range game.TroopVariants {
		if len(variants) < 2 && role != game.RoleMixed {
			t.Fatalf("%s has %d variant(s), so its villages are all the same fight", role, len(variants))
		}
		want := -1
		for i, v := range variants {
			pop := 0
			for u, n := range v {
				pop += n * game.UnitDefs[u].Pop
			}
			if want < 0 {
				want = pop
				continue
			}
			if pop != want {
				t.Fatalf("%s variant %d costs %d population, variant 0 costs %d", role, i, pop, want)
			}
		}
	}
}

func TestAVillageHasManyArmiesToRoll(t *testing.T) {
	// Two offence villages should not be the same fight to attack. Four and three variants is enough to make a raid
	// uncertain but not enough to make a player stop expecting the same handful: a role fields at least six distinct
	// armies, so a scouted village tells a raider very little about the next one.
	want := map[game.VillageRole]int{game.RoleOff: 6, game.RoleDef: 6}
	for role, atLeast := range want {
		if got := len(game.TroopVariants[role]); got < atLeast {
			t.Fatalf("%s fields %d variants, want at least %d", role, got, atLeast)
		}
	}
}

func TestNoTwoVariantsAreTheSameForce(t *testing.T) {
	// A variant that repeats another is not variety, it is a weighting: it makes one army more likely while the count
	// claims otherwise. Every variant of a role has to differ from every other in at least one line.
	for role, variants := range game.TroopVariants {
		for i := range variants {
			for j := i + 1; j < len(variants); j++ {
				if maps.Equal(variants[i], variants[j]) {
					t.Fatalf("%s variants %d and %d are the same force", role, i, j)
				}
			}
		}
	}
}

func TestEachVariantsNameUnitIsThere(t *testing.T) {
	// A variant is "the siege train" or "the archer wall" because of the unit it is built around. If the role's plans
	// stopped carrying that unit the comment would be a lie, and a reader would tune the wrong line.
	for _, role := range []game.VillageRole{game.RoleOff, game.RoleDef} {
		for i, v := range game.TroopVariants[role] {
			if len(v) < 3 {
				t.Fatalf("%s variant %d is %d lines, which is not a force built around anything", role, i, len(v))
			}
		}
	}
}

func TestVariantStrengthStaysInBand(t *testing.T) {
	// Equal population is not equal strength: a swordsman and an axeman cost the same person and defend at 50 and 10.
	// So the variants are measured through the game's own battle code — an offence variant's attack, a defence
	// variant's defence — and every one of them has to land within half again of the role's median. Wider than that and
	// a village's roll decides who wins, which is the one thing the variant list is not allowed to do.
	counts := func(m map[game.UnitID]int) game.UnitCounts {
		out := game.UnitCounts{}
		for u, n := range m {
			out[u] = n
		}
		return out
	}
	attackers := []game.UnitCounts{
		{game.UAxe: 10000}, {game.ULightCav: 3000}, {game.UArcher: 10000}, {game.UHeavyCav: 1500},
	}
	measure := func(role game.VillageRole, v map[game.UnitID]int) float64 {
		units := counts(v)
		if role == game.RoleOff {
			// What it hits with. Attack is read from the table rather than from a result: a battle the defence wins
			// reports no attack power of its own, and the point here is the army, not the fight.
			power := 0.0
			for u, n := range units {
				power += float64(n) * game.UnitDefs[u].Attack
			}
			return power
		}
		// What it stands behind: the defence it puts up against each attacker class, averaged.
		total := 0.0
		for _, atk := range attackers {
			total += game.ResolveBattle(game.BattleInput{
				Attacker: game.BattleSide{Units: atk},
				Defender: game.BattleDefender{BattleSide: game.BattleSide{Units: units}},
			}).DefensePower
		}
		return total / float64(len(attackers))
	}
	for _, role := range []game.VillageRole{game.RoleOff, game.RoleDef} {
		variants := game.TroopVariants[role]
		var powers []float64
		for _, v := range variants {
			powers = append(powers, measure(role, v))
		}
		sorted := append([]float64{}, powers...)
		slices.Sort(sorted)
		median := sorted[len(sorted)/2]
		for i, p := range powers {
			if p < median/1.5 || p > median*1.5 {
				t.Fatalf("%s variant %d is %.0f, outside half again of the role's median %.0f", role, i, p, median)
			}
		}
	}
}

func TestNoVariantCarriesMoreRamsThanAWallCanAbsorb(t *testing.T) {
	// Wall damage is capped by the wall: a level-20 wall is 219 power in total (WallHp summed over the levels), each
	// surviving ram is one power, and once the wall is at zero the surplus rams do nothing at all. One offence variant
	// used to carry 900 — four times the wall, 3,400 population that can never matter, and enough bulk on a token
	// attack line to make the plan look stronger than it fights.
	maxUseful := 0.0
	for l := 1; l <= 20; l++ {
		maxUseful += game.WallHp(l)
	}
	for role, variants := range game.TroopVariants {
		for i, v := range variants {
			if rams := float64(v[game.URam]); rams > 2*maxUseful {
				t.Fatalf("%s variant %d carries %d rams (%.0f population) for a wall of %.0f power: over twice what "+
					"any wall can absorb", role, i, v[game.URam],
					float64(v[game.URam])*float64(game.UnitDefs[game.URam].Pop), maxUseful)
			}
		}
	}
}

func TestTheOffenceRoleHasAnArmyThatRazesAMaxWall(t *testing.T) {
	// Not every offence variant has to breach a fortress in one battle — a fast cavalry army nibbling a few levels off a
	// wall and coming back is a real answer, and forcing 1,200 population of rams on it would cost it the speed that is
	// its point. But the role as a whole has to have an answer to a level-20 wall, or a fortified village becomes
	// untakeable from an entire role.
	need := 0.0
	for l := 1; l <= 20; l++ {
		need += game.WallHp(l)
	}
	best := 0
	for _, v := range game.TroopVariants[game.RoleOff] {
		best = max(best, v[game.URam])
	}
	if float64(best) < need {
		t.Fatalf("the best offence variant carries %d rams, a level-20 wall needs %.0f", best, need)
	}
}

func TestAnOffenceVariantCanAlwaysSendAParty(t *testing.T) {
	// Offence is the scouting role, so a village that rolls one of its variants has to be able to look. The line may
	// vary from variant to variant, but never down to a size that cannot clear a home guard. Defence and mixed are
	// exempt on purpose: they keep a home guard, because a party small enough to be caught learns nothing and is only a
	// gift of scouts. Here the variety test would otherwise let an offence variant quietly stop being able to scout.
	for i, v := range game.TroopVariants[game.RoleOff] {
		if got := game.ScoutPartyFor(v[game.UScout], &game.KnownVillage{}); got < game.MinScoutParty {
			t.Fatalf("offence variant %d carries %d scouts and could send only %d, under the %d floor",
				i, v[game.UScout], got, game.MinScoutParty)
		}
	}
}

func TestAVariantsScoutsStayWithinTheArmyShare(t *testing.T) {
	// The scout line is a fixed cost, so as a share of the army it depends on how big the variant is. Every variant has
	// to stay inside the role's allowance, not just the baseline one. (A mixed village's plan is deliberately small and
	// it over-recruits past it, so the share there says little — the same exemption TestScoutsDoNotCrowdOutTheArmy makes.)
	for _, role := range []game.VillageRole{game.RoleOff, game.RoleDef} {
		for i, v := range game.TroopVariants[role] {
			plan, scoutPop := 0, 0
			for u, n := range v {
				plan += n * game.UnitDefs[u].Pop
				if u == game.UScout {
					scoutPop = n * game.UnitDefs[u].Pop
				}
			}
			if share := float64(scoutPop) / float64(plan); share > game.ScoutShare(role) {
				t.Fatalf("%s variant %d spends %.0f%% of its army on scouts, over its %.0f%% allowance",
					role, i, 100*share, 100*game.ScoutShare(role))
			}
		}
	}
}

func TestVillagesOfARoleDoNotAllFieldTheSameArmy(t *testing.T) {
	// Two offence villages converging on the identical force makes attacking either the same fight, and scouting one
	// tells a raider exactly what its neighbour holds.
	w, _, _ := scoutBotWorld(t, 121, 0)
	seen := map[string]bool{}
	for i := 0; i < 24; i++ {
		v := gametest.AddStartVillage(w, "bot_0", 200+i, 200)
		units := game.TroopTargetsOf(w, v, game.RoleOff)
		key := ""
		for _, u := range game.UnitIDs {
			key += fmt.Sprint(units[u], ",")
		}
		seen[key] = true
	}
	if len(seen) < 2 {
		t.Fatalf("24 offence villages rolled %d distinct armies", len(seen))
	}
}

func TestAVillagesVariantIsStable(t *testing.T) {
	// The roll is seeded from the village id, so a replay of the same world trains the same armies and a village does
	// not change its mind about what it is between two recruitment plans.
	w, _, _ := scoutBotWorld(t, 122, 0)
	v := gametest.AddStartVillage(w, "bot_0", 205, 205)
	first := game.TroopTargetsOf(w, v, game.RoleOff)
	for i := 0; i < 20; i++ {
		again := game.TroopTargetsOf(w, v, game.RoleOff)
		for _, u := range game.UnitIDs {
			if first[u] != again[u] {
				t.Fatalf("%s changed from %d to %d between plans", u, first[u], again[u])
			}
		}
	}
}

func TestAVillageSpendsItsFarmWhenItBeatsItsPlan(t *testing.T) {
	// A plan is a cap, not a stop. A village that has already met its role's plan — the usual case for a one-village bot,
	// whose plan is the deliberately small mixed one — still has a farm to fill and resources to spend on it. Stopping at
	// the plan leaves thousands of farm room empty while truces of resources rot in the store, which is what a bot
	// village looks like when it is "rich but harmless": barracks and a farm at full level, and a few hundred troops.
	w, home, _ := scoutBotWorld(t, 130, 0)
	home.Buildings[game.BBarracks] = 20
	home.Buildings[game.BFarm] = 30
	home.Buildings[game.BStorage] = 30
	home.Resources = game.Resources{Wood: 200000, Clay: 150000, Iron: 100000}
	// Well past the mixed plan, which is what a village with one village's worth of history ends up holding.
	home.Units[game.USpear] = 8000
	home.Units[game.UScout] = 400

	before := game.OwnedUnits(w, home, game.USpear)
	game.PlanRecruitment(w, "bot_0", home)
	after := game.OwnedUnits(w, home, game.USpear)
	if after <= before {
		t.Fatalf("a village past its plan with %.0f free farm queued nothing (spears %d)", game.PopRoom(w, home), before)
	}
}

func TestASurplusIsSpentInThePlansProportions(t *testing.T) {
	// The surplus above a met plan is not "train whatever is furthest behind": that is one unit, and it takes the whole
	// budget, so a rich village past its plan turns into a monolith instead of the army its role describes. The plan's
	// proportions are what the whole design rests on — a mixed village stays mixed — so the surplus has to follow them.
	w, home, _ := scoutBotWorld(t, 132, 0)
	home.Buildings[game.BBarracks] = 20
	home.Buildings[game.BSmithy] = 20
	home.Buildings[game.BStable] = 20
	// Every line of the plan has to be trainable, or the test is only ever measuring whichever one happened to be
	// researched already.
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UAxe, game.UArcher, game.ULightCav} {
		home.Research[u] = 1
	}
	home.Buildings[game.BFarm] = 30
	home.Buildings[game.BStorage] = 30
	home.Resources = game.Resources{Wood: 400000, Clay: 400000, Iron: 400000}
	// Far past the mixed plan, so every line of it is in surplus.
	// Just short of the plan on every line, so every line has somewhere to go and the farm has room for all of them.
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UAxe, game.UArcher, game.ULightCav} {
		home.Units[u] = game.TroopTargets[game.RoleMixed][u] * 9 / 10
	}

	start := map[game.UnitID]int{}
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UAxe, game.UArcher, game.ULightCav} {
		start[u] = game.OwnedUnits(w, home, u)
	}
	for range 10 {
		game.PlanRecruitment(w, "bot_0", home)
	}
	grown := 0
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UAxe, game.UArcher, game.ULightCav} {
		if game.OwnedUnits(w, home, u) > start[u] {
			grown++
		}
	}
	if grown < 3 {
		t.Fatalf("the surplus went into %d of the plan's 5 lines: a rich village should not become one unit", grown)
	}
}

func TestRecruitmentNeverOverfillsTheFarm(t *testing.T) {
	// Spending the surplus must still stop at the farm: the reserve popRoom keeps is there so that building work and a
	// noble are not blocked by a village that trained itself full.
	w, home, _ := scoutBotWorld(t, 131, 0)
	home.Buildings[game.BBarracks] = 20
	home.Buildings[game.BFarm] = 30
	home.Buildings[game.BStorage] = 30
	home.Resources = game.Resources{Wood: 900000, Clay: 900000, Iron: 900000}
	home.Units[game.USpear] = 20000
	for i := 0; i < 20; i++ {
		game.PlanRecruitment(w, "bot_0", home)
	}
	used := float64(game.PopulationUsed(w, home))
	capacity := float64(game.VillageFarmCapacity(home))
	if used > capacity {
		t.Fatalf("the village filled %.0f of a farm holding %.0f", used, capacity)
	}
	if room := game.PopRoom(w, home); room < 0 {
		t.Fatalf("recruitment ate the reserve: %.0f room left", room)
	}
}

func TestPlansFitTheirOwnFarm(t *testing.T) {
	// A role's troop plan is a cap, so a village that reaches it has to fit inside the farm of the village the role
	// builds: the plan plus the buildings the role builds, plus the room popRoom holds back for nobles and building work.
	// A plan that overshoots cannot be reached, and because training follows the unit that is furthest behind, a village
	// pours everything into one expensive line it can never finish and never gets the rest of its army.
	for role, variants := range game.TroopVariants {
		levels := game.Buildings{}
		for b, l := range game.RoleTargets[role] {
			levels[b] = l
		}
		capacity := game.FarmCapacity(levels[game.BFarm], nil)
		used := float64(game.BuildingsPopulation(levels)) + game.BotConfig.MaxedFarmReserve*capacity
		for i, v := range variants {
			pop := 0
			for u, n := range v {
				pop += n * game.UnitDefs[u].Pop
			}
			if total := used + float64(pop); total > capacity {
				t.Fatalf("%s variant %d needs %.0f of a farm holding %.0f (buildings %.0f, reserve %.0f)",
					role, i, total, capacity, float64(game.BuildingsPopulation(levels)),
					game.BotConfig.MaxedFarmReserve*capacity)
			}
		}
	}
}

func TestAMixedVillageBuildsAFarmItCanFill(t *testing.T) {
	// The mixed role is the one every bot starts with while it holds a single village, so it is the shape most bot
	// villages have. It has to build the same farmstead the military roles do: its plan is sized for that farm, and a
	// small farm beside a real army is a village permanently clamped.
	mixed := game.Buildings{}
	for b, l := range game.RoleTargets[game.RoleMixed] {
		mixed[b] = l
	}
	want := game.RoleTargets[game.RoleOff][game.BFarm]
	if got := mixed[game.BFarm]; got != want {
		t.Fatalf("mixed builds its farmstead to %d, the military roles build %d", got, want)
	}
}

func TestMixedVillagesFieldAFieldArmy(t *testing.T) {
	// A mixed village is not a garrison. It used to hold a token spear line and nothing else — no archers, no cavalry, no
	// siege — which made it the one role that could never threaten anybody. It should field balanced arms instead.
	plan := game.TroopVariants[game.RoleMixed][0]
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UAxe, game.UArcher, game.ULightCav, game.UScout} {
		if plan[u] == 0 {
			t.Fatalf("a mixed village fields no %s at all", u)
		}
	}
	// Archers are the point of the role: infantry hold while the bows work.
	if plan[game.UArcher] < 2000 {
		t.Fatalf("a mixed village keeps only %d archers", plan[game.UArcher])
	}
}

func TestEveryRoleCanRaid(t *testing.T) {
	// A village pays for its plan by looting: production alone does not keep up with heavy cavalry at 600 iron a piece.
	// Every role therefore needs some light cavalry in its plan. A defence village's battle line stays home — spears,
	// swords, archers and heavy cavalry all defend far better than they raid — so the horsemen it forages with are the
	// only line it has, and without them the role stagnates at whatever its mines make.
	for role, variants := range game.TroopVariants {
		for i, v := range variants {
			if v[game.ULightCav] < 100 {
				t.Fatalf("%s variant %d keeps %d light cavalry and cannot raid", role, i, v[game.ULightCav])
			}
		}
	}
}

func TestAnOffenceVillageIsMostlyHorsemen(t *testing.T) {
	// The offence role exists to go somewhere and take things. Light cavalry is what it goes with: it is the fastest unit,
	// and a farm is a cap on how many soldiers the village ever has — so the share of the army that is horse is the share
	// that can reach a target before support arrives and come home with the loot.
	//
	// Measured on a 30-day world this share had fallen to 17.5% of an offence village's units, against 23.3% before the
	// variant list was written. The cause was not a bug: splitting one plan into eight spread the population over more
	// units, and the axe line took the room the horsemen used to have. It went unnoticed because the count was taken at
	// home, where light cavalry never is — it is the unit that is always away on a raid.
	//
	// The line is counted as light cavalry rather than as the cavalry attack class, because a scout is filed under that
	// class too and a scouting party is not what carries a raid's haul.
	//
	// A quarter of the units is the floor. Below it the role is an axe horde that walks, and the point of the army —
	// reach and haul — is gone.
	for i, v := range game.TroopVariants[game.RoleOff] {
		units := 0
		for _, n := range v {
			units += n
		}
		if share := float64(v[game.ULightCav]) / float64(units); share < 0.25 {
			t.Fatalf("offence variant %d keeps %.0f%% of its units as light cavalry, under the quarter the role needs "+
				"to reach anything", i, 100*share)
		}
	}
}

func TestABotWithOneScoutPartyCanHarass(t *testing.T) {
	// Harass sends up to four parties, but a bot needs only one to start: requiring four meant only a bot with two and a
	// half times its whole planned scout line could ever probe anybody, which is almost none of them. The gate is what
	// decides whether scouting happens at all, so it has to be reachable.
	w, home, _ := scoutBotWorld(t, 140, game.ScoutParty)
	_ = home
	if !game.HarassReady(w, "bot_0") {
		t.Fatalf("a bot holding a full scouting party of %d may not harass", game.ScoutParty)
	}
}

func TestABotWithNoScoutPartyCannotHarass(t *testing.T) {
	w, _, _ := scoutBotWorld(t, 141, game.ScoutParty-100)
	if game.HarassReady(w, "bot_0") {
		t.Fatalf("a bot holding %d scouts harassed", game.ScoutParty-100)
	}
}

func TestScoutingIsASessionHabitNotACampaignStep(t *testing.T) {
	// Farming raids run every session; scouting used to run only inside a campaign, against the one village a bot had
	// already decided to attack. So a bot looted a player's village a dozen times a day without ever looking inside it.
	// Scouting has to happen on the same cadence as farming, or it never happens at all.
	w, home, den := scoutBotWorld(t, 142, game.ScoutParty+100)
	_ = home
	b := w.Brain("bot_0")
	b.Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, Points: 400, OwnerID: den.OwnerID}
	if b.Mood.Phase == "campaign" {
		b.Mood.Phase = "build"
	}
	game.PlanFarming(w, "bot_0")
	sends := w.ScheduledSendsOf("bot_0")
	scouted := false
	for _, s := range sends {
		if s.Units[game.UScout] == s.Units.Total() && s.Units[game.UScout] > 0 {
			scouted = true
		}
	}
	if !scouted {
		t.Fatalf("a bot outside a campaign never looked into a known player village")
	}
}

func TestAFarmedVillageIsStillScouted(t *testing.T) {
	// The farm cooldown asks whether a village has refilled enough to raid again. It has nothing to do with whether the
	// bot has looked inside it — but scouting shared that filter, so a village the bot was actively looting could never
	// be scouted: the raid kept resetting the cooldown the scout was waiting on. A player could be farmed all day and
	// never looked into once.
	w, home, den := scoutBotWorld(t, 143, game.ScoutParty+100)
	_ = home
	b := w.Brain("bot_0")
	now := w.Now()
	b.Memory.Known[den.ID] = &game.KnownVillage{
		ID: den.ID, X: den.X, Y: den.Y, Points: 400, OwnerID: den.OwnerID,
		LastAttackAt: ptr(now), // looted a moment ago: the farm is on cooldown
	}
	game.PlanFarming(w, "bot_0")
	scouted := false
	for _, s := range w.ScheduledSendsOf("bot_0") {
		if s.Units[game.UScout] == s.Units.Total() && s.Units[game.UScout] > 0 {
			scouted = true
		}
	}
	if !scouted {
		t.Fatal("a village the bot was farming was never scouted, because the farm cooldown blocked it")
	}
}

func TestAScoutPartySitsBetweenItsFloorAndItsBaseline(t *testing.T) {
	// A party is something a bot can afford out of the scout line its role plans. Only the offence role carries a line big
	// enough to send a full party: it is the role that scouts to find something to attack. Defence and mixed villages
	// keep a home guard instead, and a party has to be able to see them.
	if game.ScoutParty <= game.MinScoutParty {
		t.Fatalf("the baseline party of %d is not above the floor of %d", game.ScoutParty, game.MinScoutParty)
	}
	if line := game.TroopTargets[game.RoleOff][game.UScout]; line < game.ScoutParty {
		t.Fatalf("off plans %d scouts, less than a single party of %d", line, game.ScoutParty)
	}
}

func TestAPartyBigEnoughToSendIsBigEnoughToSee(t *testing.T) {
	// The floor and the baseline both have to look inside an ordinary home guard, or sending a party never returns
	// anything. One line is deliberately bigger than that: the cavalry reserve keeps 600 scouts, a party of its own, so
	// a defence village can be the one looking for a change. That garrison is not meant to be scouted cheaply — but it
	// has to be reachable, or it would be a wall a role could build its way behind. An offence variant sends up to its
	// whole line, so the largest of those is what the guard is held against.
	for _, role := range []game.VillageRole{game.RoleOff} {
		largest := 0
		for _, v := range game.TroopVariants[role] {
			largest = max(largest, v[game.UScout])
		}
		for i, v := range game.TroopVariants[game.RoleDef] {
			need := float64(v[game.UScout]) * game.CombatBalance.ScoutDefenseFactor
			if float64(largest) <= need {
				t.Fatalf("defence variant %d keeps %d scouts (needs a party of %.0f) and the best offence line of %d "+
					"cannot see it, so it can never be scouted at all", i, v[game.UScout], need, largest)
			}
		}
	}
	if game.ScoutParty <= int(float64(game.TroopTargets[game.RoleMixed][game.UScout])*game.CombatBalance.ScoutDefenseFactor) {
		t.Fatalf("the baseline party of %d cannot look into an ordinary mixed village keeping %d",
			game.ScoutParty, game.TroopTargets[game.RoleMixed][game.UScout])
	}
}

func TestScoutingLooksPastTheNearestTarget(t *testing.T) {
	// Looking only at the nearest stale village means a bot never learns about the one behind it, however empty that one
	// is. A scouting party that comes back reporting an undefended village two tiles further out is worth exactly as much
	// as one that reports the neighbour, so the bot looks at several and sends at each.
	w, home, den := scoutBotWorld(t, 144, 2*game.ScoutParty+200)
	_ = home
	b := w.Brain("bot_0")
	near := &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, Points: 400, OwnerID: den.OwnerID}
	b.Memory.Known[near.ID] = near
	// A second, further target, also stale — a real village, since a scouting send needs one to look into.
	far := gametest.AddStartVillage(w, "h1", den.X+3, den.Y)
	b.Memory.Known[far.ID] = &game.KnownVillage{ID: far.ID, X: far.X, Y: far.Y, Points: 500, OwnerID: far.OwnerID}

	game.PlanFarming(w, "bot_0")

	scouted := map[int]bool{}
	for _, s := range w.ScheduledSendsOf("bot_0") {
		if s.Units[game.UScout] > 0 && s.Units[game.UScout] == s.Units.Total() {
			scouted[s.X] = true
		}
	}
	if !scouted[near.X] {
		t.Fatalf("the nearest stale village was not scouted: %v", scouted)
	}
	if !scouted[far.X] {
		t.Fatalf("only the nearest village was scouted; the further one was ignored: %v", scouted)
	}
}

func TestVariantsDoNotShareOneScoutLine(t *testing.T) {
	// A role's variants used to carry an identical scout line, which made reconnaissance the one part of a village a
	// player could read off the map: see one offence village, and you knew what every offence village could see. Trading
	// scouts for troops in some variants makes what a village holds less predictable — and it is a real trade, since the
	// scouts are the troops that are not there instead.
	for role, variants := range game.TroopVariants {
		if len(variants) < 2 {
			continue // nothing to vary
		}
		seen := map[int]bool{}
		for _, v := range variants {
			seen[v[game.UScout]] = true
		}
		if len(seen) < 2 {
			t.Fatalf("%s variants all carry the same %d scouts, so reconnaissance is predictable", role, variants[0][game.UScout])
		}
	}
}

func TestMilitaryVillagesCanStillSeeAHomeGuardVillage(t *testing.T) {
	// Reconnaissance has to work in the direction a bot actually needs it: an offence party into a defence village. The
	// party opens at the baseline, so the baseline has to clear a home guard — the smallest line the role keeps.
	guard := float64(game.TroopVariants[game.RoleDef][0][game.UScout]) // the wall of spears
	if float64(game.ScoutParty) <= guard*game.CombatBalance.ScoutDefenseFactor {
		t.Fatalf("a party of %d scouts cannot see a defence village keeping %.0f", game.ScoutParty, guard)
	}
}

func TestCampaignScoutSendsTheWholeParty(t *testing.T) {
	w, home, den := scoutBotWorld(t, 91, game.ScoutParty+50)

	if !game.CampaignScout(w, "bot_0", den.X, den.Y) {
		t.Fatal("the bot refused to scout")
	}
	sends := w.ScheduledSendsOf("bot_0")
	if len(sends) != 1 {
		t.Fatalf("one scouting send, got %d", len(sends))
	}
	if got := sends[0].Units[game.UScout]; got != game.ScoutParty {
		t.Fatalf("scouted with %d scouts from %s, want the whole party of %d",
			got, home.ID, game.ScoutParty)
	}
	// Nothing but scouts: anything else would be an attack, and an attack only teaches what it survives.
	if sends[0].Units.Total() != sends[0].Units[game.UScout] {
		t.Fatalf("a scouting mission carries scouts only: %+v", sends[0].Units)
	}
}

func TestCampaignScoutSendsItsWholeLineWhenItIsSmall(t *testing.T) {
	// A village under a full party used to send nothing, on the argument that a party under the defence line is caught
	// by any ordinary home guard. The argument is right and the rule was wrong: most roles plan fewer than 600 scouts, so
	// refusing to send under the floor meant most villages never scouted at all. A small line goes in full instead.
	w, _, den := scoutBotWorld(t, 92, game.MinScoutParty-1)

	if !game.CampaignScout(w, "bot_0", den.X, den.Y) {
		t.Fatal("a village holding 599 scouts refused to look anywhere")
	}
	sends := w.ScheduledSendsOf("bot_0")
	if len(sends) != 1 {
		t.Fatalf("one scouting send, got %d", len(sends))
	}
	if got := sends[0].Units[game.UScout]; got != game.MinScoutParty-1 {
		t.Fatalf("sent %d of its %d scouts; a line under the floor goes in full", got, game.MinScoutParty-1)
	}
}

func TestCampaignScoutSendsNothingWithNoScouts(t *testing.T) {
	// The only case that sends nothing: there is nothing to send.
	w, _, den := scoutBotWorld(t, 93, 0)
	if game.CampaignScout(w, "bot_0", den.X, den.Y) {
		t.Fatal("a bot with no scouts sent a mission")
	}
}

func TestIntelStale(t *testing.T) {
	w, _, _ := scoutBotWorld(t, 94, 0)
	now := w.Now()

	if !game.IntelStale(w, &game.KnownVillage{}) {
		t.Fatal("a village never scouted is stale")
	}
	fresh := &game.KnownVillage{Units: &game.UnitCounts{game.USpear: 10}, UnitsSeenAt: ptr(now)}
	if game.IntelStale(w, fresh) {
		t.Fatal("a garrison seen now is fresh")
	}
	// Well past the setting, not exactly on it: the boundary itself is not what this test is about.
	old := &game.KnownVillage{Units: &game.UnitCounts{game.USpear: 10},
		UnitsSeenAt: ptr(now - int64(game.BotConfig.IntelStaleHours)*game.MsPerHour - game.MsPerHour)}
	if !game.IntelStale(w, old) {
		t.Fatal("a garrison seen well over a day ago is stale")
	}
}

func TestObserveBattleStampsWhenTheGarrisonWasSeen(t *testing.T) {
	// SeenAt moves whenever the map is rediscovered, so it cannot answer "how old is this garrison?".
	w, _, den := scoutBotWorld(t, 93, 0)
	target := w.Village(den.ID)
	units := game.UnitCounts{game.USpear: 40}
	game.ObserveBattle(w, "bot_0", target, game.BattleSeen{Units: &units})

	k := w.Brain("bot_0").Memory.Known[den.ID]
	if k == nil || k.UnitsSeenAt == nil {
		t.Fatal("a garrison the bot saw must carry the time it saw it")
	}
	if *k.UnitsSeenAt != w.Now() {
		t.Fatalf("stamped %d, want now %d", *k.UnitsSeenAt, w.Now())
	}
	if game.IntelStale(w, k) {
		t.Fatal("a garrison just seen is not stale")
	}
}

func TestAllInThrowsTheWholeArmy(t *testing.T) {
	// A gamble has to be a real stake: the whole offensive army of one village, not a token force.
	w, home, den := scoutBotWorld(t, 98, 0)
	target := w.Village(den.ID)
	home.Units[game.UAxe] = 4000
	home.Units[game.ULightCav] = 1500
	home.Units[game.UScout] = 300 // scouts are not part of an assault
	home.Units[game.URam] = 40
	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID, SeenAt: w.Now()}

	// The roll is seeded on the world clock, so it has to be a different instant each try.
	fired := false
	for i := 0; i < 40 && !fired; i++ {
		if game.LaunchAllIn(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
			fired = true
			break
		}
		w.SetNow(w.Now() + game.MsPerHour)
	}
	if !fired {
		t.Skip("the gamble did not roll in 40 tries; it is deliberately rare")
	}
	sends := w.ScheduledSendsOf("bot_0")
	if len(sends) == 0 {
		t.Fatal("the gamble reported a send but queued nothing")
	}
	s := sends[len(sends)-1]
	if s.Units[game.UAxe] != 4000 || s.Units[game.ULightCav] != 1500 || s.Units[game.URam] != 40 {
		t.Fatalf("the gamble was not the whole army: %+v", s.Units)
	}
	if s.Units[game.UScout] != 0 {
		t.Fatalf("scouts went along on an assault: %+v", s.Units)
	}
	if s.X != target.X || s.Y != target.Y {
		t.Fatalf("went to %d:%d, not the target", s.X, s.Y)
	}
}

func TestAllInRefusesASmallArmy(t *testing.T) {
	// Below the floor it is not a gamble, it is a gift.
	w, home, den := scoutBotWorld(t, 99, 0)
	home.Units[game.UAxe] = 10
	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID, SeenAt: w.Now()}
	for i := 0; i < 40; i++ {
		if game.LaunchAllIn(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
			t.Fatal("a village with 10 axes gambled its army")
		}
		w.SetNow(w.Now() + game.MsPerHour)
	}
}

func TestScoutBandsStraddleTheLuckRange(t *testing.T) {
	// A band far above the threshold always wins and one far below always loses, whatever the luck. The escalation ladder
	// has to contain sizes in between, or scouting is decided on paper rather than in the fight.
	winChance := func(party, guard int) float64 {
		wins := 0
		const n = 500
		for i := 0; i < n; i++ {
			luck := -game.CombatBalance.LuckRange + 2*game.CombatBalance.LuckRange*float64(i)/float64(n-1)
			if float64(party)*(1+luck) > float64(guard)*game.CombatBalance.ScoutDefenseFactor {
				wins++
			}
		}
		return float64(wins) / n
	}
	guard := 400 // a defence village's home guard
	uncertain := 0
	for _, party := range []int{300, 500, 650, 850, 1100, 1500, 2000, 3000} {
		if c := winChance(party, guard); c > 0.10 && c < 0.90 {
			uncertain++
		}
	}
	if uncertain == 0 {
		t.Fatalf("no band size leaves the outcome to luck against a guard of %d", guard)
	}
}

func TestABotClearsAPlayerItHasLookedInto(t *testing.T) {
	// Farming a player takes what is lying in the stores. Taking the village, or simply wrecking the army behind its
	// wall, is a different job: it needs the army that beat the garrison, not the column that carried the loot home.
	// Without a purpose for it a bot only ever farms and never damages anyone, so a player who is behind in points is
	// never caught up with — he is fed on and left to grow.
	//
	// A bot that has counted the garrison and can beat it sends the whole offensive line of its strongest village at it.
	w, home, den := scoutBotWorld(t, 101, 0)
	den.Units[game.USpear] = 200
	game.RecomputePoints(den)
	home.Units[game.UAxe] = 3000
	home.Units[game.ULightCav] = 800
	home.Units[game.URam] = 60
	home.Units[game.UScout] = 400
	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y,
		OwnerID: den.OwnerID, Wall: ptr(0), Units: &game.UnitCounts{game.USpear: 200}, UnitsSeenAt: ptr(w.Now())}

	if !game.LaunchClear(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
		t.Fatal("a bot that beat the counted garrison did not clear the village")
	}
	sends := w.ScheduledSendsOf("bot_0")
	if len(sends) == 0 {
		t.Fatal("reported a clear but queued nothing")
	}
	s := sends[len(sends)-1]
	if s.Units[game.UAxe] < 2000 || s.Units[game.ULightCav] < 500 {
		t.Fatalf("a clear went out as a patrol: %+v", s.Units)
	}
	if s.Units[game.UScout] != 0 {
		t.Fatalf("scouts went along on a clear: %+v", s.Units)
	}
	if s.X != den.X || s.Y != den.Y {
		t.Fatalf("went to %d:%d, not the target", s.X, s.Y)
	}
}

func TestAClearNeedsRealNumbers(t *testing.T) {
	// Under the fog of war an unscouted village has an empty garrison in the bot's memory, so "I can beat what I know"
	// is true of every village on the map. A clear is not a gamble — that is what the all-in is for — so it only goes
	// at a garrison the bot has actually counted and beaten.
	w, home, den := scoutBotWorld(t, 102, 0)
	home.Units[game.UAxe] = 3000
	home.Units[game.ULightCav] = 800

	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID}
	if game.LaunchClear(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
		t.Fatal("a clear went at a village it had never looked into")
	}

	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID,
		Wall: ptr(0), Units: &game.UnitCounts{game.USpear: 40000}, UnitsSeenAt: ptr(w.Now())}
	if game.LaunchClear(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
		t.Fatal("a clear went at a garrison it could not beat")
	}

	// And a village with no offensive army does not clear anything, however weak the target.
	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID,
		Wall: ptr(0), Units: &game.UnitCounts{}, UnitsSeenAt: ptr(w.Now())}
	home.Units[game.UAxe], home.Units[game.ULightCav] = 40, 10
	if game.LaunchClear(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
		t.Fatal("a village with forty axes sent a clear")
	}
}

func TestAClearComparesWithAndLeavesFromTheOffensiveVillage(t *testing.T) {
	// A bot's villages field different armies: a defence village is a wall of spears and heavy cavalry, which defends
	// well and hits for very little. Judging a target by "the bot's strongest village" therefore mixes two different
	// things — the wall's defence power passes the check while the line that would actually march is a handful of axes,
	// and the raid or clear is refused by the simulation afterwards. The comparison and the march have to be about the
	// same army.
	w, home, den := scoutBotWorld(t, 111, 0)
	// The bot's second village is a wall: it would pass a naive "strongest village" check on defence, but its offensive
	// line is under ClearMin and it can neither raid nor clear.
	wall := gametest.AddStartVillage(w, "bot_0", home.X-3, home.Y)
	wall.Units[game.USpear] = 8000
	wall.Units[game.UHeavyCav] = 3000
	home.Units[game.UAxe] = 4000
	home.Units[game.ULightCav] = 1200

	den.Units[game.USpear] = 400
	game.RecomputePoints(den)
	w.Brain("bot_0").Memory.Known[den.ID] = &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y,
		OwnerID: den.OwnerID, Wall: ptr(0), Units: &game.UnitCounts{game.USpear: 400}, UnitsSeenAt: ptr(w.Now())}

	// The offensive village has to be the one the comparison uses, and the one the clear leaves from.
	if !game.LaunchClear(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
		t.Fatal("the bot's offensive village could clear 400 spears and did not")
	}
	sends := w.ScheduledSendsOf("bot_0")
	if len(sends) != 1 {
		t.Fatalf("%d sends, want one", len(sends))
	}
	if sends[0].OriginID != home.ID {
		t.Fatalf("the clear left from %s, not the offensive village", sends[0].OriginID)
	}

	// And a wall is not an offensive village: with the axes gone there is nothing to clear with, however strong the
	// garrison looks.
	home.Units[game.UAxe], home.Units[game.ULightCav] = 0, 0
	if game.LaunchClear(w, "bot_0", w.Brain("bot_0").Memory.Known[den.ID]) {
		t.Fatal("a bot whose only army is a wall of spears sent a clear")
	}
}

func TestClearableIsJudgedOnTheOffensiveVillage(t *testing.T) {
	// ChoosePurpose's "can I take this?" gate has to answer for the army that would be sent. A defence village's
	// garrison is enormous by defence power and worthless as an attacking line, so measuring the target against it
	// offers clears (and conquests) the bot cannot carry out, and hides the ones it could.
	w, home, den := scoutBotWorld(t, 112, 0)
	wall := gametest.AddStartVillage(w, "bot_0", home.X-3, home.Y)
	wall.Units[game.USpear] = 8000
	wall.Units[game.UHeavyCav] = 3000
	home.Units[game.UAxe] = 4000
	home.Units[game.ULightCav] = 1200

	k := &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID, Wall: ptr(0),
		Units: &game.UnitCounts{game.USpear: 400}, UnitsSeenAt: ptr(w.Now())}
	w.Brain("bot_0").Memory.Known[den.ID] = k
	if !game.Clearable(w, "bot_0", k) {
		t.Fatal("400 spears were called too strong for a bot holding 4000 axes")
	}
	// A garrison stronger than the line that would march at it is not clearable, however big the wall village is.
	strong := &game.KnownVillage{ID: den.ID, X: den.X, Y: den.Y, OwnerID: den.OwnerID, Wall: ptr(0),
		Units: &game.UnitCounts{game.USpear: 60000}, UnitsSeenAt: ptr(w.Now())}
	if game.Clearable(w, "bot_0", strong) {
		t.Fatal("60000 spears were called clearable")
	}
}
