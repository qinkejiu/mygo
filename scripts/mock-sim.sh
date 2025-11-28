#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "mock simulator requires at least one Verilog source" >&2
  exit 1
fi

for src in "$@"; do
  if [ ! -f "$src" ]; then
    echo "mock simulator missing input: $src" >&2
    exit 1
  fi
done

if [ -z "${MYGO_SIM_TRACE:-}" ]; then
  echo "MYGO_SIM_TRACE must point to a trace file" >&2
  exit 1
fi

cat "$MYGO_SIM_TRACE"
