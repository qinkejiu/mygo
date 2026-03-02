package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mygo/internal/ir"
)

func TestEmitVerilogRunsExportVerilog(t *testing.T) {
	design := testDesign()
	tmp := t.TempDir()

	opt := touchFakeBinary(t, tmp)
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if binary != opt {
			return fmt.Errorf("unexpected binary %s", binary)
		}
		if pipeline != "" {
			return fmt.Errorf("expected empty pipeline, got %s", pipeline)
		}
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		return os.WriteFile(verilogOutputPath, []byte("// circt-opt export\n"), 0o644)
	})

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
	design := testDesign()
	tmp := t.TempDir()

	opt := touchFakeBinary(t, tmp)
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		if binary != opt {
			return fmt.Errorf("unexpected binary %s", binary)
		}
		if pipeline != "pipeline-test" {
			return fmt.Errorf("expected pipeline-test, got %s", pipeline)
		}
		content, err := os.ReadFile(inputPath)
		if err != nil {
			return err
		}
		prefixed := append([]byte("// pipeline:"+pipeline+"\n"), content...)
		return os.WriteFile(outputPath, prefixed, 0o644)
	})
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		return copyFile(inputPath, verilogOutputPath)
	})

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
	design := testDesign()
	tmp := t.TempDir()

	opt := touchFakeBinary(t, tmp)
	dumpPath := filepath.Join(tmp, "mlir", "final.mlir")
	out := filepath.Join(tmp, "out.sv")
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		content, err := os.ReadFile(inputPath)
		if err != nil {
			return err
		}
		prefixed := append([]byte("// opt:pipeline-test\n"), content...)
		return os.WriteFile(outputPath, prefixed, 0o644)
	})
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		return copyFile(inputPath, verilogOutputPath)
	})
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

func TestEmitVerilogInlinesGeneratedFIFO(t *testing.T) {
	design := testDesignWithChannel()
	tmp := t.TempDir()
	opt := touchFakeBinary(t, tmp)
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		return os.WriteFile(verilogOutputPath, []byte(readBackendTestdata(t, "design_inline_fifo.sv")), 0o644)
	})
	out := filepath.Join(tmp, "design.sv")
	res, err := EmitVerilog(design, out, Options{
		CIRCTOptPath: opt,
		FIFOSource:   filepath.Join(tmp, "missing_external_fifo.sv"),
	})
	if err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	if len(res.AuxPaths) != 0 {
		t.Fatalf("expected no aux files, got %v", res.AuxPaths)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "module mygo_fifo_i32_d1 (") {
		t.Fatalf("expected generated fifo module to be inlined:\n%s", text)
	}
	if strings.Contains(text, "module mygo_fifo_i32_d1();") {
		t.Fatalf("expected fifo stub to be replaced:\n%s", text)
	}
	if !strings.Contains(text, "wr_en") || !strings.Contains(text, "almost_full") {
		t.Fatalf("expected modern fifo ports in generated body:\n%s", text)
	}
}

func TestEmitVerilogReplacesAnnotatedFifoStubs(t *testing.T) {
	design := testDesignWithChannel()
	tmp := t.TempDir()
	opt := touchFakeBinary(t, tmp)
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		return os.WriteFile(verilogOutputPath, []byte(readBackendTestdata(t, "design_fifo_with_attrs.sv")), 0o644)
	})
	out := filepath.Join(tmp, "design.sv")
	if _, err := EmitVerilog(design, out, Options{
		CIRCTOptPath: opt,
	}); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read design: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "endmodule : mygo_fifo_i32_d1") {
		t.Fatalf("expected annotated fifo stub to be removed:\n%s", text)
	}
	if !strings.Contains(text, "module mygo_fifo_i32_d1 (") {
		t.Fatalf("expected generated fifo module to be present:\n%s", text)
	}
}

func TestGenerateFIFOVerilogSelectsImplementationStyle(t *testing.T) {
	shallow := GenerateFIFOVerilog("fifo_shallow", 32, 16, false, 0)
	if !strings.Contains(shallow, "Register-based circular buffer") {
		t.Fatalf("expected shallow fifo to use register-based style:\n%s", shallow)
	}
	if strings.Contains(shallow, "rd_data_reg") {
		t.Fatalf("shallow fifo unexpectedly used deep fifo read register:\n%s", shallow)
	}
	if !strings.Contains(shallow, "localparam integer ALMOST_FULL_LEVEL = 15;") {
		t.Fatalf("expected default almost-full level to clamp to depth-1:\n%s", shallow)
	}

	deep := GenerateFIFOVerilog("fifo_deep", 8, 256, true, 300)
	if !strings.Contains(deep, "RAM-oriented style for deeper FIFOs.") {
		t.Fatalf("expected deep fifo RAM-oriented style:\n%s", deep)
	}
	if !strings.Contains(deep, "rd_data_reg") {
		t.Fatalf("expected deep fifo registered read datapath:\n%s", deep)
	}
	if !strings.Contains(deep, "always @(posedge clk or negedge rst_n)") {
		t.Fatalf("expected async reset sensitivity list:\n%s", deep)
	}
	if !strings.Contains(deep, "localparam integer ALMOST_FULL_LEVEL = 256;") {
		t.Fatalf("expected almost-full level to clamp to depth:\n%s", deep)
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

func stubRunPipeline(t *testing.T, fn func(binary, pipeline, inputPath, outputPath string) error) {
	t.Helper()
	prev := runPipeline
	runPipeline = fn
	t.Cleanup(func() { runPipeline = prev })
}

func stubRunExport(t *testing.T, fn func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error) {
	t.Helper()
	prev := runExport
	runExport = fn
	t.Cleanup(func() { runExport = prev })
}

func touchFakeBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "circt-opt")
	if err := os.WriteFile(path, []byte{}, 0o755); err != nil {
		t.Fatalf("touch binary: %v", err)
	}
	return path
}

func backendTestdataPath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("testdata %s: %v", name, err)
	}
	return path
}

func readBackendTestdata(t *testing.T, name string) string {
	t.Helper()
	path := backendTestdataPath(t, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return string(data)
}
