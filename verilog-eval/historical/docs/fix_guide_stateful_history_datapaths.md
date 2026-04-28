# Fix Guide: Stateful History Datapaths

## Scope

This guide covers the remaining large-state designs where the dominant risk is wrong old/new-state ordering over history registers, shift chains, cellular automata, or predictors.

Current validation list:

- [validation_cases/stateful_history_datapaths.txt](./validation_cases/stateful_history_datapaths.txt)

Current cases:

- `Prob108_rule90`
- `Prob124_rule110`
- `Prob141_count_clock`
- `Prob144_conwaylife`
- `Prob153_gshare`
- `Prob156_review2015_fancytimer`

## Why These Cases Still Matter

Several simpler sequential and packed-data cases are now fixed.
The remaining large-state cases isolate the hardest part that is still left:

- deriving next state from wide old state
- preserving update direction and boundary behavior
- preventing partial in-place updates from corrupting later reads in the same cycle

## Likely Root Causes

- next-state arrays are still emitted as partially in-place updates
- cellular automata still read a mixture of old and already-updated neighbor values
- predictor/history structures still combine current-cycle and next-cycle state

## Files To Inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/ir/builder_sensitivity.go](/home/qinkejiu/mygo/internal/ir/builder_sensitivity.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)

## Recommended Repair Order

1. Start with the local-neighbor update family:
   `Prob108_rule90`, `Prob124_rule110`, `Prob144_conwaylife`
2. Then solve the large benchmark state machines:
   `Prob141_count_clock`, `Prob153_gshare`, `Prob156_review2015_fancytimer`

## Concrete Checks

1. Confirm that next-state wide values are built separately from old-state values.
2. Confirm that cellular automata read neighbors only from old state.
3. Confirm that branch-history style updates do not alias with the structure being read.

## Validation Steps

### Step 1: history smoke suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/validate_stateful_history_smoke_current \
  --case Prob108_rule90 \
  --case Prob124_rule110 \
  --case Prob141_count_clock \
  --case Prob153_gshare \
  --case Prob156_review2015_fancytimer
```

Expected milestone:

- all five smoke cases should become `equivalent`

### Step 2: full stateful-history suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --cases-file verilog-eval/docs/validation_cases/stateful_history_datapaths.txt \
  --output-dir verilog-eval/runs/validate_stateful_history_current
```

Expected category target:

- all cases in `stateful_history_datapaths.txt` should become `equivalent`

### Step 3: full 156-case regression

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_stateful_history_fix_all
```
