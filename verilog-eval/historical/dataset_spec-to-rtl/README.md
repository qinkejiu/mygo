# dataset_spec-to-rtl

This folder contains the source benchmark materials for the spec-to-RTL recovery workflow.

## Structure

- `prompts/`: 156 `*_prompt.txt` files
- `refs/`: 156 `*_ref.sv` files
- `tests/`: 156 `*_test.sv` files
- `misc/`: non-standard helper or note files

## Rule

Do not place regenerated Go outputs back into this folder.

Keep this folder as the immutable source dataset.

## Next-step document

See:

- [../../docs/dataset_spec_to_rtl_recovery_plan.md](../../docs/dataset_spec_to_rtl_recovery_plan.md)
