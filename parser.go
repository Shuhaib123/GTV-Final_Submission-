package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	xtrace "golang.org/x/exp/trace"
	"jspt/internal/traceproc"
)

type DebugLog struct {
	TimeMs   float64 `json:"time_ms"`
	G        int64   `json:"g"`
	Category string  `json:"category"`
	Message  string  `json:"message"`
}

// WritePingPongTimelineJSON parses a trace at tracePath and writes a
// summarized goroutine timeline to jsonPath in JSON format.
// This reuses the shared event processor used by live streaming.
func WritePingPongTimelineJSON(tracePath, jsonPath string) error {
	f, err := os.Open(tracePath)
	if err != nil {
		return err
	}
	defer f.Close()

	r, err := xtrace.NewReader(f)
	if err != nil {
		return err
	}

	// Use shared processor without live synthesis; allow DropBlockNoCh via env.
	opts := traceproc.Options{SynthOnRecv: false, DropBlockNoCh: os.Getenv("GTV_DROP_BLOCK_NO_CH") == "1"}
	st := traceproc.NewParseState(opts)
	var timeline []traceproc.TimelineEvent
	emit := func(ev traceproc.TimelineEvent) error { timeline = append(timeline, ev); return nil }

	// Collect raw logs for a companion file.
	var logs []DebugLog

	for {
		e, err := r.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if e.Kind() == xtrace.EventLog {
			l := e.Log()
			logs = append(logs, DebugLog{TimeMs: float64(e.Time()) / 1e6, G: int64(e.Goroutine()), Category: l.Category, Message: l.Message})
		}
		if err := traceproc.ProcessEvent(&e, st, emit); err != nil {
			return err
		}
	}

	// Sort by time for a consistent output
	sort.Slice(timeline, func(i, j int) bool { return timeline[i].TimeMs < timeline[j].TimeMs })

	// Optional synthesis (matches live env behavior if desired)
	if os.Getenv("GTV_SYNTH_SEND") == "1" {
		type skey struct{ Ev, Ch string }
		hasSend := make(map[skey]bool)
		for _, e := range timeline {
			if strings.HasSuffix(e.Event, "_send") && e.Channel != "" {
				hasSend[skey{Ev: e.Event, Ch: strings.ToLower(e.Channel)}] = true
			}
		}
		// Choose a sender heuristically
		findSenderG := func(ch string, t float64) int64 {
			var cand int64
			for i := range timeline {
				e := timeline[i]
				if e.Event == "blocked_send" && e.TimeMs <= t {
					cand = e.G
				}
			}
			if cand != 0 {
				return cand
			}
			for i := range timeline {
				if timeline[i].Role == "worker" {
					return timeline[i].G
				}
			}
			for i := range timeline {
				if timeline[i].Role == "main" {
					return timeline[i].G
				}
			}
			return 0
		}
		var synth []traceproc.TimelineEvent
		for i := range timeline {
			e := timeline[i]
			if !strings.HasSuffix(e.Event, "_recv") || e.Channel == "" {
				continue
			}
			ch := strings.ToLower(e.Channel)
			sendEv := strings.TrimSuffix(e.Event, "_recv") + "_send"
			if hasSend[skey{Ev: sendEv, Ch: ch}] {
				continue
			}
			g := findSenderG(e.Channel, e.TimeMs)
			t := e.TimeMs - 0.001
			synth = append(synth, traceproc.TimelineEvent{TimeMs: t, Event: sendEv, Channel: e.Channel, G: g, Role: "worker", Source: "synth"})
			hasSend[skey{Ev: sendEv, Ch: ch}] = true
		}
		if len(synth) > 0 {
			timeline = append(timeline, synth...)
			sort.Slice(timeline, func(i, j int) bool { return timeline[i].TimeMs < timeline[j].TimeMs })
		}
	}

	// Deduplicate exact duplicates for stability
	timeline = dedupTimeline(timeline)
	if err := writeJSONAtomic(jsonPath, timeline); err != nil {
		return err
	}

	// Optionally write raw logs to a sibling file for inspection.
	// Enable via GTV_WRITE_LOGS=1 (default: off)
	if v := strings.ToLower(os.Getenv("GTV_WRITE_LOGS")); v == "1" || v == "true" || v == "yes" {
		logsPath := filepath.Join(filepath.Dir(jsonPath), strings.TrimSuffix(filepath.Base(jsonPath), filepath.Ext(jsonPath))+".logs.json")
		if lf, err := os.Create(logsPath); err == nil {
			_ = json.NewEncoder(lf).Encode(logs)
			_ = lf.Close()
		}
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

// writeJSONAtomic writes JSON to a temp file then renames atomically to avoid truncation.
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
