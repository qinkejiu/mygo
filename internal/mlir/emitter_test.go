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
	source := filepath.Join("..", "..", "verilog-eval", "handoff_156_current", "go_files", "Prob092_gatesv100", "main.go")
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
	source := filepath.Join("..", "..", "verilog-eval", "handoff_156_current", "go_files", "Prob092_gatesv100", "main.go")
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
	source := filepath.Join("..", "..", "verilog-eval", "handoff_156_current", "go_files", "Prob100_fsm3comb", "main.go")
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
	source := filepath.Join("..", "..", "verilog-eval", "handoff_156_current", "go_files", "Prob143_fsm_onehot", "main.go")
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
	source := filepath.Join("..", "..", "verilog-eval", "handoff_156_current", "go_files", "Prob136_m2014_q6", "main.go")
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
