package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunSimMatchesExpectedTrace(t *testing.T) {
	requirePosix(t)
	tmp := t.TempDir()
	repo := repoRoot(t)

	translate := writeScript(t, tmp, "translate.sh", `#!/bin/sh
set -e
cat <<'EOS'
module main();
endmodule
module mygo_fifo_i32_d1();
endmodule
module mygo_fifo_i32_d4();
endmodule
module mygo_fifo_i1_d1();
endmodule
EOS
`)
	fifoSrc := writeFile(t, tmp, "fifos.sv", fifoLibrary())
	simulator := filepath.Join(repo, "scripts", "mock-sim.sh")
	trace := filepath.Join(repo, "test", "e2e", "pipeline1", "expected.sim")
	t.Setenv("MYGO_SIM_TRACE", trace)

	args := []string{
		"--circt-translate", translate,
		"--fifo-src", fifoSrc,
		"--simulator", simulator,
		filepath.Join(repo, "test", "e2e", "pipeline1", "main.go"),
	}
	if err := runSim(args); err != nil {
		t.Fatalf("runSim failed: %v", err)
	}
}

func TestRunSimDetectsMismatch(t *testing.T) {
	requirePosix(t)
	tmp := t.TempDir()
	repo := repoRoot(t)

	translate := writeScript(t, tmp, "translate.sh", `#!/bin/sh
set -e
cat <<'EOS'
module main();
endmodule
module mygo_fifo_i32_d1();
endmodule
module mygo_fifo_i32_d4();
endmodule
module mygo_fifo_i1_d1();
endmodule
EOS
`)
	fifoSrc := writeFile(t, tmp, "fifos.sv", fifoLibrary())
	simulator := filepath.Join(repo, "scripts", "mock-sim.sh")
	badTrace := writeFile(t, tmp, "bad.sim", "unexpected output\n")
	t.Setenv("MYGO_SIM_TRACE", badTrace)

	args := []string{
		"--circt-translate", translate,
		"--fifo-src", fifoSrc,
		"--simulator", simulator,
		filepath.Join(repo, "test", "e2e", "pipeline1", "main.go"),
	}
	err := runSim(args)
	if err == nil || !strings.Contains(err.Error(), "simulator output mismatch") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("determine repo root: %v", err)
	}
	return root
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
	return path
}

func fifoLibrary() string {
	return `module mygo_fifo_i32_d1();
endmodule
module mygo_fifo_i32_d4();
endmodule
module mygo_fifo_i1_d1();
endmodule
`
}

func requirePosix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tests require a POSIX shell")
	}
}
