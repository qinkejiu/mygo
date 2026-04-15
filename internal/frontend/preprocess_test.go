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
