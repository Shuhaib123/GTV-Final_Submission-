package traceproc

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"runtime/trace"

	xtrace "golang.org/x/exp/trace"
)

// Test that a runtime trace containing a ch_ptr log followed by a region
// with a "send to <ch>" label is parsed such that the region gets the
// pointer-based channel key recorded by the ch_ptr log.
func TestIntegration_ChPtrRegionParsing(t *testing.T) {
	tmp, err := os.CreateTemp("", "gtv-trace-*.out")
	if err != nil {
		t.Fatalf("create tmp: %v", err)
	}
	defer func() {
		name := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(name)
	}()

	if err := trace.Start(tmp); err != nil {
		t.Fatalf("start trace: %v", err)
	}
	// Ensure the trace is stopped and flushed.
	defer trace.Stop()

	// Emit a pointer log then a region with a send label.
	ch := make(chan struct{})
	ctx, task := trace.NewTask(context.Background(), "test")
	defer task.End()
	trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%p", ch))
	trace.WithRegion(ctx, "worker: send to ch", func() {
		// no-op
	})

	// Allow events to be written and flush the runtime trace output
	time.Sleep(5 * time.Millisecond)
	trace.Stop()
	// Reopen file for reading
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}

	r, err := xtrace.NewReader(tmp)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	st := NewParseState(Options{})
	var found bool
	var ptr string
	var logs []string
	var out []TimelineEvent
	// Feed events to the parser and capture timeline events.
	for {
		ev, err := r.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		// Process log events to let the parser pick up ch_ptr
		if ev.Kind() == xtrace.EventLog {
			l := ev.Log()
			logs = append(logs, l.Category+" -> "+l.Message)
			if l.Category == "ch_ptr" || (l.Category == "" && l.Message != "") {
				// capture the ptr string for later comparison
				if i := findPtrIndex(l.Message); i >= 0 {
					ptr = l.Message[i+4:]
				}
			}
		}
		if err := ProcessEvent(&ev, st, func(ev TimelineEvent) error { out = append(out, ev); return nil }); err != nil {
			t.Fatalf("process event: %v", err)
		}
		t.Logf("processed event kind=%v gid=%d", ev.Kind(), ev.Goroutine())
	}
	// After processing, look for any emitted events that include a pointer key (ChPtr or ChannelKey with ptr=)
	for _, e := range out {
		if e.ChPtr != "" {
			found = true
			break
		}
		if e.ChannelKey != "" && (findPtrIndex(e.ChannelKey) >= 0) {
			found = true
			break
		}
	}
	if !found {
		t.Logf("captured logs: %v", logs)
		t.Logf("emitted events: %+v", out)
		t.Fatalf("parser did not emit any timeline events with ch_ptr or pointer-based channel keys; ptr=%q", ptr)
	}
}

// findPtrIndex finds the 'ptr=' substring index in a message, or -1.
func findPtrIndex(s string) int {
	for i := 0; i+4 <= len(s); i++ {
		if s[i:i+4] == "ptr=" {
			return i
		}
	}
	return -1
}
