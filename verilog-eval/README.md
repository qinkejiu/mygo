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

The current `current_go_156/` snapshot is the latest 156-case handoff set, including both equivalent and not-yet-equivalent cases.

Latest validated full recheck on `2026-04-28` after the recent partial fixes:

- total: `156`
- equivalent: `144`
- not_equivalent: `10`
- go_compile_timeout: `2`
- go_compile_failed: `0`
- iverilog_compile_failed: `0`

Result files:

- `historical/runs/recheck_current_156_after_partial_fixes_20260428_merged/results.json`
- `historical/runs/recheck_current_156_after_partial_fixes_20260428_merged/summary.json`

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

## Notes

- `current_go_156/` is a copied working set, intended to be the direct 156-case Go collection.
- All legacy materials are grouped under `historical/`.
- The original benchmark dataset is now under `historical/dataset_spec-to-rtl/`.
- Historical runs, scripts, repair notes, and the older handoff snapshot are preserved under `historical/`.
- The current full-run status above comes from a merged validated result set because the rerun was completed in two segments and then combined into one summary.
