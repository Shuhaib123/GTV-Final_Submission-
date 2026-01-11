package traceproc

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	xtrace "golang.org/x/exp/trace"
)

var skipPairingChannels map[string]bool

func init() {
	if skip := strings.TrimSpace(os.Getenv("GTV_SKIP_PAIRING_CHANNELS")); skip != "" {
		skipPairingChannels = make(map[string]bool)
		for _, part := range strings.Split(skip, ",") {
			if p := strings.TrimSpace(strings.ToLower(part)); p != "" {
				skipPairingChannels[p] = true
			}
		}
	}
}

// TimelineEvent is the canonical event shape used by visualizers.
type TimelineEvent struct {
	TimeMs     float64 `json:"time_ms"`
	TimeNs     int64   `json:"time_ns,omitempty"`
	Event      string  `json:"event"`
	G          int64   `json:"g"`
	Func       string  `json:"func,omitempty"`
	Channel    string  `json:"channel,omitempty"`
	ChannelKey string  `json:"channel_key,omitempty"`
	Role       string  `json:"role,omitempty"`
	Value      any     `json:"value,omitempty"`
	Source     string  `json:"source,omitempty"`
	Seq        int64   `json:"seq,omitempty"`

	// Optional atomic event fields (non-breaking for existing viewers)
	ID         string  `json:"id,omitempty"`
	AttemptID  string  `json:"attempt_id,omitempty"`
	PeerG      int64   `json:"peer_g,omitempty"`
	PairID     string  `json:"pair_id,omitempty"`
	DeltaMs    float64 `json:"delta_ms,omitempty"`
	MsgID      int64   `json:"msg_id,omitempty"`
	Reason     string  `json:"reason,omitempty"`
	ParentG    int64   `json:"parent_g,omitempty"`
	Cap        int     `json:"cap,omitempty"`
	ElemType   string  `json:"elem_type,omitempty"`
	ChPtr      string  `json:"ch_ptr,omitempty"`
	MissingPtr bool    `json:"missing_ptr,omitempty"`
	// Optional select fields
	SelectID string `json:"select_id,omitempty"`
	Dropped  bool   `json:"select_dropped,omitempty"`
}

const TimelineSchemaVersion = 1

type TimelineEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	Events        []TimelineEvent `json:"events"`
	Warnings      []string        `json:"warnings,omitempty"`
}

type DedupAudit struct {
	Duplicates int
}

// Options control live/offline behavior.
type Options struct {
	// If true, emit a synthetic *_send just before a *_recv when no prior send was observed.
	SynthOnRecv bool
	// If true, drop blocked_send/blocked_receive that lack a channel label.
	DropBlockNoCh bool
	// If true, emit atomic attempt/commit and block/unblock events in addition to legacy events.
	EmitAtomic bool
}

// ParseState holds per-connection parsing state.
type op struct{ kind, ch, ptr string }
type skey struct{ Ev, Ch string }
type OpKey string

type ParseState struct {
	Roles      map[int64]string
	Interested map[int64]struct{}
	Active     map[int64]op
	Last       map[int64]op

	// Synthesis bookkeeping
	hasSend     map[skey]bool
	lastMainG   int64
	lastWorkerG int64
	lastServerG int64
	lastClientG int64

	Opt Options

	// Region pairing (send → recv) bookkeeping by channel key
	sendsByKey map[OpKey][]sendRecord

	// Value notes captured from trace.Log(ctx, "value", ...)
	lastValue map[int64]string

	// Atomic events bookkeeping
	nextID        int64
	activeAttempt map[int64]string // gid -> attempt id
	attemptsByG   map[int64]struct {
		id, ch, kind, ptr string
		time              float64
	}
	// attempt id -> info (for pairing)
	attByID map[string]struct {
		G    int64
		Ch   chKey
		Kind string
		Time float64
		Ptr  string
	}
	// FIFO queues per channel for attempt pairing
	sndAttQ map[OpKey][]string // queue of send attempt ids by channel
	rcvAttQ map[OpKey][]string // queue of recv attempt ids by channel
	// Track current blocked reason/channel for a goroutine (best-effort)
	blocked map[int64]struct{ Reason, Ch string }
	// Spawn relationships by SID from instrumentation logs
	spawnParentBySID map[string]int64

	// Select accounting
	activeSelect   map[int64]string    // gid -> select id
	selCandidates  map[string][]string // select id -> attempt ids
	selChosen      map[string]string   // select id -> chosen attempt id
	selectFallback map[int64]string    // gid -> select id tracked for fallback

	// Channel identity + buffering
	// Channel identity by pointer (if instrumented via logs); keyed by logical channel key
	lastChPtrByG     map[int64]string
	lastChPtrSeqByG  map[int64]int64
	lastChNameByG    map[int64]string
	lastChNameSeqByG map[int64]int64
	channelNameByPtr map[string]string
	eventIndexByG    map[int64]int64
	capByPtr         map[string]int // channel capacity by pointer
	depthByPtr       map[string]int // buffered depth by pointer (0..cap)
	// Set of buffered sends: ptr -> set(attemptID)
	bufSends map[string]map[string]bool

	// Message pairing id counter
	nextMsgID int64
}

// NewParseState creates a fresh parser state.
func NewParseState(opt Options) *ParseState {
	return &ParseState{
		Roles:            make(map[int64]string),
		Interested:       make(map[int64]struct{}),
		Active:           make(map[int64]op),
		Last:             make(map[int64]op),
		hasSend:          make(map[skey]bool),
		Opt:              opt,
		sendsByKey:       make(map[OpKey][]sendRecord),
		lastChPtrByG:     make(map[int64]string),
		lastChPtrSeqByG:  make(map[int64]int64),
		lastChNameByG:    make(map[int64]string),
		lastChNameSeqByG: make(map[int64]int64),
		eventIndexByG:    make(map[int64]int64),
		lastValue:        make(map[int64]string),
		activeAttempt:    make(map[int64]string),
		attemptsByG: make(map[int64]struct {
			id, ch, kind, ptr string
			time              float64
		}),
		attByID: make(map[string]struct {
			G    int64
			Ch   chKey
			Kind string
			Time float64
			Ptr  string
		}),
		sndAttQ:          make(map[OpKey][]string),
		rcvAttQ:          make(map[OpKey][]string),
		blocked:          make(map[int64]struct{ Reason, Ch string }),
		activeSelect:     make(map[int64]string),
		selCandidates:    make(map[string][]string),
		selChosen:        make(map[string]string),
		selectFallback:   make(map[int64]string),
		spawnParentBySID: make(map[string]int64),
		capByPtr:         make(map[string]int),
		depthByPtr:       make(map[string]int),
		channelNameByPtr: make(map[string]string),
		bufSends:         make(map[string]map[string]bool),
	}
}

func (st *ParseState) newID(prefix string) string {
	id := fmt.Sprintf("%s%d", prefix, st.nextID)
	st.nextID++
	return id
}

func (st *ParseState) channelKey(ck chKey, ptr string) string {
	if ptr != "" {
		return ptr
	}
	base := ck.Name
	if ck.ClientID >= 0 {
		base = fmt.Sprintf("%s[%d]", ck.Name, ck.ClientID)
	}
	return base
}

func (st *ParseState) opKey(ck chKey, ptr string) OpKey {
	return OpKey(st.channelKey(ck, ptr))
}

func (st *ParseState) consumeFreshChPtr(gid int64) string {
	ptr, ok := st.lastChPtrByG[gid]
	if !ok {
		return ""
	}
	seq := st.lastChPtrSeqByG[gid]
	if seq == 0 {
		delete(st.lastChPtrByG, gid)
		delete(st.lastChPtrSeqByG, gid)
		return ptr
	}
	if st.eventIndexByG[gid]-seq <= 1 {
		delete(st.lastChPtrByG, gid)
		delete(st.lastChPtrSeqByG, gid)
		return ptr
	}
	delete(st.lastChPtrByG, gid)
	delete(st.lastChPtrSeqByG, gid)
	return ""
}

func (st *ParseState) consumeFreshChName(gid int64) string {
	name, ok := st.lastChNameByG[gid]
	if !ok {
		return ""
	}
	seq := st.lastChNameSeqByG[gid]
	if seq == 0 {
		delete(st.lastChNameByG, gid)
		delete(st.lastChNameSeqByG, gid)
		return name
	}
	if st.eventIndexByG[gid]-seq <= 1 {
		delete(st.lastChNameByG, gid)
		delete(st.lastChNameSeqByG, gid)
		return name
	}
	delete(st.lastChNameByG, gid)
	delete(st.lastChNameSeqByG, gid)
	return ""
}

func (st *ParseState) bumpEventIndex(gid int64) int64 {
	st.eventIndexByG[gid]++
	return st.eventIndexByG[gid]
}

// parseRegionOp tries to extract a channel operation from a region type string.
// Recognizes patterns like:
//
//	"<role>: send to <channel>"
//	"<role>: receive from <channel>"
//
// Also works with variations like "send 1 to ping".
func parseRegionOp(typ string) (kind, ch string, ok bool) {
	lt := strings.ToLower(typ)
	// Prefer the more specific phrasing first.
	if strings.Contains(lt, "receive") {
		if i := strings.LastIndex(lt, " from "); i >= 0 {
			rest := strings.TrimSpace(lt[i+6:])
			// Channel is the next token; keep bracketed suffixes like clientout[1]
			if rest != "" {
				ch = strings.Fields(rest)[0]
				return "recv", ch, true
			}
		}
		// Fallback: "receive <channel>"
		if i := strings.LastIndex(lt, "receive "); i >= 0 {
			rest := strings.TrimSpace(lt[i+8:])
			if rest != "" {
				ch = strings.Fields(rest)[0]
				return "recv", ch, true
			}
		}
	}
	if strings.Contains(lt, "send") {
		if i := strings.LastIndex(lt, " to "); i >= 0 {
			rest := strings.TrimSpace(lt[i+4:])
			if rest != "" {
				ch = strings.Fields(rest)[0]
				return "send", ch, true
			}
		}
		// Fallback: "send <channel>"
		if i := strings.LastIndex(lt, "send "); i >= 0 {
			rest := strings.TrimSpace(lt[i+5:])
			if rest != "" {
				ch = strings.Fields(rest)[0]
				return "send", ch, true
			}
		}
	}
	return "", "", false
}

func parseChNameMessage(msg string) (ptr, name string) {
	for _, part := range strings.Fields(msg) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "ptr=") {
			ptr = strings.TrimPrefix(part, "ptr=")
			continue
		}
		if strings.HasPrefix(part, "name=") {
			name = strings.TrimPrefix(part, "name=")
			continue
		}
	}
	return ptr, name
}

// ProcessEvent translates a trace Event into zero or more TimelineEvents via emit.
func ProcessEvent(e *xtrace.Event, st *ParseState, emit func(TimelineEvent) error) error {
	tsMs := float64(e.Time()) / 1e6
	tsNs := int64(e.Time())
	gid := int64(e.Goroutine())
	st.bumpEventIndex(gid)
	// Ensure every emitted event carries a stable nanosecond timestamp for dedup.
	baseEmit := emit
	emit = func(ev TimelineEvent) error {
		if ev.TimeNs == 0 {
			ev.TimeNs = tsNs
		}
		if ev.Seq == 0 {
			ev.Seq = st.eventIndexByG[gid]
		}
		if ev.ChPtr != "" {
			ev.MissingPtr = false
		} else if !ev.MissingPtr {
			ev.MissingPtr = true
		}
		return baseEmit(ev)
	}
	switch e.Kind() {
	case xtrace.EventLog:
		l := e.Log()
		// Channel pointer identity logs from instrumenter: category "ch_ptr", message like "ptr=0xc000014120".
		// Some instrumentation runs may not preserve the category; accept messages that contain "ptr=" as a fallback.
		if l.Category == "ch_ptr" {
			msg := strings.TrimSpace(l.Message)
			ptr := ""
			if i := strings.Index(msg, "ptr="); i >= 0 {
				ptr = strings.TrimSpace(msg[i+4:])
				if j := strings.IndexAny(ptr, " \t\n"); j >= 0 {
					ptr = ptr[:j]
				}
			} else {
				ptr = msg
			}
			if ptr != "" {
				st.lastChPtrByG[gid] = ptr
				st.lastChPtrSeqByG[gid] = st.eventIndexByG[gid]
				_ = emit(TimelineEvent{TimeMs: tsMs, Event: "ch_ptr", G: gid, Role: st.Roles[gid], ChannelKey: ptr, ChPtr: ptr, Source: "log"})
			}
			return nil
		}
		if l.Category == "ch_name" {
			msg := strings.TrimSpace(l.Message)
			if msg != "" {
				ptr, name := parseChNameMessage(msg)
				if name == "" {
					name = msg
				}
				if name != "" {
					st.lastChNameByG[gid] = name
					st.lastChNameSeqByG[gid] = st.eventIndexByG[gid]
				}
				if ptr != "" && name != "" {
					st.channelNameByPtr[ptr] = name
				}
			}
			return nil
		}
		// Fallback: sometimes the category isn't preserved but the message contains a ptr= token.
		if l.Category == "" {
			msg := strings.TrimSpace(l.Message)
			if i := strings.Index(msg, "ptr="); i >= 0 {
				ptr := strings.TrimSpace(msg[i+4:])
				if j := strings.IndexAny(ptr, " \t\n"); j >= 0 {
					ptr = ptr[:j]
				}
				if ptr != "" {
					st.lastChPtrByG[gid] = ptr
					st.lastChPtrSeqByG[gid] = st.eventIndexByG[gid]
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "ch_ptr", G: gid, Role: st.Roles[gid], ChannelKey: ptr, ChPtr: ptr, Source: "log"})
					return nil
				}
			}
		}
		// Channel creation logs from instrumenter: category "chan_make", message like "ptr=0xc000014120 cap=4 type=int"
		if l.Category == "chan_make" {
			msg := strings.TrimSpace(l.Message)
			// Extract ptr, cap, type
			ptr := ""
			capVal := 0
			elem := ""
			// ptr=
			if i := strings.Index(msg, "ptr="); i >= 0 {
				rest := msg[i+4:]
				rest = strings.TrimSpace(rest)
				if j := strings.IndexAny(rest, " \t\n"); j >= 0 {
					ptr = rest[:j]
				} else {
					ptr = rest
				}
			}
			// cap=
			if i := strings.Index(msg, "cap="); i >= 0 {
				rest := msg[i+4:]
				rest = strings.TrimSpace(rest)
				end := len(rest)
				for j := 0; j < len(rest); j++ {
					if rest[j] < '0' || rest[j] > '9' {
						end = j
						break
					}
				}
				if end > 0 {
					if n, err := strconv.Atoi(rest[:end]); err == nil {
						capVal = n
					}
				}
			}
			// type=
			if i := strings.Index(msg, "type="); i >= 0 {
				rest := msg[i+5:]
				rest = strings.TrimSpace(rest)
				if j := strings.IndexAny(rest, " \t\n"); j >= 0 {
					elem = rest[:j]
				} else {
					elem = rest
				}
			}
			if ptr != "" {
				st.capByPtr[ptr] = capVal
			}
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: "chan_make", G: gid, Role: st.Roles[gid], ChannelKey: ptr, ChPtr: ptr, Cap: capVal, ElemType: elem, Source: "log"})
			return nil
		}
		// Channel close logs from instrumenter: category "chan_close", message like "ptr=0xc000014120"
		if l.Category == "chan_close" {
			msg := strings.TrimSpace(l.Message)
			ptr := ""
			if i := strings.Index(msg, "ptr="); i >= 0 {
				rest := strings.TrimSpace(msg[i+4:])
				if j := strings.IndexAny(rest, " \t\n"); j >= 0 {
					ptr = rest[:j]
				} else {
					ptr = rest
				}
			} else {
				ptr = msg
			}
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: "chan_close", G: gid, Role: st.Roles[gid], ChannelKey: ptr, ChPtr: ptr, Source: "log"})
			return nil
		}
		if l.Category == "spawn_parent" {
			if sid := strings.TrimSpace(l.Message); sid != "" {
				if strings.HasPrefix(sid, "sid=") {
					sid = strings.TrimSpace(sid[4:])
				} else if fields := strings.Fields(sid); len(fields) > 0 {
					if strings.HasPrefix(fields[0], "sid=") {
						sid = strings.TrimSpace(fields[0][4:])
					}
				}
				st.spawnParentBySID[sid] = gid
			}
			return nil
		}
		if l.Category == "spawn_child" {
			msg := strings.TrimSpace(l.Message)
			sid := ""
			if strings.HasPrefix(msg, "sid=") {
				sid = strings.TrimSpace(msg[4:])
			} else if fields := strings.Fields(msg); len(fields) > 0 {
				if strings.HasPrefix(fields[0], "sid=") {
					sid = strings.TrimSpace(fields[0][4:])
				}
			}
			parent := st.spawnParentBySID[sid]
			if sid != "" {
				delete(st.spawnParentBySID, sid)
			}
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: "spawn", G: gid, ParentG: parent, Role: st.Roles[gid], Source: "log"})
			return nil
		}
		// Recognize common role categories. Extend beyond ping/pong.
		if l.Category == "role" {
			msg := strings.TrimSpace(l.Message)
			if msg != "" {
				st.Roles[gid] = msg
				st.Interested[gid] = struct{}{}
				switch msg {
				case "main":
					st.lastMainG = gid
				case "worker":
					st.lastWorkerG = gid
				case "server":
					st.lastServerG = gid
				case "client":
					st.lastClientG = gid
				}
			}
		}
		if l.Category == "main" || l.Category == "worker" || l.Category == "server" || l.Category == "client" {
			st.Roles[gid] = l.Category
			st.Interested[gid] = struct{}{}
			if l.Category == "main" {
				st.lastMainG = gid
			} else if l.Category == "worker" {
				st.lastWorkerG = gid
			}
			if l.Category == "server" {
				st.lastServerG = gid
			} else if l.Category == "client" {
				st.lastClientG = gid
			}
		}
		msg := l.Message
		if l.Category == "value" {
			// Stash value; attach on the next region end event for this goroutine.
			st.lastValue[gid] = msg
			return nil
		}
		// Handle select scaffolding if present in logs (instrumentation-dependent)
		if strings.HasPrefix(msg, "select_") {
			st.handleSelectLog(gid, msg, tsMs, e.Stack(), emit)
			return nil
		}
		switch msg {
		case "creating channels":
			// Legacy aggregate event for demo workloads
			if err := emit(TimelineEvent{TimeMs: tsMs, Event: "channels_created", G: gid, Role: st.Roles[gid], Source: "log"}); err != nil {
				return err
			}
			// Atomic-style channel creation hints (best-effort for demo: infer int chan ping/pong without cap info)
			if st.Opt.EmitAtomic {
				_ = emit(TimelineEvent{TimeMs: tsMs, Event: "chan_make", G: gid, Channel: "ping", Role: st.Roles[gid], ElemType: "int", Cap: 0, Source: "log"})
				_ = emit(TimelineEvent{TimeMs: tsMs, Event: "chan_make", G: gid, Channel: "pong", Role: st.Roles[gid], ElemType: "int", Cap: 0, Source: "log"})
			}
			return nil
		case "starting worker goroutine":
			return emit(TimelineEvent{TimeMs: tsMs, Event: "worker_starting", G: gid, Role: st.Roles[gid], Source: "log"})
		}
		// sent N to ping
		{
			var v int
			if strings.Contains(msg, " to ping") {
				if n, _ := fmt.Sscanf(msg, "sent %d to ping", &v); n == 1 {
					return st.handleChanOp("ping_send", "ping", gid, st.Roles[gid], tsMs, &v, "log", emit)
				}
			}
		}
		// got N from ping
		{
			var v int
			if strings.Contains(msg, " from ping") {
				if n, _ := fmt.Sscanf(msg, "got %d from ping", &v); n == 1 {
					return st.handleChanOp("ping_recv", "ping", gid, st.Roles[gid], tsMs, &v, "log", emit)
				}
			}
		}
		// sent N to pong
		{
			var v int
			if strings.Contains(msg, " to pong") {
				if n, _ := fmt.Sscanf(msg, "sent %d to pong", &v); n == 1 {
					return st.handleChanOp("pong_send", "pong", gid, st.Roles[gid], tsMs, &v, "log", emit)
				}
			}
		}
		// received N from pong
		{
			var v int
			if strings.Contains(msg, " from pong") {
				if n, _ := fmt.Sscanf(msg, "received %d from pong", &v); n == 1 {
					return st.handleChanOp("pong_recv", "pong", gid, st.Roles[gid], tsMs, &v, "log", emit)
				}
			}
		}

	case xtrace.EventRegionBegin:
		rgn := e.Region()
		gid := int64(e.Goroutine())
		if kind, ch, ok := parseRegionOp(rgn.Type); ok {
			_ = st.handleRegionBegin(kind, ch, gid, st.Roles[gid], tsMs, emit)
		}

	case xtrace.EventRegionEnd:
		rgn := e.Region()
		gid := int64(e.Goroutine())
		if op, ok := st.Active[gid]; ok {
			if kind, _, ok2 := parseRegionOp(rgn.Type); ok2 && kind == op.kind {
				// Attach any stashed value to the completion event
				val, hasVal := st.lastValue[gid]
				if hasVal {
					delete(st.lastValue, gid)
				}
				// Temporarily wrap emit to include value for legacy event
				if hasVal {
					origEmit := emit
					emit = func(ev TimelineEvent) error { ev.Value = val; return origEmit(ev) }
					_ = st.handleRegionOp(kind, op.ch, op.ptr, gid, st.Roles[gid], tsMs, emit)
					emit = func(ev TimelineEvent) error { return origEmit(ev) }
				} else {
					_ = st.handleRegionOp(kind, op.ch, op.ptr, gid, st.Roles[gid], tsMs, emit)
				}

				// Atomic commit after legacy emission (pair via attempt queues)
				if st.Opt.EmitAtomic {
					att := st.attemptsByG[gid]
					cid := st.newID("c")
					cev := "chan_send_commit"
					var peerG int64 = 0
					ck := parseChKey(att.ch)
					if kind == "recv" {
						cev = "chan_recv_commit"
						// Remove this recv attempt from its queue (front if present; else search)
						ok := st.opKey(ck, att.ptr)
						if q := st.rcvAttQ[ok]; len(q) > 0 {
							if q[0] == att.id {
								st.rcvAttQ[ok] = q[1:]
							} else {
								// linear remove
								for i := range q {
									if q[i] == att.id {
										st.rcvAttQ[ok] = append(q[:i], q[i+1:]...)
										break
									}
								}
							}
						}
						// Pair with the oldest pending send attempt on this channel
						pairID := att.id // default fallback to own recv attempt id
						sendOpKey := st.opKey(ck, att.ptr)
						if q := st.sndAttQ[sendOpKey]; len(q) > 0 {
							sid := q[0]
							st.sndAttQ[sendOpKey] = q[1:]
							if ai, ok := st.attByID[sid]; ok {
								peerG = ai.G
							}
							if sid != "" {
								pairID = sid
							}
						}
						chPtr := att.ptr
						_ = emit(TimelineEvent{TimeMs: tsMs, Event: cev, ID: cid, AttemptID: att.id, G: gid, Channel: att.ch, ChannelKey: st.channelKey(ck, chPtr), ChPtr: chPtr, MissingPtr: chPtr == "", PeerG: peerG, PairID: pairID, DeltaMs: tsMs - att.time, Role: st.Roles[gid], Source: "region"})
						// Buffered depth accounting: if the paired send attempt had been buffered, depth--
						if chPtr != "" && st.capByPtr[chPtr] > 0 {
							if st.bufSends[chPtr] != nil && st.bufSends[chPtr][pairID] {
								if st.depthByPtr[chPtr] > 0 {
									st.depthByPtr[chPtr]--
								}
								delete(st.bufSends[chPtr], pairID)
								_ = emit(TimelineEvent{TimeMs: tsMs, Event: "chan_depth", G: gid, Channel: att.ch, ChannelKey: chPtr, ChPtr: chPtr, Cap: st.capByPtr[chPtr], Value: st.depthByPtr[chPtr], Source: "region"})
							}
						}
					} else {
						// send commit: leave the send attempt queued; receiver will pair later
						cev = "chan_send_commit"
						// For send, use the send attempt id as the pair id; receiver will reuse it.
						chPtr := att.ptr
						_ = emit(TimelineEvent{TimeMs: tsMs, Event: cev, ID: cid, AttemptID: att.id, G: gid, Channel: att.ch, ChannelKey: st.channelKey(ck, chPtr), ChPtr: chPtr, MissingPtr: chPtr == "", PairID: att.id, DeltaMs: tsMs - att.time, Role: st.Roles[gid], Source: "region"})
						// Buffered depth accounting: if channel is buffered, increment and mark this attempt as buffered
						if chPtr != "" && st.capByPtr[chPtr] > 0 {
							if st.bufSends[chPtr] == nil {
								st.bufSends[chPtr] = make(map[string]bool)
							}
							st.bufSends[chPtr][att.id] = true
							st.depthByPtr[chPtr]++
							if st.depthByPtr[chPtr] > st.capByPtr[chPtr] {
								st.depthByPtr[chPtr] = st.capByPtr[chPtr]
							}
							_ = emit(TimelineEvent{TimeMs: tsMs, Event: "chan_depth", G: gid, Channel: att.ch, ChannelKey: chPtr, ChPtr: chPtr, Cap: st.capByPtr[chPtr], Value: st.depthByPtr[chPtr], Source: "region"})
						}
					}
					delete(st.activeAttempt, gid)
					delete(st.attemptsByG, gid)
				}
				delete(st.Active, gid)
			}
		}

	case xtrace.EventStateTransition:
		s := e.StateTransition()
		if s.Resource.Kind != xtrace.ResourceGoroutine {
			return nil
		}
		gid := int64(s.Resource.Goroutine())
		oldState, newState := s.Goroutine()

		// get top frame func name if available
		fn := ""
		stck := s.Stack
		if stck != xtrace.NoStack {
			for frame := range stck.Frames() {
				fn = frame.Func
				break
			}
		}

		// Creation: track goroutine creation for known workload packages
		if oldState == xtrace.GoNotExist && (newState == xtrace.GoRunnable || newState == xtrace.GoWaiting) {
			if strings.Contains(fn, "/workload.") {
				st.Interested[gid] = struct{}{}
				if _, ok := st.Roles[gid]; !ok {
					st.Roles[gid] = "worker"
				}
				return emit(TimelineEvent{TimeMs: tsMs, Event: "goroutine_created", G: gid, Func: fn, Role: st.Roles[gid], Source: "state"})
			}
			return nil
		}

		// Start: record and track main and pingpong goroutines
		if oldState == xtrace.GoRunnable && newState == xtrace.GoRunning {
			name := fn
			if name == "" {
				name = "?"
			}
			if name == "main.main" || strings.Contains(name, "/workload.") {
				st.Interested[gid] = struct{}{}
				if name == "main.main" {
					st.Roles[gid] = "main"
					st.lastMainG = gid
				} else {
					st.Roles[gid] = "worker"
				}
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "goroutine_started", G: gid, Func: name, Role: st.Roles[gid], Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					// Best-effort parent inference: main → worker
					var parent int64
					if name != "main.main" {
						parent = st.lastMainG
					}
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "go_start", ID: st.newID("g"), G: gid, Func: name, ParentG: parent, Role: st.Roles[gid], Source: "state"})
				}
				return nil
			}
			return nil
		}

		// Block on send/receive
		if newState == xtrace.GoWaiting {
			if _, ok := st.Interested[gid]; !ok {
				return nil
			}
			switch s.Reason {
			case "chan send":
				ch := ""
				if op, ok := st.Active[gid]; ok && op.kind == "send" {
					ch = op.ch
				} else if op, ok := st.Last[gid]; ok && op.kind == "send" {
					ch = op.ch
				}
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "blocked_send", G: gid, Role: st.Roles[gid], Channel: ch, Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "block", G: gid, Role: st.Roles[gid], Channel: ch, Reason: "send", Source: "state"})
				}
				st.blocked[gid] = struct{ Reason, Ch string }{Reason: "send", Ch: ch}
				return nil
			case "chan receive":
				ch := ""
				if op, ok := st.Active[gid]; ok && op.kind == "recv" {
					ch = op.ch
				} else if op, ok := st.Last[gid]; ok && op.kind == "recv" {
					ch = op.ch
				}
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "blocked_receive", G: gid, Role: st.Roles[gid], Channel: ch, Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "block", G: gid, Role: st.Roles[gid], Channel: ch, Reason: "recv", Source: "state"})
				}
				st.blocked[gid] = struct{ Reason, Ch string }{Reason: "recv", Ch: ch}
				return nil
			}
			return nil
		}

		// Unblock
		if oldState == xtrace.GoWaiting && newState == xtrace.GoRunnable {
			if _, ok := st.Interested[gid]; ok {
				ch := ""
				if op, ok := st.Active[gid]; ok {
					ch = op.ch
				} else if op, ok := st.Last[gid]; ok {
					ch = op.ch
				}
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "unblocked", G: gid, Role: st.Roles[gid], Channel: ch, Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					info := st.blocked[gid]
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "unblock", G: gid, Role: st.Roles[gid], Channel: ch, Reason: info.Reason, Source: "state"})
				}
				return nil
			}
			return nil
		}
	}
	return nil
}

func (st *ParseState) handleRegionBegin(kind, ch string, gid int64, role string, ts float64, emit func(TimelineEvent) error) error {
	chPtr := st.consumeFreshChPtr(gid)
	chName := st.consumeFreshChName(gid)
	if chName == "" && chPtr != "" {
		if mapped, ok := st.channelNameByPtr[chPtr]; ok {
			chName = mapped
		}
	}
	if chName != "" {
		ch = chName
	}
	st.Active[gid] = op{kind: kind, ch: ch, ptr: chPtr}
	st.Last[gid] = st.Active[gid]
	ev := "send_attempt"
	if kind == "recv" {
		ev = "recv_attempt"
	}
	val := ""
	if v, ok := st.lastValue[gid]; ok && kind == "send" {
		val = v
		delete(st.lastValue, gid)
	}
	ck := parseChKey(ch)
	channelKey := st.channelKey(ck, chPtr)
	event := TimelineEvent{TimeMs: ts, Event: ev, Channel: ch, G: gid, Role: role, Source: "region", ChPtr: chPtr, MissingPtr: chPtr == ""}
	if channelKey != "" {
		event.ChannelKey = channelKey
	}
	if val != "" {
		event.Value = val
	}
	if err := emit(event); err != nil {
		return err
	}
	if st.Opt.EmitAtomic {
		id := st.newID("a")
		st.activeAttempt[gid] = id
		st.attemptsByG[gid] = struct {
			id, ch, kind, ptr string
			time              float64
		}{id: id, ch: ch, kind: kind, ptr: chPtr, time: ts}
		aev := "chan_send_attempt"
		if kind == "recv" {
			aev = "chan_recv_attempt"
		}
		ck := parseChKey(ch)
		var valAny any
		if v, ok := st.lastValue[gid]; ok && kind == "send" {
			valAny = v
		}
		if err := emit(TimelineEvent{TimeMs: ts, Event: aev, ID: id, G: gid, Channel: ch, ChannelKey: st.channelKey(ck, chPtr), ChPtr: chPtr, Role: role, MissingPtr: chPtr == "", Value: valAny, Source: "region"}); err != nil {
			return err
		}
		st.attByID[id] = struct {
			G    int64
			Ch   chKey
			Kind string
			Time float64
			Ptr  string
		}{G: gid, Ch: ck, Kind: kind, Time: ts, Ptr: chPtr}
		if kind == "send" {
			st.sndAttQ[st.opKey(ck, chPtr)] = append(st.sndAttQ[st.opKey(ck, chPtr)], id)
		} else {
			st.rcvAttQ[st.opKey(ck, chPtr)] = append(st.rcvAttQ[st.opKey(ck, chPtr)], id)
		}
	}
	return nil
}

func (st *ParseState) handleChanOp(ev, ch string, gid int64, role string, t float64, val *int, source string, emit func(TimelineEvent) error) error {
	// Optional synthesis: ensure a send exists before a recv
	ck := parseChKey(ch)
	if strings.HasSuffix(ev, "_recv") && st.Opt.SynthOnRecv {
		sendEv := strings.TrimSuffix(ev, "_recv") + "_send"
		k := skey{Ev: sendEv, Ch: strings.ToLower(ch)}
		if ch != "" && !st.hasSend[k] {
			var sg int64
			if strings.HasPrefix(sendEv, "ping_") {
				if st.lastMainG != 0 {
					sg = st.lastMainG
				} else {
					sg = 1
				}
			} else if strings.HasPrefix(sendEv, "pong_") {
				if st.lastWorkerG != 0 {
					sg = st.lastWorkerG
				} else {
					sg = gid
				}
			} else {
				sg = gid
			}
			if err := emit(TimelineEvent{TimeMs: t - 0.001, Event: sendEv, Channel: ch, ChannelKey: st.channelKey(ck, ""), G: sg, Role: "worker", Source: "synth"}); err != nil {
				return err
			}
			st.hasSend[k] = true
		}
	}
	// Emit requested
	te := TimelineEvent{TimeMs: t, Event: ev, Channel: ch, ChannelKey: st.channelKey(ck, ""), G: gid, Role: role, Value: val, Source: source}
	if err := emit(te); err != nil {
		return err
	}
	if strings.HasSuffix(ev, "_send") && ch != "" {
		st.hasSend[skey{Ev: ev, Ch: strings.ToLower(ch)}] = true
	}
	return nil
}

// Key for grouping operations by logical channel.
type chKey struct {
	Name     string
	ClientID int // -1 if not per-client
}

type sendRecord struct {
	TimeMs float64
	G      int64
	Role   string
	Ptr    string
	MsgID  int64
}

func parseChKey(ch string) chKey {
	s := strings.ToLower(ch)
	if i := strings.Index(s, "["); i >= 0 && strings.HasSuffix(s, "]") {
		base := s[:i]
		idxStr := s[i+1 : len(s)-1]
		if n, err := strconv.Atoi(idxStr); err == nil {
			return chKey{Name: base, ClientID: n}
		}
	}
	return chKey{Name: s, ClientID: -1}
}

func shouldSkipPairing(key chKey) bool {
	if len(skipPairingChannels) == 0 {
		return false
	}
	return skipPairingChannels[key.Name]
}

// handleRegionOp processes the end of a region send/recv and pairs operations by channel key.
func (st *ParseState) handleRegionOp(kind, ch, chPtr string, gid int64, role string, t float64, emit func(TimelineEvent) error) error {
	// Always emit the *_complete event for timelines
	ev := "send_complete"
	if kind == "recv" {
		ev = "recv_complete"
	}
	key := parseChKey(ch)
	if err := emit(TimelineEvent{TimeMs: t, Event: ev, Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "region"}); err != nil {
		return err
	}
	switch kind {
	case "send":
		mid := st.nextMsgID
		st.nextMsgID++
		_ = emit(TimelineEvent{TimeMs: t, Event: "chan_send", Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "paired", PeerG: 0, MsgID: mid})
		st.sendsByKey[st.opKey(key, chPtr)] = append(st.sendsByKey[st.opKey(key, chPtr)], sendRecord{TimeMs: t, G: gid, Role: role, Ptr: chPtr, MsgID: mid})
	case "recv":
		q := st.sendsByKey[st.opKey(key, chPtr)]
		if len(q) == 0 {
			// Prefer not to synthesize for broadcast channels we instrument at both ends.
			// This avoids duplicate edges on join[i], clientin, clientout[i].
			lname := key.Name
			if lname == "clientin" || strings.HasPrefix(lname, "clientout") || strings.HasPrefix(lname, "join") {
				return nil
			}
			if !st.Opt.SynthOnRecv {
				return nil
			}
			// Choose a plausible sender goroutine based on channel and roles
			var sg int64
			switch {
			case lname == "clientin":
				if st.lastClientG != 0 {
					sg = st.lastClientG
				} else {
					sg = gid
				}
			case strings.HasPrefix(lname, "clientout") || strings.HasPrefix(lname, "join"):
				if st.lastServerG != 0 {
					sg = st.lastServerG
				} else {
					sg = gid
				}
			default:
				if st.lastWorkerG != 0 {
					sg = st.lastWorkerG
				} else if st.lastMainG != 0 {
					sg = st.lastMainG
				} else {
					sg = gid
				}
			}
			mid := st.nextMsgID
			st.nextMsgID++
			if err := emit(TimelineEvent{TimeMs: t - 0.001, Event: "send_complete", Channel: ch, ChannelKey: st.channelKey(key, chPtr), G: sg, Role: st.Roles[sg], Source: "synth", ChPtr: chPtr}); err != nil {
				return err
			}
			_ = emit(TimelineEvent{TimeMs: t - 0.001, Event: "chan_send", Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: sg, Role: st.Roles[sg], Source: "paired", PeerG: gid, MsgID: mid})
			// Also emit an unmatched recv with unknown peer for explicitness
			_ = emit(TimelineEvent{TimeMs: t, Event: "chan_recv", Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "paired", PeerG: sg, MsgID: mid})
			return nil
		}
		// Match with the earliest recorded send
		sr := q[0]
		st.sendsByKey[st.opKey(key, chPtr)] = q[1:]
		if shouldSkipPairing(key) {
			return nil
		}
		_ = emit(TimelineEvent{TimeMs: t, Event: "chan_recv", Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "paired", PeerG: sr.G, MsgID: sr.MsgID})
	}
	return nil
}

// handleSelectLog parses simple select_* log messages (if emitted by instrumentation)
// Supported forms (tokens space-separated, key=val pairs):
//
//	select_begin [id=<sid>]
//	select_case [id=<sid>] [attempt=<aid>] [kind=send|recv] [ch=<name>]
//	select_chosen [id=<sid>] [attempt=<aid>]
//	select_end [id=<sid>]
func (st *ParseState) handleSelectLog(gid int64, msg string, t float64, stack xtrace.Stack, emit func(TimelineEvent) error) {
	toks := strings.Fields(msg)
	if len(toks) == 0 {
		return
	}
	typ := toks[0]
	kv := make(map[string]string)
	for _, tk := range toks[1:] {
		if i := strings.IndexByte(tk, '='); i > 0 {
			k := tk[:i]
			v := tk[i+1:]
			kv[strings.ToLower(k)] = v
		}
	}
	sid := kv["id"]
	if sid == "" {
		// fall back to active select for this goroutine if any
		if s, ok := st.activeSelect[gid]; ok {
			sid = s
		}
	}
	switch typ {
	case "select_begin":
		if sid == "" {
			sid = st.newID("s")
		}
		st.activeSelect[gid] = sid
		delete(st.selectFallback, gid)
		_ = emit(TimelineEvent{TimeMs: t, Event: "select_begin", G: gid, Role: st.Roles[gid], SelectID: sid, Source: "log"})
	case "select_case":
		if sid == "" {
			sid = st.activeSelect[gid]
		}
		att := kv["attempt"]
		if att == "" {
			att = st.activeAttempt[gid]
		}
		ch := kv["ch"]
		_ = emit(TimelineEvent{TimeMs: t, Event: "select_case", G: gid, Role: st.Roles[gid], SelectID: sid, AttemptID: att, Channel: ch, Source: "log"})
		if att != "" {
			st.selCandidates[sid] = append(st.selCandidates[sid], att)
		}
	case "select_chosen":
		if sid == "" {
			sid = st.activeSelect[gid]
		}
		att := kv["attempt"]
		if att == "" {
			att = st.activeAttempt[gid]
		}
		st.selChosen[sid] = att
		_ = emit(TimelineEvent{TimeMs: t, Event: "select_chosen", G: gid, Role: st.Roles[gid], SelectID: sid, AttemptID: att, Source: "log"})
		st.maybeEmitSelectFallback(gid, sid, stack, t, emit)
	case "select_end":
		if sid == "" {
			sid = st.activeSelect[gid]
		}
		// Mark non-chosen candidates as dropped
		chosen := st.selChosen[sid]
		for _, att := range st.selCandidates[sid] {
			if att == "" || att == chosen {
				continue
			}
			// Try to include channel and goroutine from attempt table
			var ch string
			var ag int64
			if ai, ok := st.attByID[att]; ok {
				ch = ai.Ch.Name
				ag = ai.G
			}
			_ = emit(TimelineEvent{TimeMs: t, Event: "select_dropped", G: ag, Role: st.Roles[ag], SelectID: sid, AttemptID: att, Channel: ch, Dropped: true, Source: "log"})
		}
		// Cleanup
		delete(st.selCandidates, sid)
		delete(st.selChosen, sid)
		delete(st.selectFallback, gid)
		delete(st.activeSelect, gid)
		_ = emit(TimelineEvent{TimeMs: t, Event: "select_end", G: gid, Role: st.Roles[gid], SelectID: sid, Source: "log"})
	}
}

func (st *ParseState) maybeEmitSelectFallback(gid int64, sid string, stack xtrace.Stack, ts float64, emit func(TimelineEvent) error) {
	if sid == "" {
		sid = "select"
	}
	if stack == xtrace.NoStack {
		return
	}
	if _, ok := st.selectFallback[gid]; ok {
		return
	}
	if _, ok := st.Active[gid]; ok {
		return
	}
	for frame := range stack.Frames() {
		fn := frame.Func
		if strings.Contains(fn, "chanrecv") {
			chName := fmt.Sprintf("select_%s", sid)
			ck := parseChKey(chName)
			chKey := st.channelKey(ck, "")
			_ = emit(TimelineEvent{TimeMs: ts - 0.0001, Event: "recv_attempt", Channel: chName, ChannelKey: chKey, G: gid, Role: st.Roles[gid], Source: "select_fallback"})
			_ = emit(TimelineEvent{TimeMs: ts, Event: "recv_complete", Channel: chName, ChannelKey: chKey, G: gid, Role: st.Roles[gid], Source: "select_fallback"})
			st.selectFallback[gid] = sid
			return
		}
	}
}

func NormalizeTimeline(events []TimelineEvent, st *ParseState) TimelineEnvelope {
	_ = st
	normalized := make([]TimelineEvent, len(events))
	warnings := make([]string, 0)
	for i, ev := range events {
		if ev.TimeNs == 0 {
			ev.TimeNs = int64(ev.TimeMs*1e6 + 0.5)
		}
		if ev.Seq == 0 {
			ev.Seq = int64(i + 1)
		}
		name := strings.ToLower(ev.Event)
		if isGoroutineEvent(name) && ev.G == 0 {
			warnings = append(warnings, fmt.Sprintf("missing goroutine id for %s (seq=%d time_ns=%d)", ev.Event, ev.Seq, ev.TimeNs))
		}
		if isChannelEvent(name) {
			if timelineChannelIdentity(ev) == "" {
				unknown := fmt.Sprintf("unknown:%d:%d", ev.G, ev.Seq)
				if ev.Channel == "" {
					ev.Channel = unknown
				}
				if ev.ChannelKey == "" {
					ev.ChannelKey = unknown
				}
				warnings = append(warnings, fmt.Sprintf("missing channel identity for %s (g=%d seq=%d time_ns=%d)", ev.Event, ev.G, ev.Seq, ev.TimeNs))
			}
		}
		if isPairingEvent(name, ev.Source) {
			if ev.PeerG == 0 && ev.PairID == "" && ev.MsgID == 0 {
				warnings = append(warnings, fmt.Sprintf("missing pairing fields for %s (g=%d seq=%d time_ns=%d)", ev.Event, ev.G, ev.Seq, ev.TimeNs))
			}
		}
		normalized[i] = ev
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		a := normalized[i]
		b := normalized[j]
		if a.TimeNs != b.TimeNs {
			return a.TimeNs < b.TimeNs
		}
		if a.Seq != b.Seq {
			return a.Seq < b.Seq
		}
		if a.G != b.G {
			return a.G < b.G
		}
		return strings.ToLower(a.Event) < strings.ToLower(b.Event)
	})
	return TimelineEnvelope{
		SchemaVersion: TimelineSchemaVersion,
		Events:        normalized,
		Warnings:      warnings,
	}
}

// DedupTimeline removes exact duplicate parser emissions while preserving order.
// Key: time_ns (or derived from time_ms) + g + channel + event + attempt_id.
func DedupTimeline(in []TimelineEvent) ([]TimelineEvent, DedupAudit) {
	seen := make(map[string]struct{}, len(in))
	out := make([]TimelineEvent, 0, len(in))
	var audit DedupAudit
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
			audit.Duplicates++
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ev)
	}
	return out, audit
}

// AppendAuditSummary is a no-op placeholder for optional audit summary injection.
func AppendAuditSummary(events []TimelineEvent, audit DedupAudit) []TimelineEvent {
	_ = audit
	return events
}

func timelineChannelIdentity(ev TimelineEvent) string {
	if ev.ChPtr != "" {
		return ev.ChPtr
	}
	if ev.ChannelKey != "" {
		return ev.ChannelKey
	}
	return ev.Channel
}

func isGoroutineEvent(name string) bool {
	switch name {
	case "goroutine_created", "goroutine_started", "go_start", "spawn", "worker_starting", "channels_created",
		"blocked_send", "blocked_receive", "unblocked", "block", "unblock",
		"select_begin", "select_case", "select_chosen", "select_dropped", "select_end":
		return true
	default:
		return false
	}
}

func isChannelEvent(name string) bool {
	switch name {
	case "chan_make", "chan_close", "chan_send", "chan_recv", "chan_depth",
		"send_complete", "recv_attempt", "recv_complete",
		"chan_send_attempt", "chan_recv_attempt", "chan_send_commit", "chan_recv_commit",
		"blocked_send", "blocked_receive", "block", "unblock", "select_case":
		return true
	default:
		return false
	}
}

func isPairingEvent(name, source string) bool {
	switch name {
	case "chan_send", "chan_recv":
		return strings.ToLower(source) == "paired"
	case "chan_send_commit", "chan_recv_commit":
		return true
	default:
		return false
	}
}
