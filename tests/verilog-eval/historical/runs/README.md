# historical/runs

This directory stores preserved verification runs for `verilog-eval`.

## Layout

- `recheck_current_156/`
  Full-suite rechecks of `tests/verilog-eval/current_go_156/`.

Each run directory should follow:

- `YYYYMMDD_<short_label>/`

and typically contains:

- `results.json`
- `summary.json`
- one subdirectory per case with emitted Verilog and simulator artifacts

## Current Archived Runs

### `recheck_current_156/20260506_current_workspace`

- source Go tree: `tests/verilog-eval/current_go_156/`
- verification script:
  `tests/verilog-eval/historical/scripts/recheck_existing_go_suite.py`
- result:
  `156 / 156 equivalent`

Artifacts:

- [summary.json](/home/qinkejiu/mygo/tests/verilog-eval/historical/runs/recheck_current_156/20260506_current_workspace/summary.json)
- [results.json](/home/qinkejiu/mygo/tests/verilog-eval/historical/runs/recheck_current_156/20260506_current_workspace/results.json)

## Notes

- Older historical run directories are not present in the current workspace state.
- Keep this directory append-only when possible: add new dated run folders instead of overwriting old ones.
