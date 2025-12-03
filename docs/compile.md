# `mygo compile`

This guide covers every flag on the `compile` subcommand and the handoff points to CIRCT. Keep the README focused on the happy-path flow and refer back here when you need to tweak emission knobs or explain new workflows to teammates.

## Fast Path (MLIR first)

```bash
# Emit MLIR for the simplest workload.
mygo compile tests/stages/simple/main.go > /tmp/simple.mlir
```

- Defaults: `-emit=mlir`, stdout output when `-o` is omitted, diagnostics go to stderr.
- Use this flow in CI smoke tests or when iterating on the frontend without the full CIRCT stack.

## Verilog Path with FIFOs

```bash
mygo compile \
  -emit=verilog \
  --fifo-src=internal/backend/templates/simple_fifo.sv \
  -o /tmp/pipeline1.sv \
  tests/stages/pipeline1/main.go
```

- Verilog emission requires `-o` because the backend writes auxiliary FIFO/IP bundles next to that file.
- `--fifo-src` is required whenever the design instantiates channels; point it at a single `.sv` file or a directory of helper IP.
- The backend mirrors the FIFO assets alongside `pipeline1.sv` (e.g. `design_fifos.sv` or `design_fifo_lib/`).

## Flag Reference

| Flag | Purpose |
| ---- | ------- |
| `-emit` | `mlir` (default) or `verilog`. Controls which backend runs. |
| `-o` | File path for MLIR/Verilog output. MLIR supports stdout via `-o -`. |
| `-target` | Entry point in the Go package. Leave as `main` unless emitting helper modules. |
| `-diag-format` | `text` (default) or `json`. Matches `diag.Reporter`. |
| `--circt-opt` | Explicit path to `circt-opt`. Leave empty to rely on `PATH`. |
| `--circt-pipeline` | Pass pipeline string forwarded to `circt-opt --pass-pipeline`. Useful for experiments. |
| `--circt-lowering-options` | Comma-separated string passed via `--lowering-options`. Helpful when reproducing CI comparisons. |
| `--circt-mlir` | File path to dump the MLIR handed off to CIRCT before lowering. |
| `--fifo-src` | FIFO/handshake IP source. Required when `designHasChannels` is true. |

## Golden-Based Regression Flow

The stage harness (`tests/stages/stages_test.go`) consumes the compile command in three modes:

1. **MLIR goldens**: `TestMLIRGeneration` writes `main.mlir` to a temp file and diffs it against `tests/stages/<case>/main.mlir.golden` if present.
2. **Verilog goldens**: `TestVerilogGeneration` runs the `-emit=verilog` path with deterministic lowering options when `circt-opt` is available.
3. **Channel awareness**: Workloads with `NeedsFIFO` automatically append `--fifo-src internal/backend/templates/simple_fifo.sv`.

When you introduce a new workload, populate `main.mlir.golden` / `main.sv.golden` as needed and update `testCases` accordingly. Run `go test ./tests/stages` to validate the diffs locally.

## Troubleshooting Tips

- **Missing `circt-opt`**: The Verilog path returns a skip/failure message. Install CIRCT or point `--circt-opt` at a custom build.
- **`--fifo-src` errors**: Designs without channels do not need the flag. If you see `requires --fifo-src`, double-check whether your Go code introduces buffered channels.
- **Pass debugging**: Use `--circt-mlir` to capture the MLIR right before the CIRCT step, then run `circt-opt` manually with experimental pipelines.
