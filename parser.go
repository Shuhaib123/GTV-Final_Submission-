package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	xtrace "golang.org/x/exp/trace"
	"jspt/internal/traceproc"
)

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
	opts := traceproc.Options{
		SynthOnRecv:     false,
		DropBlockNoCh:   os.Getenv("GTV_DROP_BLOCK_NO_CH") == "1",
		Mode:           strings.ToLower(strings.TrimSpace(os.Getenv("GTV_MODE"))),
		GoroutineFilter: traceproc.ParseGoroutineFilterMode(os.Getenv("GTV_FILTER_GOROUTINES")),
	}
	st := traceproc.NewParseState(opts)
	var timeline []traceproc.TimelineEvent
	emit := func(ev traceproc.TimelineEvent) error { timeline = append(timeline, ev); return nil }

	for {
		e, err := r.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := traceproc.ProcessEvent(&e, st, emit); err != nil {
			return err
		}
	}
	if err := traceproc.EmitAuditSummary(st, emit, "offline"); err != nil {
		return err
	}

	// Sort by time_ns (then seq/g) for a consistent, stable output
	sort.Slice(timeline, func(i, j int) bool {
		a := timeline[i]
		b := timeline[j]
		ta := a.TimeNs
		tb := b.TimeNs
		if ta == 0 {
			ta = int64(a.TimeMs*1e6 + 0.5)
		}
		if tb == 0 {
			tb = int64(b.TimeMs*1e6 + 0.5)
		}
		if ta != tb {
			return ta < tb
		}
		if a.Seq != b.Seq {
			return a.Seq < b.Seq
		}
		if a.G != b.G {
			return a.G < b.G
		}
		return a.Event < b.Event
	})

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
	var audit traceproc.DedupAudit
	timeline, audit = traceproc.DedupTimeline(timeline)
	timeline = traceproc.AppendAuditSummary(timeline, audit)
	payload := traceproc.NormalizeTimeline(timeline, st)
	unmatchedByCh, unmatchedTotals, auditWarnings := traceproc.ComputeUnmatchedAuditFromEvents(payload.Events)
	payload.Events = traceproc.MergeUnmatchedAuditIntoSummary(payload.Events, unmatchedByCh, unmatchedTotals, auditWarnings)
	if len(auditWarnings) > 0 {
		for _, w := range auditWarnings {
			payload.WarningsV2 = append(payload.WarningsV2, traceproc.WarningEntry{Code: "audit_warning", Message: w})
		}
	}
	if unmatchedTotals.UnknownChannelID > 0 {
		payload.WarningsV2 = append(payload.WarningsV2, traceproc.WarningEntry{Code: "audit_unknown_channel_id", Message: "audit: events with unknown channel identity detected"})
	}
	if err := traceproc.ValidateTopology(payload.Events); err != nil {
		payload.WarningsV2 = append(payload.WarningsV2, traceproc.WarningEntry{Code: "validation", Message: err.Error()})
	}
	if err := writeJSONAtomic(jsonPath, payload); err != nil {
		return err
	}

	return nil
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
