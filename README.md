# MyGO

MyGO is a research compiler that lowers subsets of Go into a structural MLIR/CIRCT representation and emits SystemVerilog for simulation. The toolchain bundles a CLI, IR passes, and a Verilog backend that can be wired to other simulators or the built-in Verilator harness.

## Requirements
- Go 1.22+ with GOPATH/bin on PATH.
- `circt-opt` (from CIRCT) and `verilator` available on PATH.

```bash
circt-opt --version
verilator --version
```

---

## Simple Workload
All commands below operate on `tests/stages/simple`, which runs without FIFOs and finishes in a couple of cycles.

```bash
# 1) Compile to MLIR (default emit)
go run ./cmd/mygo compile tests/stages/simple/main.go > /tmp/simple.mlir

# 2) Simulate with the built-in Verilator harness
go run ./cmd/mygo sim tests/stages/simple/main.go
```

### Extended Regression
```bash
# Fast verification: pure-Go unit/integration coverage plus skip-aware simulator tests
go test ./...

# Full artifact validation: stage goldens plus targeted CHStone hardware/software checks
MYGO_COMPARE_GOLDENS=1 go test ./...
```
The default command keeps artifact-heavy checks disabled so a plain Go toolchain can still run the suite. Full validation enables stage goldens and the targeted `aes`/`dfsin`/`sha` CHStone regressions; simulator-dependent tests still skip if `circt-opt` or `verilator` is missing.

To refresh the committed stage baselines, run `scripts/regenerate_stage_artifacts.sh`.

---

## Docs
- `docs/compile.md` – full `mygo compile` flag reference, SSA/IR dump modes, lint workflow notes, FIFO guidance, and golden generation tips.
- `docs/sim.md` – simulator options, default Verilator flow, test structure, and how goldens/expectations work.
- `docs/backend/testdata.md` – catalog of backend SystemVerilog fixtures used in unit tests.
- `docs/fix_audit_recommendations.md` – audit follow-up checklist for regression hardening and repository cleanup.

Always update the relevant doc instead of bloating this README when you add flags or tweak flows.

---

## Repo Map
| Path | Purpose |
| ---- | ------- |
| `cmd/mygo` | CLI entry point (`compile`, `sim`, `lint`). `compile` now covers SSA/IR/MLIR/Verilog emission modes. |
| `internal/frontend`, `internal/ir`, `internal/mlir`, `internal/backend` | Compiler stages from Go loading to CIRCT emission. |
| `internal/backend/templates/simple_fifo.sv` | Reference FIFO implementation for channel-heavy workloads. |
| `tests/stages` | Golden-based stage harness (see `docs/sim.md`). |
| `scripts/` | Helper scripts such as `tidy.sh` for module hygiene. |

---

## Working for Contributors & Agents
- Run the **Workflow** commands before submitting or debugging a feature.
- When documenting or reviewing new CLI flags, place the explanation in `docs/compile.md` or `docs/sim.md` and link back here only if the quick path changes.
- Prefer `tests/stages/simple` for smoke coverage; introduce new workloads only when a behavior cannot be expressed there.
