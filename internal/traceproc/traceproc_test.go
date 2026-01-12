package traceproc

import "testing"

func TestParseRegionOpVariants(t *testing.T) {
	cases := []struct {
		label string
		kind  string
		ch    string
	}{
		{label: "worker: send ch", kind: "send", ch: "ch"},
		{label: "worker: send on ch", kind: "send", ch: "ch"},
		{label: "worker: send to ch", kind: "send", ch: "ch"},
		{label: "worker: send 1 to ping", kind: "send", ch: "ping"},
		{label: "worker: recv ch", kind: "recv", ch: "ch"},
		{label: "worker: receive ch", kind: "recv", ch: "ch"},
		{label: "worker: recv from ch", kind: "recv", ch: "ch"},
		{label: "worker: receive from ch", kind: "recv", ch: "ch"},
	}
	for _, tc := range cases {
		kind, ch, ok := parseRegionOp(tc.label)
		if !ok {
			t.Fatalf("expected %q to parse", tc.label)
		}
		if kind != tc.kind || ch != tc.ch {
			t.Fatalf("parseRegionOp(%q) = (%q, %q), want (%q, %q)", tc.label, kind, ch, tc.kind, tc.ch)
		}
	}
}
