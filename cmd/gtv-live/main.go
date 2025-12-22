package main

import (
	"bufio"
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
	event_sink "jspt/cmd/event_sink"
	"jspt/internal/instrumenter"
	"jspt/internal/traceproc"
)

// TimelineEvent type is unified via internal/traceproc.

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // dev only; tighten for prod
}

// Global options configured via flags (with env fallbacks)
var (
	listenAddr  string
	optSynth    bool
	optDrop     bool
	optWorkload string
	optBCMode   string
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
	bcModeFlag := flag.String("bc-mode", bcModeDefault, "broadcast mode: buffered|blocking")
	flag.Parse()
	listenAddr = *addrFlag
	optSynth = *synthFlag
	optDrop = *dropFlag
	optWorkload = strings.ToLower(*wlFlag)
	optBCMode = strings.ToLower(*bcModeFlag)

	// Serve static assets from ./web
	fs := http.FileServer(http.Dir("web"))
	http.Handle("/", fs)

	// WebSocket endpoint that streams timeline events while the demo runs.
	http.HandleFunc("/trace", traceHandler)
	http.HandleFunc("/trace-sse", traceSSEHandler)

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
		if req.Level != "" {
			def.Level = strings.ToLower(req.Level)
		} else {
			def.Level = "regions_logs"
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
	cmd := exec.Command("go", "run", "-tags", tagName, "./cmd/gtv-runner", "-workload", name, "-timeout", timeoutQ)
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
	tee := io.TeeReader(stdout, outFile)
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
	_ = cmd.Wait()
	// Deduplicate exact duplicates and write atomically to avoid truncation.
	timeline = dedupTimeline(timeline)
	if err := writeJSONAtomic("trace.json", timeline); err != nil {
		log.Println("write trace.json:", err)
	}
	if err := writeJSONAtomic(filepath.Join("web", "trace.json"), timeline); err != nil {
		log.Println("write web/trace.json:", err)
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(timeline); err != nil {
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
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}
	defer conn.Close()

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
	_ = conn.WriteJSON(struct {
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
	sseBatchFlushInterval = 150 * time.Millisecond
	sseBatchSize          = 32
	eventSinkCapacity     = 4096
	runHistoryCapacity    = 2000
)

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

func sendSnapshot(w io.Writer, flusher http.Flusher, runID string, events []traceproc.TimelineEvent) error {
	if err := sendSSE(w, flusher, "snapshot_begin", "", map[string]string{"run_id": runID}); err != nil {
		return err
	}
	payload := struct {
		RunID  string                    `json:"run_id"`
		Events []traceproc.TimelineEvent `json:"events"`
	}{RunID: runID, Events: events}
	if err := sendSSE(w, flusher, "snapshot", "", payload); err != nil {
		return err
	}
	return sendSSE(w, flusher, "snapshot_end", "", map[string]string{"run_id": runID})
}

func traceSSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "handler requires http.Flusher", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ctx := r.Context()
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
	lastID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	_ = cleanupOldWorkloadFiles("internal/workload", wl)
	if err := checkDuplicateWorkloads("internal/workload", "workload_"+wl); err != nil {
		_ = sendSSE(w, flusher, "error", "", map[string]string{"error": "workload configuration error: " + err.Error()})
		return
	}
	tagName := "workload_" + wl
	cmd := exec.Command("go", "run", "-tags", tagName, "./cmd/gtv-runner", "-workload", wl, "-timeout", "10s")
	if bcModeQ != "" {
		cmd.Env = append(os.Environ(), "GTV_BC_MODE="+strings.ToLower(bcModeQ))
	}
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = sendSSE(w, flusher, "error", "", map[string]string{"error": "runner stdout: " + err.Error()})
		return
	}
	if err := cmd.Start(); err != nil {
		_ = sendSSE(w, flusher, "error", "", map[string]string{"error": "runner start: " + err.Error()})
		return
	}
	runID := "run-" + time.Now().Format("20060102-150405.000")
	sink := event_sink.NewEventSink(eventSinkCapacity)
	sub := sink.Subscribe(lastID)
	defer sub.Close()
	history := newRunHistory(runHistoryCapacity)

	if err := sink.Add("server_info", struct {
		Addr          string `json:"addr"`
		Synth         bool   `json:"synth"`
		DropBlockNoCh bool   `json:"drop_block_no_ch"`
		Workload      string `json:"workload"`
		BCMode        string `json:"bc_mode,omitempty"`
	}{Addr: listenAddr, Synth: useSynth, DropBlockNoCh: useDrop, Workload: wl, BCMode: func() string {
		if bcModeQ != "" {
			return bcModeQ
		}
		return optBCMode
	}()}); err != nil {
		log.Println("traceSSEHandler server_info:", err)
	}
	if err := sink.Add("run_start", struct {
		RunID         string `json:"run_id"`
		StartedUnixMs int64  `json:"started_unix_ms"`
		Workload      string `json:"workload"`
	}{RunID: runID, StartedUnixMs: time.Now().UnixMilli(), Workload: wl}); err != nil {
		log.Println("traceSSEHandler run_start:", err)
	}

	go func() {
		defer sink.Close()
		record := func(ev traceproc.TimelineEvent) error {
			history.add(ev)
			return nil
		}
		err := streamTraceEvents(stdout, cmd, runID, useSynth, useDrop, func(ev traceproc.TimelineEvent) error {
			return sink.Add("timeline", ev)
		}, record)
		if err != nil {
			_ = sink.Add("error", map[string]string{"error": "trace read failed: " + err.Error()})
		}
		_ = sink.Add("run_end", struct{}{})
	}()

	lastSeq := parseSeqID(lastID)
	if since := parseSeqID(q.Get("since_seq")); since > lastSeq {
		lastSeq = since
	}
	if lastSeq == 0 {
		waitForHistory(history, 200*time.Millisecond)
		if err := sendSnapshot(w, flusher, runID, history.snapshot()); err != nil {
			log.Println("traceSSEHandler snapshot:", err)
		}
		lastSeq = history.lastSeq()
	}

	sendBatch := func(batch []event_sink.Event) error {
		for _, ev := range batch {
			if ev.Name == "timeline" {
				var te traceproc.TimelineEvent
				if err := json.Unmarshal(ev.Payload, &te); err == nil {
					if te.Seq <= lastSeq {
						continue
					}
					id := fmt.Sprintf("%d", te.Seq)
					if err := sendSSE(w, flusher, "timeline", id, ev.Payload); err != nil {
						return err
					}
					lastSeq = te.Seq
					continue
				}
			}
			if err := sendSSE(w, flusher, ev.Name, ev.ID, ev.Payload); err != nil {
				return err
			}
		}
		return nil
	}

	ticker := time.NewTicker(sseBatchFlushInterval)
	defer ticker.Stop()
	batch := make([]event_sink.Event, 0, sseBatchSize)
	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := sendBatch(batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := cmd.Process.Kill(); err != nil {
				log.Println("traceSSEHandler cmd kill:", err)
			}
			if err := flushBatch(); err != nil {
				log.Println("traceSSEHandler flush:", err)
			}
			return
		case ev, ok := <-sub.Events:
			if !ok {
				if err := flushBatch(); err != nil {
					log.Println("traceSSEHandler flush:", err)
				}
				return
			}
			batch = append(batch, ev)
			if len(batch) >= sseBatchSize {
				if err := flushBatch(); err != nil {
					log.Println("traceSSEHandler flush:", err)
					return
				}
			}
		case <-ticker.C:
			if err := flushBatch(); err != nil {
				log.Println("traceSSEHandler flush:", err)
				return
			}
		}
	}
}

// runOnce runs the demo under runtime/trace and streams events over the given WS connection.
func runOnce(conn *websocket.Conn, wl string, synth, drop bool, bcMode string) error {
	// Best-effort cleanup of stale duplicate workload files before the duplicate check.
	_ = cleanupOldWorkloadFiles("internal/workload", wl)
	// Validate there are no duplicates for this workload tag.
	if err := checkDuplicateWorkloads("internal/workload", "workload_"+wl); err != nil {
		_ = conn.WriteJSON(struct {
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
		_ = conn.WriteJSON(struct {
			Type  string `json:"type"`
			Error string `json:"error"`
		}{Type: "error", Error: "trace.out create: " + err.Error()})
		return err
	}
	defer outFile.Close()
	tee := io.TeeReader(stdout, outFile)
	runID := "run-" + time.Now().Format("20060102-150405.000")
	history := newRunHistory(runHistoryCapacity)
	emit := func(ev traceproc.TimelineEvent) error {
		return conn.WriteJSON(ev)
	}
	// Announce run start with embedded options and simple metadata
	_ = conn.WriteJSON(struct {
		Type          string `json:"type"`
		Addr          string `json:"addr,omitempty"`
		Synth         bool   `json:"synth,omitempty"`
		DropBlockNoCh bool   `json:"drop_block_no_ch,omitempty"`
		RunID         string `json:"run_id,omitempty"`
		StartedUnixMs int64  `json:"started_unix_ms,omitempty"`
		Workload      string `json:"workload,omitempty"`
		BCMode        string `json:"bc_mode,omitempty"`
	}{Type: "run_start", Addr: listenAddr, Synth: synth, DropBlockNoCh: drop, RunID: runID, StartedUnixMs: time.Now().UnixMilli(), Workload: wl, BCMode: optBCMode})
	_ = conn.WriteJSON(struct {
		Type string `json:"type"`
	}{Type: "snapshot_begin"})
	_ = conn.WriteJSON(struct {
		Type   string                    `json:"type"`
		RunID  string                    `json:"run_id"`
		Events []traceproc.TimelineEvent `json:"events"`
	}{Type: "snapshot", RunID: runID, Events: history.snapshot()})
	_ = conn.WriteJSON(struct {
		Type string `json:"type"`
	}{Type: "snapshot_end"})
	record := func(ev traceproc.TimelineEvent) error {
		history.add(ev)
		return nil
	}
	if err := streamTraceEvents(tee, cmd, runID, synth, drop, emit, record); err != nil {
		// send error and return
		_ = conn.WriteJSON(struct {
			Type  string `json:"type"`
			Error string `json:"error"`
		}{Type: "error", Error: "trace read failed: " + err.Error()})
		return err
	}
	_ = conn.WriteJSON(struct {
		Type string `json:"type"`
	}{Type: "run_end"})
	return nil
}

func streamTraceEvents(reader io.Reader, cmd *exec.Cmd, runID string, synth, drop bool, emit func(traceproc.TimelineEvent) error, record func(traceproc.TimelineEvent) error) error {
	traceReader, err := xtrace.NewReader(reader)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	opts := traceproc.Options{SynthOnRecv: synth, DropBlockNoCh: drop, EmitAtomic: true}
	st := traceproc.NewParseState(opts)
	liveLogPath := fmt.Sprintf("trace.live.%s.json", runID)
	var liveLogFile *os.File
	var liveLogCount int
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

func sendSSE(w io.Writer, flusher http.Flusher, event, id string, payload any) error {
	if payload == nil {
		return nil
	}
	var data []byte
	switch v := payload.(type) {
	case []byte:
		data = v
	case json.RawMessage:
		data = v
	default:
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
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
