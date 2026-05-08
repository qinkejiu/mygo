# Current Remaining 5 Cases: Focused English Repair Prompts

Note:

- This file is now a historical repair note.
- It reflects the state before the final `156 / 156 equivalent` validation.
- See `README.md` for the latest all-green status.

This file reflects the latest validated full recheck on `2026-04-28`.

Current validated baseline:

- source: `historical/runs/recheck_current_156_reverify_20260428/results.json`
- summary: `151 equivalent / 4 not_equivalent / 1 go_compile_timeout`

Current non-passing cases:

- `Prob092_gatesv100` (`go_compile_timeout`)
- `Prob108_rule90` (`not_equivalent`, `7078 / 7121` mismatches)
- `Prob124_rule110` (`not_equivalent`, `6240 / 6283` mismatches)
- `Prob144_conwaylife` (`not_equivalent`, `794 / 5023` mismatches)
- `Prob153_gshare` (`not_equivalent`, `433 / 1083` mismatches)

## Universal Prompt Prefix

Use this prefix together with one of the batch prompts below.

```text
Repair the existing MyGO Go file so that it matches the reference behavior exactly.

You are modifying the existing Go file, not replacing it with a completely different ad-hoc style.

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

## Batch 1: Compile-Time Vector Cleanup

### Cases

- `Prob092_gatesv100`

### Why This Is A Group

- This is the only remaining compile-time timeout.
- The current implementation is too expanded and should be rewritten into compact, ordinary vector logic.

### English Repair Prompt

```text
Modify the existing Go file so that it remains purely combinational, compiles quickly, and matches the 100-bit neighbor-vector specification exactly.

Specific repair requirements:
- Keep this as a pure combinational design.
- Do not introduce any clocked state.
- Rewrite the current implementation into compact, conventional fixed-bound loops.
- Avoid gigantic hand-unrolled assignment patterns.
- Implement the exact semantics:
  - `out_both[i]` uses the left neighbor, with `out_both[99] = 0`
  - `out_any[i]` uses the right neighbor, with `out_any[0] = 0`
  - `out_different[i]` uses wraparound for the left neighbor
- Preserve the benchmark bit ordering exactly.
- Keep the code simple, regular, and synthesizable.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining5_batch1 \
  --case Prob092_gatesv100
```

Success criteria:

- `Prob092_gatesv100` becomes `equivalent`
- no `go_compile_timeout`
- no compile failure

## Batch 2: Wide Cellular Automata

### Cases

- `Prob108_rule90`
- `Prob124_rule110`
- `Prob144_conwaylife`

### Why This Is A Group

- All three are wide sequential automata.
- They need persistent state plus exact full-width next-state computation from the old generation only.
- Common failure mode: wrong boundary rules or mixing old and new cell values during the same update.

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
  - Rule 90 uses zero outside both ends,
  - Rule 110 uses zero outside both ends,
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
  --output-dir tests/verilog-eval/historical/runs/validate_remaining5_batch2 \
  --case Prob108_rule90 \
  --case Prob124_rule110 \
  --case Prob144_conwaylife
```

Success criteria:

- all 3 cases are `equivalent`
- no compile failures
- no timeouts

## Batch 3: Predictor Timing Semantics

### Cases

- `Prob153_gshare`

### Why This Is A Group

- This case has unusual speculative-state semantics and is best handled alone.
- The common mistake is updating history or PHT in the wrong cycle or with the wrong priority.

### English Repair Prompt

```text
Modify the existing Go file so that it implements the gshare predictor with ordinary synthesizable register-transfer structure and exact timing semantics.

Specific repair requirements:
- Keep the PHT and global history register as package-level persistent state.
- Compute outputs from the current predictor state, not from already-updated same-cycle state unless the spec explicitly requires that.
- Respect the required timing:
  - prediction in a cycle sees the pre-training PHT state,
  - training updates the PHT on the active clock edge,
  - if both prediction and misprediction recovery want to update history in the same cycle, training recovery takes precedence.
- Preserve exact 7-bit masking and 2-bit saturating-counter semantics.
- Preserve the distinction between prediction-side history and training-side recovery semantics.
- Keep the implementation compact, normal, and synthesizable.

Return one complete repaired Go file only.
```

### Validation

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/validate_remaining5_batch3 \
  --case Prob153_gshare
```

Success criteria:

- `Prob153_gshare` is `equivalent`
- no compile failure
- no timeout

## Full Recheck After Repairs

After repairing one or more batches, run the full suite again:

```bash
env GOCACHE=/tmp/mygo-gocache python3 tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py \
  --go-root tests/verilog-eval/current_go_156 \
  --dataset-dir tests/verilog-eval/historical/dataset_spec-to-rtl \
  --output-dir tests/verilog-eval/historical/runs/recheck_after_remaining5_repairs
```

Success criteria:

- total `equivalent` is higher than the current baseline of `151`
- no new compile failures are introduced
- no previously passing cases regress
