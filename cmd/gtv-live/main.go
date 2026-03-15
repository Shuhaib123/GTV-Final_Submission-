package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	optTimeout  string
	optMVP      bool
	optMode     string
	goTraceMu   sync.Mutex
	goTraceCmd  *exec.Cmd
)

func envBool(name string) bool {
	v := os.Getenv(name)
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func parseTimeoutDuration(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

func withTimeoutSignals(cmd *exec.Cmd, timeout time.Duration) {
	if cmd == nil || cmd.Process == nil || timeout <= 0 {
		return
	}
	grace := 750 * time.Millisecond
	send := func(sig os.Signal) {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Signal(sig)
	}
	go func() {
		time.Sleep(timeout)
		send(os.Interrupt)
		time.Sleep(grace)
		if runtime.GOOS != "windows" {
			send(syscall.SIGTERM)
		}
		time.Sleep(grace)
		send(os.Kill)
	}()
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
	mvpDefault := envBool("GTV_MVP")
	mvpFlag := flag.Bool("mvp", mvpDefault, "force MVP defaults (disable IO/loop/HTTP/GRPC/value logs)")
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
	timeoutDefault := os.Getenv("GTV_TIMEOUT")
	if timeoutDefault == "" {
		timeoutDefault = "10s"
	}
	timeoutFlag := flag.String("timeout", timeoutDefault, "runner timeout (e.g., 2s, 500ms); also sets GTV_TIMEOUT_MS for child")
	modeDefault := strings.TrimSpace(os.Getenv("GTV_MODE"))
	if modeDefault == "" {
		modeDefault = "teach"
	}
	modeFlag := flag.String("mode", modeDefault, "event mode: teach or debug")
	flag.Parse()
	listenAddr = *addrFlag
	optSynth = *synthFlag
	optDrop = *dropFlag
	optWorkload = strings.ToLower(*wlFlag)
	optBCMode = strings.ToLower(*bcModeFlag)
	optLiveLog = *liveLogFlag
	optTimeout = strings.TrimSpace(*timeoutFlag)
	optMode = strings.ToLower(strings.TrimSpace(*modeFlag))
	optMVP = *mvpFlag
	if optMVP {
		_ = os.Setenv("GTV_MVP", "1")
	}

	// Serve static assets from ./web
	webRoot := "web"
	if abs, err := filepath.Abs(webRoot); err == nil {
		log.Printf("serving static web root: %s", abs)
	}
	fs := http.FileServer(http.Dir(webRoot))
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/pages/index/index.html", http.StatusFound)
			return
		}
		// Dev-friendly: always serve the latest HTML/CSS/JS without browser cache surprises.
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		fs.ServeHTTP(w, r)
	}))

	// WebSocket endpoint that streams timeline events while the demo runs.
	http.HandleFunc("/trace", traceHandler)

	// Instrumentation endpoint: POST {code, workload_name}
	http.HandleFunc("/instrument", instrumentHandler)
	// Workload read endpoint: GET ?name=workload
	http.HandleFunc("/workload", workloadHandler)
	// Workload list endpoint: GET generated workloads under internal/workload
	http.HandleFunc("/workloads", workloadsHandler)

	// Run endpoint: spawns an isolated runner and returns a JSON array timeline
	http.HandleFunc("/run", runOnceHTTP)
	// Demo endpoint: runs pingpong and opens Go's native trace viewer.
	http.HandleFunc("/demo/go-trace", demoGoTraceHTTP)
	// Clear cached runner binaries
	http.HandleFunc("/clear-build-cache", clearBuildCacheHTTP)

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
	Code           string `json:"code"`
	WorkloadName   string `json:"workload_name"`
	GuardLabels    *bool  `json:"guard_labels,omitempty"`
	GorRegions     *bool  `json:"goroutine_regions,omitempty"`
	Level          string `json:"level,omitempty"`
	SyncValidation *bool  `json:"sync_validation,omitempty"`
	BlockRegions   *bool  `json:"block_regions,omitempty"`
	IORegions      *bool  `json:"io_regions,omitempty"`
	IOJSON         *bool  `json:"io_json,omitempty"`
	IODB           *bool  `json:"io_db,omitempty"`
	IOHTTP         *bool  `json:"io_http,omitempty"`
	IOOS           *bool  `json:"io_os,omitempty"`
	ValueLogs      *bool  `json:"value_logs,omitempty"`
}

func applySyncValidationPreset(opts *instrumenter.Options) {
	if opts == nil {
		return
	}
	// Sync validation requires rich labels + block wrappers for reliable sync topology.
	opts.Level = "regions_logs"
	opts.AddBlockRegions = true
	opts.AddGoroutineRegions = true
	opts.GuardDynamicLabels = true
}

func buildInstrumentOptions(req instrumentReq) (instrumenter.Options, bool) {
	opts := instrumenter.Options{
		GuardDynamicLabels:  true,
		AddGoroutineRegions: true,
		AddBlockRegions:     true,
		Level:               "regions",
	}
	if req.GuardLabels != nil {
		opts.GuardDynamicLabels = *req.GuardLabels
	}
	if req.GorRegions != nil {
		opts.AddGoroutineRegions = *req.GorRegions
	}
	if req.BlockRegions != nil {
		opts.AddBlockRegions = *req.BlockRegions
	}
	if req.IORegions != nil {
		opts.AddIORegions = *req.IORegions
	}
	if req.IOJSON != nil {
		opts.AddIOJSONRegions = *req.IOJSON
	}
	if req.IODB != nil {
		opts.AddIODBRegions = *req.IODB
	}
	if req.IOHTTP != nil {
		opts.AddIOHTTPRegions = *req.IOHTTP
	}
	if req.IOOS != nil {
		opts.AddIOOSRegions = *req.IOOS
	}
	if req.ValueLogs != nil {
		opts.AddValueLogs = *req.ValueLogs
	}
	if req.Level != "" {
		opts.Level = strings.ToLower(req.Level)
	}
	syncValidation := req.SyncValidation != nil && *req.SyncValidation
	if syncValidation {
		applySyncValidationPreset(&opts)
	}
	return opts, syncValidation
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
	// Per-request instrumenter options (fallback to defaults when nil).
	needsOpts := req.GuardLabels != nil || req.GorRegions != nil || req.Level != "" || req.SyncValidation != nil || req.BlockRegions != nil || req.IORegions != nil || req.IOJSON != nil || req.IODB != nil || req.IOHTTP != nil || req.IOOS != nil || req.ValueLogs != nil
	syncValidationApplied := false
	if needsOpts || optMVP {
		def, syncValidation := buildInstrumentOptions(req)
		syncValidationApplied = syncValidation
		instrumenter.SetOptions(def)
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
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "workload": low, "file": path, "sync_validation_applied": syncValidationApplied})
}

func workloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "missing name")
		return
	}
	for _, ch := range name {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' {
			writeJSONError(w, http.StatusBadRequest, "invalid workload name")
			return
		}
	}
	low := strings.ToLower(name)
	path := filepath.Join("internal", "workload", low+"_gen.go")
	data, err := os.ReadFile(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "workload not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

func listGeneratedWorkloadNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		file := entry.Name()
		if !strings.HasSuffix(file, "_gen.go") {
			continue
		}
		name := strings.TrimSuffix(file, "_gen.go")
		if name == "" {
			continue
		}
		valid := true
		for _, ch := range name {
			if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)
	return names, nil
}

func workloadsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	names, err := listGeneratedWorkloadNames(filepath.Join("internal", "workload"))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list workloads")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workloads": names,
	})
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
	modeQ := r.URL.Query().Get("mode")
	timeoutMsQ := strings.TrimSpace(r.URL.Query().Get("timeout_ms"))
	buildCacheQ := strings.TrimSpace(r.URL.Query().Get("build_cache"))
	timeoutQ := strings.TrimSpace(r.URL.Query().Get("timeout"))
	debugQ := strings.TrimSpace(r.URL.Query().Get("debug"))
	timeoutMs := int64(0)
	if timeoutMsQ != "" {
		if v, err := strconv.ParseInt(timeoutMsQ, 10, 64); err == nil {
			if v < 50 {
				v = 50
			}
			if v > 20000 {
				v = 20000
			}
			timeoutMs = v
		}
	}
	if timeoutQ == "" {
		if optTimeout != "" {
			timeoutQ = optTimeout
		} else {
			timeoutQ = "1s"
		}
	}
	timeoutDur := time.Duration(timeoutMs) * time.Millisecond
	timeoutOK := timeoutDur > 0
	if !timeoutOK {
		timeoutDur, timeoutOK = parseTimeoutDuration(timeoutQ)
		if timeoutOK {
			timeoutMs = int64(timeoutDur / time.Millisecond)
		}
	} else {
		timeoutQ = fmt.Sprintf("%dms", timeoutMs)
	}
	var timeoutMS string
	if timeoutOK {
		timeoutMS = strconv.FormatInt(timeoutMs, 10)
	}
	ctxTimeout := runJSONTimeout
	buildGrace := 8 * time.Second
	if timeoutOK {
		ctxTimeout = timeoutDur + buildGrace + 2*time.Second
		if ctxTimeout < 10*time.Second {
			ctxTimeout = 10 * time.Second
		}
		if ctxTimeout > runJSONTimeout {
			ctxTimeout = runJSONTimeout
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), ctxTimeout)
	defer cancel()
	envBool := func(v string) bool {
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "1" || v == "true" || v == "yes"
	}
	optSynthHTTP := envBool(synthQ)
	optDropHTTP := envBool(dropQ)
	optBuildCache := buildCacheQ == "" || envBool(buildCacheQ)
	optDebug := envBool(debugQ)
	optModeHTTP := optMode
	if modeQ != "" {
		optModeHTTP = strings.ToLower(strings.TrimSpace(modeQ))
	}

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
	binPath, cleanup, err := ensureRunnerBinary(ctx, tagName, name, optBuildCache)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, binPath, "-workload", name, "-timeout", timeoutQ)
	// Default bc_mode=blocking unless explicitly provided
	env := os.Environ()
	if optMVP {
		env = append(env, "GTV_MVP=1")
	}
	if bcModeQ != "" {
		env = append(env, "GTV_BC_MODE="+strings.ToLower(bcModeQ))
	} else {
		env = append(env, "GTV_BC_MODE=blocking")
	}
	if timeoutMS != "" {
		env = append(env, "GTV_TIMEOUT_MS="+timeoutMS)
	}
	traceTmp, err := os.CreateTemp("", "gtv-trace-*.out")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "trace temp: "+err.Error())
		return
	}
	tracePath := traceTmp.Name()
	if err := traceTmp.Close(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "trace temp close: "+err.Error())
		return
	}
	env = append(env, "GTV_TRACE_OUT="+tracePath)
	cmd.Env = env
	// Capture stderr so we can surface build/runtime errors back to the client
	var errBuf strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	cmd.Stdout = io.Discard
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "runner start: "+err.Error())
		return
	}
	waitErr := cmd.Wait()
	exitDur := time.Since(startTime)
	if ctxErr := ctx.Err(); ctxErr != nil {
		log.Printf("runOnceHTTP: runner context aborted: %v", ctxErr)
	} else if waitErr != nil {
		log.Println("runOnceHTTP: runner wait:", waitErr)
	}

	traceInfo, statErr := os.Stat(tracePath)
	if statErr == nil && traceInfo.Size() == 0 {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = "trace file empty (build may not have finished); try a larger timeout_ms"
		}
		if optDebug {
			writeJSONErrorDetailsWithDebug(w, http.StatusInternalServerError, "trace read", msg, stderrTail(errBuf.String(), 20), map[string]string{
				"trace_path": tracePath,
				"trace_size": "0",
				"exit_ms":    fmt.Sprintf("%d", exitDur.Milliseconds()),
			})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "trace read: "+msg)
		return
	}
	traceFile, err := os.Open(tracePath)
	if err != nil {
		tail := stderrTail(errBuf.String(), 20)
		if optDebug {
			writeJSONErrorDetailsWithDebug(w, http.StatusInternalServerError, "trace open", err.Error(), tail, map[string]string{
				"trace_path": tracePath,
				"exit_ms":    fmt.Sprintf("%d", exitDur.Milliseconds()),
			})
			return
		}
		msg := err.Error()
		if tail != "" {
			msg = msg + " | stderr: " + tail
		}
		writeJSONError(w, http.StatusInternalServerError, "trace open: "+msg)
		return
	}
	defer func() {
		traceFile.Close()
		_ = os.Remove(tracePath)
	}()

	reader, err := xtrace.NewReader(traceFile)
	if err != nil {
		tail := stderrTail(errBuf.String(), 20)
		if optDebug {
			writeJSONErrorDetailsWithDebug(w, http.StatusInternalServerError, "trace read", err.Error(), tail, map[string]string{
				"trace_path": tracePath,
				"exit_ms":    fmt.Sprintf("%d", exitDur.Milliseconds()),
			})
			return
		}
		writeJSONErrorDetails(w, http.StatusInternalServerError, "trace read", err.Error(), tail)
		return
	}
	opts := traceproc.Options{SynthOnRecv: optSynthHTTP, DropBlockNoCh: optDropHTTP, EmitAtomic: true, Mode: optModeHTTP}
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
			tail := stderrTail(errBuf.String(), 20)
			parseErr := err.Error()
			if tail == "" {
				tail = strings.TrimSpace(errBuf.String())
			}
			if optDebug {
				writeJSONErrorDetailsWithDebug(w, http.StatusInternalServerError, "trace read", parseErr, tail, map[string]string{
					"trace_path": tracePath,
					"exit_ms":    fmt.Sprintf("%d", exitDur.Milliseconds()),
				})
				return
			}
			writeJSONErrorDetails(w, http.StatusInternalServerError, "trace read", parseErr, tail)
			return
		}
		if err := traceproc.ProcessEvent(&ev, st, emit); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "process: "+err.Error())
			return
		}
	}
	if err := traceproc.EmitAuditSummary(st, emit, "run"); err != nil {
		log.Println("audit summary:", err)
	}
	// Deduplicate exact duplicates and write atomically to avoid truncation.
	timeline = dedupTimeline(timeline)
	payload := traceproc.NormalizeTimeline(timeline, st)
	unmatchedByCh, unmatchedTotals, auditWarnings := traceproc.ComputeUnmatchedAuditFromEvents(payload.Events)
	payload.Events = traceproc.MergeUnmatchedAuditIntoSummary(payload.Events, unmatchedByCh, unmatchedTotals, auditWarnings)

	// Copy trace bytes to trace.out (best-effort, bounded).
	if src, err := os.Open(tracePath); err == nil {
		if outFile, err := os.Create("trace.out"); err == nil {
			limiter := newTraceOutLimiter(outFile, maxTraceOutBytes)
			_, _ = io.Copy(limiter, src)
			_ = outFile.Close()
			if limiter != nil && limiter.Limited() {
				log.Printf("trace.out truncated to %d bytes (limit %d)", limiter.written, limiter.max)
			}
		}
		_ = src.Close()
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

func demoGoTraceHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	timeoutQ := strings.TrimSpace(r.URL.Query().Get("timeout"))
	if timeoutQ == "" {
		if optTimeout != "" {
			timeoutQ = optTimeout
		} else {
			timeoutQ = "2s"
		}
	}
	if _, ok := parseTimeoutDuration(timeoutQ); !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid timeout")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	if err := runPingPongTraceOut(ctx, timeoutQ); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	viewerURL, err := launchGoTraceViewer("trace.out")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"url":    viewerURL,
	})
}

func runPingPongTraceOut(ctx context.Context, timeoutQ string) error {
	binPath, cleanup, err := ensureRunnerBinary(ctx, "workload_pingpong", "pingpong", true)
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, binPath, "-workload", "pingpong", "-timeout", timeoutQ)
	env := os.Environ()
	if optMVP {
		env = append(env, "GTV_MVP=1")
	}
	env = append(env, "GTV_BC_MODE=blocking", "GTV_TRACE_OUT=trace.out")
	if d, ok := parseTimeoutDuration(timeoutQ); ok {
		env = append(env, "GTV_TIMEOUT_MS="+strconv.FormatInt(int64(d/time.Millisecond), 10))
	}
	cmd.Env = env
	cmd.Stdout = io.Discard
	var errBuf strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("demo run failed: %s", msg)
	}
	info, err := os.Stat("trace.out")
	if err != nil {
		return fmt.Errorf("trace.out not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("trace.out is empty after pingpong run")
	}
	return nil
}

func launchGoTraceViewer(traceFile string) (string, error) {
	info, err := os.Stat(traceFile)
	if err != nil {
		return "", fmt.Errorf("trace file missing: %w", err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("trace file is empty: %s", traceFile)
	}
	addr, err := reserveLoopbackAddr()
	if err != nil {
		return "", fmt.Errorf("viewer addr: %w", err)
	}
	url := "http://" + addr
	cmd := exec.Command("go", "tool", "trace", "-http="+addr, traceFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	goTraceMu.Lock()
	if goTraceCmd != nil && goTraceCmd.Process != nil {
		_ = goTraceCmd.Process.Kill()
	}
	goTraceCmd = nil
	if err := cmd.Start(); err != nil {
		goTraceMu.Unlock()
		return "", fmt.Errorf("go tool trace start: %w", err)
	}
	goTraceCmd = cmd
	goTraceMu.Unlock()

	go func(c *exec.Cmd) {
		if err := c.Wait(); err != nil {
			log.Printf("go trace viewer exited: %v", err)
		}
		goTraceMu.Lock()
		if goTraceCmd == c {
			goTraceCmd = nil
		}
		goTraceMu.Unlock()
	}(cmd)

	if err := waitForViewerReady(url, 8*time.Second); err != nil {
		goTraceMu.Lock()
		if goTraceCmd == cmd && goTraceCmd.Process != nil {
			_ = goTraceCmd.Process.Kill()
			goTraceCmd = nil
		}
		goTraceMu.Unlock()
		return "", err
	}
	return url, nil
}

func reserveLoopbackAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr, nil
}

func waitForViewerReady(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
		}
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("go trace viewer did not start in time")
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSONErrorDetails(w http.ResponseWriter, status int, msg, parseErr, stderrTail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]string{"error": msg}
	if parseErr != "" {
		payload["parse_error"] = parseErr
	}
	if stderrTail != "" {
		payload["stderr_tail"] = stderrTail
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONErrorDetailsWithDebug(w http.ResponseWriter, status int, msg, parseErr, stderrTail string, debug map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]any{"error": msg}
	if parseErr != "" {
		payload["parse_error"] = parseErr
	}
	if stderrTail != "" {
		payload["stderr_tail"] = stderrTail
	}
	if len(debug) > 0 {
		payload["debug"] = debug
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func stderrTail(stderr string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.TrimSpace(strings.Join(lines, " | "))
}

func clearBuildCacheHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cacheDir := filepath.Join(os.TempDir(), "gtv-build")
	if err := os.RemoveAll(cacheDir); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "clear cache: "+err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func workloadHash(name string) string {
	genPath := filepath.Join("internal", "workload", name+"_gen.go")
	data, err := os.ReadFile(genPath)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ensureRunnerBinary(ctx context.Context, tagName, name string, useCache bool) (string, func(), error) {
	buildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cleanup := func() {}
	if useCache {
		cacheDir := filepath.Join(os.TempDir(), "gtv-build")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return "", cleanup, fmt.Errorf("runner cache: %w", err)
		}
		hash := workloadHash(name)
		short := ""
		if hash != "" {
			if len(hash) > 12 {
				short = hash[:12]
			} else {
				short = hash
			}
		}
		base := "gtv-runner-" + tagName
		if short != "" {
			base += "-" + short
		}
		if runtime.GOOS == "windows" {
			base += ".exe"
		}
		binPath := filepath.Join(cacheDir, base)
		stale := true
		if binInfo, err := os.Stat(binPath); err == nil {
			stale = false
			genPath := filepath.Join("internal", "workload", name+"_gen.go")
			if genInfo, err := os.Stat(genPath); err == nil {
				if genInfo.ModTime().After(binInfo.ModTime()) {
					stale = true
				}
			}
		}
		if !stale {
			return binPath, cleanup, nil
		}
		if err := buildRunner(buildCtx, tagName, binPath); err != nil {
			return "", cleanup, err
		}
		return binPath, cleanup, nil
	}
	binFile, err := os.CreateTemp("", "gtv-runner-*")
	if err != nil {
		return "", cleanup, fmt.Errorf("runner temp: %w", err)
	}
	binPath := binFile.Name()
	_ = binFile.Close()
	cleanup = func() { _ = os.Remove(binPath) }
	if err := buildRunner(buildCtx, tagName, binPath); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return binPath, cleanup, nil
}

func buildRunner(ctx context.Context, tagName, binPath string) error {
	buildCmd := exec.CommandContext(ctx, "go", "build", "-tags", tagName, "-o", binPath, "./cmd/gtv-runner")
	buildCmd.Dir = "."
	var buildErr strings.Builder
	buildCmd.Stderr = &buildErr
	if err := buildCmd.Run(); err != nil {
		msg := strings.TrimSpace(buildErr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("runner build: %s", msg)
	}
	return nil
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
	modeQ := q.Get("mode")
	timeoutMsQ := strings.TrimSpace(q.Get("timeout_ms"))
	timeoutQ := strings.TrimSpace(q.Get("timeout"))
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
	useMode := optMode
	if modeQ != "" {
		useMode = strings.ToLower(strings.TrimSpace(modeQ))
	}
	timeoutOverride := ""
	timeoutMs := int64(0)
	if timeoutMsQ != "" {
		if v, err := strconv.ParseInt(timeoutMsQ, 10, 64); err == nil {
			if v < 50 {
				v = 50
			}
			if v > 20000 {
				v = 20000
			}
			timeoutMs = v
		}
	}
	if timeoutMs > 0 {
		timeoutOverride = fmt.Sprintf("%dms", timeoutMs)
	} else if timeoutQ != "" {
		timeoutOverride = timeoutQ
	}
	timeoutInfo := timeoutOverride
	if timeoutInfo == "" {
		timeoutInfo = optTimeout
	}
	timeoutInfoMs := int64(0)
	if timeoutInfo != "" {
		if dur, ok := parseTimeoutDuration(timeoutInfo); ok {
			timeoutInfoMs = int64(dur / time.Millisecond)
		}
	}

	// Announce server options to client for HUD display
	_ = writeJSONWithDeadline(conn, struct {
		Type          string `json:"type"`
		Addr          string `json:"addr"`
		Synth         bool   `json:"synth"`
		DropBlockNoCh bool   `json:"drop_block_no_ch"`
		Workload      string `json:"workload"`
		MVP           bool   `json:"mvp"`
		Mode          string `json:"mode"`
		BCMode        string `json:"bc_mode,omitempty"`
		Timeout       string `json:"timeout,omitempty"`
		TimeoutMs     int64  `json:"timeout_ms,omitempty"`
	}{Type: "server_info", Addr: listenAddr, Synth: useSynth, DropBlockNoCh: useDrop, Workload: wl, BCMode: func() string {
		if bcModeQ != "" {
			return bcModeQ
		}
		return optBCMode
	}(), MVP: optMVP, Mode: useMode, Timeout: timeoutInfo, TimeoutMs: timeoutInfoMs})

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
		if err := runOnce(conn, wl, useSynth, useDrop, useMode, bcModeQ, timeoutOverride); err != nil {
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
func runOnce(conn *websocket.Conn, wl string, synth, drop bool, mode string, bcMode string, timeoutOverride string) error {
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
	timeoutStr := strings.TrimSpace(timeoutOverride)
	if timeoutStr == "" {
		timeoutStr = optTimeout
	}
	if timeoutStr == "" {
		timeoutStr = "10s"
	}
	timeoutDur, timeoutOK := parseTimeoutDuration(timeoutStr)
	timeoutMS := ""
	if timeoutOK {
		timeoutMS = strconv.FormatInt(int64(timeoutDur/time.Millisecond), 10)
	}
	cmd := exec.Command("go", "run", "-tags", tagName, "./cmd/gtv-runner", "-workload", wl, "-timeout", timeoutStr)
	env := os.Environ()
	if optMVP {
		env = append(env, "GTV_MVP=1")
	}
	if bcMode != "" {
		env = append(env, "GTV_BC_MODE="+strings.ToLower(bcMode))
	}
	if timeoutMS != "" {
		env = append(env, "GTV_TIMEOUT_MS="+timeoutMS)
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if timeoutOK {
		withTimeoutSignals(cmd, timeoutDur)
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
		err := streamTraceEvents(tee, cmd, runID, synth, drop, mode, optLiveLog, func(ev traceproc.TimelineEvent) error {
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

func streamTraceEvents(reader io.Reader, cmd *exec.Cmd, runID string, synth, drop bool, mode string, liveLog bool, emit func(traceproc.TimelineEvent) error, record func(traceproc.TimelineEvent) error) error {
	traceReader, err := xtrace.NewReader(reader)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	opts := traceproc.Options{SynthOnRecv: synth, DropBlockNoCh: drop, EmitAtomic: true, Mode: mode}
	st := traceproc.NewParseState(opts)
	ptrAliases := map[string]string{}
	nameByPtr := map[string]string{}
	nameByKey := map[string]string{}
	aliasSeq := 0
	normalizePointer := func(ptr string) string {
		v := strings.TrimSpace(ptr)
		if strings.HasPrefix(v, "ptr=") {
			v = strings.TrimSpace(strings.TrimPrefix(v, "ptr="))
		}
		return v
	}
	isPointerString := func(v string) bool {
		s := strings.ToLower(strings.TrimSpace(v))
		return strings.HasPrefix(s, "0x")
	}
	isAliasName := func(v string) bool {
		s := strings.ToLower(strings.TrimSpace(v))
		return strings.HasPrefix(s, "ch#") || strings.HasPrefix(s, "unknown:")
	}
	isHumanName := func(v string) bool {
		s := strings.TrimSpace(v)
		if s == "" {
			return false
		}
		return !isPointerString(s) && !isAliasName(s)
	}
	aliasForPtr := func(ptr string) string {
		key := normalizePointer(ptr)
		if key == "" || !isPointerString(key) {
			return ""
		}
		if alias, ok := ptrAliases[key]; ok {
			return alias
		}
		aliasSeq++
		alias := fmt.Sprintf("ch#%d", aliasSeq)
		ptrAliases[key] = alias
		return alias
	}
	normalizeLiveEvent := func(ev traceproc.TimelineEvent) traceproc.TimelineEvent {
		ev.Channel = strings.TrimSpace(ev.Channel)
		ev.ChannelKey = strings.TrimSpace(ev.ChannelKey)
		ev.ChPtr = normalizePointer(ev.ChPtr)
		if ev.ChPtr == "" {
			if maybePtr := normalizePointer(ev.ChannelKey); isPointerString(maybePtr) {
				ev.ChPtr = maybePtr
			}
		}
		if isHumanName(ev.Channel) {
			if ev.ChPtr != "" {
				if _, ok := nameByPtr[ev.ChPtr]; !ok {
					nameByPtr[ev.ChPtr] = ev.Channel
				}
			}
			if ev.ChannelKey != "" {
				nameByKey[ev.ChannelKey] = ev.Channel
			}
		}
		if ev.Channel == "" || isPointerString(ev.Channel) || isAliasName(ev.Channel) {
			if ev.ChPtr != "" {
				if mapped := strings.TrimSpace(nameByPtr[ev.ChPtr]); mapped != "" {
					ev.Channel = mapped
				}
			}
			if (ev.Channel == "" || isPointerString(ev.Channel) || isAliasName(ev.Channel)) && ev.ChannelKey != "" {
				if mapped := strings.TrimSpace(nameByKey[ev.ChannelKey]); mapped != "" {
					ev.Channel = mapped
				}
			}
		}
		if ev.ChPtr != "" {
			if alias := aliasForPtr(ev.ChPtr); alias != "" {
				if ev.ChannelKey == "" || isPointerString(ev.ChannelKey) {
					ev.ChannelKey = alias
				}
				if ev.Channel == "" || isPointerString(ev.Channel) || isAliasName(ev.Channel) {
					if mapped := strings.TrimSpace(nameByPtr[ev.ChPtr]); mapped != "" {
						ev.Channel = mapped
					} else {
						ev.Channel = alias
					}
				}
				if isHumanName(ev.Channel) {
					nameByKey[alias] = ev.Channel
				}
			}
		}
		if ev.ChannelKey != "" {
			nameByKey[ev.ChannelKey] = strings.TrimSpace(firstNonEmpty(nameByKey[ev.ChannelKey], ev.Channel))
		}
		return ev
	}
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
		if updated, keep, _ := traceproc.FilterTimelineEventForMode(ev, opts.Mode); !keep {
			return nil
		} else {
			ev = updated
		}
		ev = normalizeLiveEvent(ev)
		ev = traceproc.AnnotateUnknownEndpoints(ev)
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
				_ = traceproc.EmitAuditSummary(st, emitWithSeq, "live")
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
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
