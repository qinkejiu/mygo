#!/usr/bin/env bash
# Regenerate committed stage goldens for MLIR, Verilog, and simulation.
# Usage: ./scripts/regenerate_stage_artifacts.sh [case ...]
# When no cases are provided it discovers all tests/stages/*/main.go.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-${ROOT}/.gocache}"
LOWER_OPTS="${LOWER_OPTS:-locationInfoStyle=none,omitVersionComment}"
EMITS="${EMITS:-mlir sv sim}"

mkdir -p "${GOCACHE}"
export GOCACHE

discover_cases() {
	find "${ROOT}/tests/stages" -maxdepth 2 -name main.go -print \
		| sed "s#${ROOT}/tests/stages/##" \
		| sed 's#/main.go##' \
		| sort
}

sim_cycles() {
	local case="$1"
	case "${case}" in
		simple) echo 1 ;;
		simple_branch) echo 2 ;;
		simple_print) echo 1 ;;
		type_mismatch) echo 1 ;;
		comb_adder) echo 1 ;;
		comb_bitwise) echo 1 ;;
		comb_concat) echo 1 ;;
		simple_channel) echo 2 ;;
		phi_loop) echo 8 ;;
		pipeline1) echo 10 ;;
		pipeline2) echo 12 ;;
		router_csp) echo 16 ;;
		*)
			echo "unknown stage workload: ${case}" >&2
			return 1
			;;
	esac
}

run_case() {
	local case="$1"
	local src="${ROOT}/tests/stages/${case}/main.go"
	if [[ ! -f "${src}" ]]; then
		echo "skip ${case}: missing ${src}" >&2
		return
	fi
	local dir
	dir="$(dirname "${src}")"
	for kind in ${EMITS}; do
		echo "[generate] ${case} -> ${kind}"
		case "${kind}" in
			ssa)
				go run ./cmd/mygo compile -emit=ssa -o "${dir}/main.ssa" "${src}"
				;;
			ir)
				go run ./cmd/mygo compile -emit=ir -o "${dir}/main.ir" "${src}"
				;;
			mlir)
				go run ./cmd/mygo compile -emit=mlir -o "${dir}/main.mlir.golden" "${src}"
				;;
			sv|verilog)
				go run ./cmd/mygo compile \
					-emit=verilog \
					--circt-lowering-options="${LOWER_OPTS}" \
					-o "${dir}/main.sv.golden" \
					"${src}"
				;;
			sim)
				go run ./cmd/mygo sim \
					--keep-artifacts=false \
					--sim-max-cycles "$(sim_cycles "${case}")" \
					"${src}" > "${dir}/main.sim.golden"
				;;
			*)
				echo "unknown emit ${kind}, skipping" >&2
				;;
		esac
	done
}

cases=("$@")
if [[ ${#cases[@]} -eq 0 ]]; then
	while IFS= read -r c; do
		cases+=("$c")
	done < <(discover_cases)
fi

for case in "${cases[@]}"; do
	run_case "${case}"
done
