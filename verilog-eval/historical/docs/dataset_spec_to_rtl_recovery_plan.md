# Dataset Spec-to-RTL Recovery Plan

## Current Status

The folder [../dataset_spec-to-rtl](../dataset_spec-to-rtl) has been organized into four subdirectories:

- [../dataset_spec-to-rtl/prompts](../dataset_spec-to-rtl/prompts): 156 prompt files
- [../dataset_spec-to-rtl/refs](../dataset_spec-to-rtl/refs): 156 reference Verilog files
- [../dataset_spec-to-rtl/tests](../dataset_spec-to-rtl/tests): 156 test Verilog files
- [../dataset_spec-to-rtl/misc](../dataset_spec-to-rtl/misc): 3 miscellaneous files

## What Each Folder Means

### `prompts/`

Each `*_prompt.txt` is the natural-language or task prompt for a benchmark case.

Use it for:

- rebuilding missing generated Go files
- checking what the original input requirement was
- constructing a regeneration pipeline

### `refs/`

Each `*_ref.sv` is the reference Verilog implementation.

Use it for:

- semantic comparison
- confirming intended RTL behavior
- debugging mismatches after regeneration

### `tests/`

Each `*_test.sv` is the verification testbench for the corresponding reference module.

Use it for:

- simulation-based equivalence checking
- validating regenerated Go-to-Verilog output

### `misc/`

Current contents:

- [Prob062_bugs_mux2.sv](../dataset_spec-to-rtl/misc/Prob062_bugs_mux2.sv)
- [problems.txt](../dataset_spec-to-rtl/misc/problems.txt)
- [problems-temp.txt](../dataset_spec-to-rtl/misc/problems-temp.txt)

These are not part of the clean 156-case triplet structure and should be treated as notes or special-case artifacts.

## What Is Missing

What is still missing is the generated Go corpus that previously lived under a path like:

- `verilog-eval/generated_go_deepseek_rerun_156/ProbXXX_ref/main.go`

That corpus is not currently present in git and is not currently present anywhere visible on disk.

## What Needs To Be Done Next

### Phase A: Recreate the missing 156-case Go workspace layout

Create a clean workspace root, for example:

- `work/spec_to_rtl_go_rebuild/`

Inside it, recreate one folder per case:

- `Prob001_zero_ref/`
- `Prob002_m2014_q4i_ref/`
- ...
- `Prob156_review2015_fancytimer_ref/`

Each case folder should eventually contain:

- `main.go`
- optional notes or generation metadata

### Phase B: Build a manifest

Create a machine-readable manifest that maps each case name to its three source files:

- prompt
- reference Verilog
- testbench

Recommended output:

- `docs/spec_to_rtl_manifest.csv`
or
- `docs/spec_to_rtl_manifest.json`

Each record should include:

- case name
- prompt path
- ref path
- test path

### Phase C: Reconstruct the Go files

For each case:

1. read the prompt from `prompts/`
2. use `refs/` and `tests/` as semantic validation anchors
3. regenerate or rewrite `main.go`
4. compile with `mygo`
5. simulate against the corresponding testbench

### Phase D: Separate “source data” from “generated outputs”

Do not mix regenerated Go files back into `source_data`.

Recommended separation:

- source dataset stays in [../dataset_spec-to-rtl](../dataset_spec-to-rtl)
- generated Go outputs go under a new workspace such as:
  - `generated_go_rebuild/`
  - `work/spec_to_rtl_go_rebuild/`

This makes cleanup and retries much safer.

## Recommended Folder Layout Going Forward

Suggested structure:

```text
dataset_spec-to-rtl/
  prompts/
  refs/
  tests/
  misc/

work/
  spec_to_rtl_go_rebuild/
    Prob001_zero_ref/
      main.go
    ...
```

## Recommended Immediate Tasks

1. Generate a 156-case manifest from the current `prompts/`, `refs/`, and `tests/`.
2. Recreate the 156 empty case directories for regenerated Go files.
3. Decide whether to regenerate in benchmark order or prioritize previously important cases first.
4. Add a simple progress tracker file so each recovered case can be marked:
   - not started
   - regenerated
   - compiles
   - verified

## Minimal Restart Workflow

For each case:

1. Read `prompt`.
2. Write `main.go`.
3. Run `mygo compile`.
4. Compare generated Verilog against `ref` and `test`.
5. Record result in the tracker.

## Suggested Companion Files To Add Next

Useful files to create next:

- `docs/spec_to_rtl_manifest.csv`
- `docs/spec_to_rtl_progress.md`
- `scripts/build_spec_to_rtl_manifest.py`

## Summary

The dataset is now cleanly organized and ready to support reconstruction work.

The important rule from here is:

- keep source dataset files stable
- keep regenerated Go outputs in a separate workspace
- use one manifest and one progress tracker to avoid repeating the previous confusion
