# MVP Refactor Plan

Goal: a small, reliable classroom demo that only covers goroutines + channels + send/recv + blocking.

## MVP scope (keep)
- Instrumenter: goroutine regions, channel send/recv regions, blocked send/recv regions.
- Trace processor: goroutine lifecycle + channel pairing + blocked/unblocked.
- UI: play/pause/step/speed/time, color-coded nodes/channels, simple highlighting.
- Live + offline modes (same event schema).

## MVP defaults (disable)
- IO regions and IO-specific logs.
- HTTP/GRPC task tracing.
- Loop regions.
- Value logging / payload capture (use synthetic tokens only).
- Advanced edge classes (causal/bundle/commit/synth overlays).

## Minimum event set
- goroutine_created, goroutine_started
- send_attempt, send_complete
- recv_attempt, recv_complete
- chan_send, chan_recv (paired)
- blocked_send, blocked_receive, unblocked
- chan_make, chan_close (optional for UI rectangles)

## Module boundaries (MVP)
- Instrumenter: `internal/instrumenter`
  - Keep: ctx propagation, goroutine regions, send/recv regions, block regions.
  - Remove/flag: IO regions, HTTP/GRPC tasks, loop regions, value logs.
- Processor: `internal/traceproc`
  - Keep: ProcessEvent, pairing, audit counters, NormalizeTimeline.
  - Remove/flag: select scaffolding, synth edges, advanced atomic events.
- UI: `web/*`
  - Keep: minimal toolbar, token highlight state machine.
  - Remove/flag: advanced edge filters, theme controls (hide in menu), notes panel by default.

## Flags and env (MVP)
- `GTV_MVP=1` forces MVP defaults.
- `--values` or `GTV_LOG_VALUES=1` enables value logging (off by default).
- `GTV_SYNTH_SEND` remains optional and off by default.

## Refactor steps (incremental)
1) Gate non-MVP instrumentation behind `GTV_MVP` (done in instrumenter options).
2) Remove non-MVP edge classes in UI (live + offline).
3) Add validation warnings for unknown endpoints.
4) Add MVP test matrix (small workloads).
5) Clean demo workloads to only those used in MVP.

## Deletion candidates (post-MVP)
- Extra workload generators not used in tests or demos.
- Advanced UI toggles that are not reachable in MVP.
- Any code path that only exists for value logging or IO traces.
