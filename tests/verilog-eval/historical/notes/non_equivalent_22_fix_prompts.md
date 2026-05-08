# Non-Equivalent 22 Cases: Classification And English Fix Prompts

Note:

- This file reflects the earlier 22-case non-equivalent backlog used as a repair plan.
- It is preserved as a historical repair worksheet.
- After later fixes, the current validated status is no longer 22 non-equivalent cases.
- See `README.md` for the latest validated counts and the current remaining non-passing cases.

This note groups the 22 currently non-equivalent Go cases from the latest full recheck:

- source results: `historical/runs/recheck_current_156_20260428/results.json`
- summary: `134 equivalent / 22 not_equivalent`

The goal of these prompts is to repair the existing Go files in `current_go_156/`, not to regenerate from scratch without constraints.

## Universal Constraints For All Repair Prompts

Use these constraints together with every batch prompt:

- the repaired Go must use common, standard, RTL-like structure
- do not use weird tricks, ad-hoc hacks, obscure patterns, or test-specific special cases
- the result must be synthesizable
- use conventional sequential structure: persistent package-level registers, combinational next-state/next-value logic, and clocked updates on the correct edge
- do not keep persistent state in local variables inside `TopModule`
- do not introduce fake sequential state into a combinational design
- preserve exact bit ordering, reset behavior, and edge behavior
- return one complete Go file only

Reusable template:

- `repair_go_prompt_template.md`

## Batch 1: Small Sequential Primitives And Small/Medium FSMs

### Cases

- `Prob056_ece241_2013_q7`
- `Prob121_2014_q3bfsm`
- `Prob127_lemmings1`
- `Prob129_ece241_2013_q8`
- `Prob133_2014_q3fsm`
- `Prob136_m2014_q6`
- `Prob139_2013_q2bfsm`
- `Prob146_fsm_serialdata`
- `Prob151_review2015_fsm`
- `Prob152_lemmings3`

### Common Failure Pattern

- State is declared as a local variable inside `TopModule`, so it does not persist across cycles.
- The code often uses `if clk` as if level-high means "a clock event".
- Moore/Mealy outputs are sometimes computed from the wrong version of state.
- Async reset and sync reset behavior is not modeled carefully enough.

### English Prompt

```text
Repair this MyGO Go file so that it matches the reference sequential behavior exactly.

Important requirements:
- Do not regenerate a different design style. Repair the current file.
- Keep `package main`, one `func TopModule(...)`, and package-level `out_*` outputs.
- Any sequential state must be stored in package-level variables, not local variables inside `TopModule`.
- Add a package-level previous-clock register when needed, and update state only on the correct clock edge.
- If reset is asynchronous in the benchmark, make reset take effect immediately without waiting for a clock edge.
- If reset is synchronous, only apply it on the active clock edge.
- Separate the design into:
  1. persistent state registers,
  2. next-state computation from the current state and inputs,
  3. state updates on the correct edge,
  4. output logic using the correct Moore or Mealy semantics.
- Do not treat `if clk` as a generic substitute for edge-triggered behavior unless it is explicitly modeling the active edge with a stored previous clock.
- Do not declare `state`, `next`, shift registers, or history variables as local variables if they must persist between calls.
- Preserve exact port names and output names.

For this repair, prioritize correctness of:
- state persistence across cycles,
- exact reset behavior,
- exact output timing relative to state transitions,
- correct serial/FSM transition ordering.

Return one complete Go file only.
```

### Validation

Run this batch after repairing the listed Go files:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_non_equivalent_batch1 \
  --case Prob056_ece241_2013_q7 \
  --case Prob121_2014_q3bfsm \
  --case Prob127_lemmings1 \
  --case Prob129_ece241_2013_q8 \
  --case Prob133_2014_q3fsm \
  --case Prob136_m2014_q6 \
  --case Prob139_2013_q2bfsm \
  --case Prob146_fsm_serialdata \
  --case Prob151_review2015_fsm \
  --case Prob152_lemmings3
```

Success criteria:

- `summary.json` shows all 10 cases as `equivalent`
- no `go_compile_failed`
- no `iverilog_compile_failed`
- no `simulation_timeout`

## Batch 2: Stateful Counters, Timers, History Registers, And Datapaths

### Cases

- `Prob068_countbcd`
- `Prob141_count_clock`
- `Prob153_gshare`
- `Prob155_lemmings4`
- `Prob156_review2015_fancytimer`

### Common Failure Pattern

- Counter/register arrays are declared locally and reset every call.
- The implementation mixes "current state" and "next state" in the same cycle.
- Multi-register updates are not performed atomically from the old state.
- Special rollover/recovery rules are incomplete.

### English Prompt

```text
Repair this MyGO Go file so that the stateful datapath matches the reference design exactly.

Important requirements:
- Keep the MyGO structure: `package main`, one `TopModule`, package-level `out_*` outputs.
- All persistent registers must be package-level variables.
- Do not declare counters, history registers, predictor tables, timers, clock state, or fall-duration state as local variables if they must survive across calls.
- Use a package-level previous-clock register when edge detection is required.
- Model the design as real sequential hardware:
  1. compute all next values from the old register state,
  2. commit the register updates only on the correct clock edge,
  3. keep outputs consistent with the intended current-state or next-state semantics.
- Preserve exact priority rules. In particular, handle reset, load, enable, training, prediction, ack, rollover, and special-case transitions in the same priority order as the benchmark.
- For counters and clocks, implement exact BCD or bounded-counter rollover logic.
- For tables or memories, preserve the stored contents across cycles and update only the addressed entry when required.
- For branch prediction logic, prediction must observe the pre-training table state in the same cycle, and training/mispredict recovery must follow the documented priority.
- Avoid shortcuts such as recomputing outputs from partially updated state.

Return one complete Go file only.
```

### Validation

Run this batch after repairing the listed Go files:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_non_equivalent_batch2 \
  --case Prob068_countbcd \
  --case Prob141_count_clock \
  --case Prob153_gshare \
  --case Prob155_lemmings4 \
  --case Prob156_review2015_fancytimer
```

Success criteria:

- `summary.json` shows all 5 cases as `equivalent`
- no compile failures
- no timeout statuses
- especially verify that the very high-mismatch cases (`Prob141_count_clock`, `Prob156_review2015_fancytimer`) drop to zero mismatches

## Batch 3: Wide Cellular Automata And Large Sequential Array State

### Cases

- `Prob108_rule90`
- `Prob124_rule110`
- `Prob144_conwaylife`

### Common Failure Pattern

- The design is sequential, but array/state update timing is not modeled precisely.
- The current board/state and next board/state are not cleanly separated.
- Edge/wraparound behavior may be wrong.
- Outputs are updated directly while the next generation is still being computed.

### English Prompt

```text
Repair this MyGO Go file so that the wide sequential array state updates exactly match the reference automaton.

Important requirements:
- Keep the MyGO file structure unchanged except for the repair.
- Store the persistent board/state in package-level registers.
- Use package-level previous-clock state if edge detection is needed.
- On a load cycle, load the full state exactly as specified.
- On a normal update cycle, compute the entire next generation from the old generation only.
- Do not mix old and newly written cells while computing the next generation.
- Use a separate temporary `next` array for the full update, then commit it on the active clock edge.
- Implement wraparound/toroidal neighbors exactly where required.
- Preserve the required bit ordering. Index 0 must continue to represent the least-significant or first logical cell exactly as expected by the benchmark.
- Do not partially update outputs before the full next-state computation is complete.

Return one complete Go file only.
```

### Validation

Run this batch after repairing the listed Go files:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_non_equivalent_batch3 \
  --case Prob108_rule90 \
  --case Prob124_rule110 \
  --case Prob144_conwaylife
```

Success criteria:

- `summary.json` shows all 3 cases as `equivalent`
- no compile failures
- no partial improvement accepted: each case must reach zero mismatches, not just fewer mismatches

## Batch 4: Pure Combinational Bit/Vector Logic And Next-State Bit Logic

### Cases

- `Prob092_gatesv100`
- `Prob135_m2014_q6b`

### Common Failure Pattern

- Neighbor direction or wraparound is reversed.
- One bit position is handled with the wrong boundary rule.
- The implementation confuses "current state bit" with "next-state logic for that bit".

### English Prompt

```text
Repair this MyGO Go file as a pure combinational design.

Important requirements:
- Do not introduce sequential state or clock tracking.
- Keep `package main`, one `TopModule`, and package-level `out_*` outputs.
- Re-derive each output bit directly from the current inputs only.
- For vector-neighbor problems, verify the exact meaning of “left”, “right”, boundary conditions, and wraparound.
- For FSM next-state-bit problems, do not simply return the current state bit. Implement the requested next-state logic truth table exactly.
- Prefer explicit bit formulas or small case logic over ambiguous loop behavior if it improves correctness.
- Preserve exact bit ordering and output widths.

Return one complete Go file only.
```

### Validation

Run this batch after repairing the listed Go files:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_non_equivalent_batch4 \
  --case Prob092_gatesv100 \
  --case Prob135_m2014_q6b
```

Success criteria:

- `summary.json` shows both cases as `equivalent`
- no compile failures
- zero mismatches for both cases

## Batch 5: Waveform-Inferred Mixed Edge / Small Sequential Logic

### Cases

- `Prob145_circuit8`
- `Prob147_circuit10`

### Common Failure Pattern

- The design was inferred from waveforms, but the update equation is slightly wrong.
- Mixed combinational and sequential behavior is not separated cleanly.
- One design uses a negative-edge register plus a combinational output, while the other uses a one-bit majority-state register.

### English Prompt

```text
Repair this MyGO Go file by matching the waveform-defined sequential behavior exactly.

Important requirements:
- Keep one `TopModule` and package-level `out_*` outputs.
- Store the internal register as a package-level variable.
- Store the previous clock as a package-level variable if the design depends on a specific edge.
- Separate combinational outputs from sequential register updates.
- If one output is purely combinational, compute it directly from the current inputs and current clock level.
- If a register updates on the negative edge or positive edge, detect that exact edge explicitly using the stored previous clock.
- Do not change the intended one-bit state equation. Repair the exact boolean update function instead.
- Preserve exact output timing seen by the benchmark waveform.

Return one complete Go file only.
```

### Validation

Run this batch after repairing the listed Go files:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_non_equivalent_batch5 \
  --case Prob145_circuit8 \
  --case Prob147_circuit10
```

Success criteria:

- `summary.json` shows both cases as `equivalent`
- no compile failures
- zero mismatches for both cases

## Recommended Repair Order

1. Batch 4
2. Batch 5
3. Batch 1
4. Batch 2
5. Batch 3

Rationale:

- Batch 4 and Batch 5 are the smallest and should converge fastest.
- Batch 1 fixes many medium-complexity control cases with the same state-persistence issue.
- Batch 2 contains deeper sequential datapath/state interactions.
- Batch 3 contains the widest state and the most expensive full-array update logic.

## Full Recheck After Batch Repairs

After one or more batches are repaired, run the full 156-case suite:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/recheck_after_non_equivalent_repairs
```

Success criteria:

- the batch-specific repaired cases remain `equivalent`
- total `equivalent` count is higher than the current baseline of `134`
- no new compile failures are introduced in previously passing cases
