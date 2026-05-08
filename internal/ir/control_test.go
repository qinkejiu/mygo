package ir

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

const dynamicIndexProgram = `
package main

func sink(v int32) {}

func main() {
    var arr [4]int32
    var idx int
    var out int32
    arr[0] = 11
    arr[1] = 22
    arr[2] = 33
    idx = 0
    if arr[0] > 0 {
        idx = 1
    } else {
        idx = 2
    }
    out = arr[idx]
    sink(out)
}
`

const constEvalCallProgram = `
package main

var result int

func maxValue(a [8]int) int {
	i := 0
	max := a[0]
	for i < 8 {
		if a[i] > max {
			max = a[i]
		}
		i++
	}
	return max
}

func main() {
	a := [8]int{3, 7, 2, 9, 5, 8, 1, 4}
	result = maxValue(a)
}
`

const nestedGlobalArrayProgram = `
package main

var table = [2][3]int32{
	{1, 2, 3},
	{4, 5, 6},
}

var scratch [2][3]int32

func sink(v int32) {}

func main() {
	var row int
	var col int

	row = 1
	if table[0][0] > 0 {
		col = 2
	} else {
		col = 1
	}

	scratch[0][1] = table[row][col]
	sink(scratch[0][1])
}
`

const dynamicGlobalWordProgram = `
package main

var arr [2]int
var word [2][4]int

func initword() {
	for j := 0; j < 2; j++ {
		word[0][j] = 0x39 + j
		word[1][j] = 0x25 + j
	}
}

func main() {
	initword()
	for j := 0; j < 2; j++ {
		arr[j] = word[j][j]
	}
}
`

const sliceInlineProgram = `
package main

var result int

func bump(v []int) {
	v[1] = v[1] + 1
}

func update(a [2]int) ([2]int, int) {
	row := a[:]
	bump(row)
	return a, row[1]
}

func main() {
	arr := [2]int{1, 2}
	arr, result = update(arr)
	_ = arr
}
`

const nestedSliceInlineProgram = `
package main

var result int

func bump(v []int) {
	v[1] = v[1] + 1
}

func update(a [2][2]int, s int) ([2][2]int, int) {
	row := a[s][:]
	bump(row)
	return a, row[1]
}

func main() {
	arr := [2][2]int{{1, 2}, {3, 4}}
	arr, result = update(arr, 0)
	_ = arr
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

func TestDynamicIndexAddrLowering(t *testing.T) {
	design := buildDesignFromSource(t, dynamicIndexProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}
	idxCompareCount := 0
	idxMuxCount := 0
	idxExtractCount := 0
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				switch o := op.(type) {
				case *CompareOperation:
					if o.Dest != nil && strings.HasPrefix(o.Dest.Name, "idxeq_") {
						idxCompareCount++
					}
				case *MuxOperation:
					if o.Dest != nil && strings.HasPrefix(o.Dest.Name, "idxload_") {
						idxMuxCount++
					}
				case *ConvertOperation:
					if o.Dest != nil && strings.HasPrefix(o.Dest.Name, "idxextract_") {
						idxExtractCount++
					}
				}
			}
		}
	}
	if idxExtractCount == 0 && (idxCompareCount == 0 || idxMuxCount == 0) {
		t.Fatalf("expected dynamic index lowering ops, got idxeq=%d idxload=%d idxextract=%d", idxCompareCount, idxMuxCount, idxExtractCount)
	}
}

func TestConstEvalLoopCallFallback(t *testing.T) {
	design := buildDesignFromSource(t, constEvalCallProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}

	callCount := 0
	sawConstNineAssign := false

	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				switch o := op.(type) {
				case *CallOperation:
					callCount++
				case *AssignOperation:
					if o == nil || o.Value == nil || o.Value.Kind != Const {
						continue
					}
					switch v := o.Value.Value.(type) {
					case int:
						if v == 9 {
							sawConstNineAssign = true
						}
					case int64:
						if v == 9 {
							sawConstNineAssign = true
						}
					case uint64:
						if v == 9 {
							sawConstNineAssign = true
						}
					}
				}
			}
		}
	}

	if callCount != 0 {
		t.Fatalf("expected call to be eliminated, found %d call ops", callCount)
	}
	if !sawConstNineAssign {
		t.Fatalf("expected folded constant assignment of 9 for call result")
	}
}

func TestNestedGlobalArrayLowering(t *testing.T) {
	design := buildDesignFromSource(t, nestedGlobalArrayProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}

	for name, want := range map[string]int64{
		"table_0": 1,
		"table_1": 2,
		"table_2": 3,
		"table_3": 4,
		"table_4": 5,
		"table_5": 6,
	} {
		sig := design.TopLevel.Signals[name]
		if sig == nil {
			t.Fatalf("missing flattened signal %s", name)
		}
		got, ok := sig.Value.(int64)
		if !ok {
			t.Fatalf("signal %s init type = %T, want int64", name, sig.Value)
		}
		if got != want {
			t.Fatalf("signal %s init = %d, want %d", name, got, want)
		}
	}

	for _, name := range []string{"scratch_0", "scratch_1", "scratch_5"} {
		if design.TopLevel.Signals[name] == nil {
			t.Fatalf("missing flattened mutable signal %s", name)
		}
	}

	sawIndexedAdd := false
	sawIndexedLoad := false
	sawIndexedExtract := false
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				switch o := op.(type) {
				case *BinOperation:
					if o.Dest != nil && strings.HasPrefix(o.Dest.Name, "idxadd_") {
						sawIndexedAdd = true
					}
				case *MuxOperation:
					if o.Dest != nil && strings.HasPrefix(o.Dest.Name, "idxload_") {
						sawIndexedLoad = true
					}
				case *ConvertOperation:
					if o.Dest != nil && strings.HasPrefix(o.Dest.Name, "idxextract_") {
						sawIndexedExtract = true
					}
				}
			}
		}
	}
	if !sawIndexedAdd {
		t.Fatalf("expected flattened nested index arithmetic")
	}
	if !sawIndexedLoad && !sawIndexedExtract {
		t.Fatalf("expected flattened nested index load")
	}
}

func TestDynamicGlobalWordCallIsNotDropped(t *testing.T) {
	design := buildDesignFromSource(t, dynamicGlobalWordProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}

	sawWordAssign := false
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				assign, ok := op.(*AssignOperation)
				if !ok || assign == nil || assign.Dest == nil {
					continue
				}
				if assign.Dest.Name == "word_0" || assign.Dest.Name == "word_1" || assign.Dest.Name == "word_4" || assign.Dest.Name == "word_5" {
					sawWordAssign = true
				}
			}
		}
	}
	if !sawWordAssign {
		t.Fatalf("expected initword side effects to assign mutable word elements")
	}
}

func TestSliceInlineMultiResultCallAssignsResult(t *testing.T) {
	design := buildDesignFromSource(t, sliceInlineProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}

	sawResultAssign := false
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				assign, ok := op.(*AssignOperation)
				if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
					continue
				}
				if assign.Dest.Name == "result" {
					sawResultAssign = true
				}
			}
		}
	}

	if !sawResultAssign {
		t.Fatalf("expected slice-based multi-result inline call to assign result")
	}
}

func TestNestedSliceInlineMultiResultCallAssignsResult(t *testing.T) {
	design := buildDesignFromSource(t, nestedSliceInlineProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design")
	}

	sawResultAssign := false
	for _, proc := range design.TopLevel.Processes {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				assign, ok := op.(*AssignOperation)
				if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
					continue
				}
				if assign.Dest.Name == "result" {
					sawResultAssign = true
				}
			}
		}
	}

	if !sawResultAssign {
		t.Fatalf("expected nested slice-based multi-result inline call to assign result")
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
