# verilog-eval

This directory now exposes a curated top-level snapshot for the current 156-case spec-to-RTL workflow.

## Primary Files And Folders

- `prompt_template.md`
  Generation prompt template for turning one benchmark case into one MyGO Go file.
- `repair_go_prompt_template.md`
  Repair-oriented prompt template for patching an existing Go case instead of regenerating from scratch.
- `go_dsl.md`
  Concise description of the Go/MyGO source format expected by this workflow.
- `reference_verilog/`
  156 reference Verilog files copied from the benchmark dataset.
- `test_verilog/`
  156 testbench files copied from the benchmark dataset.
- `current_go_156/`
  156 current Go workspaces, one per case, each containing `main.go`.
- `historical/notes/`
  Older repair notes and batch-specific fix prompts kept for reference.
- `historical/runs/`
  Archived verification results and per-case artifacts.
- `historical/scripts/`
  Recheck/evaluation scripts used to validate the current tree.

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

Latest local workspace recheck archived on `2026-05-06`:

- `historical/runs/recheck_current_156/20260506_current_workspace/results.json`
- `historical/runs/recheck_current_156/20260506_current_workspace/summary.json`

## Notes

- `current_go_156/` is a copied working set, intended to be the direct 156-case Go collection.
- `current_go_156/**/.mygo-tmp/` is treated as disposable local build output and is ignored.
- All legacy materials are grouped under `historical/`.
- The original benchmark dataset is now under `historical/dataset_spec-to-rtl/`.
- Historical runs, scripts, repair notes, and the older handoff snapshot are preserved under `historical/`.
- Older partial-fix result sets and intermediate repair runs are preserved under `historical/runs/`.
