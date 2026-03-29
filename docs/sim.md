# `mygo sim`

Simulation stitches CIRCT-generated Verilog together with Verilator (default) or any custom simulator you provide. This document captures everything beyond the single-step README tutorial so you can debug goldens, extend the harness, or plug in different backends.

## Fast Path (Simple Workload)

```bash
mygo sim tests/stages/simple/main.go
```

- Requires `circt-opt` and `verilator` on `PATH`.
- If an `expected.sim` file exists next to the input, `mygo sim` auto-uses it. The checked-in regression suites instead pass explicit `--expect .../main.sim.golden` paths.

## Golden + Test Structure

Workloads live in `tests/stages/<case>/` with the following optional files:

| File | Purpose |
| ---- | ------- |
| `main.go` | Go source under test (always present). |
| `main.mlir.golden` | Reference MLIR for `compile -emit=mlir`. |
| `main.sv.golden` | Reference SystemVerilog for `compile -emit=verilog`. |
| `main.sim.golden` | Reference simulator stdout for `sim`. |


`tests/stages/stages_test.go` wires these assets into three suites plus targeted regressions:

1. `TestSimulation` runs `go run ./cmd/mygo sim ...` with per-workload `--sim-max-cycles` settings.
2. `TestSimulationDetectsMismatch` verifies `--expect` handling by pointing at a bad golden.
3. `TestSimulationVerilogOutWritesArtifacts` ensures `--verilog-out` mirrors the Verilog bundle when requested.

Artifact goldens are part of the explicit full-regression mode:

```bash
MYGO_COMPARE_GOLDENS=1 go test ./...
```

Without `MYGO_COMPARE_GOLDENS=1`, the stage golden suites skip with an explicit message so fast pure-Go verification remains available. Simulator-dependent cases still skip if `circt-opt` or `verilator` is missing.

## Default Verilator Harness

When `--simulator` is omitted, MyGO:

1. Emits Verilog + aux FIFO/IP files into a temp dir rooted alongside your workload (or `--verilog-out`).
2. Renders `sim_main.cpp` with `--sim-max-cycles` and `--sim-reset-cycles` baked in.
3. Invokes `verilator --cc --exe --build` with the generated bundle.
4. Runs the produced `mygo_sim` binary and optionally checks stdout against `--expect` / auto goldens.

Artifacts now stick around by default, so you can inspect Verilog, CIRCT MLIR temps, and Verilator builds under `<workload>/.mygo-tmp/`. Pass `--keep-artifacts=false` to opt back into auto-cleanup.

### Artifact Layout

- **`.mygo-tmp/.mygo-sim-*`** – Verilog bundles emitted by `mygo sim` when `--verilog-out` is omitted. Each directory contains `design.sv` plus any FIFO aux files.
- **`.mygo-tmp/.mygo-circt-*`** – CIRCT pipeline scratch (the MLIR handed between passes and `--export-verilog`). These are shared with the compile command; they are removed unless the run fails or you interrupt it.
- **`.mygo-tmp/.mygo-verilator-*/verilator`** – The full Verilator project for the built-in simulator. Expect `sim_main.cpp`, the generated `obj_dir`, and the final `mygo_sim` binary here. Because the helper installs its xargs shim in the same folder, the layout is self-contained and easy to inspect/copy.

## Flag Reference

| Flag | Purpose |
| ---- | ------- |
| `-diag-format` | Diagnostic reporter format (`text` or `json`). |
| `--circt-opt` / `--circt-pipeline` / `--circt-lowering-options` / `--circt-mlir` | Same semantics as the compile command but applied before simulation. |
| `--verilog-out` | Path to write the Verilog bundle instead of a temp dir. Creates parent directories as needed. |
| `--keep-artifacts` | Preserve the temp dir containing Verilog, Makefile, and simulator outputs (default `true`). |
| `--simulator` | Custom executable to run instead of the built-in Verilator flow. Receives the main Verilog file plus aux files. |
| `--sim-args` | Extra arguments (split by spaces) forwarded to the custom simulator. |
| `--fifo-src` | Deprecated compatibility flag. The simulator now generates FIFO support inline and ignores this option. |
| `--sim-max-cycles` | Max cycles for the built-in driver before declaring a timeout (default 16). Must be > 0. |
| `--sim-reset-cycles` | Number of cycles to hold reset high at startup (default 2). |
| `--expect` | Path to a golden stdout trace. |

## Workflow Notes for Contributors

- **Matching goldens:** Use `--expect tests/stages/<case>/main.sim.golden` during repro steps so failing diffs show up immediately. Update the golden file only after confirming the new behavior.
- **Hardware stdout policy:** `mygo sim` now always prints normalized hardware stdout. If you want to tolerate benign line-order variation for concurrent workloads, do that in test comparison code rather than rewriting runtime output.
- **FIFO generation:** Channel support is generated inline by the compiler. `--fifo-src` remains in the CLI only as a deprecated compatibility flag and is ignored.
- **Custom simulator wrappers:** Provide `--simulator=/path/to/wrapper` plus any `--sim-args`. MyGO passes the generated Verilog as positional arguments so wrappers can re-run Verilator, hook into commercial tools, etc.
- **CI expectations:** Use plain `go test ./...` for fast verification and `MYGO_COMPARE_GOLDENS=1 go test ./...` for full artifact validation. Full mode enables the stage goldens and the targeted CHStone hardware-vs-software regressions (`aes`, `dfsin`, `sha`).
- **Regenerating baselines:** Use `scripts/regenerate_stage_artifacts.sh` to refresh stage `main.mlir.golden`, `main.sv.golden`, and `main.sim.golden` files.
