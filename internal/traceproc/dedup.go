package traceproc

import "fmt"

const dedupDropWarnRate = 0.01

// DedupAudit captures counters from deduplicating timeline events.
type DedupAudit struct {
	Total                    int     `json:"total"`
	Dropped                  int     `json:"dropped"`
	DroppedMissingIdentity   int     `json:"dropped_missing_identity"`
	DroppedMissingChPtr      int     `json:"dropped_missing_ch_ptr"`
	DroppedMissingChannelKey int     `json:"dropped_missing_channel_key"`
	DroppedMissingChannel    int     `json:"dropped_missing_channel"`
	DroppedWithIdentity      int     `json:"dropped_with_identity"`
	MissingIdentity          int     `json:"missing_identity"`
	MissingChPtr             int     `json:"missing_ch_ptr"`
	MissingChannelKey        int     `json:"missing_channel_key"`
	MissingChannel           int     `json:"missing_channel"`
	DropRate                 float64 `json:"drop_rate"`
}

// AuditSummary captures diagnostic information emitted alongside the timeline.
type AuditSummary struct {
	Dedup    DedupAudit `json:"dedup"`
	Warnings []string   `json:"warnings,omitempty"`
}

// DedupTimeline removes exact duplicate parser emissions while preserving order.
// Key: time_ns (or derived from time_ms) + g + channel identity + event + attempt_id.
func DedupTimeline(in []TimelineEvent) ([]TimelineEvent, DedupAudit) {
	audit := DedupAudit{Total: len(in)}
	seen := make(map[string]struct{}, len(in))
	out := make([]TimelineEvent, 0, len(in))
	for _, ev := range in {
		chPtr := normalizePointer(ev.ChPtr)
		chKey := normalizeChannelKey(ev.ChannelKey)
		chName := normalizeChannelKey(ev.Channel)
		if chPtr == "" {
			audit.MissingChPtr++
		}
		if chKey == "" {
			audit.MissingChannelKey++
		}
		if chName == "" {
			audit.MissingChannel++
		}
		identity := dedupIdentity(chPtr, chKey, chName)
		if identity == "" {
			audit.MissingIdentity++
		}
		tns := ev.TimeNs
		if tns == 0 {
			tns = int64(ev.TimeMs*1e6 + 0.5)
		}
		att := ev.AttemptID
		if att == "" && (ev.Event == "chan_send_attempt" || ev.Event == "chan_recv_attempt") {
			att = ev.ID
		}
		keyIdentity := identity
		if keyIdentity == "" {
			keyIdentity = fmt.Sprintf("unknown:%d:%d", ev.G, ev.Seq)
		}
		key := fmt.Sprintf("%d|%d|%s|%s|%s", tns, ev.G, keyIdentity, ev.Event, att)
		if _, ok := seen[key]; ok {
			audit.Dropped++
			if identity == "" {
				audit.DroppedMissingIdentity++
			}
			if chPtr == "" {
				audit.DroppedMissingChPtr++
			}
			if chKey == "" {
				audit.DroppedMissingChannelKey++
			}
			if chName == "" {
				audit.DroppedMissingChannel++
			}
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ev)
	}
	audit.DroppedWithIdentity = audit.Dropped - audit.DroppedMissingIdentity
	if audit.Total > 0 {
		audit.DropRate = float64(audit.Dropped) / float64(audit.Total)
	}
	return out, audit
}

// AppendAuditSummary appends an audit_summary event with dedup counters.
func AppendAuditSummary(events []TimelineEvent, audit DedupAudit) []TimelineEvent {
	if audit.Total == 0 {
		return events
	}
	summary := AuditSummary{Dedup: audit}
	if audit.DropRate >= dedupDropWarnRate && audit.Dropped > 0 {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("dedup drop rate %.2f%% exceeds %.2f%% threshold", audit.DropRate*100, dedupDropWarnRate*100))
	}
	ts := 0.0
	if len(events) > 0 {
		ts = events[len(events)-1].TimeMs
	}
	events = append(events, TimelineEvent{
		TimeMs: ts,
		Event:  "audit_summary",
		Role:   "system",
		Source: "audit",
		Value:  summary,
	})
	return events
}

func dedupIdentity(chPtr, chKey, chName string) string {
	if chPtr != "" {
		return chPtr
	}
	if chKey != "" {
		return chKey
	}
	return chName
}
