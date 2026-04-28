# Fix Guide: Combinational and Small-Control Datapaths

## Scope

This guide targets the remaining local combinational datapaths that still compile and simulate but disagree on packed boolean behavior.

Current validation list:

- [validation_cases/combinational_datapaths.txt](./validation_cases/combinational_datapaths.txt)

Current cases:

- `Prob030_popcount255`
- `Prob052_gates100`
- `Prob145_circuit8`

## Why This Category Exists

Most of the earlier local truth-table regressions are now fixed.
The remaining datapath failures are fewer and more specific:

- one wide popcount-style datapath
- one wide gates-vector datapath
- one smaller branchy control/datapath case

## Likely Root Causes

- packed bit ordering or bitwise aggregation is still wrong in some wide datapaths
- reduction-style or branch-selected outputs still bind the wrong value in a few cases
- local control/dataflow merging is still incorrect in `Prob145_circuit8`

## Files To Inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/ir/ir.go](/home/qinkejiu/mygo/internal/ir/ir.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)

## Recommended Repair Order

1. Start with `Prob030_popcount255`.
2. Then solve `Prob052_gates100`.
3. Then revisit `Prob145_circuit8`.

## Concrete Checks

1. Confirm that packed bit order is stable through bitwise aggregation and reduction logic.
2. Confirm that wide output buses are not inverted or fully duplicated.
3. Confirm that branch-selected outputs preserve the intended precedence order in `Prob145_circuit8`.

## Validation Steps

### Focused smoke suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/validate_combinational_datapaths \
  --case Prob030_popcount255 \
  --case Prob052_gates100 \
  --case Prob145_circuit8
```

Expected milestone:

- `Prob030_popcount255` should become `equivalent`
- `Prob052_gates100` should become `equivalent`
- `Prob145_circuit8` should become `equivalent`

### Full combinational suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --cases-file verilog-eval/docs/validation_cases/combinational_datapaths.txt \
  --output-dir verilog-eval/runs/validate_combinational_datapaths
```

Expected category target:

- all cases in `combinational_datapaths.txt` should become `equivalent`

### Full 156-case regression

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_combinational_fix_all
```
