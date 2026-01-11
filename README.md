# Go Trace Visualizer (GTV)

GTV is a tiny experiment to visualize Go concurrency from runtime traces. It supports:

- Offline replay: run a demo, parse `trace.out` → `trace.json`, and load it in the visualizer.
- Live streaming: run the same demo and stream timeline events over WebSocket into the visualizer as they happen.


## Features

- Unified demo workload used by both offline and live paths.
- Shared trace event processor built on `golang.org/x/exp/trace.Reader`.
- Live WebSocket server that streams `TimelineEvent` JSON to the UI.
- Interactive timeline with blocking annotations, channel edges, and step/scrub controls.


## Repo Structure

- `cmd/gtv-live/main.go` — Live server (HTTP + WebSocket).
- `internal/workload/workload.go` — Shared ping–pong demo workload.
- `internal/traceproc/traceproc.go` — Shared trace → timeline event processor.
- `parser.go` — Offline parser (`trace.out` → `trace.json`) using the shared processor.
- `main.go` — Offline runner: generates `trace.out`, then `trace.json`.
- `web/graph-live.html` — Live visualizer (auto-connects to `/trace`).
- `web/graph.html` — Offline visualizer (load a `trace.json` file).
- `web/index.html` — Landing page with links to both visualizers.


## Prerequisites

- Go 1.21+ recommended (needs `runtime/trace` and x/exp/trace API).
- First run may fetch modules: `go mod tidy` (or `go get` lines below).


## Live Mode

1. Install deps (first time only):
   - `go mod tidy`
   - or: `go get github.com/gorilla/websocket@v1.5.1`
2. Start the server:
   - `go run ./cmd/gtv-live`
   - Flags (override env):
     - `-addr string` (default `:8080`) — HTTP listen address
     - `-synth` — enable send synthesis (same as `GTV_SYNTH_SEND=1`)
     - `-drop-block-no-ch` — drop unlabeled blocked events (same as `GTV_DROP_BLOCK_NO_CH=1`)
3. Open the UI:
   - `http://localhost:8080/` → choose “Live Visualizer” (or open `http://localhost:8080/graph-live.html` directly)
4. The page auto-connects to `/trace` and auto-starts a run; use the “Re-run” button for another run without reloading.

Environment options:
- `GTV_SYNTH_SEND=1` — synthesize a send just before any unmatched recv to keep edges complete.
- `GTV_DROP_BLOCK_NO_CH=1` — drop blocked events that cannot be tied to a channel.

Examples:
- Flags: `go run ./cmd/gtv-live -addr :9090 -synth -drop-block-no-ch`
- Envs: `GTV_SYNTH_SEND=1 go run ./cmd/gtv-live`
- Envs: `GTV_DROP_BLOCK_NO_CH=1 go run ./cmd/gtv-live`
Flags take precedence over env defaults.

Port: edit `cmd/gtv-live/main.go` (ListenAndServe) to change `:8080`.


## Offline Mode

1. Run the offline demo to generate trace + JSON:
   - `go run .`
   - Outputs: `trace.out` and `trace.json`.
2. Open the offline visualizer:
   - Option A (via server): `http://localhost:8080/graph.html` and use “Load JSON”.
   - Option B (file): open `web/graph.html` in your browser and load the generated `trace.json`.

Notes:
- Offline parsing uses the same event processor as live; you can enable `GTV_SYNTH_SEND=1` during `go run .` to synthesize missing sends in JSON too.


## How It Works

- `internal/workload` runs a simple ping–pong exchange over channels with `trace.Log` and `trace.WithRegion` annotations.
- Live server wraps the workload with `runtime/trace` and streams events from `x/exp/trace.Reader` over WebSocket as `TimelineEvent` JSON.
- `internal/traceproc.ProcessEvent` maps `x/exp/trace.Event` → `TimelineEvent` while tracking roles, blocking, and channel intent.
- The front-end animates edges, blocks, and message flow as events advance.

## Architecture Diagram

```mermaid
flowchart LR
  A[runtime/trace] --> B[x/exp/trace.Reader]
  B --> C[traceproc.ProcessEvent]
  C --> D[handleRegionOp (pairing)]
  D -->|emit chan_recv| E[TimelineEvent: chan_recv]
  D -->|recv end can emit chan_send<br/>with earlier time_ns| F[TimelineEvent: chan_send]
  C --> G[TimelineEvent stream]
  G --> H[cmd/gtv-live assigns seq]
  H --> I[WebSocket /trace]
  I --> J[web/graph-live.html]
```

> **Warning**: Live stream is arrival‑ordered; retroactive timestamps can invert send/recv order.

## Ordering Contract

- `time_ns` is the authoritative timestamp; it originates in the trace reader and is preserved through processing.
- `seq` is the authoritative arrival/order index for the live stream; it is assigned in `cmd/gtv-live` when events are emitted over WebSocket.


## Troubleshooting

- Live page says “Live: disconnected”
  - Make sure you opened `http://localhost:8080/graph-live.html` (not the file on disk).
  - Check the server logs for “upgrade error” and your browser console for WebSocket errors.
- Pause doesn’t stop pulses
  - Fixed: Pause freezes both playback and pulse animations. Refresh after updating.
- Missing `github.com/gorilla/websocket`
  - Run: `go get github.com/gorilla/websocket@v1.5.1` (or `go mod tidy`).
- x/exp/trace build errors
  - Ensure Go 1.21+; update your Go toolchain if `runtime/trace` is reported missing.


## Credits

## Instrumentation & Value Tracing

To see values on graph edges without hand-written comments/logs, use one of these:

- Generated workloads (instrumented path)
  - Optionally pre-annotate source automatically: `go run ./cmd/gtv-autotag -in your_main.go` or `-dir ./path`.
  - Then instrument (via the UI or `gtv-instrument`). The instrumenter and parser attach values to send/recv edges.
  - Live and offline viewers now derive the channel topology edges from `chan_send`/`chan_recv` events flagged `source:paired`; the legacy `chan_*` attempts/commits are retained only for diagnostics overlays.
  - The instrumentation level now defaults to `regions` (no per-iteration logs) to keep traces small; pick `regions_logs` explicitly via the UI Level menu or `-level=regions_logs` if you need the extra `trace.Log` annotations.
  - Value logging is disabled by default — enable it per-workload with the UI’s “Value logs” checkbox, the `value_logs` field (or `-value-logs`) when instrumenting, or globally with `GTV_LOG_VALUES=1`.

- Built-in workloads (not re-instrumented)
  - Use helpers in `internal/workload/traceutil.go`:
    - `TraceSend(ctx, label, ch, v)` — wraps a send, logs `v`.
    - `TraceRecv[T](ctx, label, ch) T` — wraps a receive, logs the value.
  - Example:
    ```go
    msg := TraceRecv[string](ctx, "server: receive from clientin", s.clientIn)
    TraceSend(ctx, "server: send to "+chName, s.clientOut[i], msg)
    ```

Notes
- For `select` case heads, wrappers can’t be used; add a `trace.Log(ctx, "value", fmt.Sprint(v))` in the case body if needed.
- The live parser attaches string values to `send_attempt` (send edges) and `recv_complete` (receive edges).
- I/O regions (opt-in): enable with env `GTV_INSTR_IO_REGIONS=1` (global) or fine-grained `GTV_INSTR_IO_JSON=1` (encoding/json) and `GTV_INSTR_IO_DB=1` (database/sql). In JSON config, you can use:
  ```json
  { "io_regions": true, "io": { "encoding_json": true, "database_sql": true } }
  ```
  Region labels: `json.marshal`, `json.unmarshal`, `db.query`, `db.exec`.
- Sample workloads (broadcast, ping-pong, skipgraph, etc.) are bounded as well; you can enforce a wall-clock or step cap with `GTV_MAX_MS` / `GTV_MAX_STEPS` to stop runs that otherwise take too long.


- Uses `golang.org/x/exp/trace` for decoding runtime trace streams.
- Live transport via `github.com/gorilla/websocket`.

### Quick IO Regions Demo

- Start the live server: `go run ./cmd/gtv-live`
- Open `http://localhost:8080/instrument.html`
- Copy the sample from `examples/json_db_sample.go` into the editor; set name to `iodemo`.
- Enable "IO Regions" (or set env: `GTV_INSTR_IO_REGIONS=1`). Optionally toggle per‑library checkboxes: JSON, DB, HTTP, and OS I/O.
- Click Instrument. Then run it from the UI or open: `http://localhost:8080/run?name=iodemo`.
- You should see regions: `json.marshal`, `json.unmarshal`, `db.query`, `db.exec`, plus a `loop:sample` region for the safe loop. If you enable HTTP/OS, matching calls will appear as `http.call` and `file.open`/`file.readfile`/`file.writefile`/`io.copy` as applicable.

### I/O Regions Reference

- Enable globally via env `GTV_INSTR_IO_REGIONS=1`, or selectively with:
  - `GTV_INSTR_IO_JSON=1` (encoding/json)
  - `GTV_INSTR_IO_DB=1` (database/sql)
  - `GTV_INSTR_IO_HTTP=1` (net/http)
  - `GTV_INSTR_IO_OS=1` (os, io, ioutil)
  - JSON config example:
    ```json
    {
      "io_regions": true,
      "io": {
        "encoding_json": true,
        "database_sql": true,
        "net_http": true,
        "os_io": true
      }
    }
    ```

- Labels by package:
  - encoding/json
    - `json.Marshal`, `json.MarshalIndent` → `json.marshal`
    - `json.Unmarshal` → `json.unmarshal`
  - database/sql (types-based with heuristic fallback)
    - `(*sql.DB|*sql.Tx|*sql.Stmt).Query`, `QueryContext` → `db.query`
    - `(*sql.DB|*sql.Tx|*sql.Stmt).Exec`, `ExecContext` → `db.exec`
    - `(*sql.DB).Begin`, `BeginTx` → `db.begin`
    - Context variants use the call’s first argument as the region context when present.
  - net/http
    - `http.Get/Post/Head/Do`, `(*http.Client).Do` → `http.call`
  - os/io/ioutil
    - `os.Open/OpenFile` → `file.open`
    - `os.ReadFile`, `ioutil.ReadFile` → `file.readfile`
    - `os.WriteFile` → `file.writefile`
    - `io.Copy` → `io.copy`
    - Instance methods `Read`/`Write` on values (heuristic) → `file.read` / `file.write`

Notes
- I/O wrapping is opt-in and gated by the toggles above. Include/Exclude package filters may downgrade to tasks-only and disable I/O regions.
- SQL classification prefers go/types receiver resolution (DB/Tx/Stmt); when type info is unavailable, a simple method-name heuristic is used.
- No ctx in scope? By default, I/O regions use `context.Background()` when no `ctx` identifier is available. To disable this behavior, set `GTV_INSTR_IO_ASSUME_BG=0` (or JSON `{ "io_assume_background": false }` or `{ "io": { "assume_background": false } }`). This affects only I/O regions; other regions still require an in-scope `ctx`.

## Scope (v1)

See ADR: `docs/adr/0001-scope-v1.md`. Summary:

- Entrypoints
  - We do not auto-wrap arbitrary `main` functions. Runners initialize tracing explicitly.
- Tests
  - We do not auto-instrument `go test`. For traces in tests, call the runner or annotate specific helpers with `// gtv:task`.
- CLI/Frameworks
  - We do not auto-detect arbitrary CLI or HTTP frameworks. Annotate root handlers with `// gtv:task`, `// gtv:http`, etc.
- Loops
  - Complex loops are not automatically instrumented. Use `// gtv:loop=label` on hot loops to opt in (forced per-iteration regions may rewrite unlabelled break/continue and, when legal, bare return).


### Short var declarations (:=) and regions

When a value is assigned with short declaration (e.g., `data, err := json.Marshal(v)`), wrapping that call inside a closure would scope `data`/`err` to the closure only. To preserve the original scope, the instrumenter uses a StartRegion/End pattern in the same block for `:=` assignments:

Before:

```go
data, err := json.Marshal(v)
```

After (conceptual):

```go
{
  __gtvRegN := trace.StartRegion(ctx, "json.marshal")
  data, err := json.Marshal(v)
  __gtvRegN.End()
}
```

For expression statements or `=` reassignments, a closure via `trace.WithRegion(ctx, label, func(){ ... })` is used instead. This approach is applied to DB/JSON/HTTP/OS I/O call sites by the instrumenter.
