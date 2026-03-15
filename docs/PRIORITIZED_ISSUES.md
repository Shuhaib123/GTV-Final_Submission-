# Prioritized Issues (with File Pointers)

This document captures the top correctness and stability risks, plus the MVP refactor plan, with concrete file/function pointers.

## How to use this doc
- Start with Priority 0/1 issues; they affect correctness of edges and node identity.
- Use the “Where” column to jump to the exact file/function.
- If a test is listed, run it before and after any change.
- Add a note if you change behavior or introduce a new warning.

| Priority | Issue | Severity | Confidence | Impacted patterns | Where (file:function) | Symptom | Fix sketch |
|---|---|---|---|---|---|---|---|
| P0 | Pairing/correlation gaps | High | High | broadcast, fan-out/fan-in, pipelines | `internal/traceproc/traceproc.go:handleRegionOp` | Missing `chan_send`/`chan_recv` edges | Ensure stable channel IDs, count unmatched, avoid dropping recvs without logging |
| P0 | Topology builder drops unknown endpoints | High | Medium | any with missing channel id | `web/topology-builder.js:buildTopology` | Edges vanish or connect to wrong nodes | Render unknown endpoints explicitly, keep placeholders |
| P1 | Live ordering/out-of-order events | Medium | Medium | live mode, fast workloads | `web/graph-live.html:handleLiveTimelineEvent` | Stuck highlights / incorrect sequencing | Buffer minimally, skip animation when ordering uncertain |
| P1 | Validator not enforced | Medium | Medium | all | `internal/traceproc/topology.go:ValidateTopology` | Warnings only, but UI proceeds | Add warnings to envelope and show in UI; keep partial render |
| P2 | UI highlight state machine stuck | Medium | Low | long streams, mixed event types | `web/graph-live.html:fire` / `web/graph.html:render` | Token remains highlighted incorrectly | Ensure highlight clears on step; suppress animation when missing endpoints |

## P0 — Pairing/correlation gaps

**Loss accounting**
- Trace → Process: `handleRegionOp` emits `chan_send` and queues sends.
- Process → JSON: unmatched sends remain queued, recvs may be dropped if no match.
- JSON → UI: missing `chan_recv` causes edge loss.

**Logging hook suggestion**
- Add per-channel unmatched counts in `audit_summary` (already required).

**Tests**
- `internal/integration/audit_summary_test.go`
- `internal/e2e/e2e_test.go` (fan_out_fan_in)

## P0 — Topology builder drops unknown endpoints

**Loss accounting**
- JSON → UI: events with missing `ChannelKey`/`ChPtr` become unaddressable.
- UI hides edges because endpoint lookup fails.

**Logging hook suggestion**
- Track `unknown_channel_id` and surface warnings in HUD.

**Tests**
- Add a fixture that omits channel identity and ensure UI labels “unknown.”

## P1 — Live ordering/out-of-order events

**Loss accounting**
- Live stream order can be out-of-order for retroactive emits.
- UI may animate edges in wrong temporal order.

**Logging hook suggestion**
- Track min/max `time_ns` deltas in live debug stats.

**Tests**
- `TestLiveOrdering` in `internal/e2e/e2e_test.go`

## P1 — Validator not enforced

**Loss accounting**
- `ValidateTopology` is warnings-only; UI proceeds even on invalid graphs.

**Logging hook suggestion**
- Include validation warnings in `trace.json` envelope.

**Tests**
- Add a unit test that injects invalid edges and checks warnings.

## P2 — UI highlight state machine stuck

**Loss accounting**
- Highlight state persists if expected follow-up event is missing.

**Logging hook suggestion**
- Count “highlight reset” actions in UI debug mode.

**Tests**
- Add a UI snapshot test for simple send/recv flow.

## MVP Refactor Plan (short form)

- Keep: goroutine lifecycle, channel send/recv, blocking.
- Gate/remove: IO regions, HTTP/GRPC tasks, loop regions, value logging, causal/bundle overlays.
- Maintain: `docs/mvp-refactor.md`, `README.md` MVP section.
