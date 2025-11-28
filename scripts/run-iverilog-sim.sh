#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <design.sv> [extra vvp args...]" >&2
  exit 1
fi

SV_INPUT="$1"
shift || true

IVERILOG_BIN="${IVERILOG:-iverilog}"
VVP_BIN="${VVP:-vvp}"

if ! command -v "$IVERILOG_BIN" >/dev/null 2>&1; then
  echo "missing iverilog (set IVERILOG=/path/to/iverilog)" >&2
  exit 1
fi
if ! command -v "$VVP_BIN" >/dev/null 2>&1; then
  echo "missing vvp (set VVP=/path/to/vvp)" >&2
  exit 1
fi

BUILD_DIR="$(mktemp -d -t mygo-sim-XXXXXX)"
trap 'rm -rf "$BUILD_DIR"' EXIT

OUTPUT_EXE="$BUILD_DIR/sim.out"

"$IVERILOG_BIN" -g2012 -o "$OUTPUT_EXE" "$SV_INPUT"
"$VVP_BIN" "$OUTPUT_EXE" "$@"
