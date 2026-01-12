package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/exp/trace"
	"jspt/internal/instrumenter"
	"jspt/internal/traceproc"
)

type summaryCounts struct {
	Nodes     int `json:"nodes"`
	Edges     int `json:"edges"`
	Paired    int `json:"paired"`
	Unmatched int `json:"unmatched"`
}

type workloadCase struct {
	Name string
	File string
}

func TestE2EExamples(t *testing.T) {
	cases := []workloadCase{
		{Name: "ping_pong", File: "ping_pong.go"},
		{Name: "broadcast", File: "broadcast.go"},
		{Name: "pipeline", File: "pipeline.go"},
		{Name: "fan_out_fan_in", File: "fan_out_fan_in.go"},
		{Name: "buffered_queue", File: "buffered_queue.go"},
		{Name: "select_default", File: "select_default.go"},
		{Name: "close_range", File: "close_range.go"},
	}

	updateGolden := os.Getenv("GTV_E2E_UPDATE_GOLDEN") == "1"
	expected := loadGoldenSummaries(t, updateGolden)
	observed := make(map[string]summaryCounts)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			result := runE2E(t, tc)
			if result.Summary.Paired == 0 {
				t.Fatalf("expected paired events for %s", tc.Name)
			}
			if updateGolden {
				observed[result.WorkloadName] = result.Summary
				return
			}
			want, ok := expected[result.WorkloadName]
			if !ok {
				t.Fatalf("missing golden summary for %s", result.WorkloadName)
			}
			if want != result.Summary {
				t.Fatalf("summary mismatch for %s: got %+v want %+v", result.WorkloadName, result.Summary, want)
			}
		})
	}
	if updateGolden {
		writeGoldenSummaries(t, observed)
	}
}

func TestLiveOrdering(t *testing.T) {
	result := runE2E(t, workloadCase{Name: "ping_pong", File: "ping_pong.go"})
	if len(result.Events) == 0 {
		t.Fatal("expected live events")
	}

	retroactive := false
	prev := result.Events[0]
	for i := 1; i < len(result.Events); i++ {
		cur := result.Events[i]
		if cur.Seq <= prev.Seq {
			t.Fatalf("non-monotonic seq at %d: %d then %d", i, prev.Seq, cur.Seq)
		}
		if cur.TimeNs < prev.TimeNs {
			retroactive = true
		}
		prev = cur
	}

	if !retroactive {
		prev = result.Events[0]
		for i := 1; i < len(result.Events); i++ {
			cur := result.Events[i]
			if cur.TimeNs < prev.TimeNs {
				t.Fatalf("unexpected retroactive event at %d: %d then %d", i, prev.TimeNs, cur.TimeNs)
			}
			prev = cur
		}
	} else {
		sorted := append([]traceproc.TimelineEvent(nil), result.Events...)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].TimeNs == sorted[j].TimeNs {
				return sorted[i].Seq < sorted[j].Seq
			}
			return sorted[i].TimeNs < sorted[j].TimeNs
		})
		for i := 1; i < len(sorted); i++ {
			prevEv := sorted[i-1]
			curEv := sorted[i]
			if prevEv.TimeNs > curEv.TimeNs || (prevEv.TimeNs == curEv.TimeNs && prevEv.Seq >= curEv.Seq) {
				t.Fatalf("(time_ns, seq) ordering violated at %d", i)
			}
		}
	}
}

func TestTimeoutStillEmitsTrace(t *testing.T) {
	tc := workloadCase{Name: "non_terminating", File: "non_terminating.go"}
	root := repoRoot(t)
	workloadTitle := camel("e2e_" + tc.Name)
	workloadName := strings.ToLower(workloadTitle)
	instrumenter.SetOptions(instrumenter.Options{
		Level:               "regions",
		GuardDynamicLabels:  true,
		AddGoroutineRegions: true,
		AddBlockRegions:     true,
	})

	srcPath := filepath.Join(root, "internal", "e2e", "testdata", "examples", tc.File)
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}

	instrumented, err := instrumenter.InstrumentProgram(src, workloadTitle)
	if err != nil {
		t.Fatalf("instrument example: %v", err)
	}

	tagName := "workload_" + workloadName
	genPath := filepath.Join(root, "internal", "workload", workloadName+"_e2e_gen.go")
	if err := writeInstrumented(genPath, tagName, instrumented); err != nil {
		t.Fatalf("write instrumented: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(genPath)
	})

	tracePath := filepath.Join(t.TempDir(), workloadName+".trace")
	if err := runWorkloadWithTimeout(root, tagName, workloadName, tracePath, "200ms"); err != nil {
		t.Fatalf("run workload: %v", err)
	}

	events := parseTrace(t, tracePath)
	if len(events) == 0 {
		t.Fatal("expected non-empty trace")
	}
	jsonPath := filepath.Join(t.TempDir(), "trace.json")
	if err := writeEventsJSON(jsonPath, events); err != nil {
		t.Fatalf("write trace.json: %v", err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read trace.json: %v", err)
	}
	var roundTrip []traceproc.TimelineEvent
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("parse trace.json: %v", err)
	}
	if len(roundTrip) == 0 {
		t.Fatal("expected non-empty parsed JSON")
	}
}

type e2eResult struct {
	WorkloadName string
	Events       []traceproc.TimelineEvent
	Summary      summaryCounts
}

func runE2E(t *testing.T, tc workloadCase) e2eResult {
	t.Helper()
	root := repoRoot(t)
	workloadTitle := camel("e2e_" + tc.Name)
	workloadName := strings.ToLower(workloadTitle)
	instrumenter.SetOptions(instrumenter.Options{
		Level:               "regions",
		GuardDynamicLabels:  true,
		AddGoroutineRegions: true,
		AddBlockRegions:     true,
	})

	srcPath := filepath.Join(root, "internal", "e2e", "testdata", "examples", tc.File)
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}

	instrumented, err := instrumenter.InstrumentProgram(src, workloadTitle)
	if err != nil {
		t.Fatalf("instrument example: %v", err)
	}

	tagName := "workload_" + workloadName
	genPath := filepath.Join(root, "internal", "workload", workloadName+"_e2e_gen.go")
	if err := writeInstrumented(genPath, tagName, instrumented); err != nil {
		t.Fatalf("write instrumented: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(genPath)
	})

	tracePath := filepath.Join(t.TempDir(), workloadName+".trace")
	if err := runWorkloadWithTimeout(root, tagName, workloadName, tracePath, "3s"); err != nil {
		t.Fatalf("run workload: %v", err)
	}

	events := parseTrace(t, tracePath)
	summary := summarize(events)

	return e2eResult{WorkloadName: workloadName, Events: events, Summary: summary}
}

func runWorkloadWithTimeout(root, tagName, workloadName, tracePath, timeout string) error {
	outFile, err := os.Create(tracePath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	cmd := exec.Command("go", "run", "-tags", tagName, "./cmd/gtv-runner", "-workload", workloadName, "-timeout", timeout)
	cmd.Dir = root
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GTV_INSTR_CONFIG=", "GTV_INSTR_LEVEL=")
	return cmd.Run()
}

func writeEventsJSON(path string, events []traceproc.TimelineEvent) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(events); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func parseTrace(t *testing.T, tracePath string) []traceproc.TimelineEvent {
	t.Helper()
	file, err := os.Open(tracePath)
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	defer file.Close()

	reader, err := trace.NewReader(file)
	if err != nil {
		t.Fatalf("trace reader: %v", err)
	}
	opts := traceproc.Options{EmitAtomic: true}
	state := traceproc.NewParseState(opts)
	var events []traceproc.TimelineEvent
	var seq int64
	emit := func(ev traceproc.TimelineEvent) error {
		seq++
		ev.Seq = seq
		events = append(events, ev)
		return nil
	}

	for {
		ev, err := reader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				return events
			}
			t.Fatalf("read event: %v", err)
		}
		if err := traceproc.ProcessEvent(&ev, state, emit); err != nil {
			t.Fatalf("process event: %v", err)
		}
	}
}

func summarize(events []traceproc.TimelineEvent) summaryCounts {
	goroutines := make(map[int64]struct{})
	channels := make(map[string]struct{})
	linkKeys := make(map[string]struct{})
	paired := 0
	unmatched := 0

	for _, ev := range events {
		if !isPaired(ev) {
			continue
		}
		if ev.G != 0 {
			goroutines[ev.G] = struct{}{}
		}
		if ch := channelIdentity(ev); ch != "" {
			channels[ch] = struct{}{}
		}
		if ev.Event == "chan_send" {
			if ev.PeerG != 0 || ev.MsgID != 0 || ev.PairID != "" {
				paired++
			}
			key := fmt.Sprintf("send:%d:%s", ev.G, channelIdentity(ev))
			if key != "send:0:" {
				linkKeys[key] = struct{}{}
			}
		}
		if ev.Event == "chan_recv" {
			if ev.PeerG == 0 && ev.MsgID == 0 && ev.PairID == "" {
				unmatched++
			}
			key := fmt.Sprintf("recv:%d:%s", ev.G, channelIdentity(ev))
			if key != "recv:0:" {
				linkKeys[key] = struct{}{}
			}
		}
	}

	return summaryCounts{
		Nodes:     len(goroutines) + len(channels),
		Edges:     len(linkKeys),
		Paired:    paired,
		Unmatched: unmatched,
	}
}

func channelIdentity(ev traceproc.TimelineEvent) string {
	ptr := strings.TrimSpace(ev.ChPtr)
	if strings.HasPrefix(ptr, "ptr=") {
		ptr = strings.TrimSpace(strings.TrimPrefix(ptr, "ptr="))
	}
	if ptr != "" {
		return ptr
	}
	if ev.ChannelKey != "" {
		return normalizeChannelKey(ev.ChannelKey)
	}
	if ev.Channel != "" {
		return normalizeChannelKey(ev.Channel)
	}
	return ""
}

func normalizeChannelKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if idx := strings.Index(key, "#g"); idx >= 0 && idx+2 < len(key) {
		tail := key[idx+2:]
		if allDigits(tail) {
			return key[:idx]
		}
	}
	return key
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func isPaired(ev traceproc.TimelineEvent) bool {
	if ev.Event != "chan_send" && ev.Event != "chan_recv" {
		return false
	}
	return strings.EqualFold(ev.Source, "paired")
}

func writeInstrumented(path, tagName string, code []byte) error {
	tag := fmt.Sprintf("//go:build %s\n// +build %s\n\n", tagName, tagName)
	return os.WriteFile(path, append([]byte(tag), code...), 0644)
}

func loadGoldenSummaries(t *testing.T, allowMissing bool) map[string]summaryCounts {
	t.Helper()
	path := filepath.Join(repoRoot(t), "internal", "e2e", "testdata", "golden_summaries.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if allowMissing && os.IsNotExist(err) {
			return make(map[string]summaryCounts)
		}
		t.Fatalf("read golden summaries: %v", err)
	}
	var out map[string]summaryCounts
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse golden summaries: %v", err)
	}
	return out
}

func writeGoldenSummaries(t *testing.T, data map[string]summaryCounts) {
	t.Helper()
	path := filepath.Join(repoRoot(t), "internal", "e2e", "testdata", "golden_summaries.json")
	ordered := make(map[string]summaryCounts, len(data))
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ordered[key] = data[key]
	}
	blob, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden summaries: %v", err)
	}
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0644); err != nil {
		t.Fatalf("write golden summaries: %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func camel(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}
