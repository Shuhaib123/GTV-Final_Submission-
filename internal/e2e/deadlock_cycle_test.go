package e2e

import (
	"strings"
	"testing"
)

func TestDeadlockCycleWarning(t *testing.T) {
	t.Setenv("GTV_DEADLOCK_WINDOW_MS", "10000")
	result := runE2E(t, workloadCase{Name: "deadlock_cycle", File: "deadlock_cycle.go"})
	found := false
	for _, ev := range result.Events {
		if strings.ToLower(ev.Event) == "warn_deadlock_cycle" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected warn_deadlock_cycle; counts=%v", eventCounts(result.Events))
	}
	audit := auditSummaryFromEvents(result.Events)
	if audit == nil {
		t.Fatalf("missing audit_summary for deadlock_cycle")
	}
	if _, ok := audit["deadlock_cycle"]; !ok {
		t.Fatalf("expected audit_summary.deadlock_cycle, got keys=%v", keysOfAudit(audit))
	}
}

func keysOfAudit(audit map[string]any) []string {
	if audit == nil {
		return nil
	}
	keys := make([]string, 0, len(audit))
	for k := range audit {
		keys = append(keys, k)
	}
	return keys
}
