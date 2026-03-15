package traceproc

import (
	"fmt"
	"sort"
	"strings"
)

type unmatchedAudit struct {
	byChannel []ChannelUnmatchedAudit
	totals    UnmatchedTotals
	warnings  []string
}

// ComputeUnmatchedAuditFromEvents computes unmatched per-channel counts from a final event list.
func ComputeUnmatchedAuditFromEvents(events []TimelineEvent) ([]ChannelUnmatchedAudit, UnmatchedTotals, []string) {
	acc := make(map[string]*ChannelUnmatchedAudit)
	sendByKey := make(map[string]string)
	recvByKey := make(map[string]string)
	totals := UnmatchedTotals{}
	var warnings []string

	for _, ev := range events {
		if !isPairedChannelEvent(ev) {
			continue
		}
		chID := channelIdentityForAudit(ev)
		if chID == "" {
			totals.UnknownChannelID++
		}
		key := msgKeyForAudit(ev)
		if key == "" {
			// Paired events without an id are expected for select-bridged recvs.
			// Avoid noisy warnings in audit output.
			if !strings.EqualFold(ev.Source, "paired") {
				warnings = append(warnings, fmt.Sprintf("missing pairing id for %s at t=%.3f", ev.Event, ev.TimeMs))
			}
			continue
		}
		if ev.Event == "chan_send" {
			sendByKey[key] = chID
		} else {
			recvByKey[key] = chID
		}
	}

	for key, ch := range sendByKey {
		if ch == "" {
			totals.Sends++
			continue
		}
		entry := acc[ch]
		if entry == nil {
			entry = &ChannelUnmatchedAudit{ChanID: ch}
			acc[ch] = entry
		}
		if recvByKey[key] != "" {
			entry.MatchedPairs++
		} else {
			entry.UnmatchedSends++
			totals.Sends++
		}
	}
	for key, ch := range recvByKey {
		if _, ok := sendByKey[key]; ok {
			continue
		}
		if ch == "" {
			totals.Recvs++
			continue
		}
		entry := acc[ch]
		if entry == nil {
			entry = &ChannelUnmatchedAudit{ChanID: ch}
			acc[ch] = entry
		}
		entry.UnmatchedRecvs++
		totals.Recvs++
	}

	keys := make([]string, 0, len(acc))
	for ch := range acc {
		keys = append(keys, ch)
	}
	sort.Strings(keys)
	ordered := make([]ChannelUnmatchedAudit, 0, len(keys))
	for _, ch := range keys {
		ordered = append(ordered, *acc[ch])
	}
	return ordered, totals, warnings
}

// MergeUnmatchedAuditIntoSummary updates the latest audit_summary event with unmatched stats.
func MergeUnmatchedAuditIntoSummary(events []TimelineEvent, by []ChannelUnmatchedAudit, totals UnmatchedTotals, warnings []string) []TimelineEvent {
	if len(by) == 0 && totals.Sends == 0 && totals.Recvs == 0 && totals.UnknownChannelID == 0 {
		return events
	}
	payload := map[string]any{
		"unmatched_by_channel": by,
		"unmatched_totals":     totals,
	}
	if len(warnings) > 0 {
		payload["warnings"] = warnings
	}

	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Event != "audit_summary" {
			continue
		}
		switch v := events[i].Value.(type) {
		case map[string]any:
			for k, val := range payload {
				v[k] = val
			}
			events[i].Value = v
		case map[string]int64:
			out := make(map[string]any, len(v)+len(payload))
			for k, val := range v {
				out[k] = val
			}
			for k, val := range payload {
				out[k] = val
			}
			events[i].Value = out
		default:
			out := make(map[string]any, len(payload)+1)
			out["summary"] = v
			for k, val := range payload {
				out[k] = val
			}
			events[i].Value = out
		}
		return events
	}

	ts := 0.0
	if len(events) > 0 {
		ts = events[len(events)-1].TimeMs
	}
	events = append(events, TimelineEvent{
		TimeMs: ts,
		Event:  "audit_summary",
		Source: "audit",
		Value:  payload,
	})
	return events
}

func isPairedChannelEvent(ev TimelineEvent) bool {
	if ev.Event != "chan_send" && ev.Event != "chan_recv" {
		return false
	}
	if strings.EqualFold(ev.Source, "paired") {
		return true
	}
	// Accept attempt/pair/message ids as evidence of pairing keys in live traces.
	if ev.AttemptID != "" || ev.PairID != "" || ev.MsgID != 0 {
		return true
	}
	return false
}

func channelIdentityForAudit(ev TimelineEvent) string {
	if ev.ChannelKey != "" {
		return ev.ChannelKey
	}
	return ev.Channel
}

func msgKeyForAudit(ev TimelineEvent) string {
	if ev.MsgID != 0 {
		return fmt.Sprintf("msg:%d", ev.MsgID)
	}
	if ev.PairID != "" {
		return "pair:" + ev.PairID
	}
	return ""
}
