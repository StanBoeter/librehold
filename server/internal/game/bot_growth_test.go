package game_test

import (
	"testing"

	"librehold/internal/game"
	"librehold/internal/game/gametest"
)

// The three things that left a grown bot village with a starter's army.

// grownVillage: a village past the opening, with the mine levels and a farm of the given size.
func grownVillage(t *testing.T, skill float64, farm int) (*gametest.MemWorld, *game.Village) {
	t.Helper()
	w, v := oneBotWorld(t, 31, skill)
	b := &v.Buildings
	*b = game.Buildings{}
	b[game.BMain], b[game.BBarracks], b[game.BSmithy], b[game.BStable], b[game.BWorkshop] = 20, 20, 20, 20, 10
	b[game.BWood], b[game.BClay], b[game.BIron], b[game.BStorage], b[game.BFarm] = 25, 25, 25, 30, farm
	b[game.BStatue] = 1
	v.Units, v.Resources = game.Units{}, game.Resources{}
	game.RecomputePoints(v)
	return w, v
}

func TestBotArmyGrowsWithTheFarm(t *testing.T) {
	t.Run("a small village is asked for exactly the table's army", func(t *testing.T) {
		w, v := grownVillage(t, strongSkill, 10)
		// Below the growth point the target is what it always was: the tables are the floor, not a starting point
		// that a farm tweak can cut.
		if got, want := game.TroopTarget(v, game.ChooseRole(w, "bot_0", v))[game.ULightCav], game.TroopTargets[game.RoleMixed][game.ULightCav]; got != want {
			t.Fatalf("farm 10: %d light cavalry targeted, want the table's %d", got, want)
		}
	})

	t.Run("a grown farm asks for an army that fills it", func(t *testing.T) {
		w, v := grownVillage(t, strongSkill, 30)
		target := game.TroopTarget(v, game.ChooseRole(w, "bot_0", v))
		pop := 0
		for u, n := range target {
			pop += n * game.UnitDefs[u].Pop
		}
		capacity := game.VillageFarmCapacity(v)
		if share := float64(pop) / float64(capacity); share < 0.4 || share > 0.6 {
			t.Fatalf("a farm-%d village targets %.0f of %d population (%.0f%%), want about half",
				v.Buildings[game.BFarm], float64(pop), capacity, 100*share)
		}
		// The growth leans on the unit that brings loot home, or the economy that pays for it is not there.
		if target[game.ULightCav] <= game.TroopTargets[game.RoleMixed][game.ULightCav] {
			t.Fatalf("light cavalry did not grow: %d", target[game.ULightCav])
		}
	})

	t.Run("the target still stops, so a village is not a bottomless farm sink", func(t *testing.T) {
		w, v := grownVillage(t, strongSkill, 30)
		for range 20 {
			v.Resources = game.Resources{Wood: 1e9, Clay: 1e9, Iron: 1e9}
			game.PlanRecruitment(w, "bot_0", v)
			for _, b := range game.RecruitBuildings {
				q := v.RecruitQueues.Q(b)
				for _, j := range *q {
					v.Units[j.Unit] += j.Total - j.Done
				}
				*q = (*q)[:0]
			}
			v.Resources = game.Resources{Wood: 1e9, Clay: 1e9, Iron: 1e9}
		}
		target := game.TroopTarget(v, game.ChooseRole(w, "bot_0", v))
		for u, n := range target {
			if v.Units[u] > n*11/10 {
				t.Fatalf("%s over target: %d > %d", u, v.Units[u], n)
			}
		}
	})
}

func TestBotResearchGoesPastUnlocking(t *testing.T) {
	t.Run("a strong bot takes its raiders to the Smithy's maximum", func(t *testing.T) {
		w, v := grownVillage(t, strongSkill, 30)
		for u := range v.Research {
			v.Research[u] = 1
		}
		for range 12 {
			v.Resources = game.Resources{Wood: 1e9, Clay: 1e9, Iron: 1e9}
			game.PlanResearch(w, "bot_0", v)
			for _, j := range v.ResearchQueue {
				v.Research[j.Unit] = j.TargetLevel
			}
			v.ResearchQueue = nil
		}
		if v.Research[game.ULightCav] < game.ResearchBalance.MaxLevel {
			t.Fatalf("strong bot's light cavalry research stopped at %d of %d", v.Research[game.ULightCav], game.ResearchBalance.MaxLevel)
		}
	})

	t.Run("a weak bot stops at level 2, medium and strong go to the maximum", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			skill float64
			want  int
		}{{"medium", mediumSkill, 3}, {"weak", weakSkill, 2}} {
			w, v := grownVillage(t, tc.skill, 30)
			for u := range v.Research {
				v.Research[u] = 1
			}
			for range 12 {
				v.Resources = game.Resources{Wood: 1e9, Clay: 1e9, Iron: 1e9}
				game.PlanResearch(w, "bot_0", v)
				for _, j := range v.ResearchQueue {
					v.Research[j.Unit] = j.TargetLevel
				}
				v.ResearchQueue = nil
			}
			if v.Research[game.ULightCav] != tc.want {
				t.Fatalf("%s bot: light cavalry at %d, want %d", tc.name, v.Research[game.ULightCav], tc.want)
			}
		}
	})

	t.Run("unlocking comes before levelling, or the role's fighters are never trainable", func(t *testing.T) {
		w, v := grownVillage(t, strongSkill, 30)
		// Scout and light cavalry unlocked, the role's fighters still locked. Without the unlock floor the bot would
		// only ever level what it can already train — and a mixed village would raid for its whole life with light
		// cavalry while its spears and swords stayed impossible to build.
		for u := range v.Research {
			v.Research[u] = 0
		}
		v.Research[game.UScout], v.Research[game.ULightCav] = 1, 1
		v.Units = game.Units{}
		v.Resources = game.Resources{Wood: 1e9, Clay: 1e9, Iron: 1e9}
		game.PlanResearch(w, "bot_0", v)
		if len(v.ResearchQueue) != 1 {
			t.Fatalf("research queue %d", len(v.ResearchQueue))
		}
		job := v.ResearchQueue[0]
		// A unit at level 0 is an unlock; anything else is a second level bought while the village still could not
		// train the units its role is built on.
		if job.TargetLevel != 1 {
			t.Fatalf("researched %s to level %d while a fighter was still locked, want an unlock at 1",
				job.Unit, job.TargetLevel)
		}
	})
}

func TestBotKeepsBuildingItsArmyAfterTheRoleList(t *testing.T) {
	t.Run("a village past its role's list still raises its barracks", func(t *testing.T) {
		// A live-shaped village: everything RoleMixed asks for is already there, and the storehouse is full.
		w, v := grownVillage(t, strongSkill, 27)
		for id, lvl := range map[game.BuildingID]int{
			game.BMain: 16, game.BBarracks: 10, game.BStable: 10, game.BSmithy: 12, game.BMarket: 10,
			game.BWood: 25, game.BClay: 25, game.BIron: 25, game.BStorage: 28, game.BHide: 3, game.BWall: 5,
		} {
			v.Buildings[id] = lvl
		}
		v.Resources = game.Resources{Wood: 100000, Clay: 100000, Iron: 300000}
		game.RecomputePoints(v)

		next, ok := game.NextBuilding(w, "bot_0", v, game.Mulberry32(1))
		if !ok {
			t.Fatal("a village with a full storehouse and barracks 10 had nothing left to build")
		}
		if next != game.BBarracks && next != game.BStable && next != game.BSmithy {
			t.Fatalf("next build was %s, want the army's own buildings", next)
		}
	})

	t.Run("the economy is not raised past its target to fill the gap", func(t *testing.T) {
		w, v := grownVillage(t, strongSkill, 27)
		for id, lvl := range map[game.BuildingID]int{
			game.BMain: 16, game.BBarracks: 10, game.BStable: 10, game.BSmithy: 12, game.BMarket: 10,
			game.BWood: 25, game.BClay: 25, game.BIron: 25, game.BStorage: 28, game.BHide: 3, game.BWall: 5,
		} {
			v.Buildings[id] = lvl
		}
		v.Resources = game.Resources{Wood: 100000, Clay: 100000, Iron: 300000}
		game.RecomputePoints(v)
		for range 6 {
			next, ok := game.NextBuilding(w, "bot_0", v, game.Mulberry32(7))
			if !ok {
				break
			}
			if next == game.BWood || next == game.BClay || next == game.BIron || next == game.BStorage {
				t.Fatalf("raised the economy (%s) with the army still at barracks 10", next)
			}
			v.Buildings[next]++
		}
	})
}
