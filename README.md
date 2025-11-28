# MyGO

MyGO is a research compiler that lowers subsets of Go into a structural MLIR/CIRCT representation and emits SystemVerilog for simulation. The toolchain bundles a CLI, IR passes, and a Verilog backend that can be wired to real simulators or the included mock harness for testing.

---

## Quick Start

```bash
# Clone and bootstrap
git clone https://github.com/.../mygo
cd mygo
go install ./cmd/mygo

# Verify prerequisites (Go 1.22+, CIRCT tools available on PATH)
circt-translate --version

# Smoke test
go test ./...
```

---

## CLI Usage

### Compile to MLIR
```bash
mygo compile -emit=mlir -o simple.mlir tests/e2e/simple/main.go
```

### Compile to Verilog
```bash
# Point --fifo-src at either a single SystemVerilog file or an entire directory of helper IP.
# The repo ships a sample at internal/backend/templates/simple_fifo.sv for quick validation.
mygo compile -emit=verilog \
    --circt-translate=$(which circt-translate) \
    --fifo-src=internal/backend/templates/simple_fifo.sv \
    -o simple.sv \
    tests/e2e/pipeline1/main.go
```

The backend removes auto-generated FIFO definitions from the main Verilog file and mirrors your FIFO/IP sources next to the output:

- `design_fifos.sv` when `--fifo-src` is a single file.
- `design_fifo_lib/<files>` when `--fifo-src` is a directory tree.

All auxiliary paths are reported via `backend.Result.AuxPaths` so you can hand them to downstream tools.

### Simulate
```bash
# With a real simulator wrapper (e.g. Verilator)
mygo sim \
    --circt-translate=$(which circt-translate) \
    --fifo-src=/path/to/my_fifo_lib \
    --simulator=/path/to/verilator-wrapper.sh \
    tests/e2e/pipeline1/main.go

# With the bundled mock simulator (great for CI or local validation)
MYGO_SIM_TRACE=tests/e2e/pipeline1/expected.sim \
mygo sim \
    --circt-translate=$(which circt-translate) \
    --fifo-src=internal/backend/templates/simple_fifo.sv \
    --simulator=./scripts/mock-sim.sh \
    tests/e2e/pipeline1/main.go
```

`mygo sim` auto-detects `expected.sim` living next to a single Go input and fails fast if the simulator output differs.

---

## Key Modules

| Path | Description |
|------|-------------|
| `cmd/mygo` | Multi-command CLI (`compile`, `sim`, `dump-*`, `lint`). Hosts the simulation harness and flag plumbing for CIRCT binaries, FIFO sources, and expected traces. |
| `internal/frontend` | Loads Go sources via go/packages/SSA and produces the high-level IR. |
| `internal/ir` | Defines the hardware IR, processes, channels, and validation helpers. |
| `internal/mlir` | Lowers the IR to structural MLIR (`hw`, `seq`, `sv` dialects) and emits FIFO extern declarations. |
| `internal/backend` | Manages CIRCT temp files, optional `circt-opt` passes, Verilog emission, FIFO stripping, and mirroring of user-provided helper IP. |
| `tests/e2e` | End-to-end workloads (ported from Argo) plus golden MLIR and simulation traces. |
| `scripts/mock-sim.sh` | Minimal simulator wrapper used by tests/CI; it prints the contents of `MYGO_SIM_TRACE` after validating Verilog inputs exist. |
| `internal/backend/templates/simple_fifo.sv` | Reference FIFO implementation for quick experimentation. Copy/modify this outside the repo for production flows. |

---

## Testing

```bash
# Unit + integration suites
go test ./...

# Focus on backend/package tests
go test ./internal/backend -run .

# Run the e2e harness (compiles real Go programs through the CLI)
go test ./tests/e2e -run TestProgramsCompileToMLIR
```

The CLI itself has regression coverage in `cmd/mygo/sim_test.go`, which exercises the `sim` command with fake CIRCT binaries and the mock simulator.

---

## Documentation & Archive

- Historical READMEs (Phase 1–4 plans and the previous monolithic README) now live in `docs/arxiv/mygo_archive.md` for reference or citation in arXiv write-ups.
- Templates, helper IP, and additional notes sit under `internal/backend/templates/` and `docs/`.

For architectural or research deep dives, start with the archived document above; keep this README handy for daily work and onboarding.
