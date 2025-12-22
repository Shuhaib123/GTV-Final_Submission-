#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/phase0-broadcast.sh [SOURCE_PATH] [OUTPUT_DIR] [WORKLOAD_NAME]

Instruments the provided source, runs the generated workload through gtv-runner,
parses the resulting trace to JSON, and drops the canonical artifacts under the
requested directory (default: artifacts/broadcast). The generated workload file
is cleaned up after the run.

Arguments:
  SOURCE_PATH   Path to the original broadcast.go source (default: examples/broadcast_raw.go)
  OUTPUT_DIR    Where to write trace.out, trace.json, and screenshot.png (default: artifacts/broadcast)
  WORKLOAD_NAME Name given to the generated workload (default: Broadcast → workload_broadcast)
EOF
  exit 1
}

if [[ "${1:-}" =~ ^(-h|--help)$ ]]; then
  usage
fi

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_PATH="${1:-examples/broadcast_raw/main.go}"
OUTPUT_DIR="${2:-$SCRIPT_ROOT/artifacts/broadcast}"
WORKLOAD_NAME="${3:-Broadcast}"
RUN_TIMEOUT="${RUN_TIMEOUT:-10s}"

TAG_NAME="$(echo "$WORKLOAD_NAME" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9')"
if [[ -z "$TAG_NAME" ]]; then
  echo "error: workload name must contain letters or numbers" >&2
  exit 1
fi

GEN_FILE="$SCRIPT_ROOT/internal/workload/${TAG_NAME}_gen.go"

cleanup() {
  rm -f "$GEN_FILE"
}
trap cleanup EXIT

cd "$SCRIPT_ROOT"
CACHE_DIR="${CACHE_DIR:-$SCRIPT_ROOT/.cache/go-build}"
mkdir -p "$CACHE_DIR"
export GOCACHE="$CACHE_DIR"
mkdir -p "$OUTPUT_DIR"

echo "Instrumenting $SRC_PATH as $WORKLOAD_NAME..."
go run ./cmd/gtv-instrument -in "$SRC_PATH" -name "$WORKLOAD_NAME"

if [[ -f "$GEN_FILE" ]]; then
  python3 - <<'PY' "$GEN_FILE" "$TAG_NAME"
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
tag = sys.argv[2]
lines = path.read_text().splitlines()
idx = 0
while idx < len(lines) and (lines[idx].startswith("//go:build") or lines[idx].startswith("// +build")):
    idx += 1
rest = "\n".join(lines[idx:]).lstrip("\n")
path.write_text(f"//go:build workload_{tag}\n// +build workload_{tag}\n\n{rest}\n")
PY
fi

echo "Running gtv-runner with workload ${TAG_NAME} (timeout=${RUN_TIMEOUT})..."
go run -tags "workload_${TAG_NAME}" ./cmd/gtv-runner -workload="$TAG_NAME" -timeout="$RUN_TIMEOUT" > "$OUTPUT_DIR/trace.out"

echo "Parsing $OUTPUT_DIR/trace.out → $OUTPUT_DIR/trace.json..."
go run . -parse "$OUTPUT_DIR/trace.out" -json "$OUTPUT_DIR/trace.json"

echo "Writing placeholder screenshot at $OUTPUT_DIR/screenshot.png..."
python3 - <<'PY' "$OUTPUT_DIR/screenshot.png"
import base64
import pathlib
PNG = b"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR4nGNgYAAAAAMAASsJTYQAAAAASUVORK5CYII="
pathlib.Path(__import__("sys").argv[1]).write_bytes(base64.b64decode(PNG))
PY

echo "Phase 0 artifacts ready under $OUTPUT_DIR"
