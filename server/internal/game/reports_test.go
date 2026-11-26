package game_test

import (
	"testing"

	"librehold/internal/game"
	"librehold/internal/game/gametest"
)

// reportIDsOf lists the reports a player kept, oldest first.
func reportIDsOf(w *gametest.MemWorld, who game.PlayerID) []string {
	var ids []string
	for _, r := range w.ReportsOf(who) {
		ids = append(ids, r.ID)
	}
	return ids
}

// A night of auto-farming writes a report per raid, and the auto-farm's reports used to share one inbox with
// everything else: an attack that landed overnight was evicted by the raids before morning. The two budgets are
// separate now, so a flood of raids can never cost a report the player reads.
func TestAutoFarmReportsCannotEvictAManualReport(t *testing.T) {
	w, _ := gametest.NewTestWorld(101)

	w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now()})
	manual := reportIDsOf(w, "h1")[0]

	for range game.CombatBalance.MaxAutoFarmReports * 3 {
		w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now(), AutoFarm: true})
	}

	kept := w.ReportsOf("h1")
	autoFarm := 0
	found := false
	for _, r := range kept {
		if r.AutoFarm {
			autoFarm++
		} else if r.ID == manual {
			found = true
		}
	}
	if !found {
		t.Fatal("the manual report was evicted by the auto-farm's raids")
	}
	if autoFarm != game.CombatBalance.MaxAutoFarmReports {
		t.Fatalf("kept %d auto-farm reports, want %d", autoFarm, game.CombatBalance.MaxAutoFarmReports)
	}
}

// The manual budget still applies to manual reports: the newest are kept and the oldest go.
func TestManualReportsKeepTheirOwnBudget(t *testing.T) {
	w, _ := gametest.NewTestWorld(102)

	over := 5
	w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now()})
	oldest := reportIDsOf(w, "h1")[0]
	for range game.CombatBalance.MaxReports + over - 1 {
		w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now()})
	}
	kept := w.ReportsOf("h1")
	if len(kept) != game.CombatBalance.MaxReports {
		t.Fatalf("kept %d manual reports, want %d", len(kept), game.CombatBalance.MaxReports)
	}
	for _, r := range kept {
		if r.ID == oldest {
			t.Fatal("the oldest manual report survived; the budget was not enforced")
		}
	}
}

// One kind never borrows the other's room: an empty auto-farm pool does not let manual reports past their cap,
// and a full one does not shorten the manual budget.
func TestReportPoolsAreIndependent(t *testing.T) {
	w, _ := gametest.NewTestWorld(103)

	for range game.CombatBalance.MaxAutoFarmReports {
		w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now(), AutoFarm: true})
	}
	for range game.CombatBalance.MaxReports {
		w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now()})
	}
	if got, want := len(w.ReportsOf("h1")), game.TotalReportsKept(); got != want {
		t.Fatalf("kept %d reports, want %d", got, want)
	}
	// One more manual report evicts only a manual one.
	w.AddReport("h1", &game.Report{Type: "attack", Time: w.Now()})
	autoFarm := 0
	for _, r := range w.ReportsOf("h1") {
		if r.AutoFarm {
			autoFarm++
		}
	}
	if autoFarm != game.CombatBalance.MaxAutoFarmReports {
		t.Fatalf("the manual report took an auto-farm slot: %d left", autoFarm)
	}
}
