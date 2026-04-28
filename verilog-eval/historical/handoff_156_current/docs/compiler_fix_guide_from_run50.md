# Compiler Fix Guide From Run-50

## Purpose

This document consolidates the failures from the first 50 spec-to-RTL cases that now look primarily like `mygo` compiler/backend issues rather than LLM output issues.

Source result set:

- [../runs/run_50_cases/results.json](../runs/run_50_cases/results.json)

Related classification note:

- [spec_to_rtl_run50_error_classification.md](./spec_to_rtl_run50_error_classification.md)

## Guiding Principle

The generated Go for the cases listed here is already close enough to the intended DSL form that further prompt tuning is unlikely to fix the root cause.

These should be treated as compiler/backend work items.

## Fix Group 1: Multi-output port export

### Affected cases

- `Prob015_vector1`
- `Prob026_alwaysblock1`
- `Prob044_vectorgates`

### Symptoms

- `iverilog_compile_failed`
- elaboration errors report missing expected ports

Examples:

- `out_hi` / `out_lo` not exported
- `out_assign` / `out_alwaysblock` not exported
- `out_or_bitwise` / `out_or_logical` / `out_not` not exported

### Likely root cause

- `out_*` globals are being collapsed or renamed incorrectly during top-level port emission
- binding/alias logic between internal signals and external ports is inconsistent for multiple outputs
- mixed scalar/vector output export may be dropping original names

### Files to inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
  - output port creation for `out_*` globals
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
  - top-level output binding logic
  - output alias naming
- [internal/backend/backend.go](/home/qinkejiu/mygo/internal/backend/backend.go)
  - only if a final Verilog post-pass rewrites names

### Concrete checks

1. Verify each `out_*` global becomes one output port with the expected external name.
2. Verify split-output modules do not collapse outputs into a single packed bus unless the benchmark expects that.
3. Verify vector outputs preserve exact bit width.

### Suggested tests

- `Prob015_vector1`
- `Prob026_alwaysblock1`
- `Prob044_vectorgates`

## Fix Group 2: Wide-vector and array lowering

### Affected cases

- `Prob017_mux2to1v`
- `Prob018_mux256to1`
- `Prob021_mux256to1v`
- `Prob023_vector100r`

### Symptoms

- large mismatch counts
- generated Go is already structurally reasonable

### Likely root cause

- incorrect bit ordering between `[N]bool` and packed Verilog vectors
- dynamic index lowering over wide arrays not matching intended LSB/MSB conventions
- assignment of whole `[N]bool` arrays lowering incorrectly

### Files to inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
  - indexed load/store lowering
  - packed vector signal typing
  - bit/element ordering assumptions
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
  - array packing and unpacking
  - top-level port flattening for wide vectors

### Concrete checks

1. Confirm `[0]` means LSB consistently across input unpacking and output repacking.
2. Confirm whole-array assignment preserves element ordering.
3. Confirm `in[sel]` on `[256]bool` matches testbench expectations exactly.
4. Confirm 4-bit slices from a 1024-bit input for `Prob021` use the correct base offset and bit significance.

### Suggested tests

- `Prob017_mux2to1v`
- `Prob018_mux256to1`
- `Prob021_mux256to1v`
- `Prob023_vector100r`

## Fix Group 3: Sequential state semantics

### Affected cases

- `Prob027_fadd`
- `Prob028_m2014_q4a`
- `Prob031_dff`
- `Prob034_dff8`
- `Prob035_count1to10`
- `Prob038_count15`
- `Prob040_count10`
- `Prob045_edgedetect2`
- `Prob046_dff8p`

### Symptoms

- outputs compile and simulate, but behavior is wrong
- mismatch patterns indicate state/update/reset errors

### Likely root causes

- latch inference still incomplete or too aggressive
- DFF semantics not preserved from generated Go to Verilog
- sequential state not held across cycles correctly
- reset style mismatches:
  - synchronous vs asynchronous
  - active-high vs active-low
- wrong edge sensitivity:
  - posedge vs negedge
- previous-value tracking not implemented correctly for edge-detect circuits

### Files to inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
  - assignment lowering
  - process construction
- [internal/ir/builder_sensitivity.go](/home/qinkejiu/mygo/internal/ir/builder_sensitivity.go)
  - sequential vs combinational classification
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
  - direct-clocked emission
  - sequential register emission
  - latch-style emission

### Per-case hints

#### `Prob027_fadd`

- sum matches, carry does not
- likely issue in boolean expression lowering or output binding for one of multiple outputs

#### `Prob028_m2014_q4a`

- known latch case
- remaining mismatch suggests latch behavior is still not completely correct in some generated forms

#### `Prob031_dff` / `Prob034_dff8`

- classic DFF behavior
- likely state retention problem

#### `Prob035_count1to10` / `Prob038_count15` / `Prob040_count10`

- counters
- likely sequential state update/reset interaction

#### `Prob045_edgedetect2`

- depends on previous cycle storage
- likely previous-state register handling issue

#### `Prob046_dff8p`

- failure message explicitly mentions reset style
- likely negative-edge and synchronous-reset semantics are mishandled

### Suggested tests

- each case above in isolation
- then grouped regression checks over all sequential cases

## Recommended Work Order

### First

Fix Group 1:

- multi-output port export

Reason:

- these fail at elaboration time and are easy to localize

### Second

Fix Group 2:

- wide-vector / array lowering

Reason:

- these are likely representation-consistency bugs and can regress multiple cases together

### Third

Fix Group 3:

- sequential semantics

Reason:

- these are the riskiest and most likely to require careful regression testing

## Minimal Regression Checklist

After every compiler-side fix, rerun:

- `Prob015_vector1`
- `Prob026_alwaysblock1`
- `Prob044_vectorgates`
- `Prob017_mux2to1v`
- `Prob018_mux256to1`
- `Prob021_mux256to1v`
- `Prob023_vector100r`
- `Prob028_m2014_q4a`
- `Prob031_dff`
- `Prob046_dff8p`

Then rerun:

- [../runs/run_50_cases/results.json](../runs/run_50_cases/results.json)

## Success Criterion

For the compiler track, the target is:

- all currently compiler-classified failures should either:
  - pass, or
  - reduce cleanly to a smaller, better-understood compiler issue set

The important point is that these cases should no longer require prompt-side workarounds.
