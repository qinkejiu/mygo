# Fix Guide: Sequential Semantics and Counters

## Scope

This guide covers the remaining storage-heavy cases that still reach simulation but exhibit wrong update timing, wrong edge behavior, or wrong counter priority.

Current validation list:

- [validation_cases/sequential_semantics.txt](./validation_cases/sequential_semantics.txt)

Current cases:

- `Prob028_m2014_q4a`
- `Prob034_dff8`
- `Prob066_edgecapture`
- `Prob068_countbcd`
- `Prob074_ece241_2014_q4`
- `Prob075_counter_2bc`
- `Prob086_lfsr5`
- `Prob088_ece241_2014_q5b`
- `Prob129_ece241_2013_q8`
- `Prob135_m2014_q6b`
- `Prob136_m2014_q6`

## Likely Root Causes

- old-cycle versus new-cycle values are still mixed in a smaller set of sequential patterns
- reset, enable, and count/load priority is still wrong in some counters
- latch and DFF behavior regressed in a few cases that were previously green
- some output decode logic is still reading the wrong internal storage signal

## Files To Inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/ir/builder_sensitivity.go](/home/qinkejiu/mygo/internal/ir/builder_sensitivity.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)

## Recommended Repair Order

1. Solve the regressed primitive storage cases first:
   `Prob028_m2014_q4a`, `Prob034_dff8`, `Prob074_ece241_2014_q4`
2. Then solve the capture/count family:
   `Prob066_edgecapture`, `Prob068_countbcd`, `Prob075_counter_2bc`
3. Then solve the wide sequential datapath cases:
   `Prob086_lfsr5`, `Prob088_ece241_2014_q5b`
4. Then revisit the remaining benchmark-specific control counters:
   `Prob129_ece241_2013_q8`, `Prob135_m2014_q6b`, `Prob136_m2014_q6`

## Concrete Checks

1. Confirm that all clocked right-hand sides read old-cycle values only.
2. Confirm that reset and load priorities match the reference module exactly.
3. Confirm that latch/DFF hold behavior is preserved in addition to edge sensitivity.
4. Confirm that output decode logic reads the stable register, not an intermediate next-state signal.

## Validation Steps

### Step 1: storage/counter smoke suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/validate_sequential_smoke_current \
  --case Prob028_m2014_q4a \
  --case Prob034_dff8 \
  --case Prob074_ece241_2014_q4 \
  --case Prob066_edgecapture \
  --case Prob068_countbcd \
  --case Prob075_counter_2bc
```

Expected milestone:

- all six cases above should become `equivalent`

### Step 2: full sequential suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --cases-file verilog-eval/docs/validation_cases/sequential_semantics.txt \
  --output-dir verilog-eval/runs/validate_sequential_semantics_current
```

Expected category target:

- all cases in `sequential_semantics.txt` should become `equivalent`

### Step 3: full 156-case regression

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_sequential_fix_all
```
