# CHStone Performance Handoff

## Date

2026-05-06

## Goal

Continue the CHStone performance repair from the current state without regressing the already-fixed functional issues.

The immediate goal is:

- make large CHStone cases, especially `tests/CHStone/dfadd/main.go`, finish `compile -emit=mlir` within the user’s shorter timeout budget
- then revalidate `tests/stages` and `tests/CHStone` with the updated scripts

## Hard Constraints

- The user explicitly said recent long runs crashed the system.
- Do not run long commands casually.
- Prefer hard timeouts around `20s` to `30s`.
- Do not start with full-suite runs.
- Do not start with full hardware simulation.
- Keep validation to short compile-only probes until the remaining hotspot is narrowed further.

## Current Files Touched

These files contain the current work and should be read first:

- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
- [internal/mlir/emitter_test.go](/home/qinkejiu/mygo/internal/mlir/emitter_test.go)
- [test_CHStone.py](/home/qinkejiu/mygo/tests/CHStone/test_CHStone.py)
- [test_stages.py](/home/qinkejiu/mygo/tests/stages/test_stages.py)

## Important Repository State

- `tests/verilog-eval/historical` was cleaned earlier.
- `tests/verilog-eval/handoff_156_current` was deleted during user-approved cleanup.
- Because of that, `go test ./internal/mlir` full-package runs currently fail on fixture-based tests that expect those deleted files.
- Use targeted `-run '...'` tests only, unless those fixtures are restored.

Do not revert the cleanup or other unrelated user worktree changes unless the user asks.

## What Is Already Fixed

### Functional fixes already in place

These regressions were already repaired before the current performance passes:

- `verilog-eval` 156-case recheck recovered to `156/156`
- `tests/stages` recovered to `13/13`
- direct-clocked old-value reuse bug was fixed
- FSM/local `make([])` undeclared `sv.read_inout` bug was fixed
- clock port resolution for `clock` vs `clk` was fixed

### Performance-oriented changes already in place

#### `internal/mlir/emitter.go`

- buffered MLIR output
- root-level immutable reg constantization
- module-level immutable-reg declaration skipping
- process-local producer cache
- indexed aggregate cache
- packed aggregate cache and invalidation
- immutable wide reg reads no longer fall back to `sv.read_inout`
- immutable packed arrays now reuse process-level packed values
- `emitResolvedConvert` / `emitEdgeResolvedConvert` / `emitConvertOperation`
  now lower width changes with:
  - `arith.extui`
  - `arith.extsi`
  - `arith.trunci`
  instead of `extract/replicate/concat`
- edge-scope convert cache
- edge-scope pure expression cache for:
  - `comb.xor` used as logical not
  - `comb.add/sub/mul/and/or/xor/shl/shru/shrs`
  - `comb.icmp`
  - `comb.mux`
- trivial fold rules now remove:
  - `mux(cond, x, x)`
  - `mux(true, a, b)`
  - `mux(false, a, b)`
  - self-comparisons such as `icmp eq x, x`

#### `test_CHStone.py` and `test_stages.py`

Hardware simulation no longer invokes `go run ./cmd/mygo ...` for every case.

They now:

1. build `./cmd/mygo` once into `.mygo-tmp/test-go/mygo-test-bin`
2. reuse that binary for hardware simulation commands

This avoids repeated self-compilation overhead during suite runs.

## Measured Status At Stop Point

### One-time `mygo` build cost

This now takes about one second:

```bash
/usr/bin/time -f 'ELAPSED=%E' timeout 20s \
  env GOCACHE=/tmp/go-build-cache GOTMPDIR=/tmp/go-tmp \
  go build -o .mygo-tmp/test-go/mygo-test-bin ./cmd/mygo
```

Observed:

- `ELAPSED=0:00.98`

### `dfadd` compile-only still not fully inside short timeout

Current probe command:

```bash
/usr/bin/time -f 'ELAPSED=%E' timeout 25s \
  env GOCACHE=/tmp/go-build-cache \
  go run ./cmd/mygo compile -emit=mlir -o /tmp/dfadd_probe.mlir tests/CHStone/dfadd/main.go
```

Latest observed behavior:

- still times out around `25s` to `26s`

Also tested:

```bash
/usr/bin/time -f 'ELAPSED=%E' timeout 28s \
  env GOCACHE=/tmp/go-build-cache \
  go run ./cmd/mygo compile -emit=mlir -o /tmp/dfadd_probe.mlir tests/CHStone/dfadd/main.go
```

Observed:

- still timed out around `29.17s`

Using the prebuilt binary did not materially change the result for compile-only:

```bash
/usr/bin/time -f 'ELAPSED=%E' timeout 25s \
  ./.mygo-tmp/test-go/mygo-test-bin compile -emit=mlir -o /tmp/dfadd_probe_bin.mlir tests/CHStone/dfadd/main.go
```

Observed:

- still timed out around `25.99s`

Conclusion:

- the remaining cost is now primarily inside compiler work, not repeated `go run` self-build overhead

## What Improved Quantitatively

The probe output size and structure improved drastically over this session.

Earlier in this effort, partial `dfadd` MLIR was near the GB scale.

Near the current stop point, the same `25s` probe produced approximately:

- size: `9437184` bytes
- lines: `191910`

Counts from the last partial output:

- `comb.mux`: `29923`
- `comb.icmp`: `24240`
- no remaining `mux(cond, x, x)`
- no remaining self-compare `icmp x, x`
- `aInput_* / bInput_* / zOutput_*` `sv.read_inout` count: `0`

This means:

- array explosion is no longer the main problem
- convert expansion is no longer the main problem
- the remaining cost is mostly control/dataflow complexity that is still genuinely being emitted

## Short Validation That Currently Works

### Targeted emitter tests

Use targeted tests only:

```bash
timeout 20s env GOCACHE=/tmp/go-build-cache go test ./internal/mlir -run 'TestConvertOpsUseArithExtAndTrunc|TestRootSignalRefUsesCachedConstantForImmutableReg|TestValueRefUsesConstantForImmutableWideRegInDirectClockedPath|TestPackArraySignalValueReusesImmutablePackedValueAcrossBlockScopes|TestEdgeValueRefReusesEquivalentBinaryOpsWithinScope|TestCachedEdgeMuxAndCompareFoldTrivialCases|TestEmitOperationMuxFoldsIdenticalArms|TestPackedWordArrayReadsUseIndexedElementState|TestEmitFSMLocalMakesliceIndexedReadsAvoidUndeclaredInouts|TestEmitDirectClockedDoesNotBakeClockIntoEdgeHelpers|TestZeroInitializedGlobalArrayDoesNotReadUndeclaredPackedWire|TestPrintVarargsScratchDoesNotReadUndeclaredWire'
```

This passed at stop time.

### Script syntax check

```bash
timeout 10s python3 -m py_compile tests/CHStone/test_CHStone.py tests/stages/test_stages.py
```

This passed at stop time.

## Remaining Hotspot

The remaining hotspot is no longer obvious textual bloat. It is mostly repeated real control/dataflow emission in large edge/FSM logic.

The dominant remaining ops in the current partial `dfadd` output are roughly:

- `comb.add`
- `comb.shl`
- `comb.mux`
- `comb.icmp`

This is visible in the tail of `/tmp/dfadd_probe.mlir`, where patterns like these still recur:

- shift/add trees built from a small set of reused sub-values
- nested mux trees that are no longer trivial, but still large

Important observation:

- the easy redundancies are already gone
- the next improvements likely require more structural simplification, not just more local caching of obvious duplicates

## Recommended Next Steps

### 1. Stay on compile-only probes

Do not jump to full `python3 tests/CHStone/test_CHStone.py` yet.

Keep using:

```bash
/usr/bin/time -f 'ELAPSED=%E' timeout 25s \
  env GOCACHE=/tmp/go-build-cache \
  go run ./cmd/mygo compile -emit=mlir -o /tmp/dfadd_probe.mlir tests/CHStone/dfadd/main.go
```

and, when isolating tool self-build overhead:

```bash
/usr/bin/time -f 'ELAPSED=%E' timeout 25s \
  ./.mygo-tmp/test-go/mygo-test-bin compile -emit=mlir -o /tmp/dfadd_probe_bin.mlir tests/CHStone/dfadd/main.go
```

### 2. Focus on structural simplification, not arrays

Do not spend more time on:

- `aInput/bInput/zOutput` immutable array reads
- sign/zero-extension expansion
- trivial `mux(x, x)` or self-compare folds

Those are already handled.

### 3. Inspect whether a higher-level pattern can collapse repeated edge logic

The remaining repeated shapes in `dfadd` look like real repeated arithmetic trees, not accidental textual duplication.

Most likely next profitable areas:

- identify whether some `edgeValueRef` producer chains repeatedly rebuild the same upstream signal through different destination signals
- check whether more result reuse can happen at the signal level rather than the expression-text level
- inspect whether repeated phi/mux subgraphs can be normalized earlier, before MLIR emission

Start in:

- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
  - `edgeValueRefWithActive`
  - `emitCondTermsEdgeRef`
  - `emitPhiAsMuxTree`
  - `resolveDirectClockedSignalValue`
  - `resolveDirectClockedOutputAtBlock`

### 4. Check if some current remaining muxes are semantically dead

Although trivial equal-arm muxes were removed, there may still be muxes whose condition is constant after prior folding, but expressed through cached named const refs.

If more folding is added, keep it cheap and local. Do not add expensive global analysis unless a short probe proves it helps.

### 5. After `dfadd` compile-only gets under budget, then do targeted runtime checks

Suggested order:

1. `dfadd` compile-only
2. `dfmul` compile-only
3. `dfdiv` compile-only
4. only then a targeted hardware sim using the prebuilt binary
5. only after that, script-level reruns

## What Not To Do First

- Do not start with full `python3 tests/CHStone/test_CHStone.py`
- Do not start with full `python3 tests/stages/test_stages.py`
- Do not start with full `go test ./internal/mlir`
- Do not restore deleted `verilog-eval` historical fixtures unless the user asks
- Do not revert unrelated cleanup or user changes

## If You Need A Quick Status Probe

After a change, collect these three numbers:

```bash
timeout 10s bash -lc "wc -c /tmp/dfadd_probe.mlir && wc -l /tmp/dfadd_probe.mlir && rg -o ' = comb\\.mux' /tmp/dfadd_probe.mlir | wc -l"
```

If size, line count, and mux count do not improve, the last change probably did not help the real hotspot.

## Summary

The work is no longer blocked by broken lowering. It is now a narrow performance problem.

The remaining task is:

- shave the last few seconds off large CHStone MLIR emission, especially `dfadd`
- then rerun the updated scripts that now reuse a prebuilt `mygo` binary
