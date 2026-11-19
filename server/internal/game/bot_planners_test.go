package game_test

import (
	"slices"
	"testing"

	"librehold/internal/game"
	"librehold/internal/game/gametest"
)

// failOnBotErrors turns refused bot orders into test failures (planners must check before acting).
func failOnBotErrors(t *testing.T) {
	t.Helper()
	game.BotActionError = func(id game.PlayerID, err error) { t.Errorf("bot %s: %v", id, err) }
	t.Cleanup(func() { game.BotActionError = nil })
}

// oneBotWorld: one bot (bot_0) and human h1, speed 8, no protection, the bot's skill set.
func oneBotWorld(t *testing.T, seed uint32, skill float64) (*gametest.MemWorld, *game.Village) {
	t.Helper()
	failOnBotErrors(t)
	cfg := game.DefaultWorldConfig
	cfg.BotCount, cfg.BeginnerProtectionDays, cfg.BotStance = 1, 0, game.StancePeacefulRetaliate
	w, _ := gametest.NewWorldWithHuman(seed, cfg)
	w.Brain("bot_0").Skill = skill
	return w, game.PlayerVillages(w, "bot_0")[0]
}

// farmingVillage gives v a stable ready and 200 light cavalry plus 20 scouts.
func farmingVillage(v *game.Village) {
	b := &v.Buildings
	b[game.BMain], b[game.BBarracks], b[game.BStable], b[game.BSmithy], b[game.BFarm] = 10, 5, 3, 5, 15
	b[game.BStorage], b[game.BWood], b[game.BClay], b[game.BIron] = 10, 10, 10, 10
	v.Research[game.ULightCav], v.Research[game.UScout] = 1, 1
	v.Units[game.ULightCav], v.Units[game.UScout] = 200, 20
	game.RecomputePoints(v)
}

func TestBotEconomyPlanners(t *testing.T) {
	t.Run("the first village is mixed", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.9)
		if game.ChooseRole(w, "bot_0", v) != game.RoleMixed {
			t.Fatal("first village not mixed")
		}
	})

	t.Run("the second village gets a fixed off or def role", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		second := game.CreateVillage(v.X+3, v.Y+3, "Second", "", w.Now())
		w.PutVillage(second)
		game.SetOwner(w, second, "bot_0")
		role := game.ChooseRole(w, "bot_0", second)
		if role == game.RoleMixed && w.Brain("bot_0").Skill > 0.9 {
			// a strong bot rarely makes the hybrid mistake; the role must still be remembered
			t.Log("hybrid mistake drawn")
		}
		if game.ChooseRole(w, "bot_0", second) != role || w.Brain("bot_0").Roles[second.ID] != role {
			t.Fatal("role not remembered")
		}
	})

	t.Run("a full farmstead is a bottleneck: Farmstead first", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		v.Units[game.USpear] = game.VillageFarmCapacity(v)
		if b, ok := game.NextBuilding(w, "bot_0", v, game.Mulberry32(1)); !ok || b != game.BFarm {
			t.Fatalf("next %s %v", b, ok)
		}
	})

	t.Run("a full storehouse: Storehouse first", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		v.Resources.Wood = game.VillageStorageCapacity(v)
		if b, ok := game.NextBuilding(w, "bot_0", v, game.Mulberry32(1)); !ok || b != game.BStorage {
			t.Fatalf("next %s %v", b, ok)
		}
	})

	t.Run("a strong bot farmed every 30 min builds wall and hideout before the rest", func(t *testing.T) {
		for seed := uint32(1); seed <= 40; seed++ {
			w, bv := oneBotWorld(t, seed, 0.95)
			b := &bv.Buildings
			b[game.BMain], b[game.BBarracks], b[game.BStorage], b[game.BFarm], b[game.BWood], b[game.BClay], b[game.BIron] = 10, 5, 10, 10, 8, 8, 8
			w.SetNow(5 * game.MsPerHour)
			for i := range int64(8) {
				game.RecordHabit(w, "bot_0", "h1", game.HabitObservation{FarmedAt: ptr(w.Now() - (7-i)*30*game.MsPerMinute)})
			}
			if game.HardFarmer(w, "bot_0") != "h1" {
				continue // this seed's bot did not notice; covered statistically in TestBotAdaptation
			}
			if next, _ := game.NextBuilding(w, "bot_0", bv, game.Mulberry32(1)); next != game.BWall && next != game.BHide {
				t.Fatalf("seed %d next %s", seed, next)
			}
			return
		}
		t.Fatal("no seed where the bot noticed")
	})

	t.Run("build planning fills the slots with opening steps", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		v.Resources = game.Resources{Wood: 700, Clay: 700, Iron: 700} // below the storage bottleneck (800 of 1000)
		game.PlanBuilding(w, "bot_0", v)
		if len(v.BuildQueue) != 2 {
			t.Fatalf("queue %d", len(v.BuildQueue))
		}
		opening := map[game.BuildingID]bool{}
		for _, s := range game.Opening[:8] {
			opening[s.Building] = true
		}
		for _, j := range v.BuildQueue {
			if !opening[j.Building] {
				t.Fatalf("queued %s, not an early opening step", j.Building)
			}
		}
		if len(w.Brain("bot_0").ActionLog) != 2 {
			t.Fatal("actions not logged")
		}
	})

	t.Run("recruitment and research towards the role's targets", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		farmingVillage(v)
		v.Units = game.Units{}
		v.Resources = game.Resources{Wood: 20000, Clay: 20000, Iron: 20000}
		game.PlanResearch(w, "bot_0", v)
		if len(v.ResearchQueue) != 1 {
			t.Fatal("no research")
		}
		game.PlanRecruitment(w, "bot_0", v)
		queued := 0
		for _, b := range game.RecruitBuildings {
			queued += len(*v.RecruitQueues.Q(b))
		}
		if queued == 0 {
			t.Fatal("no recruitment")
		}
	})

	t.Run("before sleep, queues for the break or as much as is affordable", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		farmingVillage(v)
		// Below half the storage, so only the "cover the break" step recruits (not the spending-down step).
		v.Resources = game.Resources{Wood: 3000, Clay: 3000, Iron: 3000}
		next := w.Now() + 8*game.MsPerHour
		game.QueueForOffline(w, "bot_0", v, next)
		end := w.Now()
		for _, b := range game.RecruitBuildings {
			for _, j := range *v.RecruitQueues.Q(b) {
				end = max(end, j.CompletesAt)
			}
		}
		if end < next-2*game.MsPerHour && game.CanRecruitMore(w, "bot_0", v) {
			t.Fatalf("recruitment ends %d h before the session and more could be queued", (next-end)/game.MsPerHour)
		}
	})
}

func TestAChangedRoleConvertsTheGarrisonItNoLongerWants(t *testing.T) {
	// A bot's first village is mixed, trains the mixed field army, and becomes off or def the moment the bot founds a
	// second village. Before this it kept that army and trained the new plan on top: measured over 30 days, offence
	// villages carried 35,103 units of spear, sword and archer their plan never asks for, all of it leftover from the
	// mixed role, and their farms were full, so they could not train the cavalry the new role wanted.
	//
	// Keeping the role instead was tried and was worse — it lost the only villages that had time to mature — so the
	// contract is this: the role changes, and the garrison is converted when it does.
	w, home := oneBotWorld(t, 71, 0.9)
	// While it holds one village it is mixed, and that is what it builds.
	if got := game.ChooseRole(w, "bot_0", home); got != game.RoleMixed {
		t.Fatalf("a one-village bot's village is %s, want mixed", got)
	}
	mixed := game.TroopTargetsOf(w, home, game.RoleMixed)
	home.Units[game.USpear] = mixed[game.USpear]
	home.Units[game.USword] = mixed[game.USword]
	home.Units[game.UArcher] = mixed[game.UArcher]

	// A second village, which gives the first a military role of its own.
	gametest.AddStartVillage(w, "bot_0", home.X+5, home.Y+5)
	role := game.ChooseRole(w, "bot_0", home)
	if role == game.RoleMixed {
		t.Fatal("the village kept the mixed role after the bot grew, so the role split never happens")
	}
	plan := game.TroopTargetsOf(w, home, role)
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UArcher} {
		if plan[u] > 0 {
			continue
		}
		if home.Units[u] > 0 {
			t.Fatalf("the village became %s and still holds %d of %s, which its plan does not want",
				role, home.Units[u], u)
		}
	}
}

func TestAConqueredVillageAdoptsItsNewOwnersRole(t *testing.T) {
	// A conquered village arrives with the loser's garrison. That garrison is a different army from the one the new
	// role builds, so it has to be sold off rather than kept: kept, it fills the farm and the village can never train
	// the role it just joined. (The defence role has the same problem in reverse — a captured sword wall is not a
	// cavalry reserve.)
	w, home := oneBotWorld(t, 72, 0.9)
	second := gametest.AddStartVillage(w, "bot_0", home.X+4, home.Y+4)
	taken := gametest.AddStartVillage(w, "h1", home.X+6, home.Y+6)
	w.Player("h1").ProtectedUntil = 0
	// The loser was a defence village: it leaves behind the wall's infantry, which the new role may not want.
	taken.Units[game.USpear] = 3000
	taken.Units[game.USword] = 4000
	taken.Units[game.UArcher] = 3000
	game.TransferVillage(w, taken, game.Ref("bot_0"), "")
	_ = second
	if taken.Units[game.USpear]+taken.Units[game.USword]+taken.Units[game.UArcher] == 0 {
		t.Fatal("the conquered village kept no garrison, so this test checks nothing")
	}
	role := game.ChooseRole(w, "bot_0", taken)
	plan := game.TroopTargetsOf(w, taken, role)
	// Fill the farm, which is the situation a captured garrison creates: the village is over its plan and has no room.
	taken.Buildings[game.BFarm] = 10
	game.PlanRecruitment(w, "bot_0", taken)
	// Whatever the village holds that the plan does not want must be sold, not stacked on top of the plan.
	for _, u := range []game.UnitID{game.USpear, game.USword, game.UArcher} {
		if plan[u] > 0 {
			continue
		}
		if taken.Units[u] > 0 {
			t.Fatalf("a village that is now %s still holds %d of %s, which is not in its plan", role, taken.Units[u], u)
		}
	}
}

func TestAVillageTrainsOnlyWhatItsPlanWants(t *testing.T) {
	// The guard that would have caught all of this: what a village trains has to be what its own role's plan describes.
	// Every test before this one checked the plan, which is why a village that trained a completely different army
	// satisfied all of them. The village here is relabelled off while holding the mixed garrison it built as a
	// one-village bot — the exact case found in the 30-day world — and must not go on training the old army.
	w, v := oneBotWorld(t, 73, 0.9)
	v.Buildings[game.BBarracks] = 20
	v.Buildings[game.BSmithy] = 20
	v.Buildings[game.BStable] = 20
	v.Buildings[game.BWorkshop] = 20
	v.Buildings[game.BFarm] = 30
	v.Buildings[game.BStorage] = 30
	v.Resources = game.Resources{Wood: 900000, Clay: 900000, Iron: 900000}
	// It is a one-village bot, so it is mixed, and it holds the mixed army.
	role := game.ChooseRole(w, "bot_0", v)
	if role != game.RoleMixed {
		t.Fatalf("a one-village bot is %s, want mixed", role)
	}
	plan := game.TroopTargetsOf(w, v, role)
	for _, u := range game.UnitIDs {
		if plan[u] > 0 {
			v.Units[u] = plan[u]
		}
	}
	// Now it grows, and the village becomes off or def while holding that mixed garrison.
	gametest.AddStartVillage(w, "bot_0", v.X+5, v.Y+5)
	for range 50 {
		game.PlanRecruitment(w, "bot_0", v)
	}
	newRole := game.ChooseRole(w, "bot_0", v)
	newPlan := game.TroopTargetsOf(w, v, newRole)
	for _, u := range game.UnitIDs {
		if newPlan[u] > 0 {
			continue
		}
		if n := game.OwnedUnits(w, v, u); n > 0 {
			t.Fatalf("a village that is now %s holds %d of %s, which is not in its plan", newRole, n, u)
		}
	}
}

func TestBotFarmingPlanners(t *testing.T) {
	t.Run("unknown wall: barbarians are opened up, a player's village is not", func(t *testing.T) {
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		k := &game.KnownVillage{ID: "v:1:1", X: 1, Y: 1, Hauls: []float64{}}
		if u, ok := game.SizeRaid(w, v, k, 0.95); !ok || u != (game.UnitCounts{game.ULightCav: 4, game.UScout: 1}) {
			t.Fatalf("unknown wall barbarian %v %v", u, ok)
		}
		// A player's village may hold an army, and under the fog of war a group that dies reports nothing, so this is
		// not how a bot finds out what is standing there.
		k.OwnerID = "h1"
		if u, ok := game.SizeRaid(w, v, k, 0.95); ok {
			t.Fatalf("a player's village was probed with %v", u)
		}
		k.OwnerID = ""
		k.Wall = ptr(3)
		if _, ok := game.SizeRaid(w, v, k, 0.95); ok {
			t.Fatal("walled target raided")
		}
		k.Wall, k.Units = ptr(0), &game.UnitCounts{}
		u, ok := game.SizeRaid(w, v, k, 0.95)
		if !ok || u[game.ULightCav] < 4 || u.Total() != u[game.ULightCav] {
			t.Fatalf("known empty barbarian %v %v", u, ok)
		}
	})

	t.Run("a village raids with its attack troops, not only its light cavalry", func(t *testing.T) {
		// A village holding two light cavalry and a mountain of axes can raid perfectly well. Refusing because the
		// cavalry alone is too small leaves the axes at home for ever: nothing else a bot does spends an axe.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		v.Units[game.ULightCav] = 2
		v.Units[game.UAxe] = 400
		k := &game.KnownVillage{ID: "v:1:1", X: 1, Y: 1, Wall: ptr(0), Units: &game.UnitCounts{}}
		if u, ok := game.SizeRaid(w, v, k, 0.95); !ok || u[game.UAxe] == 0 {
			t.Fatalf("the raid left the axes at home: %v %v", u, ok)
		}
	})

	t.Run("spears loot when a village has nothing better", func(t *testing.T) {
		// Spears are a soldier's unit: they carry 25 and hit for 10, so they are poor raiders beside light cavalry. But a
		// defence village that has just founded a second village has spears and no stable yet, and leaving them home
		// while it sends nothing is the one case where a spear is the best unit available. It is an early-game fallback,
		// not a plan: they are the last choice, after the cavalry and the axes.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		v.Units[game.ULightCav] = 0
		v.Units[game.UAxe] = 0
		v.Units[game.USword] = 0
		v.Units[game.USpear] = 500
		k := &game.KnownVillage{ID: "v:1:1", X: 1, Y: 1, Wall: ptr(0), Units: &game.UnitCounts{}}
		u, ok := game.SizeRaid(w, v, k, 0.95)
		if !ok || u[game.USpear] == 0 {
			t.Fatalf("a village holding 500 spears and nothing else refused to loot: %v %v", u, ok)
		}
	})

	t.Run("heavy cavalry loots, but a clearing attack is still axes and cavalry", func(t *testing.T) {
		// Heavy cavalry carries a lot and hits hardest, so it is a fine choice for a farm run. Clearing a defended
		// village is a different job, and LaunchClear builds that force itself from axes, light cavalry and rams — it
		// never consults SizeRaid. Farming may therefore loot with heavy cavalry without a bot ever marching its best
		// defenders out as an attacking line.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		v.Units[game.ULightCav] = 0
		v.Units[game.UAxe] = 0
		v.Units[game.USpear] = 0
		v.Units[game.UHeavyCav] = 3000
		k := &game.KnownVillage{ID: "v:1:1", X: 1, Y: 1, Wall: ptr(0), Units: &game.UnitCounts{}}
		u, ok := game.SizeRaid(w, v, k, 0.95)
		if !ok || u[game.UHeavyCav] == 0 {
			t.Fatalf("a village holding only heavy cavalry refused to loot: %v %v", u, ok)
		}
	})

	t.Run("a player is farmed only after being scouted, and only when the troops survive", func(t *testing.T) {
		// The raid is sized by simulating against the garrison the bot remembers. With no memory of the village that
		// garrison is empty, so every raid "wins" on paper and the troops go into whatever is standing there. Scouting a
		// player is therefore what makes farming them possible at all: with a real count the simulation means something,
		// and a village the raiders cannot clear is left alone.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		v.Units[game.ULightCav] = 5000

		unscouted := &game.KnownVillage{ID: "v:2:2", X: 2, Y: 2, Wall: ptr(0), OwnerID: "h1"}
		if u, ok := game.SizeRaid(w, v, unscouted, 0.95); ok {
			t.Fatalf("farmed a player village it had never scouted, with %v", u)
		}

		empty := &game.KnownVillage{ID: "v:2:2", X: 2, Y: 2, Wall: ptr(0), OwnerID: "h1",
			Units: &game.UnitCounts{}, UnitsSeenAt: ptr(w.Now())}
		if u, ok := game.SizeRaid(w, v, empty, 0.95); !ok || u.Total() < 25 {
			t.Fatalf("refused a scouted, empty player village: %v %v", u, ok)
		}

		// A scouted garrison the raiders cannot beat is not a farm.
		strong := &game.KnownVillage{ID: "v:2:2", X: 2, Y: 2, Wall: ptr(0), OwnerID: "h1",
			Units: &game.UnitCounts{game.USpear: 40000}, UnitsSeenAt: ptr(w.Now())}
		if u, ok := game.SizeRaid(w, v, strong, 0.95); ok {
			t.Fatalf("attacked a garrison it could not clear, with %v", u)
		}

		// A barbarian is empty by definition, so no look is needed.
		barb := &game.KnownVillage{ID: "v:3:3", X: 3, Y: 3, Wall: ptr(0), Units: &game.UnitCounts{}}
		if b, ok := game.SizeRaid(w, v, barb, 0.95); !ok || b.Total() == 0 {
			t.Fatalf("a barbarian was skipped: %v %v", b, ok)
		}
	})

	t.Run("a player's village is raided by a column, not a patrol", func(t *testing.T) {
		// A player's village holds far more than a barbarian's hut, and the whole point of farming one is the haul: its
		// stores are what a bot cannot get from barbarians. Sizing the party by simulation against the garrison alone
		// finds the smallest group that wins, which for an undefended player village is a handful of horsemen — the
		// raid that players actually see, and read as a broken attack rather than a raid.
		//
		// So a player's village is farmed with a column: a share of the raiders the village has (PlayerRaidShare), and
		// never fewer than PlayerRaidFloor. A barbarian keeps the smallest group that wins, since nobody reads those
		// reports and every horse left at home is one that could have been raiding somewhere else.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		v.Units[game.ULightCav], v.Units[game.UAxe] = 1200, 2500

		empty := &game.KnownVillage{ID: "v:2:2", X: 2, Y: 2, Wall: ptr(0), OwnerID: "h1",
			Units: &game.UnitCounts{}, UnitsSeenAt: ptr(w.Now())}
		u, ok := game.SizeRaid(w, v, empty, 0.95)
		if !ok {
			t.Fatal("refused a scouted, empty player village")
		}
		if u.Total() < game.PlayerRaidFloor {
			t.Fatalf("a player's village was raided with %d units, under the floor of %d: %v", u.Total(), game.PlayerRaidFloor, u)
		}
		// It is a share of the army rather than a minimum, so a village with four thousand raiders does not send a
		// thousand and call it a raid: the stores a player sits on are far bigger than a barbarian's, and the trip is
		// only worth making with a force that can move a real part of them.
		raiders := 0
		for _, unit := range []game.UnitID{game.ULightCav, game.UHeavyCav, game.UMountedArcher, game.UAxe, game.USpear} {
			raiders += v.Units[unit]
		}
		if min := int(float64(raiders) * game.PlayerRaidShare * 0.9); u.Total() < min {
			t.Fatalf("a village with %d raiders sent %d at a player, under %.0f%% of them", raiders, u.Total(), 90*game.PlayerRaidShare)
		}
		// And it is still capped by the village: a bot does not empty its home to loot one farm.
		if u[game.ULightCav] > v.Units[game.ULightCav] || u[game.UAxe] > v.Units[game.UAxe] {
			t.Fatalf("the column is bigger than the village: %v", u)
		}

		barb := &game.KnownVillage{ID: "v:3:3", X: 3, Y: 3, Wall: ptr(0), Units: &game.UnitCounts{}}
		b, ok := game.SizeRaid(w, v, barb, 0.95)
		if !ok {
			t.Fatal("a barbarian was skipped")
		}
		if b.Total() >= u.Total() {
			t.Fatalf("a barbarian was raided with %d and a player with %d: the player's raid has to be the bigger one", b.Total(), u.Total())
		}
	})

	t.Run("a small village farms barbarians and leaves players alone", func(t *testing.T) {
		// The floor is a promise about what a player sees. A village that cannot field a column has no raid to make on a
		// person, and sending a patrol anyway is the behaviour the floor exists to stop — so it keeps farming barbarians
		// with the little it has until it can send something worth sending.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		v.Units[game.ULightCav], v.Units[game.UAxe] = 200, 200

		empty := &game.KnownVillage{ID: "v:2:2", X: 2, Y: 2, Wall: ptr(0), OwnerID: "h1",
			Units: &game.UnitCounts{}, UnitsSeenAt: ptr(w.Now())}
		if u, ok := game.SizeRaid(w, v, empty, 0.95); ok {
			t.Fatalf("a village with 400 raiders sent %v at a player", u)
		}
		barb := &game.KnownVillage{ID: "v:3:3", X: 3, Y: 3, Wall: ptr(0), Units: &game.UnitCounts{}}
		if b, ok := game.SizeRaid(w, v, barb, 0.95); !ok || b.Total() == 0 {
			t.Fatalf("the village stopped farming barbarians too: %v %v", b, ok)
		}
	})

	t.Run("a village never loots with the infantry of its wall", func(t *testing.T) {
		// Swords and archers are the wall's infantry: they carry 15 and 10, defend best standing still, and are the last
		// thing a village should send anywhere. Heavy cavalry and mounted archers are different — they loot, but only
		// loot, never as an attacking line.
		w, v := oneBotWorld(t, 41, 0.95)
		farmingVillage(v)
		for _, u := range []game.UnitID{game.ULightCav, game.UAxe, game.USpear, game.UHeavyCav, game.UMountedArcher} {
			v.Units[u] = 0
		}
		v.Units[game.USword] = 3000
		v.Units[game.UArcher] = 3000
		k := &game.KnownVillage{ID: "v:1:1", X: 1, Y: 1, Wall: ptr(0), Units: &game.UnitCounts{}}
		if u, ok := game.SizeRaid(w, v, k, 0.95); ok {
			t.Fatalf("looted with sword and archer infantry: %v", u)
		}
	})

	t.Run("wall > 0 without rams and a dry farm are skipped", func(t *testing.T) {
		w, home := oneBotWorld(t, 41, 0.95)
		farmingVillage(home)
		near := game.GeneratedVillagesNear(w.Seed(), w.Config(), home.X, home.Y, 6)
		walled := game.EnsureVillage(w, near[0].X, near[0].Y)
		dry := game.EnsureVillage(w, near[1].X, near[1].Y)
		walled.Buildings[game.BWall] = 2
		game.ObserveBattle(w, "bot_0", walled, game.BattleSeen{Units: &game.UnitCounts{}, Wall: ptr(2)})
		for range 3 {
			game.ObserveBattle(w, "bot_0", dry, game.BattleSeen{Units: &game.UnitCounts{}, Wall: ptr(0), HaulRatio: ptr(0.1)})
		}
		var targets []string
		defer game.SpyRaids(func(id game.PlayerID, c *game.Command, night bool, next int64) { targets = append(targets, c.TargetID) })()
		game.PlanFarming(w, "bot_0")
		if len(targets) < 10 || slices.Contains(targets, walled.ID) || slices.Contains(targets, dry.ID) {
			t.Fatalf("%d raids, walled %v, dry %v", len(targets), slices.Contains(targets, walled.ID), slices.Contains(targets, dry.ID))
		}
		k := w.Brain("bot_0").Memory.Known[targets[0]]
		if k == nil || *k.LastAttackAt != w.Now() {
			t.Fatal("raid not remembered")
		}
	})

	t.Run("farm targets: no own villages and no active players under a peaceful stance", func(t *testing.T) {
		w, home := oneBotWorld(t, 41, 0.95)
		for _, k := range game.FarmCandidates(w, "bot_0", home, 25) {
			if k.OwnerID == "bot_0" || k.OwnerID == "h1" {
				t.Fatalf("candidate %+v", k)
			}
		}
	})

	t.Run("a strong bot's night raids return within ±60 min of the next session", func(t *testing.T) {
		w, home := oneBotWorld(t, 41, 0.95)
		farmingVillage(home)
		next := w.Now() + 6*game.MsPerHour
		var backs []int64
		defer game.SpyRaids(func(id game.PlayerID, c *game.Command, night bool, n int64) {
			if night {
				backs = append(backs, c.ArrivesAt+(c.ArrivesAt-c.DepartedAt))
			}
		})()
		game.PlanNightRuns(w, "bot_0", next)
		if len(backs) == 0 {
			t.Fatal("no night runs")
		}
		for _, b := range backs {
			if b < next-game.MsPerHour || b > next+game.MsPerHour {
				t.Fatalf("back %d min off", (b-next)/game.MsPerMinute)
			}
		}
	})
}

func TestBotTargets(t *testing.T) {
	t.Run("known defence, offence and target score", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		k := &game.KnownVillage{ID: "x", Units: &game.UnitCounts{game.USpear: 10}, Wall: ptr(5), Hauls: []float64{}}
		if d, ok := game.KnownDefence(k); !ok || d != 150*1.2 {
			t.Fatalf("defence %v", d)
		}
		v.Units[game.UAxe] = 100
		if game.OwnOffence(w, "bot_0") != 4000 {
			t.Fatal("offence")
		}
		human := w.Village(w.Player("h1").VillageIDs[0])
		hk := &game.KnownVillage{ID: human.ID, X: human.X, Y: human.Y, OwnerID: "h1", Points: human.Points, Hauls: []float64{}}
		if game.ScoreTarget(w, "bot_0", hk) != 0 {
			t.Fatal("peaceful stance: human scored")
		}
		w.Config().BotStance = game.StanceNormal
		normal := game.ScoreTarget(w, "bot_0", hk)
		w.Config().BotStance = game.StanceAllAgainstHumans
		if normal <= 0 || game.ScoreTarget(w, "bot_0", hk) != 5*normal {
			t.Fatalf("normal %v all-against %v", normal, game.ScoreTarget(w, "bot_0", hk))
		}
		w.Player("h1").ProtectedUntil = w.Now() + 1
		if game.ScoreTarget(w, "bot_0", hk) != 0 {
			t.Fatal("protected scored")
		}
	})

	t.Run("sleep window from the public hourly history", func(t *testing.T) {
		w, _ := oneBotWorld(t, 31, 0.95)
		p := w.Player("h1")
		w.SetNow(7 * game.MsPerDay)
		p.History.Hourly = nil
		pts := 100
		for i := range 7 * 24 {
			hour := game.LocalMinuteOfDay(w, w.Now()-int64(7*24-1-i)*game.MsPerHour-1) / 60
			if hour >= 7 {
				pts += 5
			}
			p.History.Hourly = append(p.History.Hourly, pts)
		}
		win, ok := game.LikelyOfflineWindow(w, "h1")
		if !ok || win.StartMin != 0 || win.EndMin != 7*60 {
			t.Fatalf("window %+v %v", win, ok)
		}
	})

	t.Run("a campaign under “all against humans” targets a human, even a distant one", func(t *testing.T) {
		cfg := gametest.TestConfig()
		cfg.BotCount, cfg.BeginnerProtectionDays, cfg.BotStance = 40, 0, game.StanceAllAgainstHumans
		failOnBotErrors(t)
		w, _ := gametest.NewWorldWithHuman(63, cfg)
		picks, human := 0, 0
		for _, p := range w.Players() {
			if p.IsHuman || p.Status != game.StatusActive {
				continue
			}
			w.Village(p.VillageIDs[0]).Units[game.UAxe] = 3000
			b := w.Brain(p.ID)
			b.Mood = game.Mood{Phase: "campaign", Until: w.Now() + game.MsPerDay}
			game.PickCampaignTarget(w, p.ID)
			picks++
			if b.Mood.TargetPlayer == "h1" {
				human++
			}
		}
		if float64(human) < 0.8*float64(picks) {
			t.Fatalf("%d/%d picked the human", human, picks)
		}
	})
}

func TestBotExpansionPlanner(t *testing.T) {
	t.Run("a strong bot with troops builds the path to the Academy", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		b := &v.Buildings
		b[game.BMain], b[game.BSmithy], b[game.BMarket], b[game.BStorage], b[game.BFarm], b[game.BBarracks] = 20, 20, 9, 25, 25, 20
		v.Resources = game.Resources{Wood: 300000, Clay: 300000, Iron: 300000}
		// Half an army: readyForNobles wants half the mixed role's plan, so the fixture carries half of every unit the
		// plan names. A token spear line no longer counts as troops to march a noble with.
		for u, n := range game.TroopTargets[game.RoleMixed] {
			v.Units[u] = n / 2
		}
		game.PlanExpansion(w, "bot_0")
		if len(v.BuildQueue) == 0 || v.BuildQueue[0].Building != game.BMarket {
			t.Fatalf("queue %+v", v.BuildQueue)
		}
	})

	t.Run("coins and a noble with an Academy", func(t *testing.T) {
		w, v := oneBotWorld(t, 31, 0.95)
		v.Buildings[game.BAcademy], v.Buildings[game.BFarm], v.Buildings[game.BStorage] = 1, 25, 30
		v.Resources = game.Resources{Wood: 400000, Clay: 400000, Iron: 400000}
		game.PlanExpansion(w, "bot_0")
		if w.Player("bot_0").Coins == 0 && len(v.RecruitQueues.Academy) == 0 {
			t.Fatal("no coin and no noble")
		}
		if len(v.RecruitQueues.Academy) != 1 {
			t.Fatalf("noble queue %d", len(v.RecruitQueues.Academy))
		}
	})
}
