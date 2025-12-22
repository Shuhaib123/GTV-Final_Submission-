# Broadcast Phase 0 Baseline

This document captures the “truth” checklist we expect from the `broadcast.go` workload plus the steps to lock that state down for offline inspection.

## Truth checklist

- **Goroutines**
  1. one “server/main” goroutine that runs the broadcast logic and accepts registrations
  2. ten client goroutines numbered `0…9` that register with the server, wait for a broadcast, and exit

- **Channels** (distinct identities)
  - **Shared channels** (2):
    - `s.join` (registration requests)
    - `s.clientIn` (clients → server broadcast trigger)
  - **Per-client channels** (20):
    - `c.join` (reply channel returned to client during registration)
    - `c.serverIn` (server → client broadcast channel)
  - **Total channel identities**: 22

- **Communication relationships**
  - Clients send `*bcRequest` structs on `s.join`; the server receives them.
  - The server replies on the `c.join` channel stored inside each request so that the client learns its per-client inbox (server ← client registration).
  - Client `1` sends the broadcast message on `s.clientIn` while the server receives from it.
  - The server sends the broadcast message on each `c.serverIn`.
  - Each client receives the broadcast on its own `c.serverIn`.

## Phase 0 reproduction

1. **Instrument** the raw `broadcast.go` source with (the helper defaults to `examples/broadcast_raw/main.go`):
   ```sh
   go run ./cmd/gtv-instrument -in /path/to/broadcast.go -name Broadcast
   ```
  (the `scripts/phase0-broadcast.sh` helper wraps this, defaults to `examples/broadcast_raw/main.go`, and cleans up the generated file afterwards.)
2. **Run the trace** with the generated workload:
   ```sh
   go run -tags workload_broadcast ./cmd/gtv-runner -workload=broadcast > artifacts/broadcast/trace.out
   ```
   The helper adds `-timeout 10s` so this loop terminates instead of running forever.
3. **Parse** the trace into timeline JSON:
   ```sh
   go run . -parse artifacts/broadcast/trace.out -json artifacts/broadcast/trace.json
   ```
4. **Save a screenshot** of the resulting graph into `artifacts/broadcast/screenshot.png` so we can track the “wrong” visualization that this baseline produces.

## Scripted baseline

Run `scripts/phase0-broadcast.sh` (with optional overrides for the source path, output directory, and workload name) to execute the instrument → runner → parse sequence, produce the trace artifacts, drop in a placeholder screenshot, and clean up the generated workload file. The script writes to `artifacts/broadcast/{trace.out,trace.json,screenshot.png}` as a deterministic staging area you can check into your artifact tracking system.

> **Note**: replace `artifacts/broadcast/screenshot.png` with an actual UI capture when you record the final “wrong graph” state. The helper script writes a 1×1 PNG placeholder so the path exists until you swap it out.
