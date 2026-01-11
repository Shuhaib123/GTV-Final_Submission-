package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	xtrace "golang.org/x/exp/trace"
	"jspt/internal/instrumenter"
	"jspt/internal/traceproc"
)

// TimelineEvent type is unified via internal/traceproc.

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // dev only; tighten for prod
}

const (
	runJSONTimeout   = 20 * time.Second
	maxTraceOutBytes = 1 << 20 // 1 MiB guardrail
)

// Global options configured via flags (with env fallbacks)
var (
	listenAddr  string
	optSynth    bool
	optDrop     bool
	optWorkload string
	optBCMode   string
	optLiveLog  bool
)

func envBool(name string) bool {
	v := os.Getenv(name)
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func main() {
	// Flags with env fallbacks
	addrDefault := os.Getenv("GTV_ADDR")
	if addrDefault == "" {
		addrDefault = ":8080"
	}
	synthDefault := envBool("GTV_SYNTH_SEND")
	dropDefault := envBool("GTV_DROP_BLOCK_NO_CH")
	addrFlag := flag.String("addr", addrDefault, "HTTP listen address, e.g. :8080")
	synthFlag := flag.Bool("synth", synthDefault, "synthesize *_send before unmatched *_recv")
	dropFlag := flag.Bool("drop-block-no-ch", dropDefault, "drop blocked events without channel label")
	wlDefault := os.Getenv("GTV_WORKLOAD")
	if wlDefault == "" {
		wlDefault = "pingpong"
	}
	wlFlag := flag.String("workload", wlDefault, "workload to run: pingpong|broadcast|skipgraph_full|mergesort")
	bcModeDefault := os.Getenv("GTV_BC_MODE")
	if bcModeDefault == "" {
		bcModeDefault = "blocking"
	}
	liveLogDefault := envBool("GTV_LIVE_LOG")
	bcModeFlag := flag.String("bc-mode", bcModeDefault, "broadcast mode: buffered|blocking")
	liveLogFlag := flag.Bool("live-log", liveLogDefault, "write trace.live.<run>.json while running")
	flag.Parse()
	listenAddr = *addrFlag
	optSynth = *synthFlag
	optDrop = *dropFlag
	optWorkload = strings.ToLower(*wlFlag)
	optBCMode = strings.ToLower(*bcModeFlag)
	optLiveLog = *liveLogFlag

	// Serve static assets from ./web
	fs := http.FileServer(http.Dir("web"))
	http.Handle("/", fs)

	// WebSocket endpoint that streams timeline events while the demo runs.
	http.HandleFunc("/trace", traceHandler)

	// Instrumentation endpoint: POST {code, workload_name}
	http.HandleFunc("/instrument", instrumentHandler)

	// Run endpoint: spawns an isolated runner and returns a JSON array timeline
	http.HandleFunc("/run", runOnceHTTP)

	// Friendly log and launch browser if possible
	host := strings.TrimPrefix(listenAddr, ":")
	url := fmt.Sprintf("http://localhost:%s/", host)
	log.Printf("GTV live: open %s\n", url)
	go func() {
		if err := openBrowser(url); err != nil {
			log.Printf("failed to launch browser: %v", err)
		}
	}()
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

type instrumentReq struct {
	Code         string `json:"code"`
	WorkloadName string `json:"workload_name"`
	GuardLabels  *bool  `json:"guard_labels,omitempty"`
	GorRegions   *bool  `json:"goroutine_regions,omitempty"`
	Level        string `json:"level,omitempty"`
	BlockRegions *bool  `json:"block_regions,omitempty"`
	IORegions    *bool  `json:"io_regions,omitempty"`
	IOJSON       *bool  `json:"io_json,omitempty"`
	IODB         *bool  `json:"io_db,omitempty"`
	IOHTTP       *bool  `json:"io_http,omitempty"`
	IOOS         *bool  `json:"io_os,omitempty"`
	ValueLogs    *bool  `json:"value_logs,omitempty"`
}

func instrumentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req instrumentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(req.WorkloadName)
	if name == "" || req.Code == "" {
		writeJSONError(w, http.StatusBadRequest, "missing code or workload_name")
		return
	}
	// Per-request instrumenter options (fallback to defaults when nil)
	// start from current defaults by calling SetOptions with same values is cumbersome; instead rely on instrumenter reading env and its defaults, but prefer explicit request values when provided.
	if req.GuardLabels != nil || req.GorRegions != nil || req.Level != "" || req.BlockRegions != nil || req.IORegions != nil || req.IOJSON != nil || req.IODB != nil || req.IOHTTP != nil || req.IOOS != nil {
		// Build an options struct overriding only provided fields; keep default Level when empty
		def := instrumenter.Options{}
		// ask instrumenter to keep its defaults; we only set fields with provided values
		if req.GuardLabels != nil {
			def.GuardDynamicLabels = *req.GuardLabels
		} else {
			def.GuardDynamicLabels = true
		}
		if req.GorRegions != nil {
			def.AddGoroutineRegions = *req.GorRegions
		}
		if req.BlockRegions != nil {
			def.AddBlockRegions = *req.BlockRegions
		}
		if req.IORegions != nil {
			def.AddIORegions = *req.IORegions
		}
		if req.IOJSON != nil {
			def.AddIOJSONRegions = *req.IOJSON
		}
		if req.IODB != nil {
			def.AddIODBRegions = *req.IODB
		}
		if req.IOHTTP != nil {
			def.AddIOHTTPRegions = *req.IOHTTP
		}
		if req.IOOS != nil {
			def.AddIOOSRegions = *req.IOOS
		}
		if req.ValueLogs != nil {
			def.AddValueLogs = *req.ValueLogs
		}
		if req.Level != "" {
			def.Level = strings.ToLower(req.Level)
		} else {
			def.Level = "regions"
		}
		// Block regions added below if field exists (needs new option)
		instrumenter.SetOptions(def)
		// If we later add BlockRegions to Options, set it here too via SetOptions
	}
	out, err := instrumenter.InstrumentProgram([]byte(req.Code), name)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "instrument error: "+err.Error())
		return
	}
	// Write generated file under internal/workload
	low := strings.ToLower(name)
	path := "internal/workload/" + low + "_gen.go"
	// Ensure no other files with the same build tag remain to avoid redeclarations
	if err := cleanupOldWorkloadFiles("internal/workload", low); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "cleanup error: "+err.Error())
		return
	}
	// Prefix with a build tag so multiple generated workloads can coexist.
	tag := []byte(fmt.Sprintf("//go:build workload_%s\n// +build workload_%s\n\n", low, low))
	if err := os.WriteFile(path, append(tag, out...), 0644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "write error: "+err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "workload": low, "file": path})
}

// runOnceHTTP executes a workload in an external process and returns the
// timeline as a JSON array. Query params:
//
//	name   — workload name (lowercase)
//	synth  — 1/true to enable SynthOnRecv
//	drop_block_no_ch — 1/true to drop unlabeled blocked events
func runOnceHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing name")
		return
	}
	synthQ := r.URL.Query().Get("synth")
	dropQ := r.URL.Query().Get("drop_block_no_ch")
	bcModeQ := r.URL.Query().Get("bc_mode")
	timeoutQ := strings.TrimSpace(r.URL.Query().Get("timeout"))
	if timeoutQ == "" {
		timeoutQ = "4s"
	}
	ctx, cancel := context.WithTimeout(r.Context(), runJSONTimeout)
	defer cancel()
	envBool := func(v string) bool {
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "1" || v == "true" || v == "yes"
	}
	optSynthHTTP := envBool(synthQ)
	optDropHTTP := envBool(dropQ)

	// Best-effort cleanup first; then ensure no duplicates for this tag
	_ = cleanupOldWorkloadFiles("internal/workload", name)
	// Before running, ensure we don't have duplicate files for this workload tag.
	if err := checkDuplicateWorkloads("internal/workload", "workload_"+name); err != nil {
		writeJSONError(w, http.StatusBadRequest, "workload configuration error: "+err.Error())
		return
	}

	// Spawn external runner to ensure newly generated workloads are included.
	// Pass a bounded timeout so runs complete and JSON returns
	// Pass a build tag so only the selected generated workload (if any) is included.
	tagName := "workload_" + name
	cmd := exec.CommandContext(ctx, "go", "run", "-tags", tagName, "./cmd/gtv-runner", "-workload", name, "-timeout", timeoutQ)
	// Default bc_mode=blocking unless explicitly provided
	env := os.Environ()
	if bcModeQ != "" {
		env = append(env, "GTV_BC_MODE="+strings.ToLower(bcModeQ))
	} else {
		env = append(env, "GTV_BC_MODE=blocking")
	}
	cmd.Env = env
	// Capture stderr so we can surface build/runtime errors back to the client
	var errBuf strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "runner stdout: "+err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "runner start: "+err.Error())
		return
	}
	// Tee raw trace bytes to trace.out while parsing.
	outFile, err := os.Create("trace.out")
	if err != nil {
		_ = cmd.Process.Kill()
		writeJSONError(w, http.StatusInternalServerError, "trace.out create: "+err.Error())
		return
	}
	defer outFile.Close()
	limiter := newTraceOutLimiter(outFile, maxTraceOutBytes)
	tee := io.TeeReader(stdout, limiter)
	reader, err := xtrace.NewReader(tee)
	if err != nil {
		// The runner likely failed to build/launch; include captured stderr for clarity
		_ = cmd.Process.Kill()
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		writeJSONError(w, http.StatusInternalServerError, "runner error: "+msg)
		return
	}
	opts := traceproc.Options{SynthOnRecv: optSynthHTTP, DropBlockNoCh: optDropHTTP, EmitAtomic: true}
	st := traceproc.NewParseState(opts)
	var timeline []traceproc.TimelineEvent
	emit := func(ev traceproc.TimelineEvent) error { timeline = append(timeline, ev); return nil }
	for {
		ev, err := reader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			_ = cmd.Process.Kill()
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = err.Error()
			}
			writeJSONError(w, http.StatusInternalServerError, "trace read: "+msg)
			return
		}
		if err := traceproc.ProcessEvent(&ev, st, emit); err != nil {
			_ = cmd.Process.Kill()
			writeJSONError(w, http.StatusInternalServerError, "process: "+err.Error())
			return
		}
	}
	waitErr := cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		log.Printf("runOnceHTTP: runner context aborted: %v", ctxErr)
	} else if waitErr != nil {
		log.Println("runOnceHTTP: runner wait:", waitErr)
	}
	// Deduplicate exact duplicates and write atomically to avoid truncation.
	timeline = dedupTimeline(timeline)
	payload := traceproc.NormalizeTimeline(timeline, st)
	if limiter != nil && limiter.Limited() {
		log.Printf("trace.out truncated to %d bytes (limit %d)", limiter.written, limiter.max)
	}
	if err := writeJSONAtomic("trace.json", payload); err != nil {
		log.Println("write trace.json:", err)
	}
	if err := writeJSONAtomic(filepath.Join("web", "trace.json"), payload); err != nil {
		log.Println("write web/trace.json:", err)
	}
	w.Header().Set("Content-Type", "application/json")
	if ctxErr := ctx.Err(); ctxErr != nil {
		w.Header().Set("X-Trace-Status", ctxErr.Error())
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(payload); err != nil {
		log.Println("encode timeline:", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// dedupTimeline removes exact duplicate parser emissions while preserving order.
// Key: time_ns (or derived from time_ms) + g + channel + event + attempt_id
func dedupTimeline(in []traceproc.TimelineEvent) []traceproc.TimelineEvent {
	seen := make(map[string]struct{}, len(in))
	out := make([]traceproc.TimelineEvent, 0, len(in))
	for _, ev := range in {
		tns := ev.TimeNs
		if tns == 0 {
			// derive from ms if ns not present; 1ms = 1e6 ns
			tns = int64(ev.TimeMs*1e6 + 0.5)
		}
		att := ev.AttemptID
		if att == "" && (ev.Event == "chan_send_attempt" || ev.Event == "chan_recv_attempt") {
			att = ev.ID
		}
		key := fmt.Sprintf("%d|%d|%s|%s|%s", tns, ev.G, ev.Channel, ev.Event, att)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ev)
	}
	return out
}

// writeJSONAtomic writes JSON to a temp file in the same directory and renames atomically.
func writeJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp := filepath.Join(dir, ".tmp-"+base+"-"+time.Now().Format("20060102-150405.000000000"))
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

type traceOutLimiter struct {
	w         io.Writer
	max       int64
	written   int64
	truncated bool
}

func newTraceOutLimiter(w io.Writer, max int64) *traceOutLimiter {
	return &traceOutLimiter{w: w, max: max}
}

func (l *traceOutLimiter) Write(p []byte) (int, error) {
	if l.max <= 0 {
		n, err := l.w.Write(p)
		if err != nil {
			return n, err
		}
		l.written += int64(n)
		return len(p), nil
	}
	remaining := l.max - l.written
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	toWrite := len(p)
	if int64(toWrite) > remaining {
		toWrite = int(remaining)
		l.truncated = true
	}
	if toWrite > 0 {
		if _, err := l.w.Write(p[:toWrite]); err != nil {
			return len(p), err
		}
		l.written += int64(toWrite)
	}
	return len(p), nil
}

func (l *traceOutLimiter) Limited() bool {
	return l.truncated
}

// -------- Workload files hygiene (prevent duplicate-tag compile clashes) --------

// extractWorkloadTag scans the top of a generated file and returns the
// //go:build tag value (first argument), e.g. "workload_bdcst". It stops
// scanning when it reaches the package line.
func extractWorkloadTag(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "//go:build") {
			fld := strings.Fields(line)
			if len(fld) >= 2 {
				// We only support simple single-tag headers in generated files
				return fld[1], nil
			}
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return "", sc.Err()
}

// cleanupOldWorkloadFiles deletes any other *_gen.go file under dir that has
// the same build tag (workload_<name>) as the file we are about to write,
// keeping only the canonical <name>_gen.go.
func cleanupOldWorkloadFiles(dir, name string) error {
	tag := "workload_" + name
	// Consider only *_gen.go files in the workload dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	canonical := filepath.Join(dir, name+"_gen.go")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), "_gen.go") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if p == canonical {
			continue
		}
		t, err := extractWorkloadTag(p)
		if err != nil {
			continue
		}
		if t == tag {
			// Remove stale file with same tag
			_ = os.Remove(p)
		}
	}
	return nil
}

// checkDuplicateWorkloads ensures only a single file exists for a given tag.
// Returns an error listing conflicting paths.
func checkDuplicateWorkloads(dir, tag string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), "_gen.go") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		t, err := extractWorkloadTag(p)
		if err != nil || t == "" {
			continue
		}
		if t == tag {
			matches = append(matches, p)
		}
	}
	if len(matches) > 1 {
		return fmt.Errorf("multiple workload files for %s: %s", tag, strings.Join(matches, ", "))
	}
	return nil
}

func traceHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket. Each handler owns its connection end-to-end.
	// All writes stay inside runOnce/sendSnapshotWS so we maintain a single writer goroutine.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20) // limit to 1 MiB per frame

	// Parse workload name and options from query
	q := r.URL.Query()
	wl := strings.ToLower(strings.TrimSpace(q.Get("name")))
	if wl == "" {
		wl = optWorkload
	}
	synthQ := q.Get("synth")
	dropQ := q.Get("drop_block_no_ch")
	bcModeQ := q.Get("bc_mode")
	envBool := func(v string) bool {
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "1" || v == "true" || v == "yes"
	}
	useSynth := optSynth
	useDrop := optDrop
	if synthQ != "" {
		useSynth = envBool(synthQ)
	}
	if dropQ != "" {
		useDrop = envBool(dropQ)
	}

	// Announce server options to client for HUD display
	_ = writeJSONWithDeadline(conn, struct {
		Type          string `json:"type"`
		Addr          string `json:"addr"`
		Synth         bool   `json:"synth"`
		DropBlockNoCh bool   `json:"drop_block_no_ch"`
		Workload      string `json:"workload"`
		BCMode        string `json:"bc_mode,omitempty"`
	}{Type: "server_info", Addr: listenAddr, Synth: useSynth, DropBlockNoCh: useDrop, Workload: wl, BCMode: func() string {
		if bcModeQ != "" {
			return bcModeQ
		}
		return optBCMode
	}()})

	// Read simple text commands from the client. "start" triggers a run.
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			// client closed or network error
			if !websocket.IsCloseError(err) && !strings.Contains(err.Error(), "closed") {
				log.Println("ws read:", err)
			}
			return
		}
		if mt != websocket.TextMessage {
			continue
		}
		if strings.TrimSpace(string(data)) != "start" {
			continue
		}
		if err := runOnce(conn, wl, useSynth, useDrop, bcModeQ); err != nil {
			if err == io.EOF {
				// normal end of stream
				continue
			}
			log.Println("runOnce:", err)
		}
	}
}

const (
	liveEventBuffer    = 4096
	runHistoryCapacity = 2000
	writeTimeout       = 5 * time.Second
)

func writeJSONWithDeadline(conn *websocket.Conn, v any) error {
	if conn == nil {
		return nil
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteJSON(v)
}

type runHistory struct {
	mu     sync.Mutex
	events []traceproc.TimelineEvent
	max    int
}

func newRunHistory(max int) *runHistory {
	if max <= 0 {
		max = runHistoryCapacity
	}
	return &runHistory{max: max}
}

func (h *runHistory) add(ev traceproc.TimelineEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.events) == h.max {
		copy(h.events, h.events[1:])
		h.events[len(h.events)-1] = ev
	} else {
		h.events = append(h.events, ev)
	}
}

func (h *runHistory) snapshot() []traceproc.TimelineEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]traceproc.TimelineEvent, len(h.events))
	copy(out, h.events)
	return out
}

func (h *runHistory) lastSeq() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.events) == 0 {
		return 0
	}
	return h.events[len(h.events)-1].Seq
}

func waitForHistory(history *runHistory, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for history.lastSeq() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

func parseSeqID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return v
	}
	return 0
}

func snapshotSeqBounds(events []traceproc.TimelineEvent) (int64, int64) {
	if len(events) == 0 {
		return 0, 0
	}
	return events[0].Seq, events[len(events)-1].Seq
}

func sendSnapshotWS(conn *websocket.Conn, runID string, events []traceproc.TimelineEvent) (int64, error) {
	seqStart, seqEnd := snapshotSeqBounds(events)
	if err := writeJSONWithDeadline(conn, map[string]any{"type": "snapshot_begin", "run_id": runID, "seq_start": seqStart, "seq_end": seqEnd}); err != nil {
		return seqEnd, err
	}
	for _, ev := range events {
		payload := map[string]any{"type": "snapshot_event", "run_id": runID, "seq": ev.Seq, "event": ev}
		if err := writeJSONWithDeadline(conn, payload); err != nil {
			return seqEnd, err
		}
	}
	if err := writeJSONWithDeadline(conn, map[string]any{"type": "snapshot_end", "run_id": runID, "seq_end": seqEnd}); err != nil {
		return seqEnd, err
	}
	return seqEnd, nil
}

// runOnce runs the demo under runtime/trace and streams events over the given WS connection.
func runOnce(conn *websocket.Conn, wl string, synth, drop bool, bcMode string) error {
	// All writes to conn occur in this goroutine (snapshot + live event streaming).
	// If you ever add ping/status pushes, send them through the same path (or through a dedicated write loop/channel).
	// Best-effort cleanup of stale duplicate workload files before the duplicate check.
	_ = cleanupOldWorkloadFiles("internal/workload", wl)
	// Validate there are no duplicates for this workload tag.
	if err := checkDuplicateWorkloads("internal/workload", "workload_"+wl); err != nil {
		_ = writeJSONWithDeadline(conn, struct {
			Type  string `json:"type"`
			Error string `json:"error"`
		}{Type: "error", Error: "workload configuration error: " + err.Error()})
		return err
	}
	// Spawn runner so newly created workloads are visible.
	// Use a modest timeout for live runs to avoid hanging sessions
	// Always pass a single workload-specific tag; files without build tags are still compiled.
	tagName := "workload_" + wl
	cmd := exec.Command("go", "run", "-tags", tagName, "./cmd/gtv-runner", "-workload", wl, "-timeout", "10s")
	if bcMode != "" {
		cmd.Env = append(os.Environ(), "GTV_BC_MODE="+strings.ToLower(bcMode))
	}
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Tee raw trace bytes to trace.out while parsing.
	outFile, err := os.Create("trace.out")
	if err != nil {
		_ = cmd.Process.Kill()
		_ = writeJSONWithDeadline(conn, struct {
			Type  string `json:"type"`
			Error string `json:"error"`
		}{Type: "error", Error: "trace.out create: " + err.Error()})
		return err
	}
	defer outFile.Close()
	tee := io.TeeReader(stdout, outFile)
	runID := "run-" + time.Now().Format("20060102-150405.000")
	history := newRunHistory(runHistoryCapacity)
	eventCh := make(chan traceproc.TimelineEvent, liveEventBuffer)
	liveEventCh := make(chan traceproc.TimelineEvent, liveEventBuffer)
	bufferReadyCh := make(chan struct{})
	var bufferReadyOnce sync.Once
	closeBufferReady := func() {
		bufferReadyOnce.Do(func() { close(bufferReadyCh) })
	}
	defer closeBufferReady()
	record := func(ev traceproc.TimelineEvent) error {
		history.add(ev)
		return nil
	}
	errCh := make(chan error, 1)
	go func() {
		err := streamTraceEvents(tee, cmd, runID, synth, drop, optLiveLog, func(ev traceproc.TimelineEvent) error {
			eventCh <- ev
			return nil
		}, record)
		close(eventCh)
		errCh <- err
	}()
	go func() {
		defer close(liveEventCh)
		pending := make([]traceproc.TimelineEvent, 0, 64)
		buffering := true
		for ev := range eventCh {
			if buffering {
				select {
				case <-bufferReadyCh:
					buffering = false
					for _, buffered := range pending {
						liveEventCh <- buffered
					}
					pending = pending[:0]
				default:
				}
			}
			if buffering {
				pending = append(pending, ev)
				continue
			}
			liveEventCh <- ev
		}
		if buffering {
			<-bufferReadyCh
			for _, buffered := range pending {
				liveEventCh <- buffered
			}
		}
	}()
	// Announce run start with embedded options and simple metadata
	_ = writeJSONWithDeadline(conn, struct {
		Type          string `json:"type"`
		Addr          string `json:"addr,omitempty"`
		Synth         bool   `json:"synth,omitempty"`
		DropBlockNoCh bool   `json:"drop_block_no_ch,omitempty"`
		RunID         string `json:"run_id,omitempty"`
		StartedUnixMs int64  `json:"started_unix_ms,omitempty"`
		Workload      string `json:"workload,omitempty"`
		BCMode        string `json:"bc_mode,omitempty"`
	}{Type: "run_start", Addr: listenAddr, Synth: synth, DropBlockNoCh: drop, RunID: runID, StartedUnixMs: time.Now().UnixMilli(), Workload: wl, BCMode: optBCMode})
	waitForHistory(history, 200*time.Millisecond)
	snapshotEvents := history.snapshot()
	lastSeq, snapErr := sendSnapshotWS(conn, runID, snapshotEvents)
	if snapErr != nil {
		closeBufferReady()
		_ = cmd.Process.Kill()
		return snapErr
	}
	closeBufferReady()
	sendErr := error(nil)
	for ev := range liveEventCh {
		if ev.Seq <= lastSeq {
			continue
		}
		if sendErr != nil {
			continue
		}
		if err := writeJSONWithDeadline(conn, map[string]any{"type": "event", "run_id": runID, "seq": ev.Seq, "event": ev}); err != nil {
			sendErr = err
			_ = cmd.Process.Kill()
			continue
		}
		lastSeq = ev.Seq
	}
	streamErr := <-errCh
	if sendErr != nil {
		return sendErr
	}
	if streamErr != nil {
		_ = writeJSONWithDeadline(conn, struct {
			Type  string `json:"type"`
			Error string `json:"error"`
		}{Type: "error", Error: "trace read failed: " + streamErr.Error()})
		return streamErr
	}
	_ = writeJSONWithDeadline(conn, struct {
		Type string `json:"type"`
	}{Type: "run_end"})
	return nil
}

func streamTraceEvents(reader io.Reader, cmd *exec.Cmd, runID string, synth, drop bool, liveLog bool, emit func(traceproc.TimelineEvent) error, record func(traceproc.TimelineEvent) error) error {
	traceReader, err := xtrace.NewReader(reader)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	opts := traceproc.Options{SynthOnRecv: synth, DropBlockNoCh: drop, EmitAtomic: true}
	st := traceproc.NewParseState(opts)
	var liveLogFile *os.File
	var liveLogCount int
	if liveLog {
		liveLogPath := fmt.Sprintf("trace.live.%s.json", runID)
		if f, err := os.Create(liveLogPath); err != nil {
			log.Println("live log create:", err)
		} else {
			liveLogFile = f
			if _, err := liveLogFile.WriteString("[\n"); err != nil {
				log.Println("live log initialize:", err)
				_ = liveLogFile.Close()
				liveLogFile = nil
			}
		}
	}
	defer func() {
		if liveLogFile == nil {
			return
		}
		if liveLogCount == 0 {
			if _, err := liveLogFile.WriteString("]\n"); err != nil {
				log.Println("live log finalize:", err)
			}
		} else {
			if _, err := liveLogFile.WriteString("\n]\n"); err != nil {
				log.Println("live log finalize:", err)
			}
		}
		_ = liveLogFile.Close()
	}()
	var seq int64
	emitWithSeq := func(ev traceproc.TimelineEvent) error {
		seq++
		ev.Seq = seq
		if record != nil {
			if err := record(ev); err != nil {
				return err
			}
		}
		return emit(ev)
	}

	for {
		ev, err := traceReader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				_ = cmd.Wait()
				return nil
			}
			_ = cmd.Process.Kill()
			return err
		}
		if liveLogFile != nil {
			data, err := json.Marshal(ev)
			if err != nil {
				log.Println("live event log marshal:", err)
			} else {
				if liveLogCount > 0 {
					if _, err := liveLogFile.WriteString(",\n"); err != nil {
						log.Println("live event log write:", err)
						_ = liveLogFile.Close()
						liveLogFile = nil
					}
				}
				if liveLogFile != nil {
					if _, err := liveLogFile.Write(data); err != nil {
						log.Println("live event log write:", err)
						_ = liveLogFile.Close()
						liveLogFile = nil
					} else {
						liveLogCount++
					}
				}
			}
		}
		if err := traceproc.ProcessEvent(&ev, st, emitWithSeq); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// streamEvents converts x/exp/trace events into TimelineEvent and emits them incrementally.
// old streamEvents replaced by shared processor usage

// runPingPongProgram and worker moved to internal/workload
