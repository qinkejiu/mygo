#!/usr/bin/env bash
# Remove transient stage artifacts while preserving checked-in *.golden baselines.
# Usage: ./scripts/clean_stage_artifacts.sh [case ...]
# When no cases are provided, cleans all under tests/stages.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

clean_case() {
	local case="$1"
	local dir="${ROOT}/tests/stages/${case}"
	if [[ ! -d "${dir}" ]]; then
		echo "skip ${case}: missing ${dir}" >&2
		return
	fi
	find "${dir}" -maxdepth 1 -type f \( \
		-name 'main.ssa' -o \
		-name 'main.ir' -o \
		-name 'main.mlir' -o \
		-name 'main.sv' -o \
		-name '*_fifos.sv' -o \
		-name 'hardware.stderr.txt' -o \
		-name 'hardware.stdout.txt' -o \
		-name 'software.stderr.txt' -o \
		-name 'software.stdout.txt' -o \
		-name 'simulation.result.txt' -o \
		-name 'sw_hw.diff.txt' \
	\) -print -delete
}

cases=("$@")
if [[ ${#cases[@]} -eq 0 ]]; then
	while IFS= read -r dir; do
		base="$(basename "${dir}")"
		cases+=("${base}")
	done < <(find "${ROOT}/tests/stages" -maxdepth 1 -mindepth 1 -type d | sort)
fi

for case in "${cases[@]}"; do
	clean_case "${case}"
done
