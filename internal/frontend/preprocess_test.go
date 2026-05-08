package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreprocessSourcesForOverlayAddsBlankUseForUnusedLocal(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule(y [3]bool, w bool) {
	y0 := y[0]
	var next_y1 bool
	next_y1 = w && y0
	_ = y0
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if !strings.Contains(text, "_ = next_y1") {
		t.Fatalf("expected blank use for unused local, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayRewritesBoolToUint8Conversion(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule(in bool) uint8 {
	return (uint8(in) << 7)
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if !strings.Contains(text, "mygoBoolToUint8(in)") {
		t.Fatalf("expected bool conversion helper rewrite, got:\n%s", text)
	}
	if !strings.Contains(text, "func mygoBoolToUint8(v bool) uint8") {
		t.Fatalf("expected helper injection, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayRewritesClockShadowEdgeCondition(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var prev_clk bool

func TopModule(clk bool) {
	if !prev_clk && clk {
		println(1)
	}
	prev_clk = clk
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if strings.Contains(text, "!prev_clk && clk") {
		t.Fatalf("expected clock-shadow condition rewrite, got:\n%s", text)
	}
	if !strings.Contains(text, "if clk {") {
		t.Fatalf("expected rewritten clock guard, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayUnrollsConstForLoop(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule() {
	for i := 0; i < 4; i++ {
		println(i)
	}
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if strings.Contains(text, "for i := 0; i < 4; i++") {
		t.Fatalf("expected constant loop unrolling, got:\n%s", text)
	}
	for _, want := range []string{"println(0)", "println(1)", "println(2)", "println(3)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected unrolled iteration %q, got:\n%s", want, text)
		}
	}
}

func TestPreprocessSourcesForOverlayUnrollsNestedConstLoops(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule() {
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			println(i, j)
		}
	}
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if strings.Contains(text, "for i := 0; i < 2; i++") || strings.Contains(text, "for j := 0; j < 2; j++") {
		t.Fatalf("expected nested constant loops to be unrolled, got:\n%s", text)
	}
	for _, want := range []string{"println(0, 0", "println(0, 1", "println(1, 0", "println(1, 1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected nested unrolled iteration %q, got:\n%s", want, text)
		}
	}
}

func TestPreprocessSourcesForOverlayLeavesLargeConstLoopStructured(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule() {
	for i := 0; i < 100; i++ {
		println(i)
	}
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	if overlay != nil {
		text := string(overlay[file])
		if !strings.Contains(text, "for i := 0; i < 100; i++") {
			t.Fatalf("expected large constant loop to remain structured, got:\n%s", text)
		}
	}
}

func TestPreprocessSourcesForOverlayLeavesAssignedIndexLoopStructured(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule() {
	var i int
	for i = 0; i < 4; i++ {
		println(i)
	}
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	if overlay == nil {
		return
	}
	text := string(overlay[file])
	if !strings.Contains(text, "for i = 0; i < 4; i++") {
		t.Fatalf("expected assigned-index constant loop to remain structured, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayLeavesHighCostConstLoopStructured(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

func TopModule() {
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			println(i, j)
			println(i + j)
			println(i - j)
		}
	}
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	if overlay == nil {
		return
	}
	text := string(overlay[file])
	if !strings.Contains(text, "for i := 0; i < 16; i++") {
		t.Fatalf("expected high-cost outer loop to stay structured, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayPromotesClockedLocalState(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var out_anyedge uint8

func TopModule(clk bool, in uint8) {
	var d_last uint8
	if clk {
		out_anyedge = in ^ d_last
		d_last = in
	}
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if !strings.Contains(text, "var __mygo_state_TopModule_d_last uint8") {
		t.Fatalf("expected promoted state global, got:\n%s", text)
	}
	if strings.Contains(text, "var d_last uint8") {
		t.Fatalf("expected local state declaration to be removed, got:\n%s", text)
	}
	if !strings.Contains(text, "__mygo_state_TopModule_d_last = in") || !strings.Contains(text, "in ^ __mygo_state_TopModule_d_last") {
		t.Fatalf("expected state references to be rewritten, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayPromotesClockedStateWithLocalConstInit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var out_done bool

func TopModule(clk bool, reset bool) {
	const (
		S = iota
		S1
	)
	var state uint8 = S
	if clk {
		if reset {
			state = S
		} else {
			state = S1
		}
	}
	out_done = state == S1
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if !strings.Contains(text, "var __mygo_state_TopModule_state uint8 = 0") {
		t.Fatalf("expected promoted state init to inline local const value, got:\n%s", text)
	}
	if strings.Contains(text, "__mygo_state_TopModule_state uint8 = S") {
		t.Fatalf("promoted global must not retain function-local const refs, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayPromotesClockParamStateWithoutExplicitGuard(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var out_state uint8

func TopModule(clk bool, areset bool, train_valid bool, train_taken bool) {
	var currentState uint8 = 1
	if areset {
		currentState = 1
	} else if train_valid {
		if train_taken && currentState < 3 {
			currentState = currentState + 1
		} else if !train_taken && currentState > 0 {
			currentState = currentState - 1
		}
	}
	out_state = currentState
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if !strings.Contains(text, "var __mygo_state_TopModule_currentState uint8 = 1") {
		t.Fatalf("expected currentState promotion, got:\n%s", text)
	}
	if strings.Contains(text, "var currentState uint8 = 1") {
		t.Fatalf("expected local state declaration to be removed, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayLeavesCombinationalLocalAlone(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var out_q uint8

func TopModule(a uint8, b uint8) {
	var tmp uint8
	tmp = a ^ b
	out_q = tmp
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	if overlay != nil {
		text := string(overlay[file])
		if strings.Contains(text, "__mygo_state_") {
			t.Fatalf("did not expect combinational local promotion, got:\n%s", text)
		}
	}
}

func TestPreprocessSourcesForOverlayDoesNotPromoteInputDerivedTempsForClockParamFallback(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var out_Y0 bool

func TopModule(clk bool, x bool, y uint8) {
	y0 := (y & 0x1) != 0
	var nextY0 bool
	if !y0 {
		nextY0 = x
	}
	out_Y0 = nextY0
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	if overlay == nil {
		return
	}
	text := string(overlay[file])
	if strings.Contains(text, "__mygo_state_TopModule_y0") || strings.Contains(text, "__mygo_state_TopModule_nextY0") {
		t.Fatalf("did not expect input-derived combinational temps to be promoted, got:\n%s", text)
	}
}

func TestPreprocessSourcesForOverlayRewritesBooleanMuxAssign(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	source := `package main

var out_q bool

func TopModule(clk bool, qp bool, qn bool) {
	out_q = clk && qp || !clk && qn
}
`
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	overlay, err := preprocessSourcesForOverlay([]string{file})
	if err != nil {
		t.Fatalf("preprocess sources: %v", err)
	}
	text := string(overlay[file])
	if strings.Contains(text, "clk && qp || !clk && qn") {
		t.Fatalf("expected boolean mux assignment rewrite, got:\n%s", text)
	}
	if !strings.Contains(text, "if clk {") || !strings.Contains(text, "out_q = qp") || !strings.Contains(text, "out_q = qn") {
		t.Fatalf("expected explicit if/else mux rewrite, got:\n%s", text)
	}
}
