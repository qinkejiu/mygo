package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mygo/internal/ir"
)

func TestEmitVerilogRunsExportVerilog(t *testing.T) {
	requirePosix(t)

	design := testDesign()
	tmp := t.TempDir()

	opt := writeFakeCirctOpt(t, tmp)

	out := filepath.Join(tmp, "out.sv")
	opts := Options{CIRCTOptPath: opt}
	res, err := EmitVerilog(design, out, opts)
	if err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	if res.MainPath != out {
		t.Fatalf("expected main path %s, got %s", out, res.MainPath)
	}
	if len(res.AuxPaths) != 0 {
		t.Fatalf("expected no aux files, got %v", res.AuxPaths)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "// circt-opt export") {
		t.Fatalf("expected circt-opt export banner, got:\n%s", data)
	}
}

func TestEmitVerilogRunsOptWhenPipelineProvided(t *testing.T) {
	requirePosix(t)

	design := testDesign()
	tmp := t.TempDir()

	opt := writeFakeCirctOpt(t, tmp)

	out := filepath.Join(tmp, "out.sv")
	opts := Options{
		CIRCTOptPath: opt,
		PassPipeline: "pipeline-test",
	}
	if _, err := EmitVerilog(design, out, opts); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "// pipeline:pipeline-test") {
		t.Fatalf("expected pipeline banner, got:\n%s", data)
	}
}

func TestEmitVerilogDumpsFinalMLIR(t *testing.T) {
	requirePosix(t)

	design := testDesign()
	tmp := t.TempDir()

	opt := writeFakeCirctOpt(t, tmp)
	dumpPath := filepath.Join(tmp, "mlir", "final.mlir")
	out := filepath.Join(tmp, "out.sv")
	opts := Options{
		CIRCTOptPath: opt,
		PassPipeline: "pipeline-test",
		DumpMLIRPath: dumpPath,
	}
	if _, err := EmitVerilog(design, out, opts); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read mlir dump: %v", err)
	}
	if !strings.Contains(string(data), "// opt:pipeline-test") {
		t.Fatalf("expected mlir dump to include opt output, got:\n%s", data)
	}
}

func TestEmitVerilogMissingCirctOpt(t *testing.T) {
	design := testDesign()
	opts := Options{CIRCTOptPath: filepath.Join(t.TempDir(), "missing")}
	out := filepath.Join(t.TempDir(), "out.sv")
	_, err := EmitVerilog(design, out, opts)
	if err == nil {
		t.Fatalf("expected error when circt-opt is missing")
	}
}

func TestEmitVerilogEmitsAuxiliaryFifoFile(t *testing.T) {
	requirePosix(t)
	design := testDesignWithChannel()
	tmp := t.TempDir()
	opt := writeExportVerilogScript(t, tmp, "circt-opt.sh", `module main();
endmodule
module mygo_fifo_i32_d1();
endmodule
`)
	fifoSrc := filepath.Join(tmp, "fifo_impl.sv")
	fifoBody := "// external fifo\nmodule mygo_fifo_i32_d1();\nendmodule\n"
	if err := os.WriteFile(fifoSrc, []byte(fifoBody), 0o644); err != nil {
		t.Fatalf("write fifo src: %v", err)
	}
	out := filepath.Join(tmp, "design.sv")
	res, err := EmitVerilog(design, out, Options{
		CIRCTOptPath: opt,
		FIFOSource:   fifoSrc,
	})
	if err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	if len(res.AuxPaths) != 1 {
		t.Fatalf("expected one aux file, got %v", res.AuxPaths)
	}
	mainData, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	if strings.Contains(string(mainData), "module mygo_fifo") {
		t.Fatalf("expected fifo module to be stripped:\n%s", string(mainData))
	}
	auxData, err := os.ReadFile(res.AuxPaths[0])
	if err != nil {
		t.Fatalf("read aux: %v", err)
	}
	if got := string(auxData); got != fifoBody {
		t.Fatalf("expected fifo implementation copy, got:\n%s", got)
	}
	expectedAux := strings.TrimSuffix(out, filepath.Ext(out)) + "_fifos.sv"
	if res.AuxPaths[0] != expectedAux {
		t.Fatalf("expected aux path %s, got %s", expectedAux, res.AuxPaths[0])
	}
	if _, err := os.Stat(expectedAux); err != nil {
		t.Fatalf("expected aux file to exist: %v", err)
	}
}

func TestEmitVerilogStripsAnnotatedFifoModules(t *testing.T) {
	requirePosix(t)
	design := testDesignWithChannel()
	tmp := t.TempDir()
	opt := writeExportVerilogScript(t, tmp, "circt-opt.sh", `module main();
endmodule : main
module helper();
endmodule // helper
module mygo_fifo_i32_d1();
endmodule : mygo_fifo_i32_d1
module sentinel();
endmodule // sentinel
`)
	fifoSrc := filepath.Join(tmp, "fifo_impl.sv")
	if err := os.WriteFile(fifoSrc, []byte("module mygo_fifo_i32_d1(); endmodule\n"), 0o644); err != nil {
		t.Fatalf("write fifo impl: %v", err)
	}
	out := filepath.Join(tmp, "design.sv")
	if _, err := EmitVerilog(design, out, Options{
		CIRCTOptPath: opt,
		FIFOSource:   fifoSrc,
	}); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read design: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "mygo_fifo_i32_d1") {
		t.Fatalf("expected fifo module to be stripped:\n%s", text)
	}
	if strings.Contains(text, ": mygo_fifo_i32_d1") || strings.Contains(text, "// mygo_fifo_i32_d1") {
		t.Fatalf("expected fifo annotations to be removed:\n%s", text)
	}
}

func TestEmitVerilogCopiesFifoDirectory(t *testing.T) {
	requirePosix(t)
	design := testDesignWithChannel()
	tmp := t.TempDir()
	opt := writeExportVerilogScript(t, tmp, "circt-opt.sh", `module main();
endmodule
module mygo_fifo_i32_d1();
endmodule
`)
	srcDir := filepath.Join(tmp, "fifo_lib")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir fifo dir: %v", err)
	}
	fileA := filepath.Join(srcDir, "fifo_a.sv")
	fileB := filepath.Join(srcDir, "helpers", "helper.sv")
	if err := os.MkdirAll(filepath.Dir(fileB), 0o755); err != nil {
		t.Fatalf("mkdir helper dir: %v", err)
	}
	if err := os.WriteFile(fileA, []byte("module mygo_fifo_i32_d1(); endmodule\n"), 0o644); err != nil {
		t.Fatalf("write fifo a: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("// helper content\n"), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	out := filepath.Join(tmp, "design.sv")
	res, err := EmitVerilog(design, out, Options{
		CIRCTOptPath: opt,
		FIFOSource:   srcDir,
	})
	if err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	if len(res.AuxPaths) != 2 {
		t.Fatalf("expected two aux files, got %v", res.AuxPaths)
	}
	for _, p := range res.AuxPaths {
		if !strings.HasPrefix(p, strings.TrimSuffix(out, filepath.Ext(out))+"_fifo_lib") {
			t.Fatalf("unexpected aux path %s", p)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected aux file %s to exist: %v", p, err)
		}
	}
}

func TestEmitVerilogErrorsWithoutFifoSource(t *testing.T) {
	requirePosix(t)
	design := testDesignWithChannel()
	tmp := t.TempDir()
	opt := writeExportVerilogScript(t, tmp, "circt-opt.sh", `module main();
endmodule
module mygo_fifo_i32_d1();
endmodule
`)
	out := filepath.Join(tmp, "design.sv")
	_, err := EmitVerilog(design, out, Options{CIRCTOptPath: opt})
	if err == nil || !strings.Contains(err.Error(), "fifo source") {
		t.Fatalf("expected fifo source error, got %v", err)
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

func testDesignWithChannel() *ir.Design {
	ch := &ir.Channel{
		Name:  "t0",
		Type:  &ir.SignalType{Width: 32},
		Depth: 1,
	}
	mod := &ir.Module{
		Name:      "main",
		Ports:     []ir.Port{{Name: "clk", Direction: ir.Input, Type: &ir.SignalType{Width: 1}}, {Name: "rst", Direction: ir.Input, Type: &ir.SignalType{Width: 1}}},
		Signals:   map[string]*ir.Signal{},
		Channels:  map[string]*ir.Channel{"t0": ch},
		Processes: []*ir.Process{},
	}
	return &ir.Design{
		Modules:  []*ir.Module{mod},
		TopLevel: mod,
	}
}

func writeFakeCirctOpt(t *testing.T, dir string) string {
	const script = `#!/bin/sh
set -e
PIPELINE=""
OUT=""
IN=""
EXPORT=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --pass-pipeline=*)
      PIPELINE="${1#*=}"
      shift
      ;;
    --export-verilog)
      EXPORT=1
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
if [ "$EXPORT" -eq 0 ]; then
  echo "missing --export-verilog" >&2
  exit 1
fi
if [ -z "$OUT" ]; then
  echo "missing -o" >&2
  exit 1
fi
if [ -z "$IN" ]; then
  IN="/dev/stdin"
fi
{
  echo "// opt:${PIPELINE}"
  cat "$IN"
} > "$OUT"
{
  echo "// circt-opt export"
  echo "// pipeline:${PIPELINE}"
  cat "$IN"
}
`
	return writeScript(t, dir, "circt-opt.sh", script)
}

func writeExportVerilogScript(t *testing.T, dir, name, verilogBody string) string {
	script := fmt.Sprintf(`#!/bin/sh
set -e
OUT=""
IN=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --pass-pipeline=*)
      shift
      ;;
    --export-verilog)
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
if [ -z "$IN" ]; then
  IN="/dev/stdin"
fi
cat "$IN" > "$OUT"
cat <<'__VERILOG__'
%s
__VERILOG__
`, verilogBody)
	return writeScript(t, dir, name, script)
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
