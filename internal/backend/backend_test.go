package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mygo/internal/ir"
)

func TestEmitVerilogRunsTranslateOnly(t *testing.T) {
	requirePosix(t)

	design := testDesign()
	tmp := t.TempDir()

	translate := writeScript(t, tmp, "translate.sh", `#!/bin/sh
set -e
INPUT=""
for arg in "$@"; do
  case "$arg" in
    --export-verilog)
      ;;
    *)
      INPUT="$arg"
      ;;
  esac
done
if [ -z "$INPUT" ]; then
  INPUT="/dev/stdin"
fi
echo "// verilog translator"
cat "$INPUT"
`)

	out := filepath.Join(tmp, "out.sv")
	opts := Options{CIRCTTranslatePath: translate}
	if err := EmitVerilog(design, out, opts); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "// verilog translator") {
		t.Fatalf("expected translator banner, got:\n%s", data)
	}
}

func TestEmitVerilogRunsOptWhenPipelineProvided(t *testing.T) {
	requirePosix(t)

	design := testDesign()
	tmp := t.TempDir()

	opt := writeScript(t, tmp, "opt.sh", `#!/bin/sh
set -e
PIPELINE=""
OUT=""
IN=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --pass-pipeline=*)
      PIPELINE="${1#*=}"
      shift
      ;;
    -o)
      OUT="$2"
      shift 2
      ;;
    *)
      IN="$1"
      shift
      ;;
  esac
done
if [ -z "$OUT" ]; then
  echo "missing -o" >&2
  exit 1
fi
{
  echo "// opt:${PIPELINE}"
  cat "$IN"
} > "$OUT"
`)

	translate := writeScript(t, tmp, "translate.sh", `#!/bin/sh
set -e
INPUT=""
for arg in "$@"; do
  case "$arg" in
    --export-verilog)
      ;;
    *)
      INPUT="$arg"
      ;;
  esac
done
cat "$INPUT"
`)

	out := filepath.Join(tmp, "out.sv")
	opts := Options{
		CIRCTOptPath:       opt,
		CIRCTTranslatePath: translate,
		PassPipeline:       "pipeline-test",
	}
	if err := EmitVerilog(design, out, opts); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "// opt:pipeline-test") {
		t.Fatalf("expected circt-opt banner, got:\n%s", data)
	}
}

func TestEmitVerilogMissingTranslate(t *testing.T) {
	design := testDesign()
	opts := Options{CIRCTTranslatePath: filepath.Join(t.TempDir(), "missing")}
	err := EmitVerilog(design, "", opts)
	if err == nil {
		t.Fatalf("expected error when circt-translate is missing")
	}
}

func testDesign() *ir.Design {
	mod := &ir.Module{
		Name: "main",
		Ports: []ir.Port{
			{Name: "clk", Direction: ir.Input, Type: &ir.SignalType{Width: 1}},
			{Name: "rst", Direction: ir.Input, Type: &ir.SignalType{Width: 1}},
		},
		Signals:  map[string]*ir.Signal{},
		Channels: map[string]*ir.Channel{},
	}
	return &ir.Design{
		Modules:  []*ir.Module{mod},
		TopLevel: mod,
	}
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("tests require a POSIX shell")
	}
	return path
}

func requirePosix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tests require a POSIX shell")
	}
}
