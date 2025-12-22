package traceproc

import "testing"

func TestHandleRegionBeginEmitsRecvAttempt(t *testing.T) {
	st := NewParseState(Options{})
	st.Roles[1] = "worker"
	var seen []TimelineEvent
	emit := func(ev TimelineEvent) error {
		seen = append(seen, ev)
		return nil
	}
	if err := st.handleRegionBegin("recv", "ch", 1, "worker", 1.0, emit); err != nil {
		t.Fatalf("handleRegionBegin failed: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("unexpected events: %#v", seen)
	}
	if seen[0].Event != "recv_attempt" {
		t.Fatalf("expected recv_attempt, got %q", seen[0].Event)
	}
	if seen[0].Channel != "ch" {
		t.Fatalf("expected channel ch, got %q", seen[0].Channel)
	}
	if op, ok := st.Active[1]; !ok || op.kind != "recv" {
		t.Fatalf("active op not set for gid 1: %#v", st.Active[1])
	}
}

func TestRegionBeginAndEndEmitRecvEvents(t *testing.T) {
	st := NewParseState(Options{})
	st.Roles[2] = "worker"
	var seen []TimelineEvent
	emit := func(ev TimelineEvent) error {
		seen = append(seen, ev)
		return nil
	}
	if err := st.handleRegionBegin("recv", "foo", 2, "worker", 1.5, emit); err != nil {
		t.Fatalf("handleRegionBegin failed: %v", err)
	}
	if err := st.handleRegionOp("recv", "foo", "", 2, "worker", 2.5, emit); err != nil {
		t.Fatalf("handleRegionOp failed: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 events, got %d", len(seen))
	}
	if seen[1].Event != "recv_complete" {
		t.Fatalf("expected recv_complete, got %q", seen[1].Event)
	}
	if seen[1].Channel != "foo" {
		t.Fatalf("expected channel foo on completion, got %q", seen[1].Channel)
	}
	if seen[1].ChannelKey == "" {
		t.Fatalf("expected channel key populated on recv_complete")
	}
}

func TestParseStatePointerChannelsRemainDistinct(t *testing.T) {
	st := NewParseState(Options{EmitAtomic: true})
	for gid := int64(1); gid <= 4; gid++ {
		st.Roles[gid] = "worker"
	}
	var seen []TimelineEvent
	emit := func(ev TimelineEvent) error {
		seen = append(seen, ev)
		return nil
	}
	pairChannel := func(sendG, recvG int64, ptr string, base float64) {
		st.lastChPtrByG[sendG] = ptr
		if err := st.handleRegionBegin("send", "reg", sendG, "worker", base, emit); err != nil {
			t.Fatalf("handleRegionBegin send failed: %v", err)
		}
		st.lastChPtrByG[recvG] = ptr
		if err := st.handleRegionBegin("recv", "reg", recvG, "worker", base+0.4, emit); err != nil {
			t.Fatalf("handleRegionBegin recv failed: %v", err)
		}
		if err := st.handleRegionOp("send", "reg", ptr, sendG, "worker", base+0.8, emit); err != nil {
			t.Fatalf("handleRegionOp send failed: %v", err)
		}
		if err := st.handleRegionOp("recv", "reg", ptr, recvG, "worker", base+1.2, emit); err != nil {
			t.Fatalf("handleRegionOp recv failed: %v", err)
		}
	}
	const (
		ptrA = "0xAAA"
		ptrB = "0xBBB"
	)
	pairChannel(1, 2, ptrA, 1.0)
	pairChannel(3, 4, ptrB, 3.0)

	sendPeers := make(map[string]int64)
	recvPeers := make(map[string]int64)
	sendCounts := make(map[string]int)
	recvCounts := make(map[string]int)
	for _, ev := range seen {
		if ev.Event == "chan_send" && ev.Source == "paired" {
			if ev.ChPtr == "" {
				t.Fatalf("chan_send missing ptr: %+v", ev)
			}
			sendCounts[ev.ChPtr]++
			sendPeers[ev.ChPtr] = ev.PeerG
		}
		if ev.Event == "chan_recv" && ev.Source == "paired" {
			if ev.ChPtr == "" {
				t.Fatalf("chan_recv missing ptr: %+v", ev)
			}
			recvCounts[ev.ChPtr]++
			recvPeers[ev.ChPtr] = ev.PeerG
		}
	}
	if len(sendCounts) != 2 {
		t.Fatalf("expected send events for two pointers, got %+v", sendCounts)
	}
	if len(recvCounts) != 2 {
		t.Fatalf("expected recv events for two pointers, got %+v", recvCounts)
	}
	if recvPeers[ptrA] != 1 {
		t.Fatalf("pointer %s paired with wrong sender: %d", ptrA, recvPeers[ptrA])
	}
	if recvPeers[ptrB] != 3 {
		t.Fatalf("pointer %s paired with wrong sender: %d", ptrB, recvPeers[ptrB])
	}
	if sendPeers[ptrA] != 2 {
		t.Fatalf("pointer %s send peer not receiver 2: %d", ptrA, sendPeers[ptrA])
	}
	if sendPeers[ptrB] != 4 {
		t.Fatalf("pointer %s send peer not receiver 4: %d", ptrB, sendPeers[ptrB])
	}
}
