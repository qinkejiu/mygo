# Run-50 Error Classification

## Scope

This document classifies the 20 failed cases from:

- [../runs/run_50_cases/results.json](../runs/run_50_cases/results.json)

The purpose is to separate:

- failures that are still primarily caused by LLM output quality
- failures that now look primarily like `mygo` compiler/backend problems

## Summary

Out of 50 cases:

- passed: 30
- failed: 20

Among the 20 failures, the current classification is:

- likely LLM-side: 3
- likely compiler-side: 17

## Likely LLM-Side Failures

These are cases where the generated Go source is still clearly malformed or chooses the wrong representation.

### `Prob037_review2015_count1k`

Observed failure:

- `go_compile_failed`
- unused local variable

Evidence:

- generated Go declares `counter` and never uses it

Classification:

- LLM-side

Why:

- this is a straightforward code-generation quality issue
- the compiler error is a direct consequence of malformed generated Go

### `Prob039_always_if`

Observed failure:

- `go_compile_failed`
- illegal `?` ternary syntax in Go

Evidence:

- generated Go used `cond ? a : b`

Classification:

- LLM-side

Why:

- Go has no ternary operator
- this is a direct syntax-generation failure

### `Prob043_vector5`

Observed failure:

- `not_equivalent`

Evidence:

- generated Go used a wide `uint32` output for a 25-bit problem
- the logic is overcomplicated and likely mismaps bit ordering

Classification:

- mostly LLM-side

Why:

- the chosen representation is questionable for a 25-bit port
- this looks more like a modeling choice problem than a compiler-lowering issue

## Likely Compiler-Side Failures

These are cases where the generated Go is already reasonable or canonical for the DSL, but the emitted Verilog behavior or interface is still wrong.

### Multi-output port export issues

Cases:

- `Prob015_vector1`
- `Prob026_alwaysblock1`
- `Prob044_vectorgates`

Observed failures:

- `iverilog_compile_failed`
- expected ports such as `out_hi`, `out_lo`, `out_assign`, `out_alwaysblock`, `out_or_bitwise`, `out_or_logical`, `out_not` were not exported correctly

Why this is compiler-side:

- generated Go already uses distinct `out_*` globals
- failure happens at Verilog port/interface elaboration time

### Wide-vector / wide-array lowering issues

Cases:

- `Prob017_mux2to1v`
- `Prob018_mux256to1`
- `Prob021_mux256to1v`
- `Prob023_vector100r`

Observed failures:

- `not_equivalent`
- large mismatch counts

Why this is compiler-side:

- generated Go is already a natural DSL expression:
  - array copy for 100-bit mux
  - indexed access for 256-to-1 mux
  - loop-based reversal
- remaining error is likely in bit ordering, packed/unpacked conversion, or dynamic indexing lowering

### Sequential / state-holding lowering issues

Cases:

- `Prob027_fadd`
- `Prob028_m2014_q4a`
- `Prob031_dff`
- `Prob034_dff8`
- `Prob035_count1to10`
- `Prob038_count15`
- `Prob040_count10`
- `Prob045_edgedetect2`
- `Prob046_dff8p`

Observed failures:

- `not_equivalent`
- in several cases, mismatch pattern strongly indicates wrong state retention, reset style, or edge behavior

Why this is compiler-side:

- generated Go is structurally reasonable for the requested hardware intent
- failures align with known compiler weak spots:
  - latch inference
  - DFF/register semantics
  - synchronous vs asynchronous reset
  - positive-edge vs negative-edge behavior
  - previous-cycle state tracking

### Packed-width / output-width handling

Cases:

- `Prob006_vectorr`
- `Prob009_popcount3`

Current state after prompt improvement:

- these now pass in the focused rerun

Why they matter:

- they previously exposed compiler/backend issues around packed output width handling
- they should still be kept in mind as regression-sensitive cases

## Per-Case Classification Table

### LLM-side

- `Prob037_review2015_count1k`
- `Prob039_always_if`
- `Prob043_vector5`

### Compiler-side

- `Prob015_vector1`
- `Prob017_mux2to1v`
- `Prob018_mux256to1`
- `Prob021_mux256to1v`
- `Prob023_vector100r`
- `Prob026_alwaysblock1`
- `Prob027_fadd`
- `Prob028_m2014_q4a`
- `Prob031_dff`
- `Prob034_dff8`
- `Prob035_count1to10`
- `Prob038_count15`
- `Prob040_count10`
- `Prob044_vectorgates`
- `Prob045_edgedetect2`
- `Prob046_dff8p`
- `Prob015_vector1`

## Practical Recommendation

Next work should split into two tracks:

### Prompt / LLM track

Target only:

- `Prob037_review2015_count1k`
- `Prob039_always_if`
- `Prob043_vector5`

### Compiler track

Target the remaining 17 failures through compiler/backend fixes.

See:

- [compiler_fix_guide_from_run50.md](./compiler_fix_guide_from_run50.md)
