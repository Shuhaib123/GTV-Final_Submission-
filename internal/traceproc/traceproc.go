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

const (
	auditGoroutineLifecycleFilteredByName       = "goroutine_lifecycle_filtered_by_name"
	auditGoroutineLifecycleFilteredByMissingOps = "goroutine_lifecycle_filtered_by_missing_ops"
	auditBlockedDropNoChannelSend               = "blocked_drop_no_channel_send"
	auditBlockedDropNoChannelRecv               = "blocked_drop_no_channel_recv"
	auditSynthSend                              = "synth_send"
	auditRetroSendEmit                          = "retro_send_emit"
	auditUnmatchedRecv                          = "unmatched_recv"
	auditSkipPairingHardcoded                   = "skip_pairing_hardcoded"
	auditSkipPairingEnv                         = "skip_pairing_env"
	auditUnparsedRegionOp                       = "unparsed_region_op"
	auditParsedRegionSend                       = "parsed_region_send"
	auditParsedRegionRecv                       = "parsed_region_recv"
	auditSelectCaseSeen                         = "select_case_seen"
	auditSelectChosenSeen                       = "select_chosen_seen"
	auditSelectChosenMissingCase                = "select_chosen_missing_case"
	auditSelectChosenEmittedSend                = "select_chosen_emitted_chan_send"
	auditSelectChosenEmittedRecv                = "select_chosen_emitted_chan_recv"
	auditMissingChannelIdentity                 = "missing_channel_identity"
	auditMissingBlockChannel                    = "missing_block_channel"
)

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
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line,omitempty"`
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
	// Optional blocking/diagnostic fields
	BlockKind       string `json:"block_kind,omitempty"`
	UnblockKind     string `json:"unblock_kind,omitempty"`
	BlockCause      string `json:"block_cause,omitempty"`
	BlockLen        int    `json:"block_len,omitempty"`
	BlockCap        int    `json:"block_cap,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
	Confidence      string `json:"confidence,omitempty"`
	UnknownSender   bool   `json:"unknown_sender,omitempty"`
	UnknownReceiver bool   `json:"unknown_receiver,omitempty"`
	UnknownChannel  bool   `json:"unknown_channel,omitempty"`
	// Optional select fields
	SelectID    string `json:"select_id,omitempty"`
	SelectKind  string `json:"select_kind,omitempty"`
	SelectIndex int    `json:"select_index,omitempty"`
	Dropped     bool   `json:"select_dropped,omitempty"`
}

const TimelineSchemaVersion = 1

type TimelineEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	Capabilities  []string        `json:"capabilities,omitempty"`
	Events        []TimelineEvent `json:"events"`
	Entities      *EntitiesTable  `json:"entities,omitempty"`
	Warnings      []string        `json:"warnings,omitempty"`
	WarningsV2    []WarningEntry  `json:"warnings_v2,omitempty"`
}

type WarningEntry struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type EntitiesTable struct {
	Goroutines []GoroutineEntity `json:"goroutines,omitempty"`
	Channels   []ChannelEntity   `json:"channels,omitempty"`
}

type GoroutineEntity struct {
	GID       int64  `json:"gid"`
	ParentGID int64  `json:"parent_gid,omitempty"`
	Func      string `json:"func,omitempty"`
	Role      string `json:"role,omitempty"`
}

type ChannelEntity struct {
	ChanID   string   `json:"chan_id"`
	Name     string   `json:"name,omitempty"`
	ElemType string   `json:"elem_type,omitempty"`
	Cap      int      `json:"cap,omitempty"`
	ChPtr    string   `json:"ch_ptr,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

type ChannelUnmatchedAudit struct {
	ChanID         string `json:"chan_id"`
	UnmatchedSends int64  `json:"unmatched_sends"`
	UnmatchedRecvs int64  `json:"unmatched_recvs"`
	MatchedPairs   int64  `json:"matched_pairs"`
}

type UnmatchedTotals struct {
	Sends            int64 `json:"sends"`
	Recvs            int64 `json:"recvs"`
	UnknownChannelID int64 `json:"unknown_channel_id"`
}

// Options control live/offline behavior.
type Options struct {
	// If true, emit a synthetic *_send just before a *_recv when no prior send was observed.
	SynthOnRecv bool
	// If true, drop blocked_send/blocked_receive that lack a channel label.
	DropBlockNoCh bool
	// If true, emit atomic attempt/commit and block/unblock events in addition to legacy events.
	EmitAtomic bool
	// Mode controls filtering of events ("teach" for minimal MVP, "debug" for full detail).
	Mode string
	// GoroutineFilter controls which goroutines are tracked.
	GoroutineFilter GoroutineFilterMode
}

type GoroutineFilterMode int

const (
	GoroutineFilterOps GoroutineFilterMode = iota
	GoroutineFilterAll
	GoroutineFilterLegacy
)

func ParseGoroutineFilterMode(value string) GoroutineFilterMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "legacy", "true", "yes", "on":
		return GoroutineFilterLegacy
	case "all":
		return GoroutineFilterAll
	case "ops", "", "0", "false", "no", "off":
		return GoroutineFilterOps
	default:
		return GoroutineFilterOps
	}
}

func normalizeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "teach", "teaching":
		return "teach"
	case "debug":
		return "debug"
	default:
		return ""
	}
}

func FilterTimelineEventForMode(ev TimelineEvent, mode string) (TimelineEvent, bool, string) {
	if normalizeMode(mode) != "teach" {
		return ev, true, ""
	}
	name := strings.ToLower(ev.Event)
	// In teach mode, drop low-level commit/complete variants to avoid
	// duplicate chan_send/chan_recv visuals when paired events are present.
	switch name {
	case "chan_send_commit", "chan_recv_commit", "send_complete", "recv_complete":
		return ev, false, name
	}
	allow := map[string]string{
		"goroutine_created":   "go_create",
		"goroutine_started":   "go_start",
		"go_create":           "go_create",
		"go_start":            "go_start",
		"spawn":               "spawn",
		"channels_created":    "channels_created",
		"worker_starting":     "worker_starting",
		"mutex_lock":          "mutex_lock",
		"mutex_unlock":        "mutex_unlock",
		"rwmutex_lock":        "rwmutex_lock",
		"rwmutex_unlock":      "rwmutex_unlock",
		"rwmutex_rlock":       "rwmutex_rlock",
		"rwmutex_runlock":     "rwmutex_runlock",
		"wg_add":              "wg_add",
		"wg_done":             "wg_done",
		"wg_wait":             "wg_wait",
		"cond_wait":           "cond_wait",
		"cond_signal":         "cond_signal",
		"cond_broadcast":      "cond_broadcast",
		"chan_make":           "chan_make",
		"chan_send":           "chan_send",
		"chan_recv":           "chan_recv",
		"blocked_send":        "block_send",
		"blocked_receive":     "block_recv",
		"block_send":          "block_send",
		"block_recv":          "block_recv",
		"go_block":            "go_block",
		"go_unblock":          "go_unblock",
		"unblock":             "unblock",
		"unblocked":           "unblock",
		"select_chosen":       "select_chosen",
		"audit_summary":       "audit_summary",
		"warn_quiescence":     "warn_quiescence",
		"warn_deadlock_cycle": "warn_deadlock_cycle",
	}
	mapped, ok := allow[name]
	if !ok {
		return ev, false, name
	}
	ev.Event = mapped
	return ev, true, ""
}

func AnnotateUnknownEndpoints(ev TimelineEvent) TimelineEvent {
	name := strings.ToLower(ev.Event)
	if isChannelEvent(name) && timelineChannelIdentity(ev) == "" {
		ev.UnknownChannel = true
	}
	switch name {
	case "chan_send":
		if ev.PeerG == 0 && ev.PairID == "" && ev.MsgID == 0 {
			ev.UnknownReceiver = true
		}
	case "chan_recv":
		if ev.PeerG == 0 && ev.PairID == "" && ev.MsgID == 0 {
			ev.UnknownSender = true
		}
	}
	return ev
}

// ParseState holds per-connection parsing state.
type op struct {
	kind string
	ch   string
	ptr  string
	time float64
}
type skey struct{ Ev, Ch string }
type OpKey string

type SelectCaseInfo struct {
	Kind   string
	ChPtr  string
	ChKey  string
	ChName string
	Role   string
	G      int64
	TimeNs int64
	Index  int
}

type ParseState struct {
	Roles      map[int64]string
	Interested map[int64]struct{}
	Active     map[int64]op
	Last       map[int64]op

	pendingGoroutines        map[int64]pendingGoroutine
	pendingGoroutinesAudited bool

	// Audit counters for parse decisions
	auditCounts map[string]int64
	lastTimeMs  float64
	lastTimeNs  int64

	// Synthesis bookkeeping
	hasSend     map[skey]bool
	lastMainG   int64
	lastWorkerG int64
	lastServerG int64
	lastClientG int64

	Opt Options

	// Region pairing (send → recv) bookkeeping by channel key
	sendsByKey map[OpKey][]sendRecord
	// Pending select recv with no matching send yet (by channel key)
	pendingSelectRecv map[OpKey]int
	// Pending recv with no matching send yet (by channel key)
	pendingRecvByKey map[OpKey][]pendingRecvRecord
	// Unmatched recv counts by channel key
	unmatchedRecvByChannel map[string]int64
	// Matched pairs by channel key
	matchedPairsByChannel map[string]int64
	// Unknown channel identity counts
	unknownChannelID int64

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
	blocked map[int64]struct{ Reason, Ch, ChannelID, Confidence string }
	// Block duration tracking
	blockedAt            map[int64]float64
	blockedReasonByG     map[int64]string
	blockedTotalMs       map[int64]float64
	blockedTotalByReason map[string]float64
	lastProgressMs       float64
	// Most recent sync wait region by goroutine; used to attribute mutex/wg unblock events.
	syncWaitCandidateByG map[int64]syncWaitCandidate
	// Spawn relationships by SID from instrumentation logs
	spawnParentBySID map[string]int64

	// Select accounting
	activeSelect   map[int64]string    // gid -> select id
	selCandidates  map[string][]string // select id -> attempt ids
	selChosen      map[string]string   // select id -> chosen attempt id
	selChosenIdx   map[string]int      // select id -> chosen case idx
	selPending     map[string]bool     // select id -> chosen seen before case
	selectFallback map[int64]string    // gid -> select id tracked for fallback
	selectCases    map[string][]SelectCaseInfo
	selectEmitted  map[string]bool
	selectMeta     map[string]struct {
		G      int64
		TimeNs int64
	}
	// Recent select choice (used to ignore select-clause body regions that look like send/recv ops).
	lastSelectChosenByG    map[int64]SelectCaseInfo
	lastSelectChosenSeqByG map[int64]int64

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

	unparsedRegionSamples []string

	// Deadlock/cycle inference
	lastSendByChannel map[string]lastOpInfo
	lastRecvByChannel map[string]lastOpInfo
	deadlockCycle     map[string]any
}

type lastOpInfo struct {
	G      int64
	TimeMs float64
}

type syncWaitCandidate struct {
	Kind       string
	Channel    string
	ChannelKey string
	StartMs    float64
}

type pendingGoroutine struct {
	name       string
	createdAt  float64
	startedAt  float64
	hasCreated bool
	hasStarted bool
}

// NewParseState creates a fresh parser state.
func NewParseState(opt Options) *ParseState {
	return &ParseState{
		Roles:                  make(map[int64]string),
		Interested:             make(map[int64]struct{}),
		Active:                 make(map[int64]op),
		Last:                   make(map[int64]op),
		pendingGoroutines:      make(map[int64]pendingGoroutine),
		auditCounts:            make(map[string]int64),
		hasSend:                make(map[skey]bool),
		Opt:                    opt,
		sendsByKey:             make(map[OpKey][]sendRecord),
		pendingSelectRecv:      make(map[OpKey]int),
		pendingRecvByKey:       make(map[OpKey][]pendingRecvRecord),
		unmatchedRecvByChannel: make(map[string]int64),
		matchedPairsByChannel:  make(map[string]int64),
		lastChPtrByG:           make(map[int64]string),
		lastChPtrSeqByG:        make(map[int64]int64),
		lastChNameByG:          make(map[int64]string),
		lastChNameSeqByG:       make(map[int64]int64),
		eventIndexByG:          make(map[int64]int64),
		lastValue:              make(map[int64]string),
		activeAttempt:          make(map[int64]string),
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
		sndAttQ:              make(map[OpKey][]string),
		rcvAttQ:              make(map[OpKey][]string),
		blocked:              make(map[int64]struct{ Reason, Ch, ChannelID, Confidence string }),
		blockedAt:            make(map[int64]float64),
		blockedReasonByG:     make(map[int64]string),
		blockedTotalMs:       make(map[int64]float64),
		blockedTotalByReason: make(map[string]float64),
		syncWaitCandidateByG: make(map[int64]syncWaitCandidate),
		lastSendByChannel:    make(map[string]lastOpInfo),
		lastRecvByChannel:    make(map[string]lastOpInfo),
		activeSelect:         make(map[int64]string),
		selCandidates:        make(map[string][]string),
		selChosen:            make(map[string]string),
		selChosenIdx:         make(map[string]int),
		selPending:           make(map[string]bool),
		selectFallback:       make(map[int64]string),
		selectCases:          make(map[string][]SelectCaseInfo),
		selectEmitted:        make(map[string]bool),
		selectMeta: make(map[string]struct {
			G      int64
			TimeNs int64
		}),
		lastSelectChosenByG:    make(map[int64]SelectCaseInfo),
		lastSelectChosenSeqByG: make(map[int64]int64),
		spawnParentBySID:       make(map[string]int64),
		capByPtr:               make(map[string]int),
		depthByPtr:             make(map[string]int),
		channelNameByPtr:       make(map[string]string),
		bufSends:               make(map[string]map[string]bool),
		nextMsgID:              1,
	}
}

func (st *ParseState) incAudit(reason string) {
	st.auditCounts[reason]++
}

func (st *ParseState) addBlockedDuration(gid int64, reason string, delta float64) {
	if delta <= 0 {
		return
	}
	st.blockedTotalMs[gid] += delta
	if strings.TrimSpace(reason) == "" {
		reason = "unknown"
	}
	st.blockedTotalByReason[reason] += delta
}

func (st *ParseState) markProgress(ts float64) {
	if ts > st.lastProgressMs {
		st.lastProgressMs = ts
	}
}

func parsedRegionAuditKey(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		return ""
	}
	return "parsed_region_" + strings.ReplaceAll(kind, "-", "_")
}

func syncWaitKindFromRuntimeReason(reason string) string {
	lr := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case strings.Contains(lr, "rwmutex") && strings.Contains(lr, "rlock"):
		return "rwmutex_rlock"
	case strings.Contains(lr, "rwmutex") && strings.Contains(lr, "lock"):
		return "rwmutex_lock"
	case strings.Contains(lr, "mutex") && strings.Contains(lr, "lock"):
		return "mutex_lock"
	case strings.Contains(lr, "waitgroup"):
		return "wg_wait"
	case strings.Contains(lr, "cond"):
		return "cond_wait"
	default:
		return ""
	}
}

func isSyncWaitKind(kind string) bool {
	switch kind {
	case "mutex_lock", "rwmutex_lock", "rwmutex_rlock", "wg_wait", "cond_wait":
		return true
	default:
		return false
	}
}

func unblockKindFromReason(reason string) string {
	switch reason {
	case "send":
		return "chan_send"
	case "recv":
		return "chan_recv"
	case "chan_send", "chan_recv", "mutex_lock", "rwmutex_lock", "rwmutex_rlock", "wg_wait", "cond_wait":
		return reason
	default:
		return "unknown"
	}
}

func (st *ParseState) recordChannelActivity(kind string, gid int64, channelID string, ts float64) {
	if channelID == "" {
		return
	}
	record := func(key string) {
		if key == "" {
			return
		}
		switch kind {
		case "send":
			st.lastSendByChannel[key] = lastOpInfo{G: gid, TimeMs: ts}
		case "recv":
			st.lastRecvByChannel[key] = lastOpInfo{G: gid, TimeMs: ts}
		}
	}
	record(channelID)

	alias := ""
	if _, ch, ok := parseRegionOp(channelID); ok && ch != "" && ch != channelID {
		alias = ch
		record(ch)
	}

	if strings.HasPrefix(channelID, "0x") {
		if name, ok := st.channelNameByPtr[channelID]; ok && name != "" {
			record(name)
			if _, ch, ok := parseRegionOp(name); ok && ch != "" {
				record(ch)
			}
		}
		return
	}

	nameKey := channelID
	if alias != "" {
		nameKey = alias
	}
	if nameKey != "" {
		for ptr, name := range st.channelNameByPtr {
			if name == nameKey {
				record(ptr)
				break
			}
		}
	}
}

func (st *ParseState) noteUnparsedRegion(label string) {
	if label == "" {
		return
	}
	const maxSamples = 20
	if len(st.unparsedRegionSamples) >= maxSamples {
		return
	}
	st.unparsedRegionSamples = append(st.unparsedRegionSamples, label)
}

func quiescenceThresholdMs() float64 {
	raw := strings.TrimSpace(os.Getenv("GTV_QUIESCENCE_MS"))
	if raw == "" {
		return 500
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
		return v
	}
	return 500
}

func (st *ParseState) quiescenceWarningEvent() *TimelineEvent {
	if st == nil {
		return nil
	}
	threshold := quiescenceThresholdMs()
	if threshold <= 0 {
		return nil
	}
	if len(st.blocked) == 0 {
		return nil
	}
	if st.lastProgressMs <= 0 {
		return nil
	}
	since := st.lastTimeMs - st.lastProgressMs
	if since < threshold {
		return nil
	}
	gids := make([]int64, 0, len(st.blocked))
	for gid := range st.blocked {
		gids = append(gids, gid)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })
	payload := map[string]any{
		"since_ms":      since,
		"blocked_count": len(gids),
		"blocked_gids":  gids,
	}
	ev := TimelineEvent{
		TimeMs: st.lastTimeMs,
		TimeNs: st.lastTimeNs,
		Event:  "warn_quiescence",
		Source: "audit",
		Value:  payload,
	}
	return &ev
}

func deadlockWindowMs() float64 {
	raw := strings.TrimSpace(os.Getenv("GTV_DEADLOCK_WINDOW_MS"))
	if raw == "" {
		return 1000
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
		return v
	}
	return 1000
}

func (st *ParseState) deadlockCycleEvent() *TimelineEvent {
	if st == nil {
		return nil
	}
	if len(st.blocked) == 0 {
		return nil
	}
	window := deadlockWindowMs()
	graph := make(map[int64][]int64)
	edgeInfo := make(map[string]string) // "g1->g2" -> channelID
	resolveCandidate := func(kind, channelID string) (lastOpInfo, bool, string) {
		if channelID == "" {
			return lastOpInfo{}, false, ""
		}
		var (
			info lastOpInfo
			ok   bool
			id   = channelID
		)
		lookup := func(key string) (lastOpInfo, bool) {
			if key == "" {
				return lastOpInfo{}, false
			}
			if kind == "recv" {
				v, ok := st.lastSendByChannel[key]
				return v, ok
			}
			v, ok := st.lastRecvByChannel[key]
			return v, ok
		}
		info, ok = lookup(channelID)
		if ok {
			return info, true, id
		}
		if _, ch, okParse := parseRegionOp(channelID); okParse && ch != "" {
			if info, ok = lookup(ch); ok {
				return info, true, ch
			}
		}
		if strings.HasPrefix(channelID, "0x") {
			if name, okName := st.channelNameByPtr[channelID]; okName && name != "" {
				info, ok = lookup(name)
				if ok {
					return info, true, name
				}
				if _, ch, okParse := parseRegionOp(name); okParse && ch != "" {
					if info, ok = lookup(ch); ok {
						return info, true, ch
					}
				}
			}
		}
		return lastOpInfo{}, false, ""
	}
	for gid, info := range st.blocked {
		channelID := info.ChannelID
		if channelID == "" {
			continue
		}
		kind := info.Reason
		candidate, ok, edgeLabel := resolveCandidate(kind, channelID)
		if !ok || candidate.G == 0 {
			continue
		}
		if st.lastTimeMs-candidate.TimeMs > window {
			continue
		}
		graph[gid] = append(graph[gid], candidate.G)
		if edgeLabel == "" {
			edgeLabel = channelID
		}
		edgeInfo[fmt.Sprintf("g%d->g%d", gid, candidate.G)] = edgeLabel
	}
	if os.Getenv("GTV_DEBUG_DEADLOCK") == "1" {
		fmt.Fprintf(os.Stderr, "deadlock: graph=%v\n", graph)
	}
	if len(graph) == 0 {
		return nil
	}

	visited := make(map[int64]bool)
	onStack := make(map[int64]bool)
	var stack []int64
	var cycle []int64

	var dfs func(int64) bool
	dfs = func(n int64) bool {
		visited[n] = true
		onStack[n] = true
		stack = append(stack, n)
		for _, next := range graph[n] {
			if !visited[next] {
				if dfs(next) {
					return true
				}
			} else if onStack[next] {
				// build cycle from next to current
				idx := 0
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == next {
						idx = i
						break
					}
				}
				cycle = append([]int64{}, stack[idx:]...)
				cycle = append(cycle, next)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		onStack[n] = false
		return false
	}

	for n := range graph {
		if !visited[n] {
			if dfs(n) {
				break
			}
		}
	}
	if len(cycle) == 0 {
		return nil
	}
	path := make([]string, 0, len(cycle)-1)
	for i := 0; i < len(cycle)-1; i++ {
		from := cycle[i]
		to := cycle[i+1]
		key := fmt.Sprintf("g%d->g%d", from, to)
		ch := edgeInfo[key]
		if ch == "" {
			path = append(path, fmt.Sprintf("g%d -> g%d", from, to))
		} else {
			path = append(path, fmt.Sprintf("g%d -%s-> g%d", from, ch, to))
		}
	}
	payload := map[string]any{
		"cycle_gids": cycle,
		"edges":      path,
	}
	st.deadlockCycle = payload
	ev := TimelineEvent{
		TimeMs: st.lastTimeMs,
		TimeNs: st.lastTimeNs,
		Event:  "warn_deadlock_cycle",
		Source: "audit",
		Value:  payload,
	}
	return &ev
}

func (st *ParseState) auditSummaryEvent(source string) TimelineEvent {
	st.finalizePendingGoroutines()
	counts := make(map[string]any, len(st.auditCounts))
	for k, v := range st.auditCounts {
		counts[k] = v
	}
	if st.deadlockCycle != nil {
		counts["deadlock_cycle"] = st.deadlockCycle
	}
	var pendingSends int64
	var pendingSendChans int64
	unmatchedTotals := UnmatchedTotals{}
	channelAudit := make(map[string]*ChannelUnmatchedAudit)
	for key, q := range st.sendsByKey {
		if len(q) == 0 {
			continue
		}
		pendingSendChans++
		pendingSends += int64(len(q))
		ch := string(key)
		if ch == "" {
			unmatchedTotals.Sends += int64(len(q))
			continue
		}
		entry := channelAudit[ch]
		if entry == nil {
			entry = &ChannelUnmatchedAudit{ChanID: ch}
			channelAudit[ch] = entry
		}
		entry.UnmatchedSends += int64(len(q))
		unmatchedTotals.Sends += int64(len(q))
	}
	if pendingSends > 0 {
		counts["unmatched_send_pending"] = pendingSends
		counts["unmatched_send_channels"] = pendingSendChans
	}
	for ch, n := range st.unmatchedRecvByChannel {
		if ch == "" {
			unmatchedTotals.Recvs += n
			continue
		}
		entry := channelAudit[ch]
		if entry == nil {
			entry = &ChannelUnmatchedAudit{ChanID: ch}
			channelAudit[ch] = entry
		}
		entry.UnmatchedRecvs += n
		unmatchedTotals.Recvs += n
	}
	for key, q := range st.pendingRecvByKey {
		if len(q) == 0 {
			continue
		}
		ch := string(key)
		if ch == "" {
			unmatchedTotals.Recvs += int64(len(q))
			continue
		}
		entry := channelAudit[ch]
		if entry == nil {
			entry = &ChannelUnmatchedAudit{ChanID: ch}
			channelAudit[ch] = entry
		}
		entry.UnmatchedRecvs += int64(len(q))
		unmatchedTotals.Recvs += int64(len(q))
	}
	for ch, n := range st.matchedPairsByChannel {
		if ch == "" {
			continue
		}
		entry := channelAudit[ch]
		if entry == nil {
			entry = &ChannelUnmatchedAudit{ChanID: ch}
			channelAudit[ch] = entry
		}
		entry.MatchedPairs += n
	}
	if len(channelAudit) > 0 {
		keys := make([]string, 0, len(channelAudit))
		for ch := range channelAudit {
			keys = append(keys, ch)
		}
		sort.Strings(keys)
		ordered := make([]ChannelUnmatchedAudit, 0, len(keys))
		for _, ch := range keys {
			ordered = append(ordered, *channelAudit[ch])
		}
		counts["unmatched_by_channel"] = ordered
	}
	unmatchedTotals.UnknownChannelID = st.unknownChannelID
	if unmatchedTotals.Sends > 0 || unmatchedTotals.Recvs > 0 || unmatchedTotals.UnknownChannelID > 0 {
		counts["unmatched_totals"] = unmatchedTotals
	}
	var filteredTotal int64
	if v, ok := counts[auditGoroutineLifecycleFilteredByName].(int64); ok {
		filteredTotal += v
	}
	if v, ok := counts[auditGoroutineLifecycleFilteredByMissingOps].(int64); ok {
		filteredTotal += v
	}
	if filteredTotal > 0 {
		counts["goroutine_filtered_total"] = filteredTotal
	}
	if len(st.unparsedRegionSamples) > 0 {
		counts["unparsed_region_samples"] = st.unparsedRegionSamples
	}
	if len(st.blockedTotalMs) > 0 || len(st.blockedAt) > 0 || len(st.blockedTotalByReason) > 0 {
		type pair struct {
			gid   int64
			total float64
		}
		var items []pair
		reasonTotals := make(map[string]float64, len(st.blockedTotalByReason))
		for reason, total := range st.blockedTotalByReason {
			reasonTotals[reason] = total
		}
		for gid, total := range st.blockedTotalMs {
			items = append(items, pair{gid: gid, total: total})
		}
		for gid, start := range st.blockedAt {
			delta := st.lastTimeMs - start
			if delta < 0 {
				delta = 0
			}
			items = append(items, pair{gid: gid, total: delta + st.blockedTotalMs[gid]})
			reason := st.blockedReasonByG[gid]
			if reason == "" {
				reason = "unknown"
			}
			reasonTotals[reason] += delta
		}
		sort.Slice(items, func(i, j int) bool { return items[i].total > items[j].total })
		top := make(map[string]float64)
		maxBlocked := float64(0)
		for i, item := range items {
			if i >= 5 {
				break
			}
			if item.total > maxBlocked {
				maxBlocked = item.total
			}
			top[fmt.Sprintf("g%d", item.gid)] = item.total
		}
		if maxBlocked > 0 {
			counts["max_blocked_ms"] = maxBlocked
		}
		if len(top) > 0 {
			counts["blocked_total_ms_by_gid"] = top
		}
		if len(reasonTotals) > 0 {
			counts["blocked_total_ms_by_reason"] = reasonTotals
		}
	}
	return TimelineEvent{
		TimeMs: st.lastTimeMs,
		TimeNs: st.lastTimeNs,
		Event:  "audit_summary",
		Source: source,
		Value:  counts,
	}
}

func EmitAuditSummary(st *ParseState, emit func(TimelineEvent) error, source string) error {
	if warn := st.quiescenceWarningEvent(); warn != nil {
		if err := emit(*warn); err != nil {
			return err
		}
	}
	if warn := st.deadlockCycleEvent(); warn != nil {
		if err := emit(*warn); err != nil {
			return err
		}
	}
	return emit(st.auditSummaryEvent(source))
}

func (st *ParseState) newID(prefix string) string {
	id := fmt.Sprintf("%s%d", prefix, st.nextID)
	st.nextID++
	return id
}

func (st *ParseState) markGoroutineInterested(gid int64) {
	st.Interested[gid] = struct{}{}
	delete(st.pendingGoroutines, gid)
}

func (st *ParseState) setRole(gid int64, role string) {
	st.Roles[gid] = role
	switch st.Roles[gid] {
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

func (st *ParseState) ensureRole(gid int64, role string) {
	if _, ok := st.Roles[gid]; !ok {
		st.setRole(gid, role)
	}
}

func (st *ParseState) shouldTrackGoroutine(name string) bool {
	switch st.Opt.GoroutineFilter {
	case GoroutineFilterLegacy:
		return name == "main.main" || strings.Contains(name, "/workload.")
	case GoroutineFilterAll:
		return true
	default:
		return name == "main.main" || strings.Contains(name, "/workload.")
	}
}

func (st *ParseState) roleForName(name, fallback string) string {
	if name == "main.main" {
		return "main"
	}
	if strings.Contains(name, "/workload.") {
		return "worker"
	}
	return fallback
}

func (st *ParseState) hasChannelOps(gid int64) bool {
	if _, ok := st.Active[gid]; ok {
		return true
	}
	if _, ok := st.Last[gid]; ok {
		return true
	}
	if _, ok := st.lastChPtrByG[gid]; ok {
		return true
	}
	if _, ok := st.lastChNameByG[gid]; ok {
		return true
	}
	return false
}

func (st *ParseState) activateGoroutineFromOp(gid int64, emit func(TimelineEvent) error) error {
	if _, ok := st.Interested[gid]; ok {
		return nil
	}
	pending := st.pendingGoroutines[gid]
	switch st.Opt.GoroutineFilter {
	case GoroutineFilterAll:
		st.markGoroutineInterested(gid)
		st.ensureRole(gid, st.roleForName(pending.name, "unknown"))
		return st.emitPendingLifecycle(gid, emit)
	case GoroutineFilterLegacy:
		return nil
	default:
		st.markGoroutineInterested(gid)
		st.ensureRole(gid, st.roleForName(pending.name, "worker"))
		return st.emitPendingLifecycle(gid, emit)
	}
}

func (st *ParseState) emitPendingLifecycle(gid int64, emit func(TimelineEvent) error) error {
	pending, ok := st.pendingGoroutines[gid]
	if !ok {
		return nil
	}
	name := pending.name
	if name == "" {
		name = "?"
	}
	role := st.Roles[gid]
	if pending.hasCreated {
		if err := emit(TimelineEvent{TimeMs: pending.createdAt, Event: "goroutine_created", G: gid, Func: name, Role: role, Source: "state"}); err != nil {
			return err
		}
	}
	if pending.hasStarted {
		if err := emit(TimelineEvent{TimeMs: pending.startedAt, Event: "goroutine_started", G: gid, Func: name, Role: role, Source: "state"}); err != nil {
			return err
		}
		if st.Opt.EmitAtomic {
			var parent int64
			if name != "main.main" {
				parent = st.lastMainG
			}
			_ = emit(TimelineEvent{TimeMs: pending.startedAt, Event: "go_start", ID: st.newID("g"), G: gid, Func: name, ParentG: parent, Role: role, Source: "state"})
		}
	}
	delete(st.pendingGoroutines, gid)
	return nil
}

func (st *ParseState) finalizePendingGoroutines() {
	if st.pendingGoroutinesAudited {
		return
	}
	st.pendingGoroutinesAudited = true
	if st.Opt.GoroutineFilter == GoroutineFilterLegacy {
		st.pendingGoroutines = make(map[int64]pendingGoroutine)
		return
	}
	if st.Opt.GoroutineFilter == GoroutineFilterAll {
		for gid := range st.pendingGoroutines {
			st.markGoroutineInterested(gid)
			st.ensureRole(gid, st.roleForName(st.pendingGoroutines[gid].name, "unknown"))
		}
		st.pendingGoroutines = make(map[int64]pendingGoroutine)
		return
	}
	for gid := range st.pendingGoroutines {
		if st.hasChannelOps(gid) {
			st.markGoroutineInterested(gid)
			st.ensureRole(gid, st.roleForName(st.pendingGoroutines[gid].name, "worker"))
			continue
		}
		st.incAudit(auditGoroutineLifecycleFilteredByMissingOps)
	}
	st.pendingGoroutines = make(map[int64]pendingGoroutine)
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
	// Allow a small window for adjacent ch_ptr/ch_name logs before a region begins.
	if st.eventIndexByG[gid]-seq <= 6 {
		delete(st.lastChPtrByG, gid)
		delete(st.lastChPtrSeqByG, gid)
		return ptr
	}
	delete(st.lastChPtrByG, gid)
	delete(st.lastChPtrSeqByG, gid)
	return ""
}

func (st *ParseState) peekFreshChPtr(gid int64) string {
	ptr, ok := st.lastChPtrByG[gid]
	if !ok {
		return ""
	}
	seq := st.lastChPtrSeqByG[gid]
	if seq == 0 {
		return ptr
	}
	if st.eventIndexByG[gid]-seq <= 6 {
		return ptr
	}
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
	// Allow a small window for adjacent ch_ptr/ch_name logs before a region begins.
	if st.eventIndexByG[gid]-seq <= 6 {
		delete(st.lastChNameByG, gid)
		delete(st.lastChNameSeqByG, gid)
		return name
	}
	delete(st.lastChNameByG, gid)
	delete(st.lastChNameSeqByG, gid)
	return ""
}

func (st *ParseState) peekFreshChName(gid int64) string {
	name, ok := st.lastChNameByG[gid]
	if !ok {
		return ""
	}
	seq := st.lastChNameSeqByG[gid]
	if seq == 0 {
		return name
	}
	if st.eventIndexByG[gid]-seq <= 6 {
		return name
	}
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
	// Instrumenter wraps close(ch) with a small region for readability.
	// Traceproc already emits a dedicated "chan_close" event from logs, so treat
	// this region as recognized-but-non-operational to avoid audit noise.
	if lt == "chan.close" || lt == "chan_close" || strings.HasPrefix(lt, "chan.close ") {
		return "chan_close", "", true
	}
	if strings.HasPrefix(lt, "mutex.") {
		parts := strings.Fields(lt)
		if len(parts) > 0 {
			action := strings.TrimPrefix(parts[0], "mutex.")
			if action == "lock" || action == "unlock" {
				label := ""
				if len(parts) > 1 {
					label = parts[1]
				}
				if label == "" {
					ch = "mutex"
				} else {
					ch = "mutex:" + label
				}
				return "mutex_" + action, ch, true
			}
		}
	}
	if strings.HasPrefix(lt, "rwmutex.") {
		parts := strings.Fields(lt)
		if len(parts) > 0 {
			action := strings.TrimPrefix(parts[0], "rwmutex.")
			if action == "lock" || action == "unlock" || action == "rlock" || action == "runlock" {
				label := ""
				if len(parts) > 1 {
					label = parts[1]
				}
				if label == "" {
					ch = "rwmutex"
				} else {
					ch = "rwmutex:" + label
				}
				return "rwmutex_" + action, ch, true
			}
		}
	}
	if strings.HasPrefix(lt, "wg.") {
		parts := strings.Fields(lt)
		if len(parts) > 0 {
			action := strings.TrimPrefix(parts[0], "wg.")
			if action == "add" || action == "done" || action == "wait" {
				label := ""
				if len(parts) > 1 {
					label = parts[1]
				}
				if label == "" {
					ch = "wg"
				} else {
					ch = "wg:" + label
				}
				return "wg_" + action, ch, true
			}
		}
	}
	if strings.HasPrefix(lt, "cond.") {
		parts := strings.Fields(lt)
		if len(parts) > 0 {
			action := strings.TrimPrefix(parts[0], "cond.")
			if action == "wait" || action == "signal" || action == "broadcast" {
				label := ""
				if len(parts) > 1 {
					label = parts[1]
				}
				if label == "" {
					ch = "cond"
				} else {
					ch = "cond:" + label
				}
				return "cond_" + action, ch, true
			}
		}
	}
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
	if strings.Contains(lt, "recv") {
		if i := strings.LastIndex(lt, " from "); i >= 0 {
			rest := strings.TrimSpace(lt[i+6:])
			if rest != "" {
				ch = strings.Fields(rest)[0]
				return "recv", ch, true
			}
		}
		if i := strings.LastIndex(lt, "recv "); i >= 0 {
			rest := strings.TrimSpace(lt[i+5:])
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
		if i := strings.LastIndex(lt, " on "); i >= 0 {
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

func (st *ParseState) setSyncWaitCandidate(gid int64, kind, ch string, tsMs float64) {
	if !isSyncWaitKind(kind) {
		return
	}
	ck := parseChKey(ch)
	channelKey := ""
	if ch != "" {
		channelKey = st.channelKey(ck, "")
	}
	st.syncWaitCandidateByG[gid] = syncWaitCandidate{
		Kind:       kind,
		Channel:    ch,
		ChannelKey: channelKey,
		StartMs:    tsMs,
	}
}

func (st *ParseState) clearSyncWaitCandidate(gid int64, kind string) {
	cur, ok := st.syncWaitCandidateByG[gid]
	if !ok {
		return
	}
	if kind != "" && cur.Kind != kind {
		return
	}
	delete(st.syncWaitCandidateByG, gid)
}

func (st *ParseState) inferSyncWaitContext(gid int64, runtimeReason string) (kind, ch, chKey, confidence string) {
	if cand, ok := st.syncWaitCandidateByG[gid]; ok {
		return cand.Kind, cand.Channel, cand.ChannelKey, "high"
	}
	kind = syncWaitKindFromRuntimeReason(runtimeReason)
	if kind == "" {
		return "", "", "", ""
	}
	return kind, "", "", "medium"
}

func isSyncRegionKind(kind string) bool {
	return strings.HasPrefix(kind, "mutex_") ||
		strings.HasPrefix(kind, "rwmutex_") ||
		strings.HasPrefix(kind, "wg_")
}

func parseChNameMessage(msg string) (ptr, name string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", ""
	}
	if i := strings.Index(msg, "ptr="); i >= 0 {
		rest := strings.TrimSpace(msg[i+4:])
		if fields := strings.Fields(rest); len(fields) > 0 {
			ptr = fields[0]
		}
	}
	if i := strings.Index(msg, "name="); i >= 0 {
		name = strings.TrimSpace(msg[i+5:])
		name = strings.Trim(name, "\"'")
	}
	if name != "" {
		if _, ch, ok := parseRegionOp(name); ok && ch != "" {
			name = ch
		}
	}
	return ptr, name
}

func (st *ParseState) handleRegionEnd(kind string, op op, gid int64, role string, tsMs float64, emit func(TimelineEvent) error) {
	if strings.HasPrefix(kind, "cond_") {
		st.clearSyncWaitCandidate(gid, kind)
		return
	}
	if isSyncRegionKind(kind) {
		st.clearSyncWaitCandidate(gid, kind)
		return
	}
	// Attach any stashed value to the completion event
	val, hasVal := st.lastValue[gid]
	if hasVal {
		delete(st.lastValue, gid)
	}
	// Temporarily wrap emit to include value for legacy event
	if hasVal {
		origEmit := emit
		emit = func(ev TimelineEvent) error { ev.Value = val; return origEmit(ev) }
		_ = st.handleRegionOp(kind, op.ch, op.ptr, gid, role, tsMs, emit)
		emit = func(ev TimelineEvent) error { return origEmit(ev) }
	} else {
		_ = st.handleRegionOp(kind, op.ch, op.ptr, gid, role, tsMs, emit)
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
			channelID := st.channelKey(ck, chPtr)
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: cev, ID: cid, AttemptID: att.id, G: gid, Channel: att.ch, ChannelKey: channelID, ChPtr: chPtr, MissingPtr: chPtr == "", PeerG: peerG, PairID: pairID, DeltaMs: tsMs - att.time, Role: role, Source: "region"})
			st.recordChannelActivity("recv", gid, channelID, tsMs)
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
			channelID := st.channelKey(ck, chPtr)
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: cev, ID: cid, AttemptID: att.id, G: gid, Channel: att.ch, ChannelKey: channelID, ChPtr: chPtr, MissingPtr: chPtr == "", PairID: att.id, DeltaMs: tsMs - att.time, Role: role, Source: "region"})
			st.recordChannelActivity("send", gid, channelID, tsMs)
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

// ProcessEvent translates a trace Event into zero or more TimelineEvents via emit.
func ProcessEvent(e *xtrace.Event, st *ParseState, emit func(TimelineEvent) error) error {
	tsMs := float64(e.Time()) / 1e6
	tsNs := int64(e.Time())
	gid := int64(e.Goroutine())
	st.lastTimeMs = tsMs
	st.lastTimeNs = tsNs
	if st.lastProgressMs == 0 {
		st.lastProgressMs = tsMs
	}
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
		// Treat ch_ptr as metadata only (for channel attribution), not as a timeline event.
		// This avoids event explosion in hot loops/select evaluation.
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
				st.setRole(gid, msg)
				st.markGoroutineInterested(gid)
			}
		}
		if l.Category == "select_recv" || l.Category == "select_send" {
			ptr, name := parseChNameMessage(strings.TrimSpace(l.Message))
			if name != "" && ptr != "" {
				st.channelNameByPtr[ptr] = name
			}
			if ptr != "" {
				st.lastChPtrByG[gid] = ptr
				st.lastChPtrSeqByG[gid] = st.eventIndexByG[gid]
			}
			if name != "" {
				st.lastChNameByG[gid] = name
				st.lastChNameSeqByG[gid] = st.eventIndexByG[gid]
			}
			ck := parseChKey(name)
			if name == "" && ptr != "" {
				ck = parseChKey(ptr)
			}
			chKey := st.channelKey(ck, ptr)
			evName := "recv_complete"
			chanName := "chan_recv"
			if l.Category == "select_send" {
				evName = "send_complete"
				chanName = "chan_send"
			}
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: evName, Channel: name, ChannelKey: chKey, ChPtr: ptr, G: gid, Role: st.Roles[gid], Source: "select"})
			_ = emit(TimelineEvent{TimeMs: tsMs, Event: chanName, Channel: name, ChannelKey: chKey, ChPtr: ptr, G: gid, Role: st.Roles[gid], Source: "select"})
			if sid := st.activeSelect[gid]; sid != "" && (ptr != "" || name != "") {
				st.selectEmitted[sid] = true
			}
			return nil
		}
		if l.Category == "main" || l.Category == "worker" || l.Category == "server" || l.Category == "client" {
			st.setRole(gid, l.Category)
			st.markGoroutineInterested(gid)
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
		} else {
			if !strings.HasPrefix(strings.ToLower(rgn.Type), "goroutine:") {
				st.incAudit(auditUnparsedRegionOp)
				st.noteUnparsedRegion(rgn.Type)
			}
		}

	case xtrace.EventRegionEnd:
		rgn := e.Region()
		gid := int64(e.Goroutine())
		regionKind, _, regionKnown := parseRegionOp(rgn.Type)
		if regionKnown && (isSyncWaitKind(regionKind) || strings.HasPrefix(regionKind, "cond_")) {
			st.clearSyncWaitCandidate(gid, regionKind)
		}
		if op, ok := st.Active[gid]; ok {
			if regionKnown && regionKind == op.kind {
				st.handleRegionEnd(regionKind, op, gid, st.Roles[gid], tsMs, emit)
			} else {
				if !strings.HasPrefix(strings.ToLower(rgn.Type), "goroutine:") {
					st.incAudit(auditUnparsedRegionOp)
					st.noteUnparsedRegion(rgn.Type)
				}
				if st.Opt.EmitAtomic {
					if att, ok := st.attemptsByG[gid]; ok {
						fallback := op
						if fallback.ch == "" {
							fallback.ch = att.ch
						}
						if fallback.ptr == "" {
							fallback.ptr = att.ptr
						}
						st.handleRegionEnd(att.kind, fallback, gid, st.Roles[gid], tsMs, emit)
					}
				}
			}
		}

	case xtrace.EventStateTransition:
		s := e.StateTransition()
		if s.Resource.Kind != xtrace.ResourceGoroutine {
			return nil
		}
		gid := int64(s.Resource.Goroutine())
		oldState, newState := s.Goroutine()

		// get top frame func name + file/line if available
		fn := ""
		file := ""
		line := 0
		stck := s.Stack
		if stck != xtrace.NoStack {
			for frame := range stck.Frames() {
				fn = frame.Func
				file = frame.File
				line = int(frame.Line)
				break
			}
		}
		if fn == "" {
			fn = "?"
		}

		// Creation: track goroutine creation for known workload packages
		if oldState == xtrace.GoNotExist && (newState == xtrace.GoRunnable || newState == xtrace.GoWaiting) {
			if st.Opt.GoroutineFilter == GoroutineFilterAll {
				st.markGoroutineInterested(gid)
				st.ensureRole(gid, st.roleForName(fn, "unknown"))
				return emit(TimelineEvent{TimeMs: tsMs, Event: "goroutine_created", G: gid, Func: fn, File: file, Line: line, Role: st.Roles[gid], Source: "state"})
			}
			if st.shouldTrackGoroutine(fn) {
				st.markGoroutineInterested(gid)
				st.ensureRole(gid, st.roleForName(fn, "worker"))
				return emit(TimelineEvent{TimeMs: tsMs, Event: "goroutine_created", G: gid, Func: fn, File: file, Line: line, Role: st.Roles[gid], Source: "state"})
			}
			pending := st.pendingGoroutines[gid]
			pending.name = fn
			pending.createdAt = tsMs
			pending.hasCreated = true
			st.pendingGoroutines[gid] = pending
			if st.Opt.GoroutineFilter == GoroutineFilterLegacy {
				st.incAudit(auditGoroutineLifecycleFilteredByName)
			}
			return nil
		}

		// Start: record and track main and pingpong goroutines
		if oldState == xtrace.GoRunnable && newState == xtrace.GoRunning {
			name := fn
			if st.Opt.GoroutineFilter == GoroutineFilterAll {
				st.markGoroutineInterested(gid)
				st.ensureRole(gid, st.roleForName(name, "unknown"))
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "goroutine_started", G: gid, Func: name, File: file, Line: line, Role: st.Roles[gid], Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					// Best-effort parent inference: main → worker
					var parent int64
					if name != "main.main" {
						parent = st.lastMainG
					}
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "go_start", ID: st.newID("g"), G: gid, Func: name, File: file, Line: line, ParentG: parent, Role: st.Roles[gid], Source: "state"})
				}
				return nil
			}
			if st.shouldTrackGoroutine(name) {
				st.markGoroutineInterested(gid)
				st.ensureRole(gid, st.roleForName(name, "worker"))
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "goroutine_started", G: gid, Func: name, File: file, Line: line, Role: st.Roles[gid], Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					// Best-effort parent inference: main → worker
					var parent int64
					if name != "main.main" {
						parent = st.lastMainG
					}
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "go_start", ID: st.newID("g"), G: gid, Func: name, File: file, Line: line, ParentG: parent, Role: st.Roles[gid], Source: "state"})
				}
				return nil
			}
			pending := st.pendingGoroutines[gid]
			pending.name = name
			pending.startedAt = tsMs
			pending.hasStarted = true
			st.pendingGoroutines[gid] = pending
			if st.Opt.GoroutineFilter == GoroutineFilterLegacy {
				st.incAudit(auditGoroutineLifecycleFilteredByName)
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
				ch, chPtr, chKey, confidence := st.inferBlockChannel(gid, "send", tsMs)
				blockCause, blockCap, blockLen := st.blockCauseFor("send", chPtr)
				if ch == "" && st.Opt.DropBlockNoCh {
					st.incAudit(auditBlockedDropNoChannelSend)
					return nil
				}
				channelID := ""
				if chPtr != "" {
					channelID = chKey
				} else if chKey != "" {
					channelID = chKey
				}
				if channelID == "" {
					st.incAudit(auditMissingBlockChannel)
				}
				if err := emit(TimelineEvent{
					TimeMs:     tsMs,
					Event:      "go_block",
					G:          gid,
					Role:       st.Roles[gid],
					Channel:    ch,
					ChannelKey: chKey,
					ChPtr:      chPtr,
					BlockKind:  "chan_send",
					BlockCause: blockCause,
					BlockCap:   blockCap,
					BlockLen:   blockLen,
					ChannelID:  channelID,
					Confidence: confidence,
					Source:     "state",
				}); err != nil {
					return err
				}
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "blocked_send", G: gid, Role: st.Roles[gid], Channel: ch, ChannelKey: chKey, ChPtr: chPtr, BlockCause: blockCause, BlockCap: blockCap, BlockLen: blockLen, Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "block", G: gid, Role: st.Roles[gid], Channel: ch, ChannelKey: chKey, ChPtr: chPtr, Reason: "send", BlockCause: blockCause, BlockCap: blockCap, BlockLen: blockLen, Source: "state"})
				}
				st.blocked[gid] = struct{ Reason, Ch, ChannelID, Confidence string }{
					Reason:     "send",
					Ch:         ch,
					ChannelID:  channelID,
					Confidence: confidence,
				}
				if _, ok := st.blockedAt[gid]; !ok {
					st.blockedAt[gid] = tsMs
					st.blockedReasonByG[gid] = "chan_send"
				}
				return nil
			case "chan receive":
				ch, chPtr, chKey, confidence := st.inferBlockChannel(gid, "recv", tsMs)
				blockCause, blockCap, blockLen := st.blockCauseFor("recv", chPtr)
				if ch == "" && st.Opt.DropBlockNoCh {
					st.incAudit(auditBlockedDropNoChannelRecv)
					return nil
				}
				channelID := ""
				if chPtr != "" {
					channelID = chKey
				} else if chKey != "" {
					channelID = chKey
				}
				if channelID == "" {
					st.incAudit(auditMissingBlockChannel)
				}
				if err := emit(TimelineEvent{
					TimeMs:     tsMs,
					Event:      "go_block",
					G:          gid,
					Role:       st.Roles[gid],
					Channel:    ch,
					ChannelKey: chKey,
					ChPtr:      chPtr,
					BlockKind:  "chan_recv",
					BlockCause: blockCause,
					BlockCap:   blockCap,
					BlockLen:   blockLen,
					ChannelID:  channelID,
					Confidence: confidence,
					Source:     "state",
				}); err != nil {
					return err
				}
				if err := emit(TimelineEvent{TimeMs: tsMs, Event: "blocked_receive", G: gid, Role: st.Roles[gid], Channel: ch, ChannelKey: chKey, ChPtr: chPtr, BlockCause: blockCause, BlockCap: blockCap, BlockLen: blockLen, Source: "state"}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					_ = emit(TimelineEvent{TimeMs: tsMs, Event: "block", G: gid, Role: st.Roles[gid], Channel: ch, ChannelKey: chKey, ChPtr: chPtr, Reason: "recv", BlockCause: blockCause, BlockCap: blockCap, BlockLen: blockLen, Source: "state"})
				}
				st.blocked[gid] = struct{ Reason, Ch, ChannelID, Confidence string }{
					Reason:     "recv",
					Ch:         ch,
					ChannelID:  channelID,
					Confidence: confidence,
				}
				if _, ok := st.blockedAt[gid]; !ok {
					st.blockedAt[gid] = tsMs
					st.blockedReasonByG[gid] = "chan_recv"
				}
				return nil
			default:
				blockKind, ch, chKey, confidence := st.inferSyncWaitContext(gid, s.Reason)
				if blockKind == "" {
					return nil
				}
				channelID := chKey
				if err := emit(TimelineEvent{
					TimeMs:     tsMs,
					Event:      "go_block",
					G:          gid,
					Role:       st.Roles[gid],
					Channel:    ch,
					ChannelKey: chKey,
					BlockKind:  blockKind,
					ChannelID:  channelID,
					Confidence: confidence,
					Source:     "state",
				}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					_ = emit(TimelineEvent{
						TimeMs:     tsMs,
						Event:      "block",
						G:          gid,
						Role:       st.Roles[gid],
						Channel:    ch,
						ChannelKey: chKey,
						Reason:     blockKind,
						Source:     "state",
					})
				}
				st.blocked[gid] = struct{ Reason, Ch, ChannelID, Confidence string }{
					Reason:     blockKind,
					Ch:         ch,
					ChannelID:  channelID,
					Confidence: confidence,
				}
				if _, ok := st.blockedAt[gid]; !ok {
					start := tsMs
					if cand, ok := st.syncWaitCandidateByG[gid]; ok && cand.Kind == blockKind && cand.StartMs > 0 && cand.StartMs <= tsMs {
						start = cand.StartMs
					}
					st.blockedAt[gid] = start
					st.blockedReasonByG[gid] = blockKind
				}
				return nil
			}
		}

		// Unblock
		if oldState == xtrace.GoWaiting && newState == xtrace.GoRunnable {
			if _, ok := st.Interested[gid]; ok {
				info := st.blocked[gid]
				if info.Reason == "" {
					if cand, ok := st.syncWaitCandidateByG[gid]; ok {
						info.Reason = cand.Kind
						if info.Ch == "" {
							info.Ch = cand.Channel
						}
						if info.ChannelID == "" {
							info.ChannelID = cand.ChannelKey
						}
						if info.Confidence == "" {
							info.Confidence = "high"
						}
					} else if kind, ch, chKey, conf := st.inferSyncWaitContext(gid, s.Reason); kind != "" {
						info.Reason = kind
						info.Ch = ch
						info.ChannelID = chKey
						info.Confidence = conf
					}
				}
				ch := info.Ch
				chPtr := ""
				chKey := info.ChannelID
				confidence := info.Confidence
				if info.Reason == "send" || info.Reason == "recv" {
					kind := info.Reason
					ch, chPtr, chKey, confidence = st.inferBlockChannel(gid, kind, tsMs)
				}
				if info.ChannelID != "" {
					chKey = info.ChannelID
					if ch == "" {
						ch = info.Ch
					}
				}
				channelID := ""
				if chPtr != "" {
					channelID = chKey
				} else if chKey != "" {
					channelID = chKey
				}
				unblockKind := unblockKindFromReason(info.Reason)
				if channelID == "" && (unblockKind == "chan_send" || unblockKind == "chan_recv") {
					st.incAudit(auditMissingBlockChannel)
				}
				if err := emit(TimelineEvent{
					TimeMs:      tsMs,
					Event:       "go_unblock",
					G:           gid,
					Role:        st.Roles[gid],
					Channel:     ch,
					ChannelKey:  chKey,
					ChPtr:       chPtr,
					UnblockKind: unblockKind,
					ChannelID:   channelID,
					Confidence:  confidence,
					Source:      "state",
				}); err != nil {
					return err
				}
				if st.Opt.EmitAtomic {
					if err := emit(TimelineEvent{TimeMs: tsMs, Event: "unblock", G: gid, Role: st.Roles[gid], Channel: ch, ChannelKey: chKey, ChPtr: chPtr, Reason: info.Reason, Source: "state"}); err != nil {
						return err
					}
				} else {
					if err := emit(TimelineEvent{TimeMs: tsMs, Event: "unblocked", G: gid, Role: st.Roles[gid], Channel: ch, ChannelKey: chKey, ChPtr: chPtr, Source: "state"}); err != nil {
						return err
					}
				}
				if _, ok := st.blockedAt[gid]; !ok {
					if cand, ok := st.syncWaitCandidateByG[gid]; ok && cand.StartMs > 0 && cand.StartMs <= tsMs {
						st.blockedAt[gid] = cand.StartMs
						if info.Reason != "" {
							st.blockedReasonByG[gid] = info.Reason
						} else {
							st.blockedReasonByG[gid] = cand.Kind
						}
					}
				}
				if start, ok := st.blockedAt[gid]; ok {
					reasonKey := st.blockedReasonByG[gid]
					if reasonKey == "" && info.Reason != "" {
						reasonKey = info.Reason
					}
					st.addBlockedDuration(gid, reasonKey, tsMs-start)
					delete(st.blockedAt, gid)
				}
				delete(st.blockedReasonByG, gid)
				delete(st.blocked, gid)
				st.clearSyncWaitCandidate(gid, "")
				st.markProgress(tsMs)
				return nil
			}
			return nil
		}
	}
	return nil
}

func (st *ParseState) handleRegionBegin(kind, ch string, gid int64, role string, ts float64, emit func(TimelineEvent) error) error {
	if strings.HasPrefix(kind, "cond_") {
		if key := parsedRegionAuditKey(kind); key != "" {
			st.incAudit(key)
		}
		st.setSyncWaitCandidate(gid, kind, ch, ts)
		if err := st.activateGoroutineFromOp(gid, emit); err != nil {
			return err
		}
		event := TimelineEvent{TimeMs: ts, Event: kind, Channel: ch, ChannelKey: ch, G: gid, Role: role, Source: "cond"}
		if kind == "cond_wait" {
			event.UnknownSender = true
		} else {
			event.UnknownReceiver = true
		}
		return emit(event)
	}
	if isSyncRegionKind(kind) {
		if key := parsedRegionAuditKey(kind); key != "" {
			st.incAudit(key)
		}
		st.setSyncWaitCandidate(gid, kind, ch, ts)
		if err := st.activateGoroutineFromOp(gid, emit); err != nil {
			return err
		}
		ck := parseChKey(ch)
		channelKey := ""
		if ch != "" {
			channelKey = st.channelKey(ck, "")
		}
		ev := TimelineEvent{TimeMs: ts, Event: kind, Channel: ch, G: gid, Role: role, Source: "sync"}
		if channelKey != "" {
			ev.ChannelKey = channelKey
		}
		return emit(ev)
	}
	if kind == "chan_close" {
		return nil
	}
	if kind == "send" {
		// Avoid interpreting select-clause body regions ("send to X") as channel ops.
		// The actual select comm is captured via select logs; these regions are for readability.
		// Keep a tight window so we don't suppress unrelated sends in select bodies.
		// Note: we still consume ch_ptr/ch_name below to keep attribution state consistent.
	}
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
	if chPtr != "" && ch != "" {
		st.channelNameByPtr[chPtr] = ch
	}

	if kind == "send" || kind == "recv" {
		if last, ok := st.lastSelectChosenByG[gid]; ok {
			lastSeq := st.lastSelectChosenSeqByG[gid]
			if lastSeq != 0 && st.eventIndexByG[gid]-lastSeq <= 3 {
				// Prefer ptr match when available.
				match := false
				if chPtr != "" && last.ChPtr != "" && chPtr == last.ChPtr {
					match = true
				} else if ch != "" && last.ChName != "" && strings.EqualFold(ch, last.ChName) {
					match = true
				}
				if match && last.Kind == kind {
					return nil
				}
			}
		}
	}

	if kind == "send" {
		st.incAudit(auditParsedRegionSend)
	}
	if kind == "recv" {
		st.incAudit(auditParsedRegionRecv)
	}
	st.Active[gid] = op{kind: kind, ch: ch, ptr: chPtr, time: ts}
	st.Last[gid] = st.Active[gid]
	if err := st.activateGoroutineFromOp(gid, emit); err != nil {
		return err
	}
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

func blockInferenceWindowMs() float64 {
	raw := strings.TrimSpace(os.Getenv("GTV_BLOCK_INFER_MS"))
	if raw == "" {
		return 20
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
		return v
	}
	return 20
}

func (st *ParseState) inferBlockChannel(gid int64, kind string, tsMs float64) (string, string, string, string) {
	window := blockInferenceWindowMs()
	tryOp := func(op op) (string, string, string, string, bool) {
		if kind != "" && op.kind != kind {
			return "", "", "", "", false
		}
		late := op.time > 0 && tsMs-op.time > window
		ch := op.ch
		ptr := op.ptr
		if ch == "" && ptr != "" {
			if mapped, ok := st.channelNameByPtr[ptr]; ok {
				ch = mapped
			}
		}
		ck := parseChKey(ch)
		if ck.Name == "" && ptr != "" {
			ck = parseChKey(ptr)
		}
		chKey := st.channelKey(ck, ptr)
		conf := "low"
		if ptr != "" {
			conf = "high"
		}
		if late {
			conf = "low"
		}
		return ch, ptr, chKey, conf, ch != "" || ptr != ""
	}
	if op, ok := st.Active[gid]; ok {
		if ch, ptr, chKey, conf, ok := tryOp(op); ok {
			return ch, ptr, chKey, conf
		}
	}
	if op, ok := st.Last[gid]; ok {
		if ch, ptr, chKey, conf, ok := tryOp(op); ok {
			return ch, ptr, chKey, conf
		}
	}

	// Low-confidence fallback: recent ch_ptr/ch_name logs.
	ptr := st.peekFreshChPtr(gid)
	ch := st.peekFreshChName(gid)
	if ch == "" && ptr != "" {
		if mapped, ok := st.channelNameByPtr[ptr]; ok {
			ch = mapped
		}
	}
	ck := parseChKey(ch)
	if ck.Name == "" && ptr != "" {
		ck = parseChKey(ptr)
	}
	chKey := st.channelKey(ck, ptr)
	if ch == "" && ptr == "" {
		return "", "", "", "low"
	}
	return ch, ptr, chKey, "low"
}

func (st *ParseState) blockCauseFor(kind, chPtr string) (string, int, int) {
	if chPtr == "" {
		return "unknown", 0, 0
	}
	capVal := st.capByPtr[chPtr]
	lenVal := st.depthByPtr[chPtr]
	if capVal > 0 {
		if kind == "send" {
			if lenVal >= capVal {
				return "buffer_full", capVal, lenVal
			}
		}
		if kind == "recv" {
			if lenVal <= 0 {
				return "buffer_empty", capVal, lenVal
			}
		}
	}
	if kind == "send" {
		return "no_receiver", capVal, lenVal
	}
	if kind == "recv" {
		return "no_sender", capVal, lenVal
	}
	return "unknown", capVal, lenVal
}

func (st *ParseState) handleChanOp(ev, ch string, gid int64, role string, t float64, val *int, source string, emit func(TimelineEvent) error) error {
	if err := st.activateGoroutineFromOp(gid, emit); err != nil {
		return err
	}
	st.markProgress(t)
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
			st.incAudit(auditSynthSend)
			st.incAudit(auditRetroSendEmit)
			st.hasSend[k] = true
		}
	}
	// Emit requested
	channelID := st.channelKey(ck, "")
	te := TimelineEvent{TimeMs: t, Event: ev, Channel: ch, ChannelKey: channelID, G: gid, Role: role, Value: val, Source: source}
	if err := emit(te); err != nil {
		return err
	}
	switch strings.ToLower(ev) {
	case "chan_send", "send_complete":
		st.recordChannelActivity("send", gid, channelID, t)
	case "chan_recv", "recv_complete":
		st.recordChannelActivity("recv", gid, channelID, t)
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

type pendingRecvRecord struct {
	TimeMs float64
	G      int64
	Role   string
	Ch     string
	Ptr    string
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

func (st *ParseState) findSendQueueByName(key chKey) (OpKey, []sendRecord, bool) {
	name := key.Name
	if key.ClientID >= 0 {
		name = fmt.Sprintf("%s[%d]", key.Name, key.ClientID)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", nil, false
	}
	var bestKey OpKey
	var bestQ []sendRecord
	bestTime := -1.0
	for k, q := range st.sendsByKey {
		if len(q) == 0 {
			continue
		}
		match := false
		ks := string(k)
		if strings.ToLower(ks) == name {
			match = true
		} else if strings.HasPrefix(ks, "0x") {
			if nm, ok := st.channelNameByPtr[ks]; ok && strings.ToLower(nm) == name {
				match = true
			}
		}
		if !match {
			continue
		}
		if bestTime < 0 || q[0].TimeMs < bestTime {
			bestTime = q[0].TimeMs
			bestKey = k
			bestQ = q
		}
	}
	if bestTime < 0 {
		return "", nil, false
	}
	return bestKey, bestQ, true
}

func (st *ParseState) resolvePendingSelectRecv(key chKey, chPtr string) (OpKey, bool) {
	opKey := st.opKey(key, chPtr)
	if st.pendingSelectRecv[opKey] > 0 {
		return opKey, true
	}
	name := key.Name
	if key.ClientID >= 0 {
		name = fmt.Sprintf("%s[%d]", key.Name, key.ClientID)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", false
	}
	for k, n := range st.pendingSelectRecv {
		if n <= 0 {
			continue
		}
		ks := string(k)
		if strings.ToLower(ks) == name {
			return k, true
		}
		if strings.HasPrefix(ks, "0x") {
			if nm, ok := st.channelNameByPtr[ks]; ok && strings.ToLower(nm) == name {
				return k, true
			}
		}
	}
	return "", false
}

func (st *ParseState) popPendingRecv(key chKey, chPtr string) (OpKey, pendingRecvRecord, bool) {
	opKey := st.opKey(key, chPtr)
	if q := st.pendingRecvByKey[opKey]; len(q) > 0 {
		rec := q[0]
		st.pendingRecvByKey[opKey] = q[1:]
		return opKey, rec, true
	}
	name := key.Name
	if key.ClientID >= 0 {
		name = fmt.Sprintf("%s[%d]", key.Name, key.ClientID)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", pendingRecvRecord{}, false
	}
	for k, q := range st.pendingRecvByKey {
		if len(q) == 0 {
			continue
		}
		ks := string(k)
		if strings.ToLower(ks) == name {
			rec := q[0]
			st.pendingRecvByKey[k] = q[1:]
			return k, rec, true
		}
		if strings.HasPrefix(ks, "0x") {
			if nm, ok := st.channelNameByPtr[ks]; ok && strings.ToLower(nm) == name {
				rec := q[0]
				st.pendingRecvByKey[k] = q[1:]
				return k, rec, true
			}
		}
	}
	return "", pendingRecvRecord{}, false
}

// handleRegionOp processes the end of a region send/recv and pairs operations by channel key.
func (st *ParseState) handleRegionOp(kind, ch, chPtr string, gid int64, role string, t float64, emit func(TimelineEvent) error) error {
	// Always emit the *_complete event for timelines
	ev := "send_complete"
	if kind == "recv" {
		ev = "recv_complete"
	}
	key := parseChKey(ch)
	channelID := st.channelKey(key, chPtr)
	if channelID == "" {
		channelID = key.Name
	}
	if err := emit(TimelineEvent{TimeMs: t, Event: ev, Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "region"}); err != nil {
		return err
	}
	switch kind {
	case "send":
		if channelID == "" {
			st.unknownChannelID++
		}
		mid := st.nextMsgID
		st.nextMsgID++
		if err := emit(TimelineEvent{TimeMs: t, Event: "chan_send", Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "paired", PeerG: 0, MsgID: mid}); err != nil {
			return err
		}
		if _, rec, ok := st.popPendingRecv(key, chPtr); ok {
			if channelID == "" {
				st.unknownChannelID++
			} else {
				st.matchedPairsByChannel[channelID]++
			}
			_ = emit(TimelineEvent{
				TimeMs:     rec.TimeMs,
				Event:      "chan_recv",
				Channel:    rec.Ch,
				ChannelKey: st.channelKey(key, rec.Ptr),
				ChPtr:      rec.Ptr,
				G:          rec.G,
				Role:       rec.Role,
				Source:     "paired",
				PeerG:      gid,
				MsgID:      mid,
			})
			st.recordChannelActivity("recv", rec.G, channelID, rec.TimeMs)
		} else if pendKey, ok := st.resolvePendingSelectRecv(key, chPtr); ok {
			st.pendingSelectRecv[pendKey]--
			if channelID == "" {
				st.unknownChannelID++
			} else {
				st.matchedPairsByChannel[channelID]++
			}
		} else {
			st.sendsByKey[st.opKey(key, chPtr)] = append(st.sendsByKey[st.opKey(key, chPtr)], sendRecord{TimeMs: t, G: gid, Role: role, Ptr: chPtr, MsgID: mid})
		}
	case "recv":
		origPtr := chPtr
		opKey := st.opKey(key, chPtr)
		q := st.sendsByKey[opKey]
		if len(q) == 0 && chPtr == "" {
			if altKey, altQ, ok := st.findSendQueueByName(key); ok {
				opKey = altKey
				q = altQ
				if len(q) > 0 && chPtr == "" {
					chPtr = q[0].Ptr
				}
			}
		}
		if chPtr != origPtr {
			channelID = st.channelKey(key, chPtr)
			if channelID == "" {
				channelID = key.Name
			}
		}
		if len(q) == 0 {
			st.pendingRecvByKey[opKey] = append(st.pendingRecvByKey[opKey], pendingRecvRecord{
				TimeMs: t,
				G:      gid,
				Role:   role,
				Ch:     ch,
				Ptr:    chPtr,
			})
			// Prefer not to synthesize for broadcast channels we instrument at both ends.
			// This avoids duplicate edges on join[i], clientin, clientout[i].
			lname := key.Name
			if lname == "clientin" || strings.HasPrefix(lname, "clientout") || strings.HasPrefix(lname, "join") {
				st.incAudit(auditSkipPairingHardcoded)
				return nil
			}
			return nil
		}
		// Match with the earliest recorded send
		sr := q[0]
		st.sendsByKey[opKey] = q[1:]
		if shouldSkipPairing(key) {
			st.incAudit(auditSkipPairingEnv)
			return nil
		}
		if channelID == "" {
			st.unknownChannelID++
		} else {
			st.matchedPairsByChannel[channelID]++
		}
		// Re-emit the matched send at its original time to ensure ordering (idempotent for UI)
		st.incAudit(auditRetroSendEmit)
		// Emit explicit paired events with the logical message id assigned on send.
		// Re-emit the matched send completion at its original time to ensure ordering (idempotent for UI)
		_ = emit(TimelineEvent{TimeMs: sr.TimeMs, Event: "send_complete", Channel: ch, ChannelKey: st.channelKey(key, sr.Ptr), ChPtr: sr.Ptr, G: sr.G, Role: sr.Role, Source: "region"})
		// Emit explicit recv with the pre-assigned message id

		_ = emit(TimelineEvent{TimeMs: t, Event: "chan_recv", Channel: ch, ChannelKey: st.channelKey(key, chPtr), ChPtr: chPtr, G: gid, Role: role, Source: "paired", PeerG: sr.G, MsgID: sr.MsgID})
	}
	return nil
}

// handleSelectLog parses select_* log messages (if emitted by instrumentation).
// Supported forms (tokens space-separated; name= may contain spaces and be quoted):
//
//	select_begin [id=<sid>]
//	select_case [id=<sid>] [idx=<i>] [kind=send|recv] [ptr=<p>] [name=<label>]
//	select_chosen [id=<sid>] [idx=<i>] [attempt=<aid>]
//	select_end [id=<sid>]
func parseSelectLogFields(msg string) (string, map[string]string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", nil
	}
	typ := ""
	rest := msg
	if i := strings.IndexAny(msg, " \t"); i >= 0 {
		typ = strings.TrimSpace(msg[:i])
		rest = strings.TrimSpace(msg[i+1:])
	} else {
		typ = msg
		rest = ""
	}
	fields := make(map[string]string)
	if rest == "" {
		return typ, fields
	}
	name := ""
	if i := strings.Index(rest, "name="); i >= 0 {
		name = strings.TrimSpace(rest[i+5:])
		name = strings.Trim(name, "\"'")
		rest = strings.TrimSpace(rest[:i])
	}
	for _, tk := range strings.Fields(rest) {
		if i := strings.IndexByte(tk, '='); i > 0 {
			k := tk[:i]
			v := tk[i+1:]
			fields[strings.ToLower(k)] = v
		}
	}
	if name != "" {
		fields["name"] = name
	}
	return typ, fields
}

func parseSelectIndex(raw string) int {
	if raw == "" {
		return -1
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return n
}

func (st *ParseState) selectCaseForChosen(sid string, idx int) (SelectCaseInfo, bool) {
	cases := st.selectCases[sid]
	if len(cases) == 0 {
		return SelectCaseInfo{}, false
	}
	if idx >= 0 && idx < len(cases) && cases[idx].Kind != "" {
		return cases[idx], true
	}
	if idx < 0 && len(cases) == 1 && cases[0].Kind != "" {
		return cases[0], true
	}
	return SelectCaseInfo{}, false
}

func (st *ParseState) emitSelectChosenCase(sid string, info SelectCaseInfo, t float64, gid int64, emit func(TimelineEvent) error) {
	if info.ChPtr == "" && info.ChKey == "" && info.ChName == "" {
		st.incAudit(auditMissingChannelIdentity)
		return
	}
	// Record the most recent chosen comm so we can ignore any immediately-following
	// select-clause body regions that look like the comm itself ("send/recv ...").
	st.lastSelectChosenByG[gid] = info
	st.lastSelectChosenSeqByG[gid] = st.eventIndexByG[gid]

	evName := "chan_recv"
	if info.Kind == "send" {
		evName = "chan_send"
	}
	msgID := int64(0)
	peerG := int64(0)
	source := "select"
	if info.Kind == "send" {
		// Treat select sends like region sends for pairing: assign a message id and
		// enqueue so future recvs can match.
		msgID = st.nextMsgID
		st.nextMsgID++
		source = "paired"

		key := parseChKey(info.ChName)
		ptr := info.ChPtr
		if ptr == "" && isPointerString(info.ChKey) {
			ptr = info.ChKey
		}
		channelID := info.ChKey
		if channelID == "" {
			channelID = st.channelKey(key, ptr)
		}
		if channelID == "" {
			channelID = key.Name
		}
		if channelID == "" {
			st.unknownChannelID++
		}

		// If a recv was seen first (rare, but can happen with incomplete traces),
		// complete it now so audit can pair.
		if _, rec, ok := st.popPendingRecv(key, ptr); ok {
			if channelID != "" {
				st.matchedPairsByChannel[channelID]++
			}
			_ = emit(TimelineEvent{
				TimeMs:     rec.TimeMs,
				Event:      "chan_recv",
				Channel:    rec.Ch,
				ChannelKey: st.channelKey(key, rec.Ptr),
				ChPtr:      rec.Ptr,
				G:          rec.G,
				Role:       rec.Role,
				Source:     "paired",
				PeerG:      gid,
				MsgID:      msgID,
			})
			st.recordChannelActivity("recv", rec.G, channelID, rec.TimeMs)
		} else if pendKey, ok := st.resolvePendingSelectRecv(key, ptr); ok {
			// A select recv was observed before its matching send; count it as paired.
			st.pendingSelectRecv[pendKey]--
			if channelID != "" {
				st.matchedPairsByChannel[channelID]++
			}
		} else {
			st.sendsByKey[st.opKey(key, ptr)] = append(st.sendsByKey[st.opKey(key, ptr)], sendRecord{
				TimeMs: t, G: gid, Role: st.Roles[gid], Ptr: ptr, MsgID: msgID,
			})
		}
	}
	if info.Kind == "recv" {
		if mid, pg, ok := st.matchSelectRecv(info); ok {
			msgID = mid
			peerG = pg
			// Mark as paired so audit can match against paired sends.
			source = "paired"
		}
	}
	if evName == "chan_send" {
		st.incAudit(auditSelectChosenEmittedSend)
	} else {
		st.incAudit(auditSelectChosenEmittedRecv)
	}
	_ = emit(TimelineEvent{
		TimeMs:     t,
		Event:      evName,
		G:          gid,
		Role:       st.Roles[gid],
		Channel:    info.ChName,
		ChannelKey: info.ChKey,
		ChPtr:      info.ChPtr,
		MissingPtr: info.ChPtr == "",
		SelectID:   sid,
		PeerG:      peerG,
		MsgID:      msgID,
		Source:     source,
	})
	st.markProgress(t)
	st.recordChannelActivity(info.Kind, gid, info.ChKey, t)
	if st.Opt.EmitAtomic {
		attID := st.newID("a")
		commitID := st.newID("c")
		atomAttempt := "chan_recv_attempt"
		atomCommit := "chan_recv_commit"
		if info.Kind == "send" {
			atomAttempt = "chan_send_attempt"
			atomCommit = "chan_send_commit"
		}
		_ = emit(TimelineEvent{
			TimeMs:     t,
			Event:      atomAttempt,
			ID:         attID,
			G:          gid,
			Role:       st.Roles[gid],
			Channel:    info.ChName,
			ChannelKey: info.ChKey,
			ChPtr:      info.ChPtr,
			MissingPtr: info.ChPtr == "",
			SelectID:   sid,
			Source:     "select",
		})
		_ = emit(TimelineEvent{
			TimeMs:     t,
			Event:      atomCommit,
			ID:         commitID,
			AttemptID:  attID,
			PairID:     attID,
			G:          gid,
			Role:       st.Roles[gid],
			Channel:    info.ChName,
			ChannelKey: info.ChKey,
			ChPtr:      info.ChPtr,
			MissingPtr: info.ChPtr == "",
			SelectID:   sid,
			Source:     "select",
		})
	}
	st.selectEmitted[sid] = true
	delete(st.selPending, sid)
}

func (st *ParseState) matchSelectRecv(info SelectCaseInfo) (int64, int64, bool) {
	if info.ChName == "" && info.ChKey == "" && info.ChPtr == "" {
		return 0, 0, false
	}
	ptr := info.ChPtr
	if ptr == "" && isPointerString(info.ChKey) {
		ptr = info.ChKey
	}
	key := parseChKey(info.ChName)
	opKey := st.opKey(key, ptr)
	q := st.sendsByKey[opKey]
	if len(q) == 0 {
		if altKey, altQ, ok := st.findSendQueueByName(key); ok {
			opKey = altKey
			q = altQ
		}
	}
	if len(q) == 0 {
		st.pendingSelectRecv[opKey]++
		if key.Name != "" {
			nameKey := st.opKey(key, "")
			st.pendingSelectRecv[nameKey]++
		}
		return 0, 0, false
	}
	sr := q[0]
	st.sendsByKey[opKey] = q[1:]
	channelID := info.ChKey
	if channelID == "" {
		channelID = st.channelKey(key, info.ChPtr)
	}
	if channelID == "" {
		st.unknownChannelID++
	} else {
		st.matchedPairsByChannel[channelID]++
	}
	return sr.MsgID, sr.G, true
}

func shouldSelectFallback() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GTV_SELECT_FALLBACK")))
	return v == "1" || v == "true" || v == "yes"
}

func (st *ParseState) handleSelectLog(gid int64, msg string, t float64, stack xtrace.Stack, emit func(TimelineEvent) error) {
	typ, kv := parseSelectLogFields(msg)
	if typ == "" {
		return
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
		st.selectMeta[sid] = struct {
			G      int64
			TimeNs int64
		}{G: gid, TimeNs: st.lastTimeNs}
		_ = emit(TimelineEvent{TimeMs: t, Event: "select_begin", G: gid, Role: st.Roles[gid], SelectID: sid, Source: "log"})
	case "select_case":
		if sid == "" {
			sid = st.activeSelect[gid]
		}
		st.incAudit(auditSelectCaseSeen)
		att := kv["attempt"]
		if att == "" {
			att = st.activeAttempt[gid]
		}
		kind := strings.ToLower(kv["kind"])
		ptr := kv["ptr"]
		ch := kv["name"]
		if ch == "" {
			ch = kv["ch"]
		}
		if ch != "" {
			if _, parsed, ok := parseRegionOp(ch); ok && parsed != "" {
				ch = parsed
			}
		}
		idx := parseSelectIndex(kv["idx"])
		chKey := ""
		if ch != "" || ptr != "" {
			ck := parseChKey(ch)
			if ch == "" {
				ck = parseChKey(ptr)
			}
			chKey = st.channelKey(ck, ptr)
		}
		info := SelectCaseInfo{
			Kind:   kind,
			ChPtr:  ptr,
			ChKey:  chKey,
			ChName: ch,
			Role:   st.Roles[gid],
			G:      gid,
			TimeNs: st.lastTimeNs,
			Index:  idx,
		}
		if ptr != "" && ch != "" {
			st.channelNameByPtr[ptr] = ch
		}
		if sid != "" {
			cases := st.selectCases[sid]
			if idx >= 0 {
				if len(cases) <= idx {
					next := make([]SelectCaseInfo, idx+1)
					copy(next, cases)
					cases = next
				}
				cases[idx] = info
			} else {
				cases = append(cases, info)
			}
			st.selectCases[sid] = cases
		}
		_ = emit(TimelineEvent{
			TimeMs:      t,
			Event:       "select_case",
			G:           gid,
			Role:        st.Roles[gid],
			SelectID:    sid,
			SelectKind:  kind,
			SelectIndex: idx,
			AttemptID:   att,
			Channel:     ch,
			ChannelKey:  chKey,
			ChPtr:       ptr,
			Source:      "log",
		})
		if att != "" {
			st.selCandidates[sid] = append(st.selCandidates[sid], att)
		}
		if sid != "" && st.selPending[sid] && !st.selectEmitted[sid] {
			chosenIdx := st.selChosenIdx[sid]
			if info, ok := st.selectCaseForChosen(sid, chosenIdx); ok {
				st.emitSelectChosenCase(sid, info, t, gid, emit)
			}
		}
	case "select_chosen":
		if sid == "" {
			sid = st.activeSelect[gid]
		}
		st.incAudit(auditSelectChosenSeen)
		att := kv["attempt"]
		if att == "" {
			att = st.activeAttempt[gid]
		}
		idx := parseSelectIndex(kv["idx"])
		st.selChosen[sid] = att
		st.selChosenIdx[sid] = idx
		_ = emit(TimelineEvent{
			TimeMs:      t,
			Event:       "select_chosen",
			G:           gid,
			Role:        st.Roles[gid],
			SelectID:    sid,
			SelectIndex: idx,
			AttemptID:   att,
			Source:      "log",
		})
		if sid != "" && !st.selectEmitted[sid] {
			if info, ok := st.selectCaseForChosen(sid, idx); ok {
				st.emitSelectChosenCase(sid, info, t, gid, emit)
			} else {
				st.selPending[sid] = true
			}
		}
		if shouldSelectFallback() {
			st.maybeEmitSelectFallback(gid, sid, stack, t, emit)
		}
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
		if sid != "" && st.selPending[sid] && !st.selectEmitted[sid] {
			st.incAudit(auditSelectChosenMissingCase)
		}
		// Cleanup
		delete(st.selCandidates, sid)
		delete(st.selChosen, sid)
		delete(st.selChosenIdx, sid)
		delete(st.selPending, sid)
		delete(st.selectFallback, gid)
		delete(st.activeSelect, gid)
		delete(st.selectCases, sid)
		delete(st.selectEmitted, sid)
		delete(st.selectMeta, sid)
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
	ptrAliases := map[string]string{}
	aliasSeq := 0
	nameByPtr := map[string]string{}
	isAliasName := func(name string) bool {
		if name == "" {
			return false
		}
		low := strings.ToLower(strings.TrimSpace(name))
		return strings.HasPrefix(low, "ch#") || strings.HasPrefix(low, "unknown:")
	}
	aliasForPtr := func(ptr string) string {
		if ptr == "" {
			return ""
		}
		if v, ok := ptrAliases[ptr]; ok {
			return v
		}
		aliasSeq++
		alias := fmt.Sprintf("ch#%d", aliasSeq)
		ptrAliases[ptr] = alias
		return alias
	}
	for _, ev := range events {
		if ev.ChPtr != "" && ev.Channel != "" {
			ptr := normalizePointer(ev.ChPtr)
			if ptr != "" && !isPointerString(ev.Channel) && !isAliasName(ev.Channel) {
				if _, ok := nameByPtr[ptr]; !ok {
					nameByPtr[ptr] = ev.Channel
				}
			}
		}
	}
	for i, ev := range events {
		if ev.ChannelKey != "" {
			ev.ChannelKey = normalizeChannelKey(ev.ChannelKey)
		}
		if ev.ChPtr != "" {
			ev.ChPtr = normalizePointer(ev.ChPtr)
		}
		if ev.Channel == "" || isPointerString(ev.Channel) || isAliasName(ev.Channel) {
			if ev.ChPtr != "" {
				if mapped, ok := nameByPtr[ev.ChPtr]; ok {
					ev.Channel = mapped
				}
			}
		}
		if ev.ChannelKey == "" {
			if key := normalizeChannelKey(ev.Channel); key != "" {
				ev.ChannelKey = key
			}
		}
		if ev.ChannelKey != "" {
			if key := normalizePointer(ev.ChannelKey); key != "" && isPointerString(key) {
				if ev.ChPtr == "" {
					ev.ChPtr = key
				}
				alias := aliasForPtr(key)
				if alias != "" {
					ev.ChannelKey = alias
					if ev.Channel == "" || isPointerString(ev.Channel) {
						ev.Channel = alias
					}
				}
			}
		}
		if ev.ChPtr != "" && isPointerString(ev.ChPtr) {
			alias := aliasForPtr(ev.ChPtr)
			if alias != "" {
				if ev.ChannelKey == "" || isPointerString(ev.ChannelKey) {
					ev.ChannelKey = alias
				}
				if ev.Channel == "" || isPointerString(ev.Channel) {
					ev.Channel = alias
				}
			}
		}
		if ev.TimeNs == 0 {
			ev.TimeNs = int64(ev.TimeMs*1e6 + 0.5)
		}
		if ev.Seq == 0 {
			ev.Seq = int64(i + 1)
		}
		name := strings.ToLower(ev.Event)
		if isChannelEvent(name) && ev.ChannelKey == "" {
			if key := normalizeChannelKey(ev.Channel); key != "" {
				ev.ChannelKey = key
			}
		}
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
				ev.MissingPtr = true
				warnings = append(warnings, fmt.Sprintf("missing channel identity for %s (g=%d seq=%d time_ns=%d)", ev.Event, ev.G, ev.Seq, ev.TimeNs))
			}
		}
		if isPairingEvent(name, ev.Source) {
			if ev.PeerG == 0 && ev.PairID == "" && ev.MsgID == 0 {
				warnings = append(warnings, fmt.Sprintf("missing pairing fields for %s (g=%d seq=%d time_ns=%d)", ev.Event, ev.G, ev.Seq, ev.TimeNs))
			}
		}
		ev = AnnotateUnknownEndpoints(ev)
		normalized[i] = ev
	}
	warningsV2 := make([]WarningEntry, 0, len(warnings))
	for _, w := range warnings {
		code := "warning"
		msg := w
		low := strings.ToLower(w)
		switch {
		case strings.HasPrefix(low, "missing goroutine id"):
			code = "missing_goroutine_id"
		case strings.HasPrefix(low, "missing channel identity"):
			code = "missing_channel_identity"
		case strings.HasPrefix(low, "missing pairing fields"):
			code = "missing_pairing_fields"
		case strings.HasPrefix(low, "validation: channel identity missing"):
			code = "validation_channel_identity_missing"
		}
		warningsV2 = append(warningsV2, WarningEntry{Code: code, Message: msg})
	}
	beforeCount := len(normalized)
	dropCounts := map[string]int64{}
	if st != nil && normalizeMode(st.Opt.Mode) == "teach" {
		filtered := make([]TimelineEvent, 0, len(normalized))
		for _, ev := range normalized {
			updated, keep, dropName := FilterTimelineEventForMode(ev, st.Opt.Mode)
			if !keep {
				dropCounts[dropName]++
				continue
			}
			filtered = append(filtered, updated)
		}
		normalized = filtered
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
	normalized, auditSummary := mergeAuditSummaries(normalized)
	warnings = append(warnings, validateChannelIdentities(normalized)...)
	if st != nil && normalizeMode(st.Opt.Mode) == "teach" && len(normalized) > 0 {
		afterCount := len(normalized)
		if auditSummary.Event == "audit_summary" {
			val, ok := auditSummary.Value.(map[string]any)
			if !ok || val == nil {
				val = map[string]any{}
			}
			val["mode"] = "teach"
			val["events_before"] = beforeCount
			val["events_after"] = afterCount
			if len(dropCounts) > 0 {
				val["mode_drop_counts"] = dropCounts
			}
			auditSummary.Value = val
		}
	}
	if auditSummary.Event == "audit_summary" {
		normalized = append(normalized, auditSummary)
	}
	if len(normalized) > 0 {
		counts := make(map[string]int64)
		for _, ev := range normalized {
			if ev.Event == "" {
				continue
			}
			counts[ev.Event]++
		}
		if len(normalized) > 0 && strings.ToLower(normalized[len(normalized)-1].Event) == "audit_summary" {
			val, ok := normalized[len(normalized)-1].Value.(map[string]any)
			if !ok || val == nil {
				val = map[string]any{}
			}
			val["event_counts_by_type"] = counts
			normalized[len(normalized)-1].Value = val
		}
	}
	entities := buildEntitiesFromTimeline(normalized)
	caps := computeCapabilities(normalized, entities)
	return TimelineEnvelope{
		SchemaVersion: TimelineSchemaVersion,
		Capabilities:  caps,
		Events:        normalized,
		Entities:      entities,
		Warnings:      nil,
		WarningsV2:    warningsV2,
	}
}

func computeCapabilities(events []TimelineEvent, entities *EntitiesTable) []string {
	caps := map[string]bool{}
	if len(events) == 0 {
		return nil
	}
	for _, ev := range events {
		switch strings.ToLower(ev.Event) {
		case "spawn":
			caps["spawn_edges"] = true
		case "chan_depth":
			caps["chan_depth"] = true
		case "select_case", "select_chosen", "select_begin", "select_end":
			caps["select_cases"] = true
		case "chan_send_attempt", "chan_recv_attempt", "chan_send_commit", "chan_recv_commit":
			caps["attempt_commit"] = true
		case "blocked_send", "blocked_receive", "go_block", "go_unblock":
			caps["blocking_events"] = true
		case "goroutine_created", "goroutine_started", "go_start":
			caps["goroutine_lifecycle"] = true
		}
		if ev.File != "" && ev.Line > 0 {
			caps["file_line"] = true
		}
		if ev.Role != "" {
			caps["roles"] = true
		}
		if ev.BlockKind != "" || ev.UnblockKind != "" {
			caps["block_kind"] = true
		}
	}
	if entities != nil {
		for _, ch := range entities.Channels {
			if len(ch.Aliases) > 0 {
				caps["channel_aliases"] = true
				break
			}
		}
	}
	out := make([]string, 0, len(caps))
	for k := range caps {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mergeAuditSummaries(events []TimelineEvent) ([]TimelineEvent, TimelineEvent) {
	var merged TimelineEvent
	out := make([]TimelineEvent, 0, len(events))
	mergeValue := func(dst map[string]any, src any) map[string]any {
		if dst == nil {
			dst = map[string]any{}
		}
		switch v := src.(type) {
		case map[string]any:
			for k, val := range v {
				dst[k] = val
			}
		case map[string]int64:
			for k, val := range v {
				dst[k] = val
			}
		case AuditSummary:
			dst["dedup"] = v.Dedup
			if len(v.Warnings) > 0 {
				dst["warnings"] = v.Warnings
			}
		default:
			if v != nil {
				dst["summary"] = v
			}
		}
		return dst
	}
	for _, ev := range events {
		if strings.ToLower(ev.Event) != "audit_summary" {
			out = append(out, ev)
			continue
		}
		if merged.Event == "" {
			merged = ev
			merged.Value = mergeValue(nil, ev.Value)
			continue
		}
		merged.Value = mergeValue(merged.Value.(map[string]any), ev.Value)
		if ev.TimeNs > merged.TimeNs {
			merged.TimeNs = ev.TimeNs
			merged.TimeMs = ev.TimeMs
		}
		merged.Source = ev.Source
	}
	return out, merged
}

func buildEntitiesFromTimeline(events []TimelineEvent) *EntitiesTable {
	if len(events) == 0 {
		return nil
	}
	gMap := make(map[int64]GoroutineEntity)
	chMap := make(map[string]ChannelEntity)
	addAlias := func(ce *ChannelEntity, alias string) {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return
		}
		for _, a := range ce.Aliases {
			if a == alias {
				return
			}
		}
		ce.Aliases = append(ce.Aliases, alias)
	}
	for _, ev := range events {
		if ev.G != 0 {
			ge := gMap[ev.G]
			if ge.GID == 0 {
				ge.GID = ev.G
			}
			if ge.ParentGID == 0 && ev.ParentG != 0 {
				ge.ParentGID = ev.ParentG
			}
			if ge.Func == "" && ev.Func != "" {
				ge.Func = ev.Func
			}
			if ge.Role == "" && ev.Role != "" {
				ge.Role = ev.Role
			}
			gMap[ev.G] = ge
		}
		name := strings.ToLower(ev.Event)
		if isChannelEvent(name) || name == "chan_make" || name == "chan_close" {
			id := timelineChannelIdentity(ev)
			if id == "" {
				continue
			}
			ce := chMap[id]
			if ce.ChanID == "" {
				ce.ChanID = id
			}
			if ce.ChPtr == "" && ev.ChPtr != "" {
				ce.ChPtr = ev.ChPtr
			}
			if ce.Name == "" && ev.Channel != "" {
				ce.Name = ev.Channel
			}
			if ev.Channel != "" {
				addAlias(&ce, ev.Channel)
			}
			if ev.ChannelKey != "" {
				addAlias(&ce, ev.ChannelKey)
			}
			if ev.ChPtr != "" {
				addAlias(&ce, ev.ChPtr)
			}
			if ev.Event == "chan_make" {
				if ce.ElemType == "" && ev.ElemType != "" {
					ce.ElemType = ev.ElemType
				}
				if ce.Cap == 0 && ev.Cap > 0 {
					ce.Cap = ev.Cap
				}
			}
			chMap[id] = ce
		}
	}
	goroutines := make([]GoroutineEntity, 0, len(gMap))
	for _, g := range gMap {
		goroutines = append(goroutines, g)
	}
	sort.Slice(goroutines, func(i, j int) bool { return goroutines[i].GID < goroutines[j].GID })
	channels := make([]ChannelEntity, 0, len(chMap))
	for _, ch := range chMap {
		channels = append(channels, ch)
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].ChanID < channels[j].ChanID })
	return &EntitiesTable{Goroutines: goroutines, Channels: channels}
}

func timelineChannelIdentity(ev TimelineEvent) string {
	if ev.ChannelKey != "" {
		return ev.ChannelKey
	}
	return ev.Channel
}

func isGoroutineEvent(name string) bool {
	switch name {
	case "goroutine_created", "goroutine_started", "go_create", "go_start", "spawn", "worker_starting", "channels_created",
		"blocked_send", "blocked_receive", "block_send", "block_recv", "unblocked", "block", "unblock",
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
		"blocked_send", "blocked_receive", "block_send", "block_recv", "block", "unblock", "select_case":
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

func validateChannelIdentities(events []TimelineEvent) []string {
	warnings := make([]string, 0)
	for _, ev := range events {
		name := strings.ToLower(ev.Event)
		if !isChannelEvent(name) {
			continue
		}
		if timelineChannelIdentity(ev) == "" {
			warnings = append(warnings, fmt.Sprintf("validation: channel identity missing for %s (g=%d seq=%d time_ns=%d)", ev.Event, ev.G, ev.Seq, ev.TimeNs))
		}
	}
	return warnings
}
