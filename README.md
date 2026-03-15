# Go Trace Visualizer (GTV)

GTV visualizes Go concurrency from `runtime/trace` in two modes:

- Live replay over WebSocket
- Offline replay from `trace.json`

It includes an instrumenter, a shared trace processor, and two graph viewers with synchronization-aware topology.

## Current Pipeline

```mermaid

flowchart LR
  %% =========================
  %% Top-level execution path
  %% =========================
  A["Go Workload Source<br/>existing program / generated workload / instrumented program"]
  B["Instrumentation<br/>optional but recommended<br/>internal/instrumenter"]
  C["Run Go Binary"]

  A --> B
  A --> C
  B --> C

  %% Outputs from execution
  C --> D["runtime/trace live stream"]
  C --> E["trace.out file"]

  %% =========================
  %% Offline path
  %% =========================
  subgraph OFF["Offline Path - Post-Mortem Replay"]
    direction LR
    G["Offline Trace Processing<br/>internal/traceproc"]
    I["trace.json<br/>events + entities + metadata"]
    L["Offline Graph Viewer<br/>web/pages/graph"]

    E --> G
    G --> I
  end

  %% =========================
  %% Live path
  %% =========================
  subgraph LIVE["Live Path - Real-Time Streaming"]
    direction LR
    F["Live Trace Reader + Stream Processing<br/>x/exp/trace.Reader + internal/traceproc"]
    H["Normalized live events/entities<br/>JSON envelope over WebSocket"]
    K["Live Graph Viewer<br/>web/pages/graph-live"]

    D --> F
    F --> H
  end

  %% =========================
  %% Shared frontend topology
  %% =========================
  J["Shared Topology Builder<br/>web/shared/topology-builder.js"]

  I --> J
  H --> J

  J --> L
  J --> K

  %% =========================
  %% Viewer-side derived features
  %% =========================
  M["Topology Narration / Structural Summary"]
  V["Shared Viewer Features<br/>Topology narration<br/>Sync / layer filters<br/>Causal overlays<br/>Debug / teach modes"]

  J -.-> M
  V -.-> L
  V -.-> K

  %% =========================
  %% Semantics / structural layers
  %% =========================
  subgraph S["Graph Semantics / Structural Layers"]
    direction TB
    N["spawn: parent goroutine -> child goroutine"]
    O["channel layer: send / recv"]
    P["sync layer: lock / unlock / wg / cond"]
    Q["causal layer: optional overlay"]
    R["resource interaction / creation edges"]
  end

  J --- N
  J --- O
  J --- P
  J --- Q
  J --- R

  %% =========================
  %% Optional styling
  %% =========================
  classDef source fill:#f3f4f6,stroke:#9ca3af,color:#111827,stroke-width:1px;
  classDef optional fill:#f59e0b,stroke:#b45309,color:#ffffff,stroke-width:1px;
  classDef exec fill:#14b8a6,stroke:#0f766e,color:#ffffff,stroke-width:1px;
  classDef offline fill:#facc15,stroke:#ca8a04,color:#111827,stroke-width:1px;
  classDef live fill:#d9f99d,stroke:#65a30d,color:#111827,stroke-width:1px;
  classDef topology fill:#2dd4bf,stroke:#0f766e,color:#ffffff,stroke-width:1px;
  classDef viewer fill:#e9d5ff,stroke:#8b5cf6,color:#111827,stroke-width:1px;
  classDef feature fill:#f8fafc,stroke:#94a3b8,color:#111827,stroke-dasharray: 4 3;

  class A source;
  class B optional;
  class C exec;
  class G,I offline;
  class F,H live;
  class J topology;
  class K,L viewer;
  class M,V,N,O,P,Q,R feature;


  ```


## What Is Current

- Web UI lives under `web/pages/*` (not the legacy flat `web/*.html` paths).
- Synchronization is first-class in topology (`sync` layer), separate from channel transport.
- Topology edges use stronger IDs (`seq`/`time_ns` aware) to avoid link collapsing.
- Offline and live viewers both support:
  - `Sync only`
  - optional layer filters (`Channel/Sync/Spawn/Causal`)
  - topology narration panel (`Compact` / `Normal` + copy)
- Instrument flow supports loading generated workloads directly from `internal/workload`.

## Quick Start

### Live (recommended)

1. Start server:

```bash
go run ./cmd/gtv-live
```

2. Open:

- `http://localhost:8080/` (redirects to start page)
- direct: `http://localhost:8080/pages/index/index.html`

3. Use **Start** (`/pages/instrument/instrument.html`):

- Create workload from source, or **Load Workload** from `internal/workload`
- Run once or open live graph

4. Live graph:

- `http://localhost:8080/pages/graph-live/graph-live.html`

### Offline

1. Generate trace files:

```bash
go run .
```

Outputs: `trace.out`, `trace.json`

2. Open offline graph:

- `http://localhost:8080/pages/graph/graph.html`
- then load `trace.json` if needed

## Core URLs and APIs

### Pages

- Start: `/pages/index/index.html`
- Instrument: `/pages/instrument/instrument.html`
- Offline viewer: `/pages/graph/graph.html`
- Live viewer: `/pages/graph-live/graph-live.html`
- Demo page: `/pages/demo/demo.html`

### Server endpoints

- `GET /trace` (WebSocket): live event stream
- `POST /instrument`: instrument source into generated workload
- `GET /workloads`: list generated workloads in `internal/workload`
- `GET /workload?name=<workload>`: fetch generated workload source
- `GET /run?...`: run workload once and return timeline JSON
- `GET /demo/go-trace`: open Go native trace flow
- `POST /clear-build-cache`: clear runner build cache

## Instrumentation Workflows

### Browser instrument flow

From `/pages/instrument/instrument.html`:

- **Create Workload** sends source to `/instrument`
- Generated file is written to `internal/workload/<name>_gen.go`
- **Load Workload** queries `/workloads` and `/workload`, always from `internal/workload`

### CLI instrument flow

```bash
go run ./cmd/gtv-instrument -in ./your_main.go -name MyWork
```

Important flags:

- `-outdir` (default `internal/workload`)
- `-level tasks_only|regions|regions_logs` (default `regions`)
- `-sync-validation` (enforces sync preset)
- `-block-regions`, `-goroutine-regions`, `-guard-labels`
- `-value-logs`
- `-io-regions`, `-io-json`, `-io-db`
- `-http-tasks`, `-grpc-tasks`, `-loop-regions`

### Sync validation preset

When sync validation is enabled (UI preset or CLI `-sync-validation`):

- level forced to `regions_logs`
- block regions forced on
- goroutine regions forced on
- guarded labels forced on

This is enforced in both the server-side `/instrument` path and CLI path.

## Topology and Synchronization Model

### Layer contract

Topology links carry `layer`:

- `channel`
- `sync`
- `spawn`
- `causal`

`kind` is still present for style/backward compatibility.

### Semantic edge rules

- `create`: `main -> channel`
- `spawn`: `parent goroutine -> child goroutine`

### Sync events remain explicit

Parser/normalization preserves sync events such as:

- `mutex_lock`, `mutex_unlock`
- `rwmutex_lock`, `rwmutex_unlock`, `rwmutex_rlock`, `rwmutex_runlock`
- `wg_add`, `wg_done`, `wg_wait`
- `cond_wait`, `cond_signal`, `cond_broadcast`

They are not flattened into generic channel `send/recv` semantics.

## Viewer Features

### Live viewer (`/pages/graph-live/graph-live.html`)

- Event mode: `teach` / `debug`
- `Sync View` preset button for sync debugging
- Toggles: `Commits`, `Attempts`, `Causal`, `Messages only`, `Sync only`
- Optional layer filter group (query `?layers=1` or `?layer_filters=1`)
- `Run HUD`, `Debug / Audit`, `Download Events`
- Topology narration panel: **Topology** button (`Compact`/`Normal`, copy)
- Event buffer cap (default 12,000) with topology retained

### Offline viewer (`/pages/graph/graph.html`)

- Event list with `Topology build` vs `All events`
- Display/Layout/Interaction grouped controls
- `Sync only` + optional layer filters
- `Debug / Audit`
- Topology narration panel (`Compact`/`Normal`, copy)

## Sync Debug Validation Mode

Recommended live settings while diagnosing synchronization:

- `Events = debug`
- `Messages only = off`
- `Causal = on`
- `Commits = on`
- `Attempts = on` only when pairing ambiguity needs inspection

Fast path:

- Instrument page: click **Sync Capture Preset**
- Live page: click **Sync View**

## Runtime Ordering Contract

- `time_ns` is the authoritative trace timestamp.
- `seq` is the authoritative live arrival order index assigned by `cmd/gtv-live`.

## Key Flags and Environment Variables

### `cmd/gtv-live`

Flags:

- `-addr` (default from `GTV_ADDR`, else `:8080`)
- `-workload`
- `-bc-mode`
- `-mode teach|debug`
- `-timeout` (also propagates to child as `GTV_TIMEOUT_MS`)
- `-synth`
- `-drop-block-no-ch`
- `-live-log`
- `-mvp`

Env highlights:

- `GTV_ADDR`
- `GTV_WORKLOAD`
- `GTV_BC_MODE`
- `GTV_MODE`
- `GTV_TIMEOUT`
- `GTV_SYNTH_SEND`
- `GTV_DROP_BLOCK_NO_CH`
- `GTV_LIVE_LOG`
- `GTV_MVP`

### Trace processor (`internal/traceproc`)

- `GTV_FILTER_GOROUTINES=ops|all|legacy`
- `GTV_SKIP_PAIRING_CHANNELS`
- `GTV_QUIESCENCE_MS`
- `GTV_DEADLOCK_WINDOW_MS`
- `GTV_BLOCK_INFER_MS`
- `GTV_SELECT_FALLBACK`

### Instrumenter (`internal/instrumenter`)

- `GTV_INSTR_LEVEL`
- `GTV_INSTR_GUARD_LABELS`
- `GTV_INSTR_GOROUTINE_REGIONS`
- `GTV_INSTR_BLOCK_REGIONS`
- `GTV_INSTR_HTTP_TASKS`
- `GTV_INSTR_GRPC_TASKS`
- `GTV_INSTR_LOOP_REGIONS`
- `GTV_LOG_VALUES`
- `GTV_INSTR_IO_REGIONS`
- `GTV_INSTR_IO_JSON`
- `GTV_INSTR_IO_DB`
- `GTV_INSTR_IO_HTTP`
- `GTV_INSTR_IO_OS`
- `GTV_INSTR_IO_ASSUME_BG`
- `GTV_INSTR_CONFIG`
- `GTV_MVP`

## Repo Layout (current)

- `cmd/gtv-live/main.go` — HTTP/WebSocket live server
- `cmd/gtv-instrument/main.go` — CLI instrumenter
- `cmd/gtv-runner` — workload runner entrypoint
- `internal/traceproc/traceproc.go` — shared trace -> timeline processor
- `internal/instrumenter/*` — source instrumentation pipeline
- `internal/workload/*` — built-in + generated workloads
- `main.go` + `parser.go` — offline run + `trace.out` -> `trace.json`
- `web/pages/*` — UI pages
- `web/shared/*` — shared frontend topology/render helpers + tests

## Testing

Useful checks used during recent sync/topology changes:

```bash
node web/shared/topology-builder.test.mjs
node web/shared/topology-description.test.mjs
node web/shared/viewer-layer-utils.test.mjs
node web/shared/viewer-renderer-integration.test.mjs

go test ./internal/traceproc ./internal/instrumenter
```

## Troubleshooting

- Live page disconnected:
  - ensure server is running and page is served over `http://localhost:8080/...`
- No generated workload listed in Instrument page:
  - verify file exists under `internal/workload/*_gen.go`
  - check `/workloads` response
- Missing sync edges in viewer:
  - use sync preset (`regions_logs` + block regions)
  - switch live to `Events=debug`
  - keep `Messages only` off

## Notes

- `GTV_MVP=1` keeps UI and instrumentation in a reduced, classroom-friendly configuration.
- Value logs are optional and can increase trace volume.
- For non-terminating runs, timeout/signal hooks flush traces (`GTV_TIMEOUT_MS` supported by runner/instrumented workloads).
