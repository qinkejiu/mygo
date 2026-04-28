# Fix Guide: Compile Pipeline Regressions

## Scope

This guide covers the remaining case that still fails before Verilog generation completes.

Current validation list:

- [validation_cases/compile_pipeline_regressions.txt](./validation_cases/compile_pipeline_regressions.txt)

Current case:

- `Prob092_gatesv100`

Observed symptom:

- `go_compile_failed`
- CIRCT reports:
  `use of value '%in' expects different type than prior uses: '!hw.inout<i100>' vs 'i100'`

Likely cause:

- the emitter or backend now mixes plain packed input values with `sv.read_inout` on the same symbol
- one lowering path treats the top-level input as a normal `i100`, while another path treats it as an `!hw.inout<i100>`

## Files To Inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/ir/builder_sensitivity.go](/home/qinkejiu/mygo/internal/ir/builder_sensitivity.go)
- [internal/ir/ir.go](/home/qinkejiu/mygo/internal/ir/ir.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
- [internal/backend/backend.go](/home/qinkejiu/mygo/internal/backend/backend.go)
- [cmd/mygo/main.go](/home/qinkejiu/mygo/cmd/mygo/main.go)

## Recommended Repair Order

1. Reproduce the failure on `Prob092_gatesv100` only.
2. Trace where `%in` is declared and where `sv.read_inout %in` is emitted.
3. Normalize the port handling so the same symbol is not reused as both `i100` and `!hw.inout<i100>`.

## Concrete Checks

1. Confirm that top-level packed inputs are emitted consistently as either plain value ports or inout-backed ports, but not both.
2. Confirm that `sv.read_inout` is only used on real inout objects.
3. Confirm that the generated MLIR for `Prob092_gatesv100` no longer references `%in` with conflicting types.

## Validation Steps

### Focused validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/validate_compile_pipeline_regressions \
  --case Prob092_gatesv100
```

Expected milestone:

- `Prob092_gatesv100` should stop failing in MLIR/CIRCT
- it should downgrade to `not_equivalent` at worst, or become `equivalent`

### Full 156-case regression

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_compile_pipeline_fix_all
```
