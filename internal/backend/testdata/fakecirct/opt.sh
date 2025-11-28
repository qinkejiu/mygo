#!/bin/sh
set -e
PIPELINE=""
OUT=""
IN=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --pass-pipeline=*)
      PIPELINE="${1#*=}"
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
{
  echo "// opt:${PIPELINE}"
  cat "$IN"
} > "$OUT"
