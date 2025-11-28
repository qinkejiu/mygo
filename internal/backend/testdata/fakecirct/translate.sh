#!/bin/sh
set -e
INPUT=""
for arg in "$@"; do
  case "$arg" in
    --export-verilog)
      ;;
    *)
      INPUT="$arg"
      ;;
  esac
done
if [ -z "$INPUT" ]; then
  INPUT="/dev/stdin"
fi
{
  echo "// verilog translator"
  cat "$INPUT"
}
