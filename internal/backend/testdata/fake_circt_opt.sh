#!/bin/sh
set -e
PIPELINE=""
OUT=""
IN=""
EXPORT=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --pass-pipeline=*)
      PIPELINE="${1#*=}"
      shift
      ;;
    --export-verilog)
      EXPORT=1
      shift
      ;;
    -o)
      OUT="$2"
      shift 2
      ;;
    *)
      IN="$1"
      shift
      ;;
  esac
done
if [ -z "$OUT" ]; then
  echo "missing -o" >&2
  exit 1
fi
if [ -z "$IN" ]; then
  IN="/dev/stdin"
fi
PIPELINE_VALUE="$PIPELINE"
if [ -z "$PIPELINE_VALUE" ] && [ -f "$IN" ]; then
  PIPELINE_VALUE="$(grep -m1 '^// opt:' "$IN" | sed 's#// opt:##')"
fi
{
  echo "// opt:${PIPELINE}"
  cat "$IN"
} > "$OUT"
if [ "$EXPORT" -ne 0 ]; then
  {
    echo "// circt-opt export"
    echo "// pipeline:${PIPELINE_VALUE}"
    cat "$IN"
  }
fi
