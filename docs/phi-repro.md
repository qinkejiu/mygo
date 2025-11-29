# Phi Lowering Reproduction

The `tests/e2e/phi_loop` workload is a tiny Go program that exercises a pair of
counted loops (one writer, one reader) connected by a buffered channel.  When
lowered to the MyGO IR each loop introduces `ir.PhiOperation`s to thread the
loop-carried state, so it is the smallest example we have that still hits the
current "phis are not lowered" gap in the MLIR backend.

## Running the Repro

```bash
cd /path/to/mygo
GOCACHE=$PWD/.gocache \
go run ./cmd/mygo compile \
    -emit=verilog \
    --circt-opt=third_party/circt/build/bin/circt-opt \
    --fifo-src=internal/backend/templates/simple_fifo.sv \
    -o /tmp/phi_loop.sv \
    tests/e2e/phi_loop/main.go
```

Expected failure (abridged):

```
/tmp/mygo-XXXX/design.mlir:42:24: error: expected SSA operand
    %v2 = comb.icmp ult, %t1_11, %c2 : i32
                       ^
backend: circt-opt --export-verilog failed: exit status 1
```

The undefined `%t1_11` value is a phi result that currently lowers to a comment
in `internal/mlir/emitter.go`.  Once the phi lowering pass materializes real
SSA values (likely via `seq.compreg` + muxes per block predicate) this command
should succeed and the documentation above can be retired.
