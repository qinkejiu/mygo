package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProgramsCompileToMLIR(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	testcases := []string{
		"simple",
		"type_mismatch",
		"channel_basic",
		"simple_branch",
		"pipeline1",
		"pipeline2",
		"router_csp",
	}
	for _, name := range testcases {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := filepath.Join("test", "e2e", name, "main.go")
			output := filepath.Join(t.TempDir(), name+".mlir")
			cmd := exec.Command("go", "run", "./cmd/mygo", "compile", "-emit=mlir", "-o", output, source)
			cmd.Dir = repoRoot
			cmd.Env = os.Environ()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("mygo compile %s failed: %v\n%s", name, err, string(out))
			}
			if _, err := os.Stat(output); err != nil {
				t.Fatalf("expected mlir output for %s: %v", name, err)
			}
			verifyGolden(t, name, output)
		})
	}
}

func verifyGolden(t *testing.T, name, actualPath string) {
	t.Helper()
	expectedPath := filepath.Join("test", "e2e", name, "expected.mlir")
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read expected mlir for %s: %v", name, err)
	}
	actual, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("read actual mlir for %s: %v", name, err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("mlir mismatch for %s\nexpected:\n%s\nactual:\n%s", name, expected, actual)
	}
}
