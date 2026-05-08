# Current Remaining 12 Cases: Regrouped English Repair Prompts

This file replaces the old 22-case repair grouping for active work.

Current validated baseline:

- source: `historical/runs/recheck_current_156_after_partial_fixes_20260428_merged/results.json`
- summary: `144 equivalent / 10 not_equivalent / 2 go_compile_timeout`

Current non-passing cases:

- `Prob092_gatesv100` (`go_compile_timeout`)
- `Prob108_rule90`
- `Prob124_rule110`
- `Prob139_2013_q2bfsm`
- `Prob141_count_clock` (`go_compile_timeout`)
- `Prob144_conwaylife`
- `Prob145_circuit8`
- `Prob146_fsm_serialdata`
- `Prob151_review2015_fsm`
- `Prob153_gshare`
- `Prob155_lemmings4`
- `Prob156_review2015_fancytimer`

## Universal Prompt Prefix

Use this prefix together with one batch prompt below.

```text
Repair the existing MyGO Go file so that it matches the reference behavior exactly.

You are modifying the existing Go file, not writing a new unrelated implementation style.

Hard requirements:
- Return one complete Go file only.
- Keep `package main`.
- Keep exactly one `func TopModule(...)`.
- Keep outputs as package-level `out_*` globals only.
- Preserve the interface and output names exactly.

Code structure requirements:
- The repaired Go must use common, standard, RTL-like structure.
- The result must be synthesizable.
- Do not use weird tricks, ad-hoc hacks, obscure patterns, or testbench-specific special cases.
- Do not write code that only tries to game the testbench.
- Prefer a normal hardware structure:
  1. package-level persistent registers for sequential state,
  2. clear combinational next-state or next-value logic,
  3. clocked state updates on the correct edge,
  4. straightforward output logic.
- For combinational designs, do not introduce fake sequential state.
- For sequential designs, do not keep persistent state in local variables inside `TopModule`.
- Preserve exact bit ordering, boundary behavior, reset behavior, and clock-edge behavior.

Validation requirement:
- The repair is only successful if the case recheck finishes with zero mismatches, no compile failure, and no timeout.

Return one complete repaired Go file only.
```

## Batch 1: Wide Vector Compile-Time Cleanup

### Cases

- `Prob092_gatesv100`

### Why This Is A Group

- The current file times out at Go compilation.
- The implementation is excessively expanded by writing many repeated scalar assignments.
- This should be rewritten into a compact, ordinary, synthesizable vector loop structure.

### English Repair Prompt

```text
Modify the existing Go file for this case so that it remains purely combinational but compiles quickly and matches the vector-neighbor specification exactly.

Specific repair requirements:
- Keep this as a pure combinational design.
- Do not introduce any clocked state.
- Rewrite the current over-expanded assignment style into a compact, conventional loop-based implementation.
- Use straightforward fixed-bound loops over the 100-bit vector.
- Implement the exact semantics:
  - `out_both[i]` compares the bit with its left neighbour, with `out_both[99] = 0`
  - `out_any[i]` compares the bit with its right neighbour, with `out_any[0] = 0`
  - `out_different[i]` uses wraparound for the left neighbour
- Preserve the benchmark bit ordering exactly.
- Avoid gigantic hand-unrolled logic that increases compile cost unnecessarily.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining12_batch1 \
  --case Prob092_gatesv100
```

Success criteria:

- `Prob092_gatesv100` becomes `equivalent`
- no `go_compile_timeout`
- no compile failure

## Batch 2: Wide Cellular Automata And Large Sequential Arrays

### Cases

- `Prob108_rule90`
- `Prob124_rule110`
- `Prob144_conwaylife`

### Why This Is A Group

- All three are large sequential cellular automata.
- They require a persistent current state plus a clean full-width next-state computation.
- Common failure mode: mixing old and new cell values during one update, or using the wrong boundary rules.

### English Repair Prompt

```text
Modify the existing Go file so that it implements the cellular automaton using a normal synthesizable sequential-array structure.

Specific repair requirements:
- Store the persistent current state in package-level registers.
- Use a package-level previous-clock register if explicit edge detection is needed.
- On each active clock edge:
  - if `load` is asserted, load the full state from the input,
  - otherwise compute the complete next generation from the old generation only, then commit it.
- Do not mix partially updated cells into the same generation calculation.
- Use a dedicated temporary `next` array for the full update.
- Implement the correct boundary rules:
  - Rule 90 and Rule 110 use zero outside the two boundaries,
  - Conway Life uses 16x16 toroidal wraparound.
- Preserve the exact bit ordering expected by the benchmark.
- Keep the code compact and conventional rather than massively hand-unrolled.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining12_batch2 \
  --case Prob108_rule90 \
  --case Prob124_rule110 \
  --case Prob144_conwaylife
```

Success criteria:

- all 3 cases are `equivalent`
- no compile failures
- no timeouts

## Batch 3: Timers And Counter Datapaths

### Cases

- `Prob141_count_clock`
- `Prob156_review2015_fancytimer`

### Why This Is A Group

- Both are timer/counter-heavy sequential designs.
- They need normal register-transfer structure with exact carry/rollover/update priority.
- One current failure is compile-time, the other is large behavioral mismatch.

### English Repair Prompt

```text
Modify the existing Go file so that it uses a conventional synthesizable timer/counter datapath structure.

Specific repair requirements:
- Keep all persistent state as package-level registers.
- Use a compact and ordinary RTL-like implementation.
- Do not create giant expanded logic or awkward special-case chains when a normal counter structure is clearer.
- Compute next values from the old register state, then commit on the correct active clock edge.
- Respect exact priority rules between reset, enable, counting, shifting, decrementing, rollover, and acknowledgement.
- For the 12-hour clock:
  - implement exact BCD seconds/minutes/hours behavior,
  - keep hours in the range 01..12,
  - toggle `pm` only at the correct transition.
- For the fancy timer:
  - detect `1101`,
  - shift in exactly 4 delay bits MSB-first,
  - count correctly using the expected FSM/datapath interaction,
  - expose `count`, `counting`, and `done` with the correct timing.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining12_batch3 \
  --case Prob141_count_clock \
  --case Prob156_review2015_fancytimer
```

Success criteria:

- both cases are `equivalent`
- no `go_compile_timeout`
- no compile failures

## Batch 4: Small Sequential Protocols And Control FSMs

### Cases

- `Prob139_2013_q2bfsm`
- `Prob145_circuit8`
- `Prob146_fsm_serialdata`
- `Prob151_review2015_fsm`
- `Prob155_lemmings4`

### Why This Is A Group

- All five are moderate-sized sequential control problems.
- Common repair theme: exact edge semantics, exact state/output timing, and exact protocol/FSM transition behavior.
- These cases should be written in a very standard FSM style, not improvised ad-hoc logic.

### English Repair Prompt

```text
Modify the existing Go file so that it uses a clean, conventional synthesizable FSM or small sequential-control structure.

Specific repair requirements:
- Keep the persistent state in package-level registers.
- Use a package-level previous-clock register where the design depends on a specific edge.
- Separate:
  1. current-state decoding,
  2. next-state computation,
  3. state updates on the correct edge,
  4. output logic.
- Match the benchmark semantics exactly:
  - synchronous active-low reset for the motor-control FSM,
  - negative-edge storage and combinational high-clock output for `circuit8`,
  - correct serial framing and LSB-first byte capture for `fsm_serialdata`,
  - exact pattern-detect / shift-enable / counting / done control for `review2015_fsm`,
  - exact falling, splatting, digging, and direction behavior for `lemmings4`.
- Do not use level-sensitive `if clk` as a substitute for edge-triggered behavior unless you are explicitly modeling edge detection with a stored previous clock.
- Keep the implementation compact, ordinary, and readable.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining12_batch4 \
  --case Prob139_2013_q2bfsm \
  --case Prob145_circuit8 \
  --case Prob146_fsm_serialdata \
  --case Prob151_review2015_fsm \
  --case Prob155_lemmings4
```

Success criteria:

- all 5 cases are `equivalent`
- no compile failures
- no timeouts

## Batch 5: Predictor And Speculative-State Update Semantics

### Cases

- `Prob153_gshare`

### Why This Is A Group

- This case has unusual sequential semantics due to simultaneous prediction and training interfaces.
- The common mistake is updating history or PHT in the wrong cycle or with the wrong priority.

### English Repair Prompt

```text
Modify the existing Go file so that it implements the gshare predictor with ordinary synthesizable register-transfer structure and exact timing semantics.

Specific repair requirements:
- Keep the PHT and global history register as package-level persistent state.
- Compute outputs from the current architectural predictor state, not from already-updated same-cycle state unless the spec explicitly requires that.
- Respect the required timing:
  - prediction in a cycle sees the pre-training PHT state,
  - training updates the PHT on the following active edge,
  - if both prediction and misprediction recovery want to update history in the same cycle, training recovery takes precedence.
- Preserve exact 7-bit masking and 2-bit saturating counter semantics.
- Keep the implementation compact, normal, and synthesizable.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining12_batch5 \
  --case Prob153_gshare
```

Success criteria:

- `Prob153_gshare` is `equivalent`
- no compile failure
- no timeout

## Full Recheck After Repairs

After one or more batches are repaired, run a full verification pass:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/recheck_after_remaining12_repairs
```

Success criteria:

- total `equivalent` is higher than the current baseline of `144`
- no new compile failures are introduced
- no previously passing cases regress
