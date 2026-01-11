package traceproc

import "fmt"

// NormalizeTimeline normalizes channel identities and removes exact duplicates.
func NormalizeTimeline(in []TimelineEvent) []TimelineEvent {
	normalized := make([]TimelineEvent, 0, len(in))
	for _, ev := range in {
		if ev.ChannelKey != "" {
			ev.ChannelKey = normalizeChannelKey(ev.ChannelKey)
		}
		if ev.ChPtr != "" {
			ev.ChPtr = normalizePointer(ev.ChPtr)
		}
		if ev.ChannelKey == "" {
			if key := normalizeChannelKey(ev.Channel); key != "" {
				ev.ChannelKey = key
			}
		}
		if ev.ChPtr == "" {
			if key := normalizePointer(ev.ChannelKey); key != "" && isPointerString(key) {
				ev.ChPtr = key
			}
		}
		normalized = append(normalized, ev)
	}
	return DedupTimeline(normalized)
}

// DedupTimeline removes exact duplicate parser emissions while preserving order.
// Key: time_ns (or derived from time_ms) + g + channel_key/ch_ptr + event + attempt_id
func DedupTimeline(in []TimelineEvent) []TimelineEvent {
	seen := make(map[string]struct{}, len(in))
	out := make([]TimelineEvent, 0, len(in))
	for _, ev := range in {
		tns := ev.TimeNs
		if tns == 0 {
			tns = int64(ev.TimeMs*1e6 + 0.5)
		}
		att := ev.AttemptID
		if att == "" && (ev.Event == "chan_send_attempt" || ev.Event == "chan_recv_attempt") {
			att = ev.ID
		}
		identity := channelIdentity(ev)
		key := fmt.Sprintf("%d|%d|%s|%s|%s", tns, ev.G, identity, ev.Event, att)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ev)
	}
	return out
}
