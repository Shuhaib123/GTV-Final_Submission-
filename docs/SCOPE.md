# Scope Summary (v1)

This project follows ADR 0001 (docs/adr/0001-scope-v1.md). Highlights:

- Entrypoints: We do not auto-wrap arbitrary `main` functions. Runners initialize tracing.
- Tests: We do not auto-instrument `go test`. To trace tests, call the runner or annotate helpers.
- CLI/Frameworks: We do not auto-detect arbitrary CLI/HTTP frameworks. Use `// gtv:task`, `// gtv:http`, etc.
- Loops: Complex loops are not instrumented automatically. Use `// gtv:loop=label` to opt in (forced per-iteration regions may rewrite unlabelled break/continue and, when legal, bare return).
- Config: No per-call template DSL. A small set of built-in I/O regions with include/exclude and per-library toggles.

See the README for usage, IO toggles, and loop behavior. See ADR 0001 for full rationale and decisions.
