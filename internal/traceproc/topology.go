package traceproc

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateTopology asserts invariants about pointer-based channel identities
// and send/receive events. It ensures every send/recv event has a pointer, and
// that each pointer always maps to the same normalized identity.
func ValidateTopology(events []TimelineEvent) error {
	if len(events) == 0 {
		return nil
	}
	v := topologyValidator{
		pointerToIdentity: make(map[string]string),
	}
	var errs []string
	for idx, ev := range events {
		if err := v.observe(ev, idx); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("topology validation failed:\n%s", strings.Join(errs, "\n"))
}

type topologyValidator struct {
	pointerToIdentity map[string]string
}

func (v *topologyValidator) observe(ev TimelineEvent, idx int) error {
	ptr := pointerFromEvent(ev)
	identity := channelIdentity(ev)
	if ptr != "" {
		if identity == "" {
			return fmt.Errorf("event[%d] %q has pointer %q but no normalized identity", idx, ev.Event, ptr)
		}
		if identity != ptr {
			return fmt.Errorf("event[%d] %q pointer %q mismatches identity %q", idx, ev.Event, ptr, identity)
		}
		if prev, ok := v.pointerToIdentity[ptr]; ok && prev != identity {
			return fmt.Errorf("pointer %q mapped to %q and %q", ptr, prev, identity)
		}
		v.pointerToIdentity[ptr] = identity
	}
	if kind := eventKind(ev.Event); kind != "" && ptr == "" {
		return fmt.Errorf("event[%d] %q (%s) missing pointer identity (channel=%q channel_key=%q)", idx, ev.Event, kind, ev.Channel, ev.ChannelKey)
	}
	return nil
}

func pointerFromEvent(ev TimelineEvent) string {
	if ptr := normalizePointer(ev.ChPtr); ptr != "" {
		return ptr
	}
	if key := normalizeChannelKey(ev.ChannelKey); key != "" && isPointerString(key) {
		return key
	}
	return ""
}

func channelIdentity(ev TimelineEvent) string {
	if ptr := normalizePointer(ev.ChPtr); ptr != "" {
		return ptr
	}
	if key := normalizeChannelKey(ev.ChannelKey); key != "" {
		return key
	}
	if key := normalizeChannelKey(ev.Channel); key != "" {
		return key
	}
	return ""
}

func normalizeChannelKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	return stripGoroutineSuffix(key)
}

func stripGoroutineSuffix(key string) string {
	if key == "" {
		return ""
	}
	if idx := strings.Index(key, "#g"); idx >= 0 && idx+2 < len(key) {
		if _, err := strconv.Atoi(key[idx+2:]); err == nil {
			return key[:idx]
		}
	}
	return key
}

func normalizePointer(ptr string) string {
	ptr = strings.TrimSpace(ptr)
	if ptr == "" {
		return ""
	}
	if strings.HasPrefix(ptr, "ptr=") {
		ptr = strings.TrimSpace(ptr[4:])
	}
	return ptr
}

func isPointerString(val string) bool {
	if val == "" {
		return false
	}
	v := normalizePointer(val)
	if v == "" {
		return false
	}
	return strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X")
}

func eventKind(name string) string {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "blocked_") {
		return ""
	}
	if strings.Contains(name, "send") && !strings.Contains(name, "recv") && !strings.Contains(name, "receive") {
		return "send"
	}
	if strings.Contains(name, "recv") || strings.Contains(name, "receive") {
		return "recv"
	}
	if strings.Contains(name, "send") {
		return "send"
	}
	return ""
}
