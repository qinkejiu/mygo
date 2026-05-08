package mlir

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mygo/internal/diag"
	"mygo/internal/frontend"
	"mygo/internal/ir"
)

func TestPrintVerbSpecifier(t *testing.T) {
	tests := []struct {
		name string
		seg  ir.PrintSegment
		want string
	}{
		{
			name: "decimal default",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbDec},
			want: "%0d",
		},
		{
			name: "hex zero padded width",
			seg: ir.PrintSegment{
				Verb:    ir.PrintVerbHex,
				Width:   16,
				ZeroPad: true,
			},
			want: "%016x",
		},
		{
			name: "hex width no zero pad",
			seg: ir.PrintSegment{
				Verb:  ir.PrintVerbHex,
				Width: 8,
			},
			want: "%8x",
		},
		{
			name: "hex default uses minimal width",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbHex},
			want: "%0x",
		},
		{
			name: "binary default uses minimal width",
			seg: ir.PrintSegment{
				Verb:    ir.PrintVerbBin,
				ZeroPad: true,
			},
			want: "%0b",
		},
		{
			name: "float verb",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbFloat},
			want: "%f",
		},
		{
			name: "bool verb",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbBool},
			want: "%0s",
		},
		{
			name: "decimal no width uses zero flag",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbDec},
			want: "%0d",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := printVerbSpecifier(tc.seg)
			if got != tc.want {
				t.Fatalf("printVerbSpecifier() = %q, want %q", got, tc.want)
			}
		})
	}
}

const mutableRegPortSeparationProgram = `
package main

import "fmt"

var test_data = [2]int{1, 2}
var test_result = [2]int{}

func main() {
	for i := 0; i < 2; i++ {
		test_result[i] = test_data[i]
	}
	if test_result[1] != 0 {
		fmt.Printf("%d\n", test_result[1])
	}
}
`

const fsmPrintUsesUpdatedRegValueProgram = `
package main

import "fmt"

var out int

func main() {
	out = 0x1f
	out = 0x39
	fmt.Printf("%x\n", out)
}
`

func TestValueRefDistinguishesPortsFromMutableRegs(t *testing.T) {
	design := buildMLIRDesignFromSource(t, mutableRegPortSeparationProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sv.read_inout %test_data_1") {
		t.Fatalf("port test_data_1 should not be read via sv.read_inout:\n%s", text)
	}
	if !strings.Contains(text, "sv.read_inout %test_result_1") && !strings.Contains(text, "sv.read_inout %test_result") {
		t.Fatalf("mutable test_result storage should be read via sv.read_inout:\n%s", text)
	}
}

func TestPackedTopInputPortDoesNotReadAsInOutRegression(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "verilog-eval", "handoff_156_current", "go_files", "Prob092_gatesv100", "main.go")
	design := buildMLIRDesignFromFile(t, source)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "hw.module @TopModule(in %in: i100") {
		t.Fatalf("expected packed input port in emitted MLIR:\n%s", text)
	}
	if strings.Contains(text, "sv.read_inout %in : !hw.inout<i100>") {
		t.Fatalf("packed input port %%in must remain a plain value port:\n%s", text)
	}
}

func TestPackedLocalArrayReadsAvoidRawIndexedNamesRegression(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "verilog-eval", "handoff_156_current", "go_files", "Prob092_gatesv100", "main.go")
	design := buildMLIRDesignFromFile(t, source)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, ", %different_") || strings.Contains(text, ", %any_") || strings.Contains(text, ", %both_") {
		t.Fatalf("expected packed local array reads to resolve to materialized SSA values instead of raw indexed names:\n%s", text)
	}
}

func TestEncodedNextStateOutputIsNotGuardedByStateZeroRegression(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "verilog-eval", "handoff_156_current", "go_files", "Prob100_fsm3comb", "main.go")
	design := buildMLIRDesignFromFile(t, source)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "next_state_guard") || strings.Contains(text, "state_empty") {
		t.Fatalf("encoded next_state outputs must not be forced to zero when state==0:\n%s", text)
	}
}

func TestAccumulatedNextStateOutputKeepsStateZeroGuardRegression(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "verilog-eval", "handoff_156_current", "go_files", "Prob143_fsm_onehot", "main.go")
	design := buildMLIRDesignFromFile(t, source)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "next_state_guard") || !strings.Contains(text, "state_empty") {
		t.Fatalf("single-assignment accumulated next_state outputs should keep the state==0 guard:\n%s", text)
	}
}

func TestDirectClockedPhiOutputDoesNotUseRawTemporaryRegression(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "verilog-eval", "handoff_156_current", "go_files", "Prob136_m2014_q6", "main.go")
	design := buildMLIRDesignFromFile(t, source)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "hw.output %t") {
		t.Fatalf("direct-clocked phi-backed outputs must be resolved before hw.output:\n%s", text)
	}
}

func TestFSMPrintUsesLatestAssignedRegValue(t *testing.T) {
	design := buildMLIRDesignFromSource(t, fsmPrintUsesUpdatedRegValueProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `sv.fwrite`) || !strings.Contains(text, `"%0x`) {
		t.Fatalf("expected hex fwrite in MLIR output:\n%s", text)
	}
	if strings.Contains(text, "sv.read_inout %out") {
		t.Fatalf("print should use the latest assigned value, not re-read %%out:\n%s", text)
	}
}

const fsmPrintAfterInlineSliceMutationProgram = `
package main

import "fmt"

var arr [2]int

func fill(a []int) {
	a[0] = 0x39
	a[1] = 0x25
}

func main() {
	fill(arr[:])
	fmt.Printf("%x%x\n", arr[0], arr[1])
}
`

const printVarargsDeadScratchProgram = `
package main

import "fmt"

func main() {
	var a int16 = -12
	var b int16 = 25
	var carry uint16 = 0x1234

	partial := int32(a) + int32(b)
	widened := uint32(uint16(partial)) + uint32(carry)
	fmt.Printf("partial=%d widened=0x%x\n", partial, widened)
}
`

const zeroInitializedGlobalArrayProgram = `
package main

var input [8]int
var result int

func main() {
	i := 0
	max := input[0]
	for i < 8 {
		if input[i] > max {
			max = input[i]
		}
		i++
	}
	result = max
}
`

const processRegPortProgram = `
package main

var seen int32

func sink(in <-chan int32) {
	seen = <-in
}

func main() {
	ch := make(chan int32, 1)
	go sink(ch)
	ch <- 7
}
`

const convertOpsProgram = `
package main

var outWide uint64
var outSigned int64
var outNarrow uint8

func TopModule(a uint32, b int32) {
	outWide = uint64(a)
	outSigned = int64(b)
	outNarrow = uint8(a)
}
`

const packedWordArrayProgram = `
package main

var arr = [2]uint64{0x1111111111111111, 0x2222222222222222}
var idx int
var out uint64

func main() {
	idx = 1
	out = arr[idx]
}
`

const multiProducerArbitrationProgram = `
package main

func writer0(out chan<- int32) {
	out <- 1
}

func writer1(out chan<- int32) {
	out <- 2
}

func main() {
	ch := make(chan int32, 1)
	go writer0(ch)
	go writer1(ch)
	_ = <-ch
	_ = <-ch
}
`

const directClockedOutputProgram = `
package main

var state uint8
var out_q uint8

func TopModule(clk bool, a bool) {
	if clk {
		if a {
			state = 4
		} else if state == 6 {
			state = 0
		} else {
			state = state + 1
		}
	}

	out_q = state & 0x7
}
`

const branchedDirectClockedOutputsProgram = `
package main

var state uint8
var out_a bool
var out_b bool

func TopModule(clk bool, reset bool, in bool) {
	next := state
	if state == 0 {
		if in {
			next = 1
		}
	} else {
		if in {
			next = 0
		}
	}
	if clk {
		if reset {
			state = 0
		} else {
			state = next
		}
	}
	if state == 0 {
		out_a = true
		out_b = false
	} else {
		out_a = false
		out_b = true
	}
}
`

const directClockedPromotedStateProgram = `
package main

var out_q uint8

func TopModule(clk bool, load bool, data uint8) {
	var reg uint8
	if clk {
		if load {
			reg = data
		} else {
			reg = reg + 1
		}
		out_q = reg
	}
}
`

const directClockedClockLeakProgram = `
package main

var out_q uint8

func TopModule(clk bool, reset bool) {
	var state uint8 = 1
	if clk {
		if reset {
			state = 1
		} else {
			lsb := state & 0x1
			next := state >> 1
			if lsb != 0 {
				next |= 0x10
			}
			if lsb != 0 {
				next ^= 0x04
			}
			state = next
		}
	}
	out_q = state & 0x1f
}

func main() {}
`

const fsmIndexedHistoryProgram = `
package main

var out_q [4]bool

func TopModule(clk bool, load bool, data [4]bool) {
	if clk {
		var next [4]bool
		if load {
			for i := 0; i < 4; i++ {
				next[i] = data[i]
			}
		} else {
			lsb := out_q[0]
			for i := 0; i < 3; i++ {
				next[i] = out_q[i+1]
			}
			next[3] = lsb
		}
		for i := 0; i < 4; i++ {
			out_q[i] = next[i]
		}
	}
}
`

const fsmLocalMakesliceProgram = `
package main

import "fmt"

func lookup(sel int) int {
	ret := make([]int, 4)
	ret[0] = 11
	ret[1] = 22
	ret[2] = 33
	ret[3] = 44
	return ret[sel]
}

func main() {
	sel := 0
	if lookup(0) > 0 {
		sel = 2
	}
	fmt.Printf("%d\n", lookup(sel))
}
`

func TestFSMPrintAfterInlineSliceMutationUsesUpdatedValues(t *testing.T) {
	design := buildMLIRDesignFromSource(t, fsmPrintAfterInlineSliceMutationProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "sv.bpassign %print_reg") {
		t.Fatalf("expected print scratch register materialization:\n%s", text)
	}
	if !strings.Contains(text, "sv.fwrite") || !strings.Contains(text, "%print_val") {
		t.Fatalf("expected print to use scratch-backed values:\n%s", text)
	}
}

func TestPrintVarargsScratchDoesNotReadUndeclaredWire(t *testing.T) {
	design := buildMLIRDesignFromSource(t, printVarargsDeadScratchProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sv.read_inout %varargs") {
		t.Fatalf("dead print varargs scratch must not emit an undeclared read:\n%s", text)
	}
}

func TestZeroInitializedGlobalArrayDoesNotReadUndeclaredPackedWire(t *testing.T) {
	design := buildMLIRDesignFromSource(t, zeroInitializedGlobalArrayProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sv.read_inout %input :") {
		t.Fatalf("zero-initialized packed globals must not emit undeclared reads:\n%s", text)
	}
}

func TestRootSignalRefUsesCachedConstantForImmutableReg(t *testing.T) {
	var out strings.Builder
	sig := &ir.Signal{
		Name:  "table_0",
		Type:  &ir.SignalType{Width: 32},
		Kind:  ir.Reg,
		Value: int32(42),
	}
	em := &emitter{
		w:               &out,
		currentAssigned: map[string]struct{}{},
		immutableConsts: make(map[*ir.Signal]string),
	}
	refA := em.rootSignalRef(sig)
	refB := em.normalizeResolvedSignalRef(sig, "%table_0")
	if refA == "" || refB == "" {
		t.Fatalf("expected immutable reg to resolve to a constant ref")
	}
	if refA != refB {
		t.Fatalf("expected immutable reg reads to reuse the cached constant, got %q and %q", refA, refB)
	}
	text := out.String()
	if strings.Contains(text, "sv.read_inout %table_0") {
		t.Fatalf("immutable reg root reads must not emit sv.read_inout:\n%s", text)
	}
	if strings.Count(text, "hw.constant 42 : i32") != 1 {
		t.Fatalf("expected one cached constant for immutable reg, got:\n%s", text)
	}
}

func TestValueRefUsesConstantForImmutableWideRegInDirectClockedPath(t *testing.T) {
	var out strings.Builder
	sig := &ir.Signal{
		Name:  "table_0",
		Type:  &ir.SignalType{Width: 64},
		Kind:  ir.Reg,
		Value: uint64(0x1234),
	}
	p := &processPrinter{
		w:             &out,
		moduleSignals: map[string]*ir.Signal{"table_0": sig},
		emitter:       &emitter{w: &out},
		proc:          &ir.Process{},
	}
	p.resetState()
	p.directClocked = true
	p.beginBlockValueScope()
	refA := p.valueRef(sig)
	p.endBlockValueScope()
	p.beginBlockValueScope()
	refB := p.valueRef(sig)
	p.endBlockValueScope()
	if refA == "" || refB == "" {
		t.Fatalf("expected immutable wide reg to resolve to a usable ref")
	}
	if refA != refB {
		t.Fatalf("expected immutable wide reg constant to be reused across block scopes, got %q and %q", refA, refB)
	}
	text := out.String()
	if strings.Contains(text, "sv.read_inout %table_0") {
		t.Fatalf("direct-clocked immutable wide regs must not fall back to sv.read_inout:\n%s", text)
	}
	if strings.Count(text, "hw.constant 4660 : i64") != 1 {
		t.Fatalf("expected immutable wide reg constant to be emitted once and reused:\n%s", text)
	}
}

func TestPackArraySignalValueReusesImmutablePackedValueAcrossBlockScopes(t *testing.T) {
	var out strings.Builder
	base := &ir.Signal{Name: "table", Type: &ir.SignalType{Width: 128}, Kind: ir.Reg}
	elem0 := &ir.Signal{Name: "table_0", Type: &ir.SignalType{Width: 64}, Kind: ir.Reg, Value: uint64(0x11)}
	elem1 := &ir.Signal{Name: "table_1", Type: &ir.SignalType{Width: 64}, Kind: ir.Reg, Value: uint64(0x22)}
	p := &processPrinter{
		w: &out,
		moduleSignals: map[string]*ir.Signal{
			"table":   base,
			"table_0": elem0,
			"table_1": elem1,
		},
		emitter: &emitter{w: &out, currentAssigned: map[string]struct{}{}},
		proc:    &ir.Process{},
	}
	p.resetState()
	p.beginBlockValueScope()
	refA := p.valueRef(base)
	p.endBlockValueScope()
	p.beginBlockValueScope()
	refB := p.valueRef(base)
	p.endBlockValueScope()
	if refA == "" || refB == "" {
		t.Fatalf("expected immutable packed array to resolve to usable refs")
	}
	if refA != refB {
		t.Fatalf("expected immutable packed array value to be reused across block scopes, got %q and %q", refA, refB)
	}
	text := out.String()
	if strings.Count(text, "comb.concat") != 1 {
		t.Fatalf("expected immutable packed array to be concatenated once and reused:\n%s", text)
	}
	if strings.Count(text, "hw.constant 17 : i64") != 1 || strings.Count(text, "hw.constant 34 : i64") != 1 {
		t.Fatalf("expected immutable packed array elements to be constantized once:\n%s", text)
	}
}

func TestAssignConstReusesEquivalentLiterals(t *testing.T) {
	var out strings.Builder
	a := &ir.Signal{Type: &ir.SignalType{Width: 1}, Kind: ir.Const, Value: true}
	b := &ir.Signal{Type: &ir.SignalType{Width: 1}, Kind: ir.Const, Value: true}
	p := &processPrinter{
		w:       &out,
		emitter: &emitter{w: &out},
	}
	p.resetState()
	refA := p.assignConst(a)
	refB := p.assignConst(b)
	if refA == "" || refB == "" {
		t.Fatalf("expected assignConst to produce valid refs")
	}
	if refA != refB {
		t.Fatalf("expected equivalent literals to share one const ref, got %q and %q", refA, refB)
	}
}

func TestEdgeValueRefReusesEquivalentBinaryOpsWithinScope(t *testing.T) {
	var out strings.Builder
	c1 := &ir.Signal{Type: &ir.SignalType{Width: 32}, Kind: ir.Const, Value: int32(1)}
	c2 := &ir.Signal{Type: &ir.SignalType{Width: 32}, Kind: ir.Const, Value: int32(2)}
	a := &ir.Signal{Name: "a", Type: &ir.SignalType{Width: 32}, Kind: ir.Wire}
	b := &ir.Signal{Name: "b", Type: &ir.SignalType{Width: 32}, Kind: ir.Wire}
	block := &ir.BasicBlock{
		Ops: []ir.Operation{
			&ir.BinOperation{Op: ir.Add, Dest: a, Left: c1, Right: c2},
			&ir.BinOperation{Op: ir.Add, Dest: b, Left: c1, Right: c2},
		},
	}
	proc := &ir.Process{Blocks: []*ir.BasicBlock{block}}
	p := &processPrinter{
		w:       &out,
		emitter: &emitter{w: &out},
		proc:    proc,
	}
	p.resetState()
	p.beginBlockValueScope()
	refA := p.edgeValueRef(a)
	refB := p.edgeValueRef(b)
	p.endBlockValueScope()
	if refA == "" || refB == "" {
		t.Fatalf("expected edge refs for equivalent binary ops")
	}
	if refA != refB {
		t.Fatalf("expected equivalent edge binary ops to reuse one SSA value, got %q and %q", refA, refB)
	}
	text := out.String()
	if strings.Count(text, " = comb.add ") != 1 {
		t.Fatalf("expected one cached comb.add for equivalent edge ops:\n%s", text)
	}
}

func TestSequentialPrecomputeReusesEquivalentPureOpsAcrossBlocks(t *testing.T) {
	var out strings.Builder
	a := &ir.Signal{Name: "a", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	b := &ir.Signal{Name: "b", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	cond := &ir.Signal{Name: "cond", Type: &ir.SignalType{Width: 1}, Kind: ir.Wire}
	addA := &ir.Signal{Name: "add_a", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	addB := &ir.Signal{Name: "add_b", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	cmpA := &ir.Signal{Name: "cmp_a", Type: &ir.SignalType{Width: 1}, Kind: ir.Wire}
	cmpB := &ir.Signal{Name: "cmp_b", Type: &ir.SignalType{Width: 1}, Kind: ir.Wire}
	muxA := &ir.Signal{Name: "mux_a", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	muxB := &ir.Signal{Name: "mux_b", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	convA := &ir.Signal{Name: "conv_a", Type: &ir.SignalType{Width: 16}, Kind: ir.Wire}
	convB := &ir.Signal{Name: "conv_b", Type: &ir.SignalType{Width: 16}, Kind: ir.Wire}
	block1 := &ir.BasicBlock{
		Ops: []ir.Operation{
			&ir.BinOperation{Op: ir.Add, Dest: addA, Left: a, Right: b},
			&ir.CompareOperation{Predicate: ir.CompareEQ, Dest: cmpA, Left: a, Right: b},
			&ir.MuxOperation{Dest: muxA, Cond: cond, TrueValue: addA, FalseValue: a},
			&ir.ConvertOperation{Dest: convA, Value: addA},
		},
	}
	block2 := &ir.BasicBlock{
		Ops: []ir.Operation{
			&ir.BinOperation{Op: ir.Add, Dest: addB, Left: a, Right: b},
			&ir.CompareOperation{Predicate: ir.CompareEQ, Dest: cmpB, Left: a, Right: b},
			&ir.MuxOperation{Dest: muxB, Cond: cond, TrueValue: addB, FalseValue: a},
			&ir.ConvertOperation{Dest: convB, Value: addB},
		},
	}
	proc := &ir.Process{Blocks: []*ir.BasicBlock{block1, block2}}
	p := &processPrinter{
		w:       &out,
		emitter: &emitter{w: &out},
		proc:    proc,
	}
	p.resetState()
	p.directClocked = true
	p.emitPrecomputedPureOps(proc)
	text := out.String()
	if strings.Count(text, " = comb.add ") != 1 {
		t.Fatalf("expected one shared comb.add across direct-clocked precompute blocks:\n%s", text)
	}
	if strings.Count(text, " = comb.icmp eq ") != 1 {
		t.Fatalf("expected one shared comb.icmp across direct-clocked precompute blocks:\n%s", text)
	}
	if strings.Count(text, " = comb.mux ") != 1 {
		t.Fatalf("expected one shared comb.mux across direct-clocked precompute blocks:\n%s", text)
	}
	if strings.Count(text, "arith.extui") != 1 {
		t.Fatalf("expected one shared arith.extui across direct-clocked precompute blocks:\n%s", text)
	}
}

func TestCachedEdgeMuxAndCompareFoldTrivialCases(t *testing.T) {
	var out strings.Builder
	p := &processPrinter{
		w:       &out,
		emitter: &emitter{w: &out},
	}
	p.resetState()
	p.beginBlockValueScope()
	sameArms := p.cachedEdgeMux("%cond", "%x", "%x", "i64")
	trueArm := p.cachedEdgeMux(p.boolConst(true), "%a", "%b", "i64")
	falseArm := p.cachedEdgeMux(p.boolConst(false), "%a", "%b", "i64")
	eqSelf := p.cachedEdgeCompare("eq", "%z", "%z", "i16")
	neSelf := p.cachedEdgeCompare("ne", "%z", "%z", "i16")
	p.endBlockValueScope()
	if sameArms != "%x" || trueArm != "%a" || falseArm != "%b" {
		t.Fatalf("expected trivial muxes to fold directly")
	}
	if eqSelf != p.boolConst(true) || neSelf != p.boolConst(false) {
		t.Fatalf("expected self compares to fold to bool constants")
	}
	text := out.String()
	if strings.Contains(text, "comb.mux") || strings.Contains(text, "comb.icmp") {
		t.Fatalf("trivial edge mux/compare folds must not emit comb ops:\n%s", text)
	}
}

func TestEmitOperationMuxFoldsIdenticalArms(t *testing.T) {
	var out strings.Builder
	cond := &ir.Signal{Type: &ir.SignalType{Width: 1}, Kind: ir.Const, Value: true}
	val := &ir.Signal{Type: &ir.SignalType{Width: 8}, Kind: ir.Const, Value: uint8(7)}
	dest := &ir.Signal{Name: "dest", Type: &ir.SignalType{Width: 8}, Kind: ir.Wire}
	op := &ir.MuxOperation{Dest: dest, Cond: cond, TrueValue: val, FalseValue: val}
	p := &processPrinter{
		w:       &out,
		emitter: &emitter{w: &out},
	}
	p.resetState()
	block := &ir.BasicBlock{}
	p.emitOperation(block, op, nil)
	if got := p.valueNames[dest]; got != p.assignConst(val) {
		t.Fatalf("expected mux with identical arms to fold to the shared value, got %q", got)
	}
	if strings.Contains(out.String(), "comb.mux") {
		t.Fatalf("identical-arm mux must not emit comb.mux:\n%s", out.String())
	}
}

func TestProcessRegPortsDoNotNestInOutTypes(t *testing.T) {
	design := buildMLIRDesignFromSource(t, processRegPortProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "!hw.inout<!hw.inout<") {
		t.Fatalf("process instance bindings must not double-wrap inout types:\n%s", text)
	}
}

func TestPackedWordArrayReadsUseIndexedElementState(t *testing.T) {
	design := buildMLIRDesignFromSource(t, packedWordArrayProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "comb.shru %c0_i128") || strings.Contains(text, "comb.shru %c_zero") {
		t.Fatalf("packed word array access must not read from a zero placeholder:\n%s", text)
	}
	if !strings.Contains(text, "comb.concat") {
		t.Fatalf("packed word array access should rebuild from indexed element state or constants:\n%s", text)
	}
}

func TestConvertOpsUseArithExtAndTrunc(t *testing.T) {
	design := buildMLIRDesignFromSource(t, convertOpsProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "arith.extui") || !strings.Contains(text, "arith.extsi") || !strings.Contains(text, "arith.trunci") {
		t.Fatalf("expected convert lowering to use compact arith ext/trunc ops:\n%s", text)
	}
	if strings.Contains(text, "comb.replicate") {
		t.Fatalf("convert lowering must not expand sign extension via comb.replicate anymore:\n%s", text)
	}
}

func TestEmitMultiProducerChannelArbitration(t *testing.T) {
	design := buildMLIRDesignFromSource(t, multiProducerArbitrationProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "chan_t0_prod0_writer0_wdata") {
		t.Fatalf("expected dedicated write wires for writer0:\n%s", text)
	}
	if !strings.Contains(text, "chan_t0_prod1_writer1_wdata") {
		t.Fatalf("expected dedicated write wires for writer1:\n%s", text)
	}
	if !strings.Contains(text, "sv.assign %chan_t0_wvalid") || !strings.Contains(text, "comb.and") {
		t.Fatalf("expected top-level arbitration on the shared FIFO write interface:\n%s", text)
	}
	if !strings.Contains(text, "sv.assign %chan_t0_prod0_writer0_wready") {
		t.Fatalf("expected ready to be routed back to writer0-specific wires:\n%s", text)
	}
	if !strings.Contains(text, "sv.assign %chan_t0_prod1_writer1_wready") {
		t.Fatalf("expected ready to be routed back to writer1-specific wires:\n%s", text)
	}
	if !strings.Contains(text, "chan_t0_wdata: %chan_t0_prod0_writer0_wdata") {
		t.Fatalf("expected writer0 instance to bind to producer-local wires:\n%s", text)
	}
	if !strings.Contains(text, "chan_t0_wdata: %chan_t0_prod1_writer1_wdata") {
		t.Fatalf("expected writer1 instance to bind to producer-local wires:\n%s", text)
	}
}

func TestEmitDirectClockedOutputUsesResolvedSSAValue(t *testing.T) {
	design := buildMLIRDesignFromSource(t, directClockedOutputProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "hw.output %out_q") {
		t.Fatalf("expected resolved output SSA value, got raw out_q reference:\n%s", text)
	}
}

func TestEmitBranchedDirectClockedOutputsAvoidRawOutputRefs(t *testing.T) {
	design := buildMLIRDesignFromSource(t, branchedDirectClockedOutputsProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "hw.output %out_a") || strings.Contains(text, "hw.output %out_b") {
		t.Fatalf("expected resolved branched outputs, got raw out_* references:\n%s", text)
	}
}

func TestEmitDirectClockedUsesUpdatedPromotedStateValue(t *testing.T) {
	design := buildMLIRDesignFromSource(t, directClockedPromotedStateProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sv.passign %out_q, %__mygo_state_TopModule_reg") ||
		strings.Contains(text, "sv.passign %out_q_0, %__mygo_state_TopModule_reg") {
		t.Fatalf("expected direct-clocked output to use the updated promoted state value:\n%s", text)
	}
}

func TestEmitDirectClockedDoesNotBakeClockIntoEdgeHelpers(t *testing.T) {
	design := buildMLIRDesignFromSource(t, directClockedClockLeakProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	start := strings.Index(text, "sv.always posedge %clk {")
	if start < 0 {
		t.Fatalf("expected direct-clocked sv.always block:\n%s", text)
	}
	alwaysBody := text[start+len("sv.always posedge %clk {"):]
	if strings.Contains(alwaysBody, "comb.and %clk") || strings.Contains(alwaysBody, "comb.mux %clk") {
		t.Fatalf("direct-clocked edge helpers inside sv.always must not depend on %%clk:\n%s", text)
	}
}

func TestEmitFSMIndexedHistoryReadsPackedElementState(t *testing.T) {
	design := buildMLIRDesignFromSource(t, fsmIndexedHistoryProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sv.read_inout %out_q : !hw.inout<i4>") ||
		strings.Contains(text, "sv.read_inout %out_q_0 : !hw.inout<i4>") ||
		strings.Contains(text, "sv.read_inout %next : !hw.inout<i4>") {
		t.Fatalf("expected indexed history state to be rebuilt from element values instead of a stale packed shadow:\n%s", text)
	}
}

func TestEmitFSMLocalMakesliceIndexedReadsAvoidUndeclaredInouts(t *testing.T) {
	design := buildMLIRDesignFromSource(t, fsmLocalMakesliceProgram)
	out := filepath.Join(t.TempDir(), "design.mlir")
	if err := Emit(design, out); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mlir output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sv.read_inout %makeslice_") {
		t.Fatalf("local make([]) temporaries must not lower to undeclared makeslice inout reads:\n%s", text)
	}
}

func buildMLIRDesignFromSource(t *testing.T, source string) *ir.Design {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(goMod, []byte("module testcase\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	reporter := diag.NewReporter(io.Discard, "text")
	cfg := frontend.LoadConfig{Sources: []string{file}}
	pkgs, _, err := frontend.LoadPackages(cfg, reporter)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	prog, _, err := frontend.BuildSSA(pkgs, reporter)
	if err != nil {
		t.Fatalf("build ssa: %v", err)
	}
	design, err := ir.BuildDesign(prog, reporter)
	if err != nil {
		t.Fatalf("build design: %v", err)
	}
	return design
}

func buildMLIRDesignFromFile(t *testing.T, source string) *ir.Design {
	t.Helper()
	reporter := diag.NewReporter(io.Discard, "text")
	cfg := frontend.LoadConfig{Sources: []string{source}}
	pkgs, _, err := frontend.LoadPackages(cfg, reporter)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	prog, _, err := frontend.BuildSSA(pkgs, reporter)
	if err != nil {
		t.Fatalf("build ssa: %v", err)
	}
	design, err := ir.BuildDesign(prog, reporter)
	if err != nil {
		t.Fatalf("build design: %v", err)
	}
	return design
}
