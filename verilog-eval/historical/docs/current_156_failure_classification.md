# Current 156-Case Failure Classification

This document describes the current failure state after the latest full recheck on April 21, 2026.

Primary result set:

- [../runs/recheck_current_156_20260421/results.json](../runs/recheck_current_156_20260421/results.json)
- [../runs/recheck_current_156_20260421/summary.json](../runs/recheck_current_156_20260421/summary.json)

Comparison reference:

- [../runs/recheck_current_156_20260420_rerun3/results.json](../runs/recheck_current_156_20260420_rerun3/results.json)

## Current Status

- total: 156
- equivalent: 10
- go compile failed: 146

## Core Interpretation

This is not a broad collection of unrelated compile failures.

The dominant issue is a single systemic regression in benchmark-interface output binding.

Most cases now fail with backend errors like:

- `backend: build benchmark interface wrapper: no implementation binding found for expected output zero`
- `backend: build benchmark interface wrapper: no implementation binding found for expected output out`
- `backend: build benchmark interface wrapper: no implementation binding found for expected output z`

## Current Surviving Equivalent Cases

These are the 10 cases that still pass end-to-end:

- `Prob015_vector1`
- `Prob026_alwaysblock1`
- `Prob039_always_if`
- `Prob044_vectorgates`
- `Prob051_gates4`
- `Prob052_gates100`
- `Prob058_alwaysblock2`
- `Prob070_ece241_2013_q2`
- `Prob087_gates`
- `Prob094_gatesv`

These cases are useful as negative controls while repairing the wrapper-binding logic.

## Primary Failure Class

### Benchmark Interface Wrapper Output Binding Regression

Scope:

- effectively all currently failing cases

Symptom:

- backend cannot map expected benchmark-visible outputs to implementation-side output bindings

Likely code ownership:

- [internal/backend/backend.go](/home/qinkejiu/mygo/internal/backend/backend.go)
- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
- [internal/ir/ir.go](/home/qinkejiu/mygo/internal/ir/ir.go)

Representative failing cases:

- `Prob001_zero`
- `Prob002_m2014_q4i`
- `Prob010_mt2015_q4a`
- `Prob014_andgate`
- `Prob019_m2014_q4f`

## Primary Guide

Use the unified current guide as the main repair plan:

- [fix_guide_unified_current.md](./fix_guide_unified_current.md)

## Validation Workflow

Use the current-go recheck script:

- [../scripts/recheck_existing_go_suite.py](../scripts/recheck_existing_go_suite.py)

In this workspace sandbox, set `GOCACHE` to a writable path under `/tmp`:

```bash
env GOCACHE=/tmp/mygo-gocache python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_fix_all
```
