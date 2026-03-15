package e2e

import "testing"

func TestQuiescenceWarningDeadlock(t *testing.T) {
	t.Setenv("GTV_QUIESCENCE_MS", "10")
	result := runE2E(t, workloadCase{Name: "deadlock", File: "deadlock.go"})
	var warnCount int64
	for _, ev := range result.Events {
		if ev.Event != "warn_quiescence" {
			continue
		}
		if payload, ok := ev.Value.(map[string]any); ok {
			if v, ok := payload["blocked_count"]; ok {
				switch n := v.(type) {
				case int64:
					warnCount = n
				case int:
					warnCount = int64(n)
				case float64:
					warnCount = int64(n)
				}
			}
		}
	}
	if warnCount == 0 {
		t.Fatalf("expected warn_quiescence with blocked_count > 0")
	}
}
