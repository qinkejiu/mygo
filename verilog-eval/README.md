# verilog-eval

This directory now exposes a curated top-level snapshot for the current 156-case spec-to-RTL workflow.

## Primary Files And Folders

- `prompt_template.md`
  Generation prompt template for turning one benchmark case into one MyGO Go file.
- `go_dsl.md`
  Concise description of the Go/MyGO source format expected by this workflow.
- `reference_verilog/`
  156 reference Verilog files copied from the benchmark dataset.
- `test_verilog/`
  156 testbench files copied from the benchmark dataset.
- `current_go_156/`
  156 current Go workspaces, one per case, each containing `main.go`.

## Current Status

The current `current_go_156/` snapshot is now fully passing.

Latest validated full recheck on `2026-04-29`:

- total: `156`
- equivalent: `156`
- not_equivalent: `0`
- go_compile_timeout: `0`
- go_compile_failed: `0`
- iverilog_compile_failed: `0`

Result files:

- `historical/runs/recheck_current_156_reverify_20260429_rerun2/results.json`
- `historical/runs/recheck_current_156_reverify_20260429_rerun2/summary.json`

## Notes

- `current_go_156/` is a copied working set, intended to be the direct 156-case Go collection.
- All legacy materials are grouped under `historical/`.
- The original benchmark dataset is now under `historical/dataset_spec-to-rtl/`.
- Historical runs, scripts, repair notes, and the older handoff snapshot are preserved under `historical/`.
- Older partial-fix result sets and intermediate repair runs are preserved under `historical/runs/`.
