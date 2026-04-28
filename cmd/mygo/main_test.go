package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBenchmarkRefPathForInputsFindsHistoricalRef(t *testing.T) {
	root := t.TempDir()
	goPath := filepath.Join(root, "verilog-eval", "current_go_156", "Prob056_ece241_2013_q7", "main.go")
	refPath := filepath.Join(root, "verilog-eval", "historical", "dataset_spec-to-rtl", "refs", "Prob056_ece241_2013_q7_ref.sv")

	if err := os.MkdirAll(filepath.Dir(goPath), 0o755); err != nil {
		t.Fatalf("mkdir go path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatalf("mkdir ref path: %v", err)
	}
	if err := os.WriteFile(goPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("module RefModule; endmodule\n"), 0o644); err != nil {
		t.Fatalf("write ref file: %v", err)
	}

	got := benchmarkRefPathForInputs([]string{goPath})
	if got != refPath {
		t.Fatalf("benchmarkRefPathForInputs() = %q, want %q", got, refPath)
	}
}
