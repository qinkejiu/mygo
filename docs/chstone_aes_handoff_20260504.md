# CHStone AES Handoff

## Update 2026-05-05

Resolved.

- `tests/CHStone/aes/main.go` now runs through the real `go run ./cmd/mygo sim ...` hardware path.
- The AES-specific fallback has been removed from [cmd/mygo/main.go](/home/qinkejiu/mygo/cmd/mygo/main.go) and its unit test coverage was removed from [cmd/mygo/sim_internal_test.go](/home/qinkejiu/mygo/cmd/mygo/sim_internal_test.go).
- Real AES hardware simulation was revalidated on the normal path with:

```bash
go run ./cmd/mygo sim --sim-max-cycles 256 tests/CHStone/aes/main.go
```

Observed terminal output:

- `encrypted message 	3925841d02dc09fbdc118597196a0b32`
- `decrypto message	3243f6a8885a308d313198a2e0370734`
- final result `0`

Key implementation changes that made AES feasible on the real path:

- IR global/indexed-signal classification now uses direct signal-to-global caching instead of repeated whole-module scans.
- combinational-latch analysis now memoizes per-block global path results.
- packed indexed reads now handle flattened multi-dimensional arrays, including the inliner path for global tables like `Sbox` / `invSbox`.
- MLIR emission now uses buffered output.

The remainder of this document is the historical handoff context from before the fix.

## Goal

Remove the last non-real-hardware fallback and make `tests/CHStone/aes/main.go` pass through the real `go run ./cmd/mygo sim ...` path.

When this is done:

- `test_CHStone.py` should continue to call only `go run ./cmd/mygo sim ...` for hardware.
- `cmd/mygo/main.go` should no longer special-case `tests/CHStone/aes/main.go`.
- `CHS_clean_simulation_results.txt` should still end with `HW=12/12 | SW=12/12`.

## Current Status

These CHStone cases now pass through the real hardware path:

- `adpcm`
- `blowfish`
- `common`
- `dfadd`
- `dfdiv`
- `dfmul`
- `dfsin`
- `gsm`
- `mips`
- `motion`
- `sha`

The only remaining special-case fallback is `aes`.

It is currently implemented in:

- [cmd/mygo/main.go](/home/qinkejiu/mygo/cmd/mygo/main.go)
  - `shouldUseSoftwareSimulationFallback(...)`
  - `runSoftwareSimulationFallback(...)`

The corresponding test coverage is in:

- [cmd/mygo/sim_internal_test.go](/home/qinkejiu/mygo/cmd/mygo/sim_internal_test.go)

`test_CHStone.py` no longer contains any case-level fallback set. It now runs real hardware for all cases, but `aes` still falls back inside `mygo sim`.

## What Was Already Fixed

These fixes are already in place and should not be regressed:

- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)
  - fixed cross-state phi reuse in FSM lowering
  - fixed root FSM reset visibility for synthetic `%rst`
  - fixed packed array reconstruction from indexed element storage
  - fixed module-scope wire-with-init handling so initialized globals do not collapse to zero
  - fixed block-scoped `valueNames` and `internalSignalReads` leakage across `sv.case` arms

- [internal/frontend/preprocess.go](/home/qinkejiu/mygo/internal/frontend/preprocess.go)
  - stopped unrolling `for i = ...` loops that caused `adpcm` package loading failures

- [cmd/mygo/main.go](/home/qinkejiu/mygo/cmd/mygo/main.go)
  - built-in Verilator driver now adapts to whether top-level `clk/rst` exist
  - Verilator build env now sets `MAKEFLAGS=-jN`
  - sim diagnostics are filtered to `Error` severity to reduce noise

## Known Good Commands

These real hardware commands now work:

```bash
go run ./cmd/mygo sim --sim-max-cycles 256 tests/CHStone/dfadd/main.go
go run ./cmd/mygo sim --sim-max-cycles 512 tests/CHStone/dfdiv/main.go
go run ./cmd/mygo sim --sim-max-cycles 256 tests/CHStone/dfmul/main.go
go run ./cmd/mygo sim --sim-max-cycles 8192 tests/CHStone/dfsin/main.go
```

Observed behavior:

- `dfadd` now matches software and completes in about 30s.
- `dfdiv` now matches software and completes in about 7s with `--sim-max-cycles 512`.
- `dfmul` now matches software and completes in about 23s.
- `dfsin` now matches software and completes in about 78s with `--sim-max-cycles 8192`.

`test_CHStone.py` currently uses:

- `dfdiv: 512`
- `dfsin: 8192`

## Remaining Problem: AES

### Symptom

Real `aes` hardware simulation still does not finish cleanly through the normal path. The script passes only because `cmd/mygo sim` intercepts `tests/CHStone/aes/main.go` and runs plain `go run` instead.

### Current Repro

```bash
go run ./cmd/mygo sim --sim-max-cycles 256 tests/CHStone/aes/main.go
```

or, to isolate compile-side behavior:

```bash
go run ./cmd/mygo compile -emit=verilog -o /tmp/aes_hw.sv tests/CHStone/aes/main.go
```

### Observed Behavior

The compile/verilog path does not quickly produce Verilog output.

During reproduction, it repeatedly emits warnings like:

- `for loop has variable bound; lowering will use loop FSM generation`
- `warning: no signal mapping for value *ssa.Alloc`

At the point work stopped:

- no successful real `aes` Verilog artifact had been captured
- `tests/CHStone/aes/.mygo-tmp` stayed essentially empty
- the fallback in `cmd/mygo/main.go` was still required

## Most Likely Next Debug Path

### 1. Remove the internal fallback only after real AES works

Search for:

```bash
rg -n 'shouldUseSoftwareSimulationFallback|runSoftwareSimulationFallback|tests/CHStone/aes/main.go' cmd/mygo/main.go cmd/mygo/sim_internal_test.go
```

Do not remove this first. Fix real `aes` first, then delete it.

### 2. Reproduce on compile-only path

Use:

```bash
env GOCACHE=/home/qinkejiu/mygo/.mygo-tmp/test-go/go-build \
    GOTMPDIR=/home/qinkejiu/mygo/.mygo-tmp/test-go/go-tmp \
    go run ./cmd/mygo compile -emit=verilog -o /tmp/aes_hw.sv tests/CHStone/aes/main.go
```

If this hangs or never produces `/tmp/aes_hw.sv`, the problem is before Verilator.

### 3. Focus on `*ssa.Alloc` warnings in AES

The `no signal mapping for value *ssa.Alloc` warnings strongly suggest some local aggregate or addressable temporary in AES is not being lowered into IR/MLIR correctly.

Useful entry points:

- [internal/ir/builder.go](/home/qinkejiu/mygo/internal/ir/builder.go)
- [internal/mlir/emitter.go](/home/qinkejiu/mygo/internal/mlir/emitter.go)

Search:

```bash
rg -n 'no signal mapping for value \\*ssa.Alloc|Alloc' internal/ir internal/mlir
```

### 4. Compare AES against the already-fixed CHStone array cases

The array-related fixes that unblocked `dfadd/dfdiv/dfmul/dfsin` are likely relevant, but AES probably also has a harder case involving nested arrays or mutable local storage.

Relevant examples:

- `dfadd`, `dfdiv`, `dfmul`, `dfsin` all use large constant arrays and now work
- `aes` uses structures like:
  - `Sbox = [16][16]int{...}`
  - `invSbox = [16][16]int{...}`
  - `Rcon0 = [30]int{...}`

So AES may need:

- nested indexed-array lowering fixes
- local aggregate storage fixes
- `ssa.Alloc` mapping fixes

## Validation Sequence After Fixing AES

Run these in order:

```bash
go run ./cmd/mygo sim --sim-max-cycles 256 tests/CHStone/aes/main.go
```

If that succeeds and matches software:

1. remove `shouldUseSoftwareSimulationFallback` / `runSoftwareSimulationFallback`
2. remove the AES-specific test that expects fallback behavior
3. rerun:

```bash
python3 tests/CHStone/test_CHStone.py
python3 tests/stages/test_stages.py
```

4. confirm:

- no `hardware fallback` text exists in `CHS_clean_simulation_results.txt`
- `CHS_clean_simulation_results.txt` has no hardware/software mismatches
- `stages_clean_simulation_results.txt` still has no hardware/software mismatches

## Helpful One-Liners

Check whether any fallback text remains:

```bash
rg -n 'fallback' cmd/mygo/main.go cmd/mygo/sim_internal_test.go tests/CHStone/test_CHStone.py tests/CHStone/CHS_clean_simulation_results.txt
```

Check final CHStone mismatch status:

```bash
python3 - <<'PY'
from pathlib import Path
import re
text=Path('CHS_clean_simulation_results.txt').read_text(encoding='utf-8')
parts=text.split('\n' + '='*70 + '\n')
sections=[]
for part in parts:
    lines=part.strip('\n').splitlines()
    if not lines or not lines[0].startswith('📁 '):
        continue
    m=re.match(r'📁\s+(.*?)\s+\|\s+(.*)', lines[0])
    case, kind = m.group(1), m.group(2)
    body='\n'.join(lines[3:]).strip()
    sections.append((case, kind, body))
by_case={}
for case,kind,body in sections:
    by_case.setdefault(case,{})[kind]=body
print([case for case,kinds in sorted(by_case.items())
       if next((v for k,v in kinds.items() if k.startswith('hardware')), None) != kinds.get('software simulation')])
PY
```

## Bottom Line

The broad CHStone fallback removal work is mostly done.

The remaining task for the next conversation is narrow:

- make real `aes` verilog/sim work
- then delete the internal AES fallback from `cmd/mygo/main.go`
