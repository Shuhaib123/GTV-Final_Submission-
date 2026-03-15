package traceproc

import "testing"

func TestAuditSummaryIncludesSyncParsedCountersAndBlockedByReason(t *testing.T) {
	st := NewParseState(Options{})
	st.auditCounts[auditParsedRegionSend] = 1
	st.auditCounts[parsedRegionAuditKey("mutex_lock")] = 2
	st.auditCounts[parsedRegionAuditKey("wg_wait")] = 1
	st.blockedTotalMs[25] = 100
	st.blockedTotalByReason["chan_send"] = 100
	st.blockedAt[41] = 10
	st.blockedReasonByG[41] = "wg_wait"
	st.lastTimeMs = 15

	ev := st.auditSummaryEvent("test")
	payload, ok := ev.Value.(map[string]any)
	if !ok {
		t.Fatalf("audit summary payload has unexpected type %T", ev.Value)
	}

	if got := payload["parsed_region_mutex_lock"]; got != int64(2) {
		t.Fatalf("parsed_region_mutex_lock=%v, want 2", got)
	}
	if got := payload["parsed_region_wg_wait"]; got != int64(1) {
		t.Fatalf("parsed_region_wg_wait=%v, want 1", got)
	}

	reasons, ok := payload["blocked_total_ms_by_reason"].(map[string]float64)
	if !ok {
		t.Fatalf("blocked_total_ms_by_reason has unexpected type %T", payload["blocked_total_ms_by_reason"])
	}
	if got := reasons["chan_send"]; got != 100 {
		t.Fatalf("blocked_total_ms_by_reason[chan_send]=%v, want 100", got)
	}
	if got := reasons["wg_wait"]; got != 5 {
		t.Fatalf("blocked_total_ms_by_reason[wg_wait]=%v, want 5", got)
	}
}

func TestInferSyncWaitContextPrefersRegionCandidate(t *testing.T) {
	st := NewParseState(Options{})
	st.setSyncWaitCandidate(7, "mutex_lock", "mutex:mu", 1.25)

	kind, ch, chKey, confidence := st.inferSyncWaitContext(7, "sync.Mutex.Lock")
	if kind != "mutex_lock" {
		t.Fatalf("kind=%q, want mutex_lock", kind)
	}
	if ch != "mutex:mu" {
		t.Fatalf("channel=%q, want mutex:mu", ch)
	}
	if chKey == "" {
		t.Fatal("expected non-empty channel key for sync wait candidate")
	}
	if confidence != "high" {
		t.Fatalf("confidence=%q, want high", confidence)
	}
}
