package game

// The battle simulator: a battle the player asks for without sending anything. It runs the same ResolveBattle the
// real one does, so the numbers it shows cannot drift from a real fight — and it never touches the world.

// SimulateInput is what the player fills in. Only troops and the state that changes the outcome are required; the
// rest defaults to what a village of that wall would look like.
type SimInput struct {
	Attacker SimSide `json:"attacker"`
	Defender SimSide `json:"defender"`
	// Luck in [-CombatBalance.LuckRange, +CombatBalance.LuckRange]; 0 is the honest middle.
	Luck float64 `json:"luck"`
	// Morale as a multiplier; 1 against barbarians, as a real attack uses.
	Morale float64 `json:"morale"`
	// CatapultTarget names the building to aim at, when the attacker has catapults.
	CatapultTarget *BuildingID `json:"catapultTarget,omitempty"`
}

// SimSide is one army plus the parts of its village that matter. A side is the attacker or the defender; the
// defender's wall, hideout and resources are ignored when the side is the attacker.
type SimSide struct {
	Units    UnitCounts `json:"units"`
	Research UnitCounts `json:"research"`
	// Wall is the defender's wall level; the attacker's is ignored.
	Wall int `json:"wall"`
	// Buildings of the defender's village, for what catapults may aim at. Empty means "a wall and a town hall".
	Buildings Buildings `json:"buildings,omitempty"`
	Resources Resources `json:"resources"`
	// HideLevel is how much the defender's hideout saves from the haul.
	HideLevel int `json:"hideLevel"`
}

// SimResult is the battle plus the two numbers the player is really asking about: what each side loses.
type SimResult struct {
	BattleResult
	// AttackerCost and DefenderCost are the losses valued at each unit's own cost, for comparing two runs.
	AttackerCost float64 `json:"attackerCost"`
	DefenderCost float64 `json:"defenderCost"`
}

// SimDefaultMainLevel is the town hall a simulated defender gets when the player names no buildings. Level 1 is the
// floor of the rule and cannot be damaged, so the simulator would never show a siege breaking anything.
const SimDefaultMainLevel = 5

// MaxSimUnits is the largest army a simulation takes. Far beyond any real one, it keeps the loss distribution and
// the haul arithmetic well inside what an int and a float64 hold.
const MaxSimUnits = 10_000_000

// ClampLuck holds luck to the range a real battle draws from, so a typo cannot show a fight that cannot happen.
func ClampLuck(luck float64) float64 {
	if luck > CombatBalance.LuckRange {
		return CombatBalance.LuckRange
	}
	if luck < -CombatBalance.LuckRange {
		return -CombatBalance.LuckRange
	}
	return luck
}

// SimulateBattle runs one battle and values both sides' losses.
func SimulateBattle(in SimInput, lossMode BattleLossMode) SimResult {
	defender := in.Defender
	// A side with no buildings given is an ordinary grown village: a town hall at the level a siege can actually
	// damage, and the wall the player typed. A bare level-1 hall is the floor of the rule, so leaving it there made
	// every catapult run report nothing broken — a simulator that cannot show the damage is not answering the
	// question it is asked. The wall still comes from the field, not from this guess.
	if defender.Buildings == (Buildings{}) {
		defender.Buildings[BMain] = SimDefaultMainLevel
		defender.Buildings[BWall] = defender.Wall
	}
	res := ResolveBattle(BattleInput{
		Attacker: BattleSide{Units: in.Attacker.Units, Research: in.Attacker.Research},
		Defender: BattleDefender{
			BattleSide: BattleSide{Units: defender.Units, Research: defender.Research},
			Wall:       defender.Wall, Buildings: defender.Buildings, Resources: defender.Resources,
			HideLevel: defender.HideLevel,
		},
		Luck: ClampLuck(in.Luck), Morale: in.Morale, CatapultTarget: in.CatapultTarget,
		// Catapults pick their target at random in a real battle. A simulator that shuffles would show a different
		// building on every press, so it aims at the first one it could hit and leaves the choice to the player.
		Rng:      func() float64 { return 0 },
		LossMode: lossMode,
	})
	return SimResult{BattleResult: res, AttackerCost: costOf(res.AttackerLosses), DefenderCost: costOf(res.DefenderLosses)}
}

// costOf values an army's losses at what it costs to train one of each.
func costOf(u UnitCounts) float64 {
	total := 0.0
	for _, id := range UnitIDs {
		c := UnitDefs[id].Cost
		total += float64(u[id]) * (c.Wood + c.Clay + c.Iron)
	}
	return total
}
