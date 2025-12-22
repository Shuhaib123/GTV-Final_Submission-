package traceproc

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	xtrace "golang.org/x/exp/trace"
)

// findRepoRoot walks up from cwd to find the directory containing go.mod
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// TestCI_BdcstBuildRun builds a small runner that imports the workload package and
// runs RunbdcstProgram under a runtime/trace file; the test parses the produced
// trace and asserts that pointer-based channel keys or ch_ptr entries exist.
var (
	ciTraceOnce sync.Once
	ciTraceData struct {
		events []TimelineEvent
		state  *ParseState
		err    error
	}
)

func requireCITrace(t *testing.T) []TimelineEvent {
	if testing.Short() {
		t.Skip("skip CI build/run test in -short")
	}
	ciTraceOnce.Do(func() {
		ciTraceData.events, ciTraceData.state, ciTraceData.err = captureCITraceData()
	})
	if ciTraceData.err != nil {
		t.Fatalf("collect broadcast trace: %v", ciTraceData.err)
	}
	return ciTraceData.events
}

func captureCITraceData() ([]TimelineEvent, *ParseState, error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, nil, fmt.Errorf("find repo root: %w", err)
	}

	binPath := filepath.Join(repoRoot, "gtv_ci_bin")
	defer func() { _ = os.Remove(binPath) }()

	runDir := filepath.Join(repoRoot, "tmp_gtv_runner")
	_ = os.RemoveAll(runDir)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("mkdir runDir: %w", err)
	}
	mainSrc := `package main
import (
	"context"
	"os"
	"time"
	"runtime/trace"
	"log"
	"jspt/internal/workload"
)
func main(){
	tf := os.Getenv("GTV_TRACE_FILE")
	var f *os.File
	var err error
	if tf != "" {
		f, err = os.Create(tf)
		if err != nil {
			log.Fatalf("create trace file: %v", err)
		}
		if err := trace.Start(f); err != nil {
			log.Fatalf("trace start: %v", err)
		}
		defer func(){ trace.Stop(); f.Close() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	workload.RunbdcstProgram(ctx)
}
`
	srcPath := filepath.Join(runDir, "main.go")
	if err := os.WriteFile(srcPath, []byte(mainSrc), 0644); err != nil {
		return nil, nil, fmt.Errorf("write main.go: %w", err)
	}

	tf, err := os.CreateTemp("", "gtv-ci-trace-*.out")
	if err != nil {
		return nil, nil, fmt.Errorf("create tmp trace: %w", err)
	}
	fName := tf.Name()
	_ = tf.Close()
	defer os.Remove(fName)

	cmdBuild := exec.Command("go", "build", "-tags=workload_bdcst", "-o", binPath, "./tmp_gtv_runner")
	cmdBuild.Dir = repoRoot
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("build runner: %w; out=%s", err, string(out))
	}

	runCtx, runCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer runCancel()
	cmd := exec.CommandContext(runCtx, binPath)
	cmd.Env = append(os.Environ(), "GTV_TRACE_FILE="+fName)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("run runner returned error: %v; out=%s\n", err, string(out))
	}

	fr, err := os.Open(fName)
	if err != nil {
		return nil, nil, fmt.Errorf("open trace file: %w", err)
	}
	defer fr.Close()
	reader, err := xtrace.NewReader(fr)
	if err != nil {
		return nil, nil, fmt.Errorf("new trace reader: %w", err)
	}
	st := NewParseState(Options{})
	var events []TimelineEvent
	emit := func(ev TimelineEvent) error { events = append(events, ev); return nil }
	for {
		ev, err := reader.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read event: %w", err)
		}
		if err := ProcessEvent(&ev, st, emit); err != nil {
			return nil, nil, fmt.Errorf("process event: %w", err)
		}
	}
	return events, st, nil
}

func TestCI_BdcstBuildRun(t *testing.T) {
	events := requireCITrace(t)
	foundPtr := false
	for _, ev := range events {
		if strings.Contains(strings.ToLower(ev.ChannelKey), "0x") {
			foundPtr = true
			break
		}
		if ev.ChPtr != "" {
			foundPtr = true
			break
		}
	}
	if !foundPtr {
		t.Fatalf("no pointer-based channel_key or ch_ptr found")
	}
}

func TestCI_BdcstBroadcastTopology(t *testing.T) {
	events := requireCITrace(t)
	joinSends := filterEvents(events, "chan_send_commit", func(ev *TimelineEvent) bool {
		return strings.Contains(strings.ToLower(ev.Channel), "join")
	})
	if len(joinSends) != 10 {
		t.Fatalf("expected 10 join sends, got %d", len(joinSends))
	}
	joinRecvs := filterEvents(events, "chan_recv_commit", func(ev *TimelineEvent) bool {
		return strings.Contains(strings.ToLower(ev.Channel), "join")
	})
	if len(joinRecvs) != 10 {
		t.Fatalf("expected 10 join receives, got %d", len(joinRecvs))
	}
	for _, ev := range joinRecvs {
		if strings.Contains(strings.ToLower(ev.Role), "worker") {
			t.Fatalf("unexpected worker recv on join: g=%d role=%s", ev.G, ev.Role)
		}
	}
	regSend := filterEvents(events, "chan_send_commit", func(ev *TimelineEvent) bool {
		return strings.Contains(strings.ToLower(ev.Channel), "reg")
	})
	if len(regSend) != 10 {
		t.Fatalf("expected 10 reg sends, got %d", len(regSend))
	}
	regRecv := filterEvents(events, "chan_recv_commit", func(ev *TimelineEvent) bool {
		return strings.Contains(strings.ToLower(ev.Channel), "reg")
	})
	if len(regRecv) != 10 {
		t.Fatalf("expected 10 reg receives, got %d", len(regRecv))
	}
	regPtrs := uniquePointers(regSend, regRecv)
	if len(regPtrs) != 10 {
		t.Fatalf("expected 10 distinct reg pointers, got %d", len(regPtrs))
	}
	clientOutSends := filterEvents(events, "chan_send_commit", func(ev *TimelineEvent) bool {
		return strings.Contains(strings.ToLower(ev.Channel), "clientout")
	})
	if len(clientOutSends) != 10 {
		t.Fatalf("expected 10 clientout sends, got %d", len(clientOutSends))
	}
	serverInRecvs := filterEvents(events, "chan_recv_commit", func(ev *TimelineEvent) bool {
		return strings.Contains(strings.ToLower(ev.Channel), "serverin")
	})
	if len(serverInRecvs) != 10 {
		t.Fatalf("expected 10 serverin receives, got %d", len(serverInRecvs))
	}
}

func TestCI_ChannelPointerInvariants(t *testing.T) {
	events := requireCITrace(t)
	ptrLabel := make(map[string]string)
	for _, ev := range events {
		if ev.ChPtr != "" {
			if ev.MissingPtr {
				t.Fatalf("event %s marked missing_ptr despite having ch_ptr", ev.Event)
			}
			if prev, ok := ptrLabel[ev.ChPtr]; ok && prev != ev.ChannelKey {
				t.Fatalf("pointer %s has conflicting channel keys %q vs %q", ev.ChPtr, prev, ev.ChannelKey)
			}
			ptrLabel[ev.ChPtr] = ev.ChannelKey
		} else {
			if !ev.MissingPtr {
				t.Fatalf("event %s missing ptr but not flagged", ev.Event)
			}
		}
	}
}

func filterEvents(events []TimelineEvent, kind string, fn func(*TimelineEvent) bool) []TimelineEvent {
	var out []TimelineEvent
	for _, ev := range events {
		if ev.Event != kind {
			continue
		}
		if fn(&ev) {
			out = append(out, ev)
		}
	}
	return out
}

func uniquePointers(groups ...[]TimelineEvent) map[string]int {
	seen := make(map[string]int)
	for _, group := range groups {
		for _, ev := range group {
			if ev.ChPtr == "" {
				continue
			}
			seen[ev.ChPtr]++
		}
	}
	return seen
}
