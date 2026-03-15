# Teaching-First JSON Contract

This document captures the expected JSON envelope and event contract for
classroom use. Default behavior is **teach mode** (low noise). Debug mode is
opt-in.

## Envelope

```json
{
  "schema_version": 1,
  "events": [ ... ],
  "entities": {
    "goroutines": [ ... ],
    "channels": [ ... ]
  },
  "warnings_v2": [ { "code": "...", "message": "..." } ],
  "warnings": [ "...legacy strings..." ]
}
```

## Modes

Teach mode (default):
- Minimal event set for comprehension.
- Filtered server-side in `internal/traceproc/traceproc.go`.

Debug mode:
- Full event stream (scheduler-level + diagnostic events).

Teach-mode event list:
- `go_create`, `go_start`, `go_end` (if present)
- `chan_make`
- `chan_send`, `chan_recv`
- `block_send`, `block_recv`, `unblock`
- `select_chosen` (optional)

Notes:
- Atomic events are normalized to teach events, e.g. `chan_send_commit` → `chan_send`.
- `ch_ptr` is metadata only and is **not** emitted as a timeline event.

## Entities

`entities.goroutines[]` (best-effort):
- `gid` (stable)
- `parent_gid` (if known)
- `func` (best-effort)
- `role` (best-effort)

`entities.channels[]`:
- `chan_id` (stable, e.g. pointer or derived key)
- `name` (best-effort label)
- `elem_type`, `cap` (if known)

UI behavior:
- Prefer entities when present.
- Fall back to event inference when entities are absent.

## Warnings

`warnings_v2` uses stable codes:
- `missing_goroutine_id`
- `missing_channel_identity`
- `missing_pairing_fields`
- `validation_channel_identity_missing`
- `warning` (fallback)

These are advisory and do not block rendering.

## Audit Summary

`audit_summary` includes:
- `event_counts_by_type`
- `drops_by_reason` (teach-mode filtering)
- unmatched send/recv counts (if enabled)

## Unknown Endpoint Semantics

When send/recv pairing is ambiguous:
- Do **not** invent endpoints.
- Mark `unknown_sender` / `unknown_receiver` / `unknown_channel`.
- UI renders a placeholder marker near the channel.

## Validation Checklist (Short)

- Teach mode contains only the allowed event list.
- No `ch_ptr` timeline events.
- Unknown endpoints render without dropping channels.
- Live out-of-order events pause animation rather than misordering.
