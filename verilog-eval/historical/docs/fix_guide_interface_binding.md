# Fix Guide: Interface and Port Binding

## Scope

This guide covers failures where the generated Verilog does not expose the interface expected by the benchmark testbench.

Current cases:

- `Prob099_m2014_q6c`
- `Prob148_2013_q2afsm`

Current validation list:

- [validation_cases/interface_binding.txt](./validation_cases/interface_binding.txt)

## Current Symptoms

### `Prob099_m2014_q6c`

- the benchmark testbench wiring was inconsistent with the reference module and needed to be normalized back to `Y1`/`Y3`
- the generated top-level module shape widened `y` and exposed helper ports instead of the benchmark-facing header

### `Prob148_2013_q2afsm`

- missing output port `g`
- one input width is inconsistent with the testbench's expected interface

## Likely Root Causes

- `TopModule` parameter export is not preserving benchmark port names or widths exactly
- `out_*` globals are exported, but aliasing or binding is still wrong for specific port shapes
- scalar and vector ports are being normalized too aggressively during IR-to-Verilog emission
- top-level naming and binding logic may still assume MyGO-internal names rather than benchmark-visible names

## Files To Inspect

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/ir/ir.go](/home/qinkejiu/mygo/internal/ir/ir.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
- [internal/backend/backend.go](/home/qinkejiu/mygo/internal/backend/backend.go)
- [cmd/mygo/main.go](/home/qinkejiu/mygo/cmd/mygo/main.go)

## Recommended Repair Order

1. Verify that every `TopModule` parameter becomes one input port with the original name and exact width.
2. Verify that every exported output port is bound to the intended `out_*` global, not only to an internal helper signal.
3. Compare the emitted Verilog module header against the reference/testbench port list before looking at behavior.
4. Only after interface shape is correct should you investigate logic mismatches.

## Concrete Checks

1. Inspect the generated Verilog header for both cases.
2. Confirm that input/output order is stable and names are not silently rewritten.
3. Confirm that vector widths are exact, especially on ports that the testbench connects positionally.
4. Confirm that helper globals such as `_reg` or `_r` versions are not exported instead of the benchmark-facing signal.

## Validation Steps

### Focused validation

Run the interface suite against the current handoff Go files:

```bash
python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --cases-file verilog-eval/docs/validation_cases/interface_binding.txt \
  --output-dir verilog-eval/runs/validate_interface_binding
```

Expected milestone:

- `Prob099_m2014_q6c` should stop failing elaboration
- `Prob148_2013_q2afsm` should stop failing elaboration
- any remaining mismatches after elaboration should be treated as behavioral follow-up work, not interface-binding failures

### Broader regression

After the focused suite passes, rerun the full handoff corpus:

```bash
python3 verilog-eval/scripts/recheck_existing_go_suite.py \
  --go-root verilog-eval/handoff_156_current/go_files \
  --dataset-dir verilog-eval/dataset_spec-to-rtl \
  --output-dir verilog-eval/runs/recheck_after_interface_fix_all
```

Success criterion:

- both interface cases move out of `iverilog_compile_failed`
- no previously equivalent case regresses

## Optional Prompt-Side Cross-Check

If a compiler-side interface fix lands cleanly, you can optionally regenerate these same cases with:

- [../scripts/deepseek_spec_to_rtl_eval.py](../scripts/deepseek_spec_to_rtl_eval.py)

That step is secondary. The primary acceptance check is the current-go recheck above.
