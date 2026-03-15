package traceproc

import "testing"

func TestFilterTimelineEventForModeTeach(t *testing.T) {
	cases := []struct {
		name    string
		in      TimelineEvent
		keep    bool
		outName string
	}{
		{"drop ch_ptr", TimelineEvent{Event: "ch_ptr"}, false, "ch_ptr"},
		{"drop send commit to avoid duplicates", TimelineEvent{Event: "chan_send_commit"}, false, "chan_send_commit"},
		{"drop recv complete to avoid duplicates", TimelineEvent{Event: "recv_complete"}, false, "recv_complete"},
		{"map unblocked", TimelineEvent{Event: "unblocked"}, true, "unblock"},
		{"map go create", TimelineEvent{Event: "goroutine_created"}, true, "go_create"},
		{"map go start", TimelineEvent{Event: "goroutine_started"}, true, "go_start"},
		{"keep spawn", TimelineEvent{Event: "spawn"}, true, "spawn"},
		{"keep channels_created", TimelineEvent{Event: "channels_created"}, true, "channels_created"},
		{"keep worker_starting", TimelineEvent{Event: "worker_starting"}, true, "worker_starting"},
		{"keep mutex lock", TimelineEvent{Event: "mutex_lock"}, true, "mutex_lock"},
		{"keep cond wait", TimelineEvent{Event: "cond_wait"}, true, "cond_wait"},
		{"keep waitgroup wait", TimelineEvent{Event: "wg_wait"}, true, "wg_wait"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, keep, _ := FilterTimelineEventForMode(tc.in, "teach")
			if keep != tc.keep {
				t.Fatalf("keep=%v, want %v", keep, tc.keep)
			}
			if keep && out.Event != tc.outName {
				t.Fatalf("event=%q, want %q", out.Event, tc.outName)
			}
		})
	}
}

func TestFilterTimelineEventForModeDebug(t *testing.T) {
	in := TimelineEvent{Event: "chan_send_commit"}
	out, keep, _ := FilterTimelineEventForMode(in, "debug")
	if !keep {
		t.Fatal("debug mode should keep events")
	}
	if out.Event != in.Event {
		t.Fatalf("debug mode should not remap event: got %q want %q", out.Event, in.Event)
	}
}

func TestParseRegionOpSyncKinds(t *testing.T) {
	cases := []struct {
		label string
		kind  string
		ch    string
	}{
		{"mutex.lock mu", "mutex_lock", "mutex:mu"},
		{"mutex.unlock mu", "mutex_unlock", "mutex:mu"},
		{"rwmutex.rlock rw", "rwmutex_rlock", "rwmutex:rw"},
		{"rwmutex.runlock rw", "rwmutex_runlock", "rwmutex:rw"},
		{"wg.add wg", "wg_add", "wg:wg"},
		{"wg.done wg", "wg_done", "wg:wg"},
		{"wg.wait wg", "wg_wait", "wg:wg"},
		{"cond.wait cv", "cond_wait", "cond:cv"},
		{"cond.signal cv", "cond_signal", "cond:cv"},
		{"cond.broadcast cv", "cond_broadcast", "cond:cv"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			kind, ch, ok := parseRegionOp(tc.label)
			if !ok {
				t.Fatalf("parseRegionOp(%q) not recognized", tc.label)
			}
			if kind != tc.kind {
				t.Fatalf("kind=%q, want %q", kind, tc.kind)
			}
			if ch != tc.ch {
				t.Fatalf("channel=%q, want %q", ch, tc.ch)
			}
		})
	}
}
