package game

// JSONNear is jsonNear (combat_test.go) for the external test package.
var JSONNear = jsonNear

// RegisterTestHandler registers a handler for a test-only event kind and returns a func that removes it.
func RegisterTestHandler(kind string, h EventHandler) (restore func()) {
	prev, had := handlers[kind]
	registerHandler(kind, h)
	return func() {
		if had {
			handlers[kind] = prev
		} else {
			delete(handlers, kind)
		}
	}
}

// The bot growth rules, for the tests that pin what an army and a raid may cost a village.

// TroopTarget is the troop target a village of this role aims for: the table's own count, grown with the farm.
func TroopTarget(v *Village, role VillageRole) map[UnitID]int {
	return scaledTroopTargets(role, EffectiveBuildingLevels(v), v.Bonus)
}

// WithinGuard is a group held to what the village may spare, and Garrison is what it must keep.
func WithinGuard(v *Village, u UnitCounts) (UnitCounts, bool) { return withinGuard(v, u) }

func Garrison(v *Village) float64 { return garrisonPop(v) }

func Raidable(v *Village) float64 { return raidable(v) }

func OffScouts(v *Village) int { return offScouts(v) }
