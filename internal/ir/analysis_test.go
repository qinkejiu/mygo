package ir

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"

	"mygo/internal/diag"
	"mygo/internal/frontend"
)

const repeatedGoroutineChannelsProgram = `
package main

func writer(out chan<- int32) {
	var v int32
	v = 1
	out <- v
}

func reader(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	ch0 := make(chan int32, 1)
	ch1 := make(chan int32, 1)
	go writer(ch0)
	go reader(ch0)
	go writer(ch1)
	go reader(ch1)
}
`

const sameGoroutineSPSCProgram = `
package main

func main() {
	ch := make(chan int32, 1)
	var v int32
	v = 7
	ch <- v
	v = <-ch
	_ = v
}
`

const multiProducerProgram = `
package main

func writer0(out chan<- int32) {
	var v int32
	v = 1
	out <- v
}

func writer1(out chan<- int32) {
	var v int32
	v = 2
	out <- v
}

func reader(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	ch := make(chan int32, 1)
	go writer0(ch)
	go writer1(ch)
	go reader(ch)
}
`

const variableLoopProgram = `
package main

func main() {
	var sum int32
	for i := int32(0); i < 4; i++ {
		sum = sum + i
	}
	_ = sum
}
`

const noLoopBranchProgram = `
package main

func main() {
	var a int32
	var b int32
	a = 1
	b = 2
	if a < b {
		a = a + 1
	} else {
		b = b + 1
	}
	_ = a
	_ = b
}
`

func TestChannelAddEndpointDeduplicates(t *testing.T) {
	ch := &Channel{Name: "ch"}
	proc := &Process{Name: "p"}

	ch.AddEndpoint(proc, ChannelSend)
	ch.AddEndpoint(proc, ChannelSend)
	ch.AddEndpoint(proc, ChannelReceive)
	ch.AddEndpoint(proc, ChannelReceive)

	if len(ch.Producers) != 1 {
		t.Fatalf("expected 1 producer endpoint, got %d", len(ch.Producers))
	}
	if len(ch.Consumers) != 1 {
		t.Fatalf("expected 1 consumer endpoint, got %d", len(ch.Consumers))
	}
}

func TestCallGraphChannelBindingsDependenciesAndDepth(t *testing.T) {
	design := buildDesignFromSource(t, repeatedGoroutineChannelsProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}

	checked := 0
	for _, ch := range design.TopLevel.Channels {
		if ch == nil || ch.DeclaredDepth != 1 {
			continue
		}
		checked++
		if !containsProcess(ch.Producers, "writer") {
			t.Fatalf("channel %s missing writer producer endpoint", ch.Name)
		}
		if !containsProcess(ch.Consumers, "reader") {
			t.Fatalf("channel %s missing reader consumer endpoint", ch.Name)
		}
		if len(ch.Dependencies) != 1 {
			t.Fatalf("channel %s expected exactly 1 dependency, got %d", ch.Name, len(ch.Dependencies))
		}
		dep := ch.Dependencies[0]
		if dep.Producer == nil || dep.Producer.Name != "writer" {
			t.Fatalf("channel %s unexpected dependency producer %v", ch.Name, dep.Producer)
		}
		if dep.Consumer == nil || dep.Consumer.Name != "reader" {
			t.Fatalf("channel %s unexpected dependency consumer %v", ch.Name, dep.Consumer)
		}
		if ch.Depth != 2 {
			t.Fatalf("channel %s expected inferred depth 2, got %d", ch.Name, ch.Depth)
		}
		if ch.InferredDepth != 2 {
			t.Fatalf("channel %s expected inferred depth metadata 2, got %d", ch.Name, ch.InferredDepth)
		}
		if !strings.Contains(ch.DepthReason, "SPSC cross-goroutine") {
			t.Fatalf("channel %s expected SPSC cross-goroutine reason, got %q", ch.Name, ch.DepthReason)
		}
	}

	if checked != 2 {
		t.Fatalf("expected 2 declared channels to validate, got %d", checked)
	}
}

func TestInferChannelDepthKeepsSameGoroutineSPSC(t *testing.T) {
	design := buildDesignFromSource(t, sameGoroutineSPSCProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}

	for _, ch := range design.TopLevel.Channels {
		if ch == nil {
			continue
		}
		if ch.DeclaredDepth != 1 {
			t.Fatalf("channel %s expected declared depth 1, got %d", ch.Name, ch.DeclaredDepth)
		}
		if ch.InferredDepth != 1 {
			t.Fatalf("channel %s expected inferred depth 1, got %d", ch.Name, ch.InferredDepth)
		}
		if ch.Depth != 1 {
			t.Fatalf("channel %s expected final depth 1, got %d", ch.Name, ch.Depth)
		}
		if !strings.Contains(ch.DepthReason, "same goroutine") {
			t.Fatalf("channel %s expected same-goroutine reason, got %q", ch.Name, ch.DepthReason)
		}
		return
	}
	t.Fatalf("expected at least one channel")
}

func TestInferChannelDepthBumpsMultiProducer(t *testing.T) {
	design := buildDesignFromSource(t, multiProducerProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}

	for _, ch := range design.TopLevel.Channels {
		if ch == nil {
			continue
		}
		if ch.DeclaredDepth != 1 {
			t.Fatalf("channel %s expected declared depth 1, got %d", ch.Name, ch.DeclaredDepth)
		}
		if ch.InferredDepth != 2 {
			t.Fatalf("channel %s expected inferred depth 2, got %d", ch.Name, ch.InferredDepth)
		}
		if ch.Depth != 2 {
			t.Fatalf("channel %s expected final depth 2, got %d", ch.Name, ch.Depth)
		}
		if !strings.Contains(ch.DepthReason, "Multi-producer") {
			t.Fatalf("channel %s expected multi-producer reason, got %q", ch.Name, ch.DepthReason)
		}
		return
	}
	t.Fatalf("expected at least one channel")
}

func TestFindLoopsIdentifiesLoopStructure(t *testing.T) {
	fn := buildSSAFunctionFromSource(t, variableLoopProgram, "main")
	loops := findLoops(fn)
	if len(loops) != 1 {
		t.Fatalf("expected one loop, got %d", len(loops))
	}

	loop := loops[0]
	if loop.Header == nil {
		t.Fatalf("expected loop header")
	}
	if len(loop.Body) == 0 {
		t.Fatalf("expected loop body blocks")
	}
	if len(loop.Latches) == 0 {
		t.Fatalf("expected at least one loop latch")
	}
	if len(loop.Exits) == 0 {
		t.Fatalf("expected at least one loop exit block")
	}
	if len(loop.ExitConditions) == 0 {
		t.Fatalf("expected at least one loop exit condition")
	}

	headerIf := ifTerminator(loop.Header)
	if headerIf == nil {
		t.Fatalf("expected loop header to terminate with *ssa.If")
	}

	foundHeaderCondition := false
	for _, cond := range loop.ExitConditions {
		if cond == headerIf {
			foundHeaderCondition = true
			break
		}
	}
	if !foundHeaderCondition {
		t.Fatalf("expected loop exit conditions to include the header condition")
	}

	for _, latch := range loop.Latches {
		if jumpTerminator(latch) == nil {
			t.Fatalf("expected latch block %d to end with *ssa.Jump", latch.Index)
		}
		hasBackEdge := false
		for _, succ := range latch.Succs {
			if succ == loop.Header {
				hasBackEdge = true
				break
			}
		}
		if !hasBackEdge {
			t.Fatalf("expected latch block %d to jump to header block %d", latch.Index, loop.Header.Index)
		}
	}
}

func TestFindLoopsSkipsNonLoopBranches(t *testing.T) {
	fn := buildSSAFunctionFromSource(t, noLoopBranchProgram, "main")
	loops := findLoops(fn)
	if len(loops) != 0 {
		t.Fatalf("expected no loops, got %d", len(loops))
	}
}

func TestBuildFSMForLoopCanonicalStatesAndTransitions(t *testing.T) {
	fn := buildSSAFunctionFromSource(t, variableLoopProgram, "main")
	loops := findLoops(fn)
	if len(loops) != 1 {
		t.Fatalf("expected one loop, got %d", len(loops))
	}
	b := &builder{}
	fsm := b.buildFSMForLoop(loops[0])
	if fsm == nil {
		t.Fatalf("expected non-nil loop fsm")
	}
	if len(fsm.states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(fsm.states))
	}
	if fsm.states[0].name != "CHECK" || fsm.states[1].name != "BODY" || fsm.states[2].name != "UPDATE" {
		t.Fatalf("unexpected state order: %q, %q, %q", fsm.states[0].name, fsm.states[1].name, fsm.states[2].name)
	}
	if len(fsm.transitions) != 4 {
		t.Fatalf("expected 4 transitions, got %d", len(fsm.transitions))
	}
	expectedTransitions := []fsmTransition{
		{from: "CHECK", to: "BODY", when: "true"},
		{from: "CHECK", to: "EXIT", when: "false"},
		{from: "BODY", to: "UPDATE", when: "always"},
		{from: "UPDATE", to: "CHECK", when: "always"},
	}
	for i, expected := range expectedTransitions {
		got := fsm.transitions[i]
		if got != expected {
			t.Fatalf("transition %d mismatch: got %+v want %+v", i, got, expected)
		}
	}
	if len(fsm.states[0].instrs) == 0 {
		t.Fatalf("expected CHECK state to have instructions")
	}
	if _, ok := fsm.states[0].instrs[len(fsm.states[0].instrs)-1].(*ssa.If); !ok {
		t.Fatalf("expected CHECK state to end with *ssa.If")
	}
	for _, instr := range fsm.states[1].instrs {
		switch instr.(type) {
		case *ssa.If, *ssa.Jump, *ssa.Return:
			t.Fatalf("BODY state should not contain terminators; got %T", instr)
		}
	}
	for _, instr := range fsm.states[2].instrs {
		switch instr.(type) {
		case *ssa.If, *ssa.Jump, *ssa.Return:
			t.Fatalf("UPDATE state should not contain terminators; got %T", instr)
		}
	}
}

func TestBuildLoopFSMsStoresDynamicLoopFSM(t *testing.T) {
	fn := buildSSAFunctionFromSource(t, variableLoopProgram, "main")
	b := &builder{}
	b.buildLoopFSMs(fn)
	fsms := b.loopFSMs[fn]
	if len(fsms) != 1 {
		t.Fatalf("expected one stored loop fsm, got %d", len(fsms))
	}
	if len(fsms[0].states) != 3 {
		t.Fatalf("expected canonical loop fsm states, got %d", len(fsms[0].states))
	}
}

func containsProcess(endpoints []*ChannelEndpoint, name string) bool {
	for _, endpoint := range endpoints {
		if endpoint != nil && endpoint.Process != nil && endpoint.Process.Name == name {
			return true
		}
	}
	return false
}

func buildSSAFunctionFromSource(t *testing.T, source, fnName string) *ssa.Function {
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
	mainPkg := findMainPackage(prog)
	if mainPkg == nil {
		t.Fatalf("expected main package")
	}
	fn := mainPkg.Func(fnName)
	if fn == nil {
		t.Fatalf("expected function %q", fnName)
	}
	return fn
}
