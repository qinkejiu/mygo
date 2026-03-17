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
			name: "zero pad ignored without width",
			seg: ir.PrintSegment{
				Verb:    ir.PrintVerbBin,
				ZeroPad: true,
			},
			want: "%b",
		},
		{
			name: "float verb",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbFloat},
			want: "%f",
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
	if !strings.Contains(text, "sv.read_inout %test_result_1") {
		t.Fatalf("mutable reg test_result_1 should be read via sv.read_inout:\n%s", text)
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
	if !strings.Contains(text, `sv.fwrite`) || !strings.Contains(text, `"%x`) {
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
