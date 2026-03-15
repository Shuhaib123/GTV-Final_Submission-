package e2e

import (
	"fmt"
	"strings"
	"testing"

	"jspt/internal/traceproc"
)

func TestSelectRecvEmitsChanRecv(t *testing.T) {
	result := runE2E(t, workloadCase{Name: "select_recv", File: "select_recv.go"})
	count := 0
	for _, ev := range result.Events {
		if ev.Event == "chan_recv" {
			count++
		}
	}
	if count == 0 {
		counts := eventCounts(result.Events)
		preview := previewEvents(result.Events, 30)
		t.Fatalf("expected chan_recv from select; counts=%v events=%s", counts, preview)
	}
}

func TestUnparsedRegionOpBounded(t *testing.T) {
	cases := []workloadCase{
		{Name: "ping_pong", File: "ping_pong.go"},
		{Name: "broadcast", File: "broadcast.go"},
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			result := runE2E(t, tc)
			audit := auditSummaryFromEvents(result.Events)
			if audit == nil {
				t.Fatalf("missing audit_summary for %s", tc.Name)
			}
			unparsed := int64FromAudit(audit, "unparsed_region_op")
			if unparsed > 2 {
				samples := audit["unparsed_region_samples"]
				t.Fatalf("unparsed_region_op too high for %s: %d samples=%v", tc.Name, unparsed, samples)
			}
		})
	}
}

func auditSummaryFromEvents(events []traceproc.TimelineEvent) map[string]any {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Event != "audit_summary" {
			continue
		}
		if m, ok := ev.Value.(map[string]any); ok {
			return m
		}
	}
	return nil
}

func int64FromAudit(audit map[string]any, key string) int64 {
	if audit == nil {
		return 0
	}
	v, ok := audit[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func eventCounts(events []traceproc.TimelineEvent) map[string]int {
	out := make(map[string]int)
	for _, ev := range events {
		name := strings.ToLower(ev.Event)
		out[name]++
	}
	return out
}

func previewEvents(events []traceproc.TimelineEvent, max int) string {
	if len(events) == 0 {
		return ""
	}
	if max > len(events) {
		max = len(events)
	}
	parts := make([]string, 0, max)
	for i := 0; i < max; i++ {
		ev := events[i]
		parts = append(parts, fmt.Sprintf("%s:g=%d ch=%s key=%s src=%s", ev.Event, ev.G, ev.Channel, ev.ChannelKey, ev.Source))
	}
	return strings.Join(parts, "; ")
}
