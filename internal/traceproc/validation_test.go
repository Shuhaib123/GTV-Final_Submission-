package traceproc

import (
	"fmt"
	"testing"
)

func verifyChPtrWindow(events []TimelineEvent, window int64) error {
	lastPtrSeq := make(map[int64]int64)
	for _, ev := range events {
		if ev.Event == "ch_ptr" {
			lastPtrSeq[ev.G] = ev.Seq
			continue
		}
		if (ev.Event == "chan_send_attempt" || ev.Event == "chan_recv_attempt") && ev.G != 0 {
			seq, ok := lastPtrSeq[ev.G]
			if !ok || seq == 0 {
				return fmt.Errorf("event %s seq=%d goroutine=%d missing preceding ch_ptr", ev.Event, ev.Seq, ev.G)
			}
			if window >= 0 && ev.Seq-seq > window {
				return fmt.Errorf("event %s seq=%d goroutine=%d ch_ptr stale (gap=%d)", ev.Event, ev.Seq, ev.G, ev.Seq-seq)
			}
			if ev.MissingPtr {
				return fmt.Errorf("event %s seq=%d goroutine=%d flagged missing_ptr despite recent ch_ptr", ev.Event, ev.Seq, ev.G)
			}
		}
	}
	return nil
}

func TestChPtrWindowPass(t *testing.T) {
	events := []TimelineEvent{
		{Event: "ch_ptr", G: 1, Seq: 1, ChPtr: "0x1"},
		{Event: "chan_send_attempt", G: 1, Seq: 2, ChPtr: "0x1"},
		{Event: "ch_ptr", G: 2, Seq: 3, ChPtr: "0x2"},
		{Event: "chan_recv_attempt", G: 2, Seq: 4, ChPtr: "0x2"},
		{Event: "ch_ptr", G: 1, Seq: 5, ChPtr: "0x1"},
		{Event: "chan_send_attempt", G: 1, Seq: 6, ChPtr: "0x1"},
	}
	if err := verifyChPtrWindow(events, 3); err != nil {
		t.Fatalf("expected window pass: %v", err)
	}
}

func TestChPtrWindowGap(t *testing.T) {
	events := []TimelineEvent{
		{Event: "ch_ptr", G: 1, Seq: 1, ChPtr: "0x1"},
		{Event: "chan_send_attempt", G: 1, Seq: 5, ChPtr: "0x1"},
	}
	if err := verifyChPtrWindow(events, 3); err == nil {
		t.Fatalf("expected window error for gap")
	}
}

func TestChPtrMissingPtrFlag(t *testing.T) {
	events := []TimelineEvent{
		{Event: "ch_ptr", G: 1, Seq: 1, ChPtr: "0x1"},
		{Event: "chan_send_attempt", G: 1, Seq: 2, ChPtr: "", MissingPtr: true},
	}
	if err := verifyChPtrWindow(events, 3); err == nil {
		t.Fatalf("expected missing ptr flag error")
	}
}
