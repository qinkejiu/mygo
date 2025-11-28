package passes

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"mygo/internal/diag"
	"mygo/internal/ir"
)

func TestWidthInferencePropagatesUnknownBinary(t *testing.T) {
	left := &ir.Signal{Name: "a", Type: &ir.SignalType{Width: 8, Signed: false}}
	right := &ir.Signal{Name: "b", Type: &ir.SignalType{Width: 8, Signed: false}}
	dest := &ir.Signal{Name: "sum", Type: &ir.SignalType{}}

	design := buildTestDesign([]ir.Operation{
		&ir.BinOperation{Op: ir.Add, Dest: dest, Left: left, Right: right},
	}, left, right, dest)

	reporter := diag.NewReporter(io.Discard, "text")
	pass := NewWidthInference(reporter)
	if err := pass.Run(design); err != nil {
		t.Fatalf("width inference failed: %v", err)
	}

	if dest.Type == nil || dest.Type.Width != 8 {
		t.Fatalf("expected inferred width 8, got %+v", dest.Type)
	}
}

func TestWidthInferenceAssignmentTruncation(t *testing.T) {
	src := &ir.Signal{Name: "src", Type: &ir.SignalType{Width: 16, Signed: false}}
	dst := &ir.Signal{Name: "dst", Type: &ir.SignalType{Width: 8, Signed: false}}

	design := buildTestDesign([]ir.Operation{
		&ir.AssignOperation{Dest: dst, Value: src},
	}, src, dst)

	var buf bytes.Buffer
	reporter := diag.NewReporter(&buf, "text")
	pass := NewWidthInference(reporter)
	if err := pass.Run(design); err == nil {
		t.Fatalf("expected width inference to fail due to truncation")
	}
	if !reporter.HasErrors() {
		t.Fatalf("expected reporter to contain errors")
	}
	if !strings.Contains(buf.String(), "truncates") {
		t.Fatalf("expected truncation diagnostic, got %q", buf.String())
	}
}

func TestWidthInferenceSignedMismatch(t *testing.T) {
	left := &ir.Signal{Name: "lhs", Type: &ir.SignalType{Width: 8, Signed: true}}
	right := &ir.Signal{Name: "rhs", Type: &ir.SignalType{Width: 8, Signed: false}}
	dest := &ir.Signal{Name: "out", Type: &ir.SignalType{Width: 8, Signed: true}}

	design := buildTestDesign([]ir.Operation{
		&ir.BinOperation{Op: ir.Add, Dest: dest, Left: left, Right: right},
	}, left, right, dest)

	var buf bytes.Buffer
	reporter := diag.NewReporter(&buf, "text")
	pass := NewWidthInference(reporter)
	if err := pass.Run(design); err == nil {
		t.Fatalf("expected width inference to fail due to signed mismatch")
	}
	if !strings.Contains(buf.String(), "mixed signed/unsigned") {
		t.Fatalf("expected signed mismatch diagnostic, got %q", buf.String())
	}
}

func buildTestDesign(ops []ir.Operation, signals ...*ir.Signal) *ir.Design {
	module := &ir.Module{
		Name:    "test",
		Signals: make(map[string]*ir.Signal),
		Processes: []*ir.Process{
			{
				Sensitivity: ir.Sequential,
				Blocks: []*ir.BasicBlock{
					{Label: "entry", Ops: ops},
				},
			},
		},
	}
	for _, sig := range signals {
		if sig == nil {
			continue
		}
		name := sig.Name
		if name == "" {
			name = fmt.Sprintf("sig_%p", sig)
			sig.Name = name
		}
		module.Signals[name] = sig
	}
	return &ir.Design{Modules: []*ir.Module{module}, TopLevel: module}
}
