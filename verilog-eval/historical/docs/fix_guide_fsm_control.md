# Fix Guide: FSM and Protocol Control

## Scope

This guide targets the current FSM and protocol-control cases that still compile and simulate but disagree on state-transition timing or output decode timing.

Current validation list:

- [validation_cases/fsm_control.txt](./validation_cases/fsm_control.txt)

Current cases:

- `Prob079_fsm3onehot`
- `Prob107_fsm1s`
- `Prob121_2014_q3bfsm`
- `Prob127_lemmings1`
- `Prob133_2014_q3fsm`
- `Prob139_2013_q2bfsm`
- `Prob143_fsm_onehot`
- `Prob146_fsm_serialdata`
- `Prob147_circuit10`
- `Prob149_ece241_2013_q4`
- `Prob151_review2015_fsm`
- `Prob152_lemmings3`
- `Prob155_lemmings4`

## Likely Root Causes

- next-state and output logic are still not separated cleanly in some FSM shapes
- one-hot state decode still regresses in a few designs
- protocol-detection outputs are still asserted one cycle early or late
- a few formerly compile-failing FSM cases now reach simulation and expose real timing mismatches

## Files To Inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/ir/builder_sensitivity.go](/home/qinkejiu/mygo/internal/ir/builder_sensitivity.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)

## Recommended Repair Order

1. Start with the simplest remaining FSMs:
   `Prob107_fsm1s`, `Prob079_fsm3onehot`, `Prob143_fsm_onehot`
2. Then solve the protocol-style and newly exposed control cases:
   `Prob146_fsm_serialdata`, `Prob147_circuit10`, `Prob149_ece241_2013_q4`
3. Then solve the larger benchmark FSM suites:
   `Prob121_2014_q3bfsm`, `Prob127_lemmings1`, `Prob133_2014_q3fsm`, `Prob139_2013_q2bfsm`, `Prob151_review2015_fsm`, `Prob152_lemmings3`, `Prob155_lemmings4`

## Concrete Checks

1. Confirm that the state register has exactly one update point.
2. Confirm that outputs are driven from the intended state and input phase.
3. Confirm that one-hot states stay one-hot through emission and lowering.
4. Confirm that protocol detectors hold transient states long enough for the required output pulse.

## Validation Steps

### Step 1: canonical FSM smoke suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/validate_fsm_smoke_current \
  --case Prob107_fsm1s \
  --case Prob079_fsm3onehot \
  --case Prob143_fsm_onehot \
  --case Prob146_fsm_serialdata \
  --case Prob147_circuit10 \
  --case Prob149_ece241_2013_q4
```

Expected milestone:

- all six focused FSM smoke cases should become `equivalent`

### Step 2: full FSM suite

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --cases-file verilog-eval/docs/validation_cases/fsm_control.txt \
  --output-dir verilog-eval/runs/validate_fsm_control_current
```

Expected category target:

- all cases in `fsm_control.txt` should become `equivalent`

### Step 3: full 156-case regression

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_fsm_fix_all
```
