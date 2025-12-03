# IR Dump + Lint Commands

Use these tools to inspect what the frontend produced without touching CIRCT. They are handy when triaging regressions or updating docs for new compiler features.

## `mygo dump-ssa`

```bash
mygo dump-ssa tests/stages/simple/main.go
```

- Loads the Go package (via `go/packages`), builds SSA, and prints it to stdout.
- Respects `-diag-format` for reporter output (`text` default, `json` for tooling).
- Pass the same set of files or globs you would hand to `go build`.

## `mygo dump-ir`

```bash
mygo dump-ir tests/stages/simple/main.go
```

- Runs validation + default passes, then dumps the MyGO hardware IR before MLIR lowering.
- Requires well-formed SSA: fix any frontend errors reported via diagnostics first.
- Use this when writing docs for new IR constructs, because it shows channel/process metadata before CIRCT rewrites it.

## `mygo lint`

```bash
mygo lint -concurrency=false tests/stages/simple/main.go
```

- Executes validation-only checks such as concurrency rules without producing IR.
- Flags:
  - `-concurrency` (default `true`): disable specific rule families when iterating on new lowering strategies.
  - `-diag-format`: same behavior as other commands.

### When to Use

- **Doc updates**: capture SSA/IR snippets straight from the tool instead of copying stale README excerpts.
- **Bug triage**: confirm whether a miscompile originates before MLIR by comparing `dump-ir` output against expectations.
- **Workflow automation**: these commands are pure-Go and do not require `circt-opt` or `verilator`, so they are safe to run in lightweight agents.
