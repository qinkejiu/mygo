# Runs Directory Guide

This directory contains all generated evaluation outputs for the spec-to-RTL workflow.

## Canonical Pointers

Latest full 156-case recheck:

- [recheck_current_156_20260421](./recheck_current_156_20260421)

Last mostly-working full 156-case recheck before the current wrapper-binding regression:

- [recheck_current_156_20260420_rerun3](./recheck_current_156_20260420_rerun3)

Current handoff snapshot consumed by rechecks:

- [../handoff_156_current](../handoff_156_current)

## Directory Naming Conventions

### `recheck_current_156_*`

Full-suite rechecks over the current handoff Go corpus using the current compiler/backend state.

These are the most important directories when tracking current compiler progress.

### `validate_*`

Focused validation runs for a subset of benchmark cases.

These are useful during iterative debugging and should usually be treated as temporary or intermediate checkpoints.

### `run_*`, `pilot_*`, `failed8_*`, `basic_fix_*`

Historical benchmark-generation or rerun outputs from earlier workflow phases.

These remain useful for provenance and comparison, but they are not the primary current-status source.

## Recommended Reading Order

1. [../README.md](../README.md)
2. [../docs/current_156_failure_classification.md](../docs/current_156_failure_classification.md)
3. [../docs/fix_guide_unified_current.md](../docs/fix_guide_unified_current.md)
4. latest full recheck results under [recheck_current_156_20260421](./recheck_current_156_20260421)

## Rule

Do not delete older run directories unless you intentionally want to discard debugging history.

Prefer adding new recheck or validation directories with descriptive names rather than overwriting prior runs.
