# ADR 0001: Instrumentation Scope for v1

## Decisions

- Do not auto-wrap arbitrary `main` functions.
  - Runners (gtv-live/gtv-runner) are responsible for starting/stopping tracing.
  - Users explicitly call runner entrypoints; libraries remain unmodified.

- Do not implement automatic test or benchmark instrumentation.
  - Tests can opt-in by calling the runner or using `// gtv:task` hints on helpers.

- Do not build generic CLI/framework auto-detection (cobra, urfave, etc.).
  - Annotate roots with `// gtv:task` or specific hints instead.

- Do not implement a per-call template DSL in configuration.
  - Keep a small fixed set of I/O regions with package/function filters.

- Do not automatically instrument loops with complex control flow.
  - Prefer explicit `// gtv:loop=label` hints for hot loops; still conservatively skip unsafe bodies.

## Rationale

- Keeps the implementation simple and predictable.
- Reduces “magic” and surprising behavior across diverse codebases.
- Focuses on high-impact, broadly useful instrumentation first (goroutines, channels, common I/O).

## Notes

- I/O classification is opt-in via flags/env/config. Fine-grained toggles exist for `encoding/json` and `database/sql` in addition to a global IO switch.
- Loop hints (`// gtv:loop=label`) allow per-loop regions; for safety, only applied when the loop body is considered safe to wrap.

