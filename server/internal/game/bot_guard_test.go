package game_test

import (
	"testing"

	"librehold/internal/game"
	"librehold/internal/game/gametest"
)

// A village must not be emptied by its own raids: a scout that finds nobody home is a free village.

// guardedVillage: a village with the given units and the farms to hold them.
func guardedVillage(t *testing.T, skill float64, units game.UnitCounts) (*gametest.MemWorld, *game.Village) {
	t.Helper()
	w, v := oneBotWorld(t, 31, skill)
	b := &v.Buildings
	*b = game.Buildings{}
	b[game.BMain], b[game.BBarracks], b[game.BSmithy], b[game.BStable] = 20, 20, 20, 20
	b[game.BFarm], b[game.BStorage], b[game.BWood], b[game.BClay], b[game.BIron] = 30, 30, 25, 25, 25
	v.Units = game.Units(units)
	v.Resources = game.Resources{}
	game.RecomputePoints(v)
	return w, v
}

func TestHomeGuard(t *testing.T) {
	t.Run("the defensive units never leave on a raid", func(t *testing.T) {
		// Your rule: swordsmen, archers and mounted archers hold the village. A raid takes haulers and siege.
		_, v := guardedVillage(t, strongSkill, game.UnitCounts{
			game.USword: 500, game.UArcher: 400, game.UMountedArcher: 300, game.UHeavyCav: 200,
			game.ULightCav: 1000,
		})
		got, ok := game.WithinGuard(v, game.UnitCounts{
			game.USword: 500, game.UArcher: 400, game.UMountedArcher: 300, game.UHeavyCav: 200, game.ULightCav: 300,
		})
		if !ok {
			t.Fatal("a village with 1000 light cavalry could not raid at all")
		}
		for _, u := range []game.UnitID{game.USword, game.UArcher, game.UMountedArcher, game.UHeavyCav} {
			if got[u] != 0 {
				t.Fatalf("%s left on a raid: %d", u, got[u])
			}
		}
		if got[game.ULightCav] != 300 {
			t.Fatalf("the haulers were trimmed: %d of 300", got[game.ULightCav])
		}
	})

	t.Run("a garrison-only village sends nothing", func(t *testing.T) {
		_, v := guardedVillage(t, strongSkill, game.UnitCounts{game.USword: 400, game.UArcher: 300})
		if _, ok := game.WithinGuard(v, game.UnitCounts{game.USword: 400}); ok {
			t.Fatal("a village of swordsmen sent them on a raid")
		}
	})

	t.Run("no share of the raiders is held back", func(t *testing.T) {
		_, v := guardedVillage(t, strongSkill, game.UnitCounts{game.ULightCav: 1000})
		// 1000 light cavalry is 4000 population and none of it is garrison, so nearly all of it may go: keeping a
		// quarter home would only make the bot farm worse than the player it imitates.
		// The floor is 10 population, so 3990 of the 4000 may go — not a quarter held back.
		if raidable := game.Raidable(v); raidable < 3990 {
			t.Fatalf("raidable %.0f of 4000, want all but the floor", raidable)
		}
		got, ok := game.WithinGuard(v, game.UnitCounts{game.ULightCav: 1000})
		if !ok || got[game.ULightCav] < 990 {
			t.Fatalf("%d of 1000 light cavalry could raid", got[game.ULightCav])
		}
	})

	t.Run("a small village still shows a scout a garrison", func(t *testing.T) {
		_, v := guardedVillage(t, strongSkill, game.UnitCounts{game.ULightCav: 30})
		// 30 light cavalry is 120 population; a quarter of that is 30, under the floor of 10 units of population,
		// so the floor is what is kept.
		if g := game.Garrison(v); g < game.BotConfig.HomeGuardPop {
			t.Fatalf("garrison %.0f is under the floor %.0f", g, game.BotConfig.HomeGuardPop)
		}
	})

	t.Run("a share of the scouts stays home", func(t *testing.T) {
		_, v := guardedVillage(t, strongSkill, game.UnitCounts{game.ULightCav: 400, game.UScout: 40})
		got, ok := game.WithinGuard(v, game.UnitCounts{game.ULightCav: 20, game.UScout: 40})
		if !ok {
			t.Fatal("nothing could raid")
		}
		if got[game.UScout] != 20 {
			t.Fatalf("raid took %d scouts, want half of 40", got[game.UScout])
		}
		if game.OffScouts(v) != 20 {
			t.Fatalf("offScouts %d, want 20", game.OffScouts(v))
		}
		// The last scout never leaves, even when there is only one.
		v.Units[game.UScout] = 1
		if game.OffScouts(v) != 0 {
			t.Fatal("the village's only scout was allowed to leave")
		}
	})

	t.Run("the guard never trims a village below its garrison, and says so", func(t *testing.T) {
		_, v := guardedVillage(t, strongSkill, game.UnitCounts{game.ULightCav: 100})
		// A group far larger than the raid-able pool is cut down to it rather than refused.
		got, ok := game.WithinGuard(v, game.UnitCounts{game.ULightCav: 100})
		if !ok {
			t.Fatal("nothing could raid")
		}
		if float64(game.UnitCounts(got).Total()) == 100 {
			t.Fatal("the whole army left")
		}
		if raidable := game.Raidable(v); float64(game.UnitCounts(got).Total()*game.UnitDefs[game.ULightCav].Pop) > raidable {
			t.Fatalf("sent more than is raidable: %.0f of %.0f", float64(game.UnitCounts(got).Total()*game.UnitDefs[game.ULightCav].Pop), raidable)
		}
	})
}

// A bot's villages must not be raided away by their own raids, and must not strip themselves to help a neighbour.

// twoDefVillages: bot_0 with two villages, the second a def-role garrison of `spears`.
func twoDefVillages(t *testing.T, spears int) (*gametest.MemWorld, *game.Village, *game.Village) {
	t.Helper()
	cfg := game.DefaultWorldConfig
	cfg.BotCount, cfg.BeginnerProtectionDays, cfg.BotStance = 1, 0, game.StancePeacefulRetaliate
	w, _ := gametest.NewWorldWithHuman(31, cfg)
	b := w.Brain("bot_0")
	b.Skill = 0.95

	first := game.PlayerVillages(w, "bot_0")[0]
	second := game.CreateVillage(first.X+6, first.Y+6, "Second", "", w.Now())
	w.PutVillage(second)
	game.SetOwner(w, second, "bot_0")
	second.Buildings[game.BBarracks] = 10
	second.Buildings[game.BFarm] = 20
	b.Roles[second.ID] = game.RoleDef
	second.Units = game.Units{}
	second.Units[game.USpear] = spears
	game.RecomputePoints(second)
	return w, first, second
}

// hitFrom sends an attack at v from a human village with `swords` swordsmen.
func hitFrom(t *testing.T, w *gametest.MemWorld, v *game.Village, swords int) *game.Command {
	t.Helper()
	human := game.PlayerVillages(w, "h1")[0]
	human.Units = game.Units{}
	human.Units[game.USword] = swords
	c, err := game.SendCommand(w, "h1", human.ID, v.X, v.Y, game.UnitCounts{game.USword: swords}, "attack", game.SendOpts{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func supportsFrom(w *gametest.MemWorld, v *game.Village) int {
	n := 0
	for _, c := range w.CommandsHome(v.ID) {
		if c.Type == "support" {
			n += c.Units.Total()
		}
	}
	return n
}

func TestSupportReinforcesOneVillageWithoutStripingTheOther(t *testing.T) {
	t.Run("a village under attack gets support from a free one, but not all of it", func(t *testing.T) {
		w, first, second := twoDefVillages(t, 20000)
		hitFrom(t, w, first, 200)
		game.PlanDefence(w, "bot_0")
		sent := supportsFrom(w, second)
		if sent == 0 {
			t.Fatal("a village under attack got no support from a free neighbour")
		}
		// The helper may be the next wave's target, so it keeps a share of its own garrison.
		if sent >= 20000 {
			t.Fatalf("the helper sent its whole army (%d)", sent)
		}
	})

	t.Run("a village under attack sends no support, so both are not stripped by one wave", func(t *testing.T) {
		w, first, second := twoDefVillages(t, 20000)
		a := hitFrom(t, w, first, 200)
		b := hitFrom(t, w, second, 100)
		b.ArrivesAt = a.ArrivesAt + 5*game.MsPerMinute
		game.PlanDefence(w, "bot_0")
		if sent := supportsFrom(w, second); sent != 0 {
			t.Fatalf("a village under attack sent %d of its garrison to help a neighbour", sent)
		}
		if total := game.UnitCounts(second.Units).Total(); total != 20000 {
			t.Fatalf("the attacked helper lost troops: %d of 20000", total)
		}
	})
}

// The floor, not a share: the raiders exist to raid.
func TestHomeGuardKeepsNoShareOfTheRaiders(t *testing.T) {
	_, v := guardedVillage(t, strongSkill, game.UnitCounts{game.ULightCav: 1000})
	// 1000 light cavalry is 4000 population and the garrison list holds none of them, so all of it may go — the
	// floor is population, not a quarter of the army.
	got, ok := game.WithinGuard(v, game.UnitCounts{game.ULightCav: 1000})
	if !ok {
		t.Fatal("nothing could raid")
	}
	if got[game.ULightCav] < 990 {
		t.Fatalf("only %d of 1000 light cavalry could raid, want nearly all", got[game.ULightCav])
	}
}
