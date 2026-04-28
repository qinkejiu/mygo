# Handoff 156 Current

This directory is the consolidated handoff snapshot for the current 156-case spec-to-RTL evaluation state.

## Contents

- `go_files/`: latest Go file for every one of the 156 cases
- `scripts/deepseek_spec_to_rtl_eval.py`: current end-to-end generation and evaluation script
- `results/latest_results_156.json`: merged latest results for all 156 cases
- `results/latest_manifest.csv`: per-case manifest showing which run supplied the current Go file
- `results/latest_status_summary.json`: top-level counts
- `docs/failure_classification_current.md`: current failing-case classification

## Merge Policy

Base coverage comes from:
- `verilog-eval/runs/run_50_cases/results.json`
- `verilog-eval/runs/run_remaining_106_merged_results.json`

Later reruns override earlier runs when present:
- `verilog-eval/runs/failed8_rerun/results.json`
- `verilog-eval/runs/basic_fix_batch_rerun_v2/results.json`

## Current Counts

- Total: 156
- Equivalent: 66
- Not equivalent: 88
- Iverilog compile failed: 2

## Notes

- This handoff directory is a copy-only snapshot. Original run directories remain unchanged under `verilog-eval/runs/`.
- The `go_files/` tree is intended to be the starting point for the next conversation.
