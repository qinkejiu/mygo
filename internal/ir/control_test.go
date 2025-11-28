package ir

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"mygo/internal/diag"
	"mygo/internal/frontend"
)

const branchProgram = `
package main

func sink(v int32) {}

func main() {
    var a, b, out int32
    a = 5
    b = 7
    if a < b {
        out = a + 1
    } else {
        out = b - 1
    }
    sink(out)
}
`

const pipelineProgram = `
package main

func source(out chan<- int32) {
    var v int32
    v = 1
    out <- v
}

func middle(in <-chan int32, out chan<- int32) {
    var v int32
    v = <-in
    out <- v + 1
}

func drain(in <-chan int32) {
    var v int32
    v = <-in
    _ = v
}

func main() {
    ch0 := make(chan int32, 2)
    ch1 := make(chan int32, 2)
    go source(ch0)
    go middle(ch0, ch1)
    go drain(ch1)
}
`

const occupancyProgram = `
package main

func writer(out chan<- int32) {
    var v int32
    v = 5
    out <- v
}

func main() {
    ch0 := make(chan int32, 4)
    go writer(ch0)
}
`

func TestControlFlowMuxLowering(t *testing.T) {
	design := buildDesignFromSource(t, branchProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}
	muxCount := 0
	cmpCount := 0
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				switch op.(type) {
				case *MuxOperation:
					muxCount++
				case *CompareOperation:
					cmpCount++
				}
			}
		}
	}
	if muxCount == 0 {
		t.Fatalf("expected at least one mux operation in lowered IR")
	}
	if cmpCount == 0 {
		t.Fatalf("expected a compare operation for the branch predicate")
	}
}

func TestControlFlowBranchMetadata(t *testing.T) {
	design := buildDesignFromSource(t, branchProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}
	var branch *BranchTerminator
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			if term, ok := block.Terminator.(*BranchTerminator); ok {
				branch = term
				break
			}
		}
	}
	if branch == nil {
		t.Fatalf("expected a branch terminator in control-flow graph")
	}
	if branch.Cond == nil {
		t.Fatalf("branch terminator missing predicate signal")
	}
	if branch.True == nil || branch.False == nil {
		t.Fatalf("branch terminator missing successors")
	}
}

func TestSchedulerAssignsStages(t *testing.T) {
	design := buildDesignFromSource(t, pipelineProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}
	stageMap := make(map[string]int)
	for _, proc := range design.TopLevel.Processes {
		stageMap[proc.Name] = proc.Stage
	}
	if stageMap["main"] != 0 {
		t.Fatalf("expected main process stage 0, got %d", stageMap["main"])
	}
	sourceStage := stageMap["source"]
	middleStage := stageMap["middle"]
	drainStage := stageMap["drain"]
	if !(sourceStage < middleStage && middleStage < drainStage) {
		t.Fatalf("expected strictly increasing stages, got source=%d middle=%d drain=%d", sourceStage, middleStage, drainStage)
	}
}

func TestChannelOccupancyTracking(t *testing.T) {
	design := buildDesignFromSource(t, occupancyProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}
	foundNonZero := false
	for _, ch := range design.TopLevel.Channels {
		if ch.Occupancy > 0 {
			foundNonZero = true
			break
		}
	}
	if !foundNonZero {
		t.Fatalf("expected at least one channel to record non-zero occupancy")
	}
}

func buildDesignFromSource(t *testing.T, source string) *Design {
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
	design, err := BuildDesign(prog, reporter)
	if err != nil {
		t.Fatalf("build design: %v", err)
	}
	return design
}
