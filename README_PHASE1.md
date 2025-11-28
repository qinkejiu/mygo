# MyGO Phase 1 Implementation Guide

This document complements the original spec in `README.md` with a snapshot of
the code that currently exists in this repository. It focuses on how the
prototype works today, how to exercise it, and what remains for the next phases.

---

## 1. Current Capabilities

| Area | Status | Notes |
| --- | --- | --- |
| CLI (`cmd/mygo`) | ✅ | Commands: `compile`, `dump-ssa`, `dump-ir`. Uses a shared frontend pipeline. |
| Frontend (`internal/frontend`) | ✅ | Loads local Go sources through `go/packages`, builds SSA via `ssautil`. Sandbox-safe caches (`.cache/go-build`,`gomod`). |
| Hardware IR (`internal/ir`) | ✅ | Modules, ports, signals, processes, and ops (`Assign`, `Convert`, `Bin`). SSA→IR builder covers alloc/const/store/binop/convert paths used by the sample program. |
| MLIR emission (`internal/mlir`) | ✅ | Emits `hw.module`, `hw.constant`, `comb.{add,sext,zext}`, `seq.compreg`, and `hw.output`. No external CIRCT tooling required yet. |
| Sample inputs (`test/e2e`) | ✅ | `simple.go` mirrors the spec example and drives end-to-end tests. |

What is **not** implemented yet:

* Validation of unsupported SSA constructs (fmt.Printf side-effects are ignored).
* Control-flow lowering (if/loops) or phis.
* Optimization passes (const folding, width inference) beyond the trivial ones baked into SSA.
* Verilog emission or CIRCT tool invocation.

---

## 2. Repository Layout (Implemented Pieces)

```
cmd/mygo/          CLI. Parses flags and orchestrates phases.
internal/frontend/ Load Go packages, build SSA.
internal/ir/       Hardware IR types + SSA→IR builder + IR text dump.
internal/mlir/     Printer that turns the IR into textual MLIR.
test/e2e/          Example Go inputs (currently `simple.go` only).
.cache/            Local build + module caches (auto-created).
bin/               CLI build output when you run `go build -o bin/mygo ...`.
```

*Running tools* use only the standard Go toolchain (`go >= 1.22`). No CIRCT
install is required yet.

---

## 3. Building & Running

1. **Install Go 1.22+** (only once).
2. **Build the CLI**
   ```bash
   go build -o bin/mygo ./cmd/mygo
   ```
3. **Dump SSA for the sample program**
   ```bash
   GOCACHE=$(pwd)/.cache/go-build ./bin/mygo dump-ssa test/e2e/simple.go
   ```
4. **Inspect the internal IR**
   ```bash
   GOCACHE=$(pwd)/.cache/go-build ./bin/mygo dump-ir test/e2e/simple.go
   ```
5. **Emit MLIR**
   ```bash
   GOCACHE=$(pwd)/.cache/go-build ./bin/mygo compile \
     -emit=mlir \
     -o simple.mlir \
     test/e2e/simple.go
   cat simple.mlir
   ```

**Flags** (shared across commands):

* `-diag-format={text,json}` – reporter style (JSON currently reserved).
* `-emit={mlir,verilog}` – `verilog` placeholder, returns `not implemented`.
* `-o <file>` – output destination; omit or use `-` to print to stdout.

> **Note:** CLI flags must appear *before* positional Go files because we use
> Go’s standard `flag` package.

---

## 4. Development Notes

* `internal/frontend/loader.go` configures `go/packages` with a sandbox-friendly
  cache rooted at `.cache/`. This avoids macOS sandbox errors when no global
  cache is available.
* SSA construction uses `ssautil.AllPackages(..., ssa.SanityCheckFunctions)`.
  We call `prog.Build()` immediately so later phases can walk function bodies.
* The IR builder currently translates:
  * `*ssa.Alloc` → `SignalKind.Reg`.
  * `*ssa.Store` → `AssignOperation` (captured as `seq.compreg`).
  * `*ssa.Const` → `SignalKind.Const`.
  * `*ssa.BinOp` → `BinOperation` mapped to `comb.*`.
  * `*ssa.Convert` → `ConvertOperation` → `comb.{sext,zext,bitcast}`.
* `internal/mlir/emitter` allocates deterministic `%vN` names so the output
  is stable for diff-based tests or future golden files.

---

## 6. Tips for Contributors

* Always run commands with the repo root as the working directory; we rely on
  relative paths for `.cache/` and `test/e2e`.
* When adding new Go dependencies, update both `go.mod` and `go.sum` and ensure
  the local caches are cleaned if you encounter permission errors:
  ```bash
  rm -rf .cache/go-build .cache/gomod
  ```
* Prefer writing new Phase‑specific docs here rather than modifying the original
  spec—the goal is to keep this file a living log of the implementation while
  `README.md` remains the long-term vision.

---

## 7. Appendix: Command Reference

```bash
# Build CLI
go build -o bin/mygo ./cmd/mygo

# Dump SSA for files
./bin/mygo dump-ssa path/to/file.go

# Dump IR
./bin/mygo dump-ir path/to/file.go

# Emit MLIR (stdout)
./bin/mygo compile -emit=mlir path/to/file.go

# Emit MLIR (file)
./bin/mygo compile -emit=mlir -o output.mlir path/to/file.go
```

If you run into `no packages supplied for SSA construction`, make sure the Go
files you pass are within a valid module (this repo already provides one).

---

_Last updated: 2025-02-15_
