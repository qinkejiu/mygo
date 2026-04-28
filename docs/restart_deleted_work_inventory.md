# Deleted Work Inventory

## Purpose

This note records what the deleted or transient work was trying to do, based on:

- files that were still identifiable during recovery
- directory naming patterns observed before cleanup
- verification and debugging flows that were repeatedly used

It is meant as a restart aid, not as a perfect reconstruction.

## Confirmed Special-Purpose Files

### `internal/ir/mixed_clock.go`

Likely purpose:

- add a special IR path for "mixed clock" style modules
- detect modules using more than one clock edge style or clock-like control split
- bypass the generic process builder for those cases
- produce a `MixedClockModuleSpec`-style intermediate representation for direct backend emission

Typical use cases:

- dual-edge style examples
- explicit `if clk { ... }` and `if !clk { ... }` in the same top module
- problems where generic sequential lowering produced the wrong clock semantics

### `internal/ir/builder_sensitivity.go`

Likely purpose:

- infer whether a process should be treated as combinational or sequential
- classify global signals as `wire` vs `reg`
- detect clocked vs non-clocked assignment regions
- preserve latch-like globals as registers
- keep non-clocked outputs as combinational wires even inside sequential modules

Typical use cases:

- `Prob028`-style latch behavior
- `Prob129`-style sequential state with combinational outputs
- array outputs that should stay combinational

### `internal/backend/mixed_clock_verilog.go`

Likely purpose:

- emit Verilog directly from a mixed-clock module spec
- avoid the generic CIRCT lowering path when mixed-clock lowering was clearer or safer
- generate hand-shaped always blocks for clock/reset/output-reg handling

Typical use cases:

- cases where generic MLIR/CIRCT output was hard to control
- mixed-clock or dual-edge special handling

## Likely Purpose Of Deleted Temporary Directories

These names appeared to fall into a few repeatable buckets.

### Per-case scratch directories

Examples:

- `.tmp028*`
- `.tmp049*`
- `.tmp129*`
- `.tmp143*`
- `.tmp156*`

Likely purpose:

- isolate one benchmark case
- copy a single `go_files/<case>` directory into a scratch area
- run `run_verify_by_dir.sh` on just that reduced target
- compare before/after results without paying the cost of the full 156-case suite

These were useful for:

- regression triage
- validating one fix before full verification
- preserving failing artifacts long enough to inspect them

### Scratch verification reports

Examples:

- `.tmp*_verify_report/`

Likely purpose:

- store the JSON results and summaries from `run_verify_by_dir.sh`
- keep isolated verification evidence for a single case or a small subset

These were useful for:

- checking whether a specific fix passed in isolation
- comparing mismatch counts before and after a local change

### Reduced reproducer groups

Examples:

- `.tmp_rule90_*`
- `.tmp_rule110_*`
- `.tmp_case_variants*`
- `.tmp_min_cases`
- `.tmp_prob153_reduce`

Likely purpose:

- reduce a larger failing family into a smaller reproducer
- test one operator pattern repeatedly
- compare several syntactic variants of the same logic

These were useful for:

- debugging codegen patterns
- finding the smallest example that still reproduced a compiler bug

### One-off inspection sandboxes

Examples:

- `.tmpinspect*`
- `.tmp_irprobe_hidden`
- `.tmp_normisolated`

Likely purpose:

- inspect emitted IR/MLIR/Verilog for a narrow subproblem
- test one normalization or lowering hypothesis
- probe hidden interactions without contaminating the main corpus

## Likely Purpose Of Deleted Reports / Notes

Files with names like:

- `report.txt`
- `report_to_be_complete.md`
- `docs/topmodule_regression.md`
- `clean_simulation_results.txt`

Likely purpose:

- capture current pass rate snapshots
- record regressions introduced by a refactor
- note problem-case families and observed symptoms
- track what had already been tried

If you restart, recreate these as:

- one stable `docs/restart_log.md`
- one stable `docs/regression_log.md`

instead of many ad hoc files.

## Workstreams The Deleted Material Appeared To Support

### 1. Latch inference work

Intent:

- handle incomplete assignments correctly
- emit latch-like procedural logic instead of collapsing to plain mux expressions

Primary target:

- `Prob028`

### 2. Clocked vs combinational output separation

Intent:

- keep state updates in clocked logic
- keep derived outputs outside the clock tree when they are pure combinational decodes of state

Primary target:

- `Prob129`

### 3. Array / loop / indexing debugging

Intent:

- validate indexed loads/stores
- debug loop-generated output wiring
- reduce rule90/rule110 and similar bit-array issues

Primary targets:

- `Prob105`
- `Prob108`
- `Prob124`
- `Prob144`

### 4. Mixed-clock special handling

Intent:

- support designs that the generic sequential lowering handled badly
- generate more direct or more explicit Verilog for awkward clock patterns

Primary targets:

- dual-edge or mixed-edge style benchmark cases

## Recommended Restart Structure

If restarting from scratch, use this structure instead of many transient repo-root directories.

### Keep all scratch work under one root

Recommended:

- `scratch/cases/<case>/`
- `scratch/reports/`
- `scratch/reproducers/`

This keeps repo root clean and makes cleanup safe.

### Keep one rolling notebook

Recommended file:

- `docs/restart_log.md`

Suggested sections:

- date
- target bug
- exact command run
- result
- next hypothesis

### Commit only verified milestones

Suggested cadence:

- one bug
- one minimal fix
- one isolated verification
- one full verification
- then commit

## Suggested First Rebuild Order

1. Rebuild the Phase 1 latch fix only.
2. Reconfirm the known-good pass rate checkpoint.
3. Rebuild the `Prob129` work as a narrow change.
4. Rebuild loop/index work only after the sequential/combinational boundary is stable.
5. Reintroduce mixed-clock special handling only if a specific benchmark requires it.

## Short Summary

The deleted work was not one thing; it was mostly four categories:

- mixed-clock special lowering experiments
- process sensitivity / wire-vs-reg classification
- single-case verification sandboxes
- reduced reproducer folders for hard array/index/control-flow bugs

If restarting, the most valuable parts to intentionally recreate are:

- a clean scratch directory layout
- a stable regression log
- narrow, isolated per-case verification loops
- small commits after full-suite confirmation
