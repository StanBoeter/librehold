package game

import (
	"math"
	"testing"
)

func simInput(attacker, defender UnitCounts, wall int, luck float64) SimInput {
	return SimInput{
		Attacker: SimSide{Units: attacker},
		Defender: SimSide{Units: defender, Wall: wall},
		Luck:     luck,
		Morale:   CombatBalance.MoraleVsBarbarian,
	}
}

// The simulator must show the same fight a raid would, so a run of it is the real ResolveBattle and not a second
// implementation of the rules that could quietly disagree with it.
func TestSimulateBattleMatchesARealBattle(t *testing.T) {
	attacker, defender := UnitCounts{USword: 300, URam: 10}, UnitCounts{USpear: 200, UArcher: 50}
	for _, wall := range []int{0, 5, 20} {
		for _, luck := range []float64{-0.25, 0, 0.25} {
			got := SimulateBattle(simInput(attacker, defender, wall, luck), "normal")
			want := ResolveBattle(battleInput(battleOpts{
				attacker: attacker, defender: defender, wall: wall, luck: luck, lossMode: "normal",
				rng: func() float64 { return 0 },
			}))
			if got.Outcome != want.Outcome || got.AttackerLosses != want.AttackerLosses ||
				got.DefenderLosses != want.DefenderLosses || got.WallAfter != want.WallAfter ||
				got.AttackPower != want.AttackPower || got.DefensePower != want.DefensePower {
				t.Fatalf("wall %d luck %v: simulator %+v, real battle %+v", wall, luck, got.BattleResult, want)
			}
		}
	}
}

func TestSimulateBattleClampsLuckToTheRealRange(t *testing.T) {
	att := UnitCounts{USword: 100}
	def := UnitCounts{USpear: 100}
	for _, luck := range []float64{99, -99, math.Inf(1)} {
		got := SimulateBattle(simInput(att, def, 0, luck), "normal")
		if want := ClampLuck(luck); got.Luck != want {
			t.Fatalf("luck %v: shown %v, want %v", luck, got.Luck, want)
		}
	}
	if ClampLuck(0.1) != 0.1 {
		t.Fatal("a luck inside the range was changed")
	}
}

// A region of no troops vs no troops is not a battle, and the simulator says so instead of dividing by zero.
func TestSimulateBattleOfEmptyArmies(t *testing.T) {
	got := SimulateBattle(simInput(UnitCounts{}, UnitCounts{}, 3, 0), "normal")
	if got.Outcome != "defenderWon" {
		t.Fatalf("an empty attack cannot win: %s", got.Outcome)
	}
	if got.AttackerCost != 0 || got.DefenderCost != 0 {
		t.Fatalf("no troops, no cost: %v %v", got.AttackerCost, got.DefenderCost)
	}
}

// The wall is the defender's, so raising it must never help the attacker.
func TestSimulateBattleTheWallOnlyHelpsTheDefender(t *testing.T) {
	att := UnitCounts{USword: 500}
	def := UnitCounts{USpear: 400}
	prev := 0.0
	for wall := 0; wall <= 20; wall++ {
		got := SimulateBattle(simInput(att, def, wall, 0), "normal")
		if wall > 0 && got.DefensePower < prev {
			t.Fatalf("wall %d defends less than wall %d: %v < %v", wall, wall-1, got.DefensePower, prev)
		}
		prev = got.DefensePower
	}
}

// A defender given no buildings is an ordinary grown village, not a bare level-1 hall: the hall must be high enough
// for a siege to actually break it, or every catapult run would report nothing and answer the wrong question.
func TestSimulateBattleDefaultsToAVillageASiegeCanBreak(t *testing.T) {
	in := simInput(UnitCounts{USword: 20000, UCatapult: 2000}, UnitCounts{USpear: 10}, 0, 0)
	th := BMain
	in.CatapultTarget = &th
	got := SimulateBattle(in, "normal")
	if got.Outcome != "attackerWon" {
		t.Fatalf("the siege did not go through: %s", got.Outcome)
	}
	if got.BuildingDamage == nil {
		t.Fatal("a catapult army broke nothing in the default village")
	}
	if got.BuildingDamage.Building != BMain {
		t.Fatalf("aimed at %s, want the town hall it was told to hit", got.BuildingDamage.Building)
	}
	if got.BuildingDamage.To >= got.BuildingDamage.From {
		t.Fatalf("the town hall did not fall: %+v", *got.BuildingDamage)
	}
}

// The wall the player typed is the wall that is fought over, whether or not buildings were named.
func TestSimulateBattleDefaultsTheWallFromTheField(t *testing.T) {
	in := simInput(UnitCounts{USword: 20000, URam: 2000}, UnitCounts{USpear: 10}, 5, 0)
	got := SimulateBattle(in, "normal")
	if got.WallBefore != 5 {
		t.Fatalf("fought over a wall of %d, want the 5 that was asked for", got.WallBefore)
	}
	if got.WallAfter >= got.WallBefore {
		t.Fatalf("2000 rams left the wall standing: %d -> %d", got.WallBefore, got.WallAfter)
	}
}

// Naming the buildings explicitly is still honoured: what the player typed wins over the default.
func TestSimulateBattleHonoursTheBuildingsItIsGiven(t *testing.T) {
	in := simInput(UnitCounts{USword: 20000, UCatapult: 2000}, UnitCounts{USpear: 10}, 0, 0)
	in.Defender.Buildings = Buildings{}
	in.Defender.Buildings[BMain] = 1 // the floor: nothing to break
	th := BMain
	in.CatapultTarget = &th
	if got := SimulateBattle(in, "normal"); got.BuildingDamage != nil {
		t.Fatalf("a level-1 hall was broken: %+v", *got.BuildingDamage)
	}
}

// Catapults must not shuffle between runs: the player compares two armies and expects the same target each time.
func TestSimulateBattleIsRepeatable(t *testing.T) {
	in := simInput(UnitCounts{USword: 50, UCatapult: 40}, UnitCounts{USpear: 5}, 0, 0)
	first := SimulateBattle(in, "normal")
	for i := 0; i < 5; i++ {
		want := SimulateBattle(in, "normal")
		if first.BuildingDamage == nil || want.BuildingDamage == nil {
			continue
		}
		if *first.BuildingDamage != *want.BuildingDamage {
			t.Fatalf("run %d broke a different building: %+v vs %+v", i, *first.BuildingDamage, *want.BuildingDamage)
		}
	}
}

// Losses are valued at what it costs to train them, which is what makes two runs comparable.
func TestSimulateBattleValuesTheLosses(t *testing.T) {
	noCost := UnitCounts{}
	if costOf(noCost) != 0 {
		t.Fatal("an empty army costs nothing")
	}
	one := UnitCounts{USword: 1}
	c := UnitDefs[USword].Cost
	if want := c.Wood + c.Clay + c.Iron; costOf(one) != want {
		t.Fatalf("one swordsman valued at %v, want %v", costOf(one), want)
	}
	if costOf(UnitCounts{USword: 2}) != 2*costOf(one) {
		t.Fatal("cost does not scale with the count")
	}
}

// A run that leaves the attacker empty must not be reported as a win, however the defender's wall is set.
func TestSimulateBattleOverwhelmingDefence(t *testing.T) {
	got := SimulateBattle(simInput(UnitCounts{USpear: 1}, UnitCounts{USword: 5000}, 20, -0.25), "normal")
	if got.Outcome != "defenderWon" {
		t.Fatalf("one spearman beat a full defence: %s", got.Outcome)
	}
	if got.AttackerSurvivors.Total() != 0 {
		t.Fatalf("survivors from a wiped attack: %v", got.AttackerSurvivors)
	}
}
