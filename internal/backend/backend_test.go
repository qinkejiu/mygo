package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mygo/internal/diag"
	"mygo/internal/frontend"
	"mygo/internal/ir"
	"mygo/internal/passes"
)

func TestEmitVerilogRunsExportVerilog(t *testing.T) {
	design := testDesign()
	tmp := t.TempDir()

	opt := touchFakeBinary(t, tmp)
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		if binary != opt {
			return fmt.Errorf("unexpected binary %s", binary)
		}
		if pipeline != defaultVerilogPassPipeline {
			return fmt.Errorf("expected default pipeline %s, got %s", defaultVerilogPassPipeline, pipeline)
		}
		return copyFile(inputPath, outputPath)
	})
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
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		if pipeline != defaultVerilogPassPipeline {
			return fmt.Errorf("expected default pipeline %s, got %s", defaultVerilogPassPipeline, pipeline)
		}
		return copyFile(inputPath, outputPath)
	})
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
	if !strings.Contains(text, "module mygo_fifo #(") {
		t.Fatalf("expected shared parametric fifo module to be inlined:\n%s", text)
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
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		if pipeline != defaultVerilogPassPipeline {
			return fmt.Errorf("expected default pipeline %s, got %s", defaultVerilogPassPipeline, pipeline)
		}
		return copyFile(inputPath, outputPath)
	})
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
	if !strings.Contains(text, "module mygo_fifo #(") {
		t.Fatalf("expected shared parametric fifo module to be present:\n%s", text)
	}
}

func TestGenerateReusableParametricFIFOVerilog(t *testing.T) {
	text := GenerateReusableParametricFIFOVerilog("mygo_fifo")
	if !strings.Contains(text, "parameter integer DATA_WIDTH = 32") {
		t.Fatalf("expected reusable fifo parameters:\n%s", text)
	}
	if !strings.Contains(text, "parameter bit USE_REGISTERED_READ = 1'b0") {
		t.Fatalf("expected reusable fifo to accept precomputed read-path policy:\n%s", text)
	}
	if !strings.Contains(text, "if (USE_REGISTERED_READ != 1'b0) begin : gen_registered_read") {
		t.Fatalf("expected reusable fifo generate split to depend on Go-computed policy:\n%s", text)
	}
	if !strings.Contains(text, "if (ASYNC_RESET != 1'b0) begin : gen_async_reset") {
		t.Fatalf("expected reusable fifo reset-policy generate split:\n%s", text)
	}
	if strings.Contains(text, "$clog2") {
		t.Fatalf("expected reusable fifo to avoid deriving widths in Verilog:\n%s", text)
	}
}

func TestRewriteFIFOStubInstances(t *testing.T) {
	content := "  mygo_fifo_i32_d4 t0_fifo (\n    .clk(clk)\n  );\n"
	decl := &ir.FIFODecl{
		ModuleName:            "mygo_fifo_i32_d4",
		ReusableModuleName:    "mygo_fifo",
		DataWidth:             32,
		Depth:                 4,
		AddrWidth:             2,
		CountWidth:            3,
		LastPtrValue:          3,
		DepthCountValue:       4,
		AlmostFullLevel:       3,
		AlmostEmptyLevel:      1,
		AlmostFullCountValue:  3,
		AlmostEmptyCountValue: 1,
	}
	rewritten, err := rewriteFIFOStubInstances(content, decl)
	if err != nil {
		t.Fatalf("rewriteFIFOStubInstances() error: %v", err)
	}
	if !strings.Contains(rewritten, "mygo_fifo #(.DATA_WIDTH(32), .DEPTH(4), .ADDR_WIDTH(2), .COUNT_WIDTH(3), .LAST_PTR_VALUE(3), .DEPTH_COUNT_VALUE(4), .ALMOST_FULL_LEVEL(3), .ALMOST_EMPTY_LEVEL(1), .ALMOST_FULL_COUNT_VALUE(3), .ALMOST_EMPTY_COUNT_VALUE(1), .USE_REGISTERED_READ(0), .ALMOST_EMPTY_USES_EMPTY(0), .ASYNC_RESET(0)) t0_fifo (") {
		t.Fatalf("expected parametric fifo instance rewrite, got:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "mygo_fifo_i32_d4 t0_fifo") {
		t.Fatalf("expected shape-specific fifo instance name to be replaced, got:\n%s", rewritten)
	}
}

func TestBuildBenchmarkWrapperBindsOutPrefixedImplementationOutputs(t *testing.T) {
	expected := []verilogPort{
		{Name: "zero", Direction: "output", Width: 1},
		{Name: "out", Direction: "output", Width: 1},
		{Name: "z", Direction: "output", Width: 1},
	}
	actual := []verilogPort{
		{Name: "out_zero", Direction: "output", Width: 1},
		{Name: "out_out", Direction: "output", Width: 1},
		{Name: "out_z", Direction: "output", Width: 1},
	}
	wrapper, err := buildBenchmarkWrapper("TopModule", "TopModule__impl", expected, actual)
	if err != nil {
		t.Fatalf("buildBenchmarkWrapper() error: %v", err)
	}
	for _, want := range []string{
		".out_zero(zero)",
		".out_out(out)",
		".out_z(z)",
	} {
		if !strings.Contains(wrapper, want) {
			t.Fatalf("expected wrapper to contain %q, got:\n%s", want, wrapper)
		}
	}
}

func TestBuildBenchmarkWrapperBindsPackedOutPrefixedImplementationOutputs(t *testing.T) {
	expected := []verilogPort{
		{Name: "g", Direction: "output", Width: 3},
	}
	actual := []verilogPort{
		{Name: "out_g0", Direction: "output", Width: 1},
		{Name: "out_g1", Direction: "output", Width: 1},
		{Name: "out_g2", Direction: "output", Width: 1},
	}
	wrapper, err := buildBenchmarkWrapper("TopModule", "TopModule__impl", expected, actual)
	if err != nil {
		t.Fatalf("buildBenchmarkWrapper() error: %v", err)
	}
	for _, want := range []string{
		".out_g0(__mygo_out_g0)",
		".out_g1(__mygo_out_g1)",
		".out_g2(__mygo_out_g2)",
		"assign g[0] = __mygo_out_g0;",
		"assign g[1] = __mygo_out_g1;",
		"assign g[2] = __mygo_out_g2;",
	} {
		if !strings.Contains(wrapper, want) {
			t.Fatalf("expected wrapper to contain %q, got:\n%s", want, wrapper)
		}
	}
}

func TestGenerateFIFOVerilogSelectsImplementationStyle(t *testing.T) {
	shallow := GenerateFIFOVerilog("fifo_shallow", 32, 16, false, 0)
	if !strings.Contains(shallow, "localparam integer ALMOST_FULL_LEVEL = 15;") {
		t.Fatalf("expected default almost-full level to clamp to depth-1:\n%s", shallow)
	}
	if !strings.Contains(shallow, "localparam integer USE_REGISTERED_READ = 0;") {
		t.Fatalf("expected shallow fifo to precompute direct-read policy:\n%s", shallow)
	}
	if !strings.Contains(shallow, "mygo_fifo #(") {
		t.Fatalf("expected concrete fifo wrapper to bind the reusable fifo module:\n%s", shallow)
	}

	deep := GenerateFIFOVerilog("fifo_deep", 8, 256, true, 300)
	if !strings.Contains(deep, "localparam integer ALMOST_FULL_LEVEL = 256;") {
		t.Fatalf("expected almost-full level to clamp to depth:\n%s", deep)
	}
	if !strings.Contains(deep, "localparam integer USE_REGISTERED_READ = 1;") {
		t.Fatalf("expected deep fifo to precompute registered-read policy:\n%s", deep)
	}
	if !strings.Contains(deep, "localparam integer ASYNC_RESET = 1;") {
		t.Fatalf("expected deep fifo wrapper to precompute async reset policy:\n%s", deep)
	}
}

func TestEmitVerilogGeneratesLoopFSMFallback(t *testing.T) {
	design := testDesignWithDynamicLoopProcess()
	tmp := t.TempDir()
	opt := touchFakeBinary(t, tmp)
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		if pipeline != defaultVerilogPassPipeline {
			return fmt.Errorf("expected default pipeline %s, got %s", defaultVerilogPassPipeline, pipeline)
		}
		return copyFile(inputPath, outputPath)
	})
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		const verilog = `module main(
  input clk,
        rst
);
endmodule

module main__proc_worker(
  input clk,
        rst
);
endmodule
`
		return os.WriteFile(verilogOutputPath, []byte(verilog), 0o644)
	})

	out := filepath.Join(tmp, "fsm.sv")
	if _, err := EmitVerilog(design, out, Options{CIRCTOptPath: opt}); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "module main__proc_worker__loop0_fsm(") {
		t.Fatalf("expected loop fsm fallback module in output:\n%s", text)
	}
	for _, fragment := range []string{
		"STATE_CHECK",
		"STATE_BODY",
		"STATE_UPDATE",
		"STATE_EXIT",
		"if (check_cond)",
		"next_state = STATE_BODY;",
		"next_state = STATE_EXIT;",
		"next_state = STATE_UPDATE;",
		"next_state = STATE_CHECK;",
		"(* fsm_encoding = \"sequential\" *)",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected fragment %q in fsm fallback output:\n%s", fragment, text)
		}
	}
}

func TestEmitVerilogSkipsLoopFSMFallbackWhenFSMAlreadyPresent(t *testing.T) {
	design := testDesignWithDynamicLoopProcess()
	tmp := t.TempDir()
	opt := touchFakeBinary(t, tmp)
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		if pipeline != defaultVerilogPassPipeline {
			return fmt.Errorf("expected default pipeline %s, got %s", defaultVerilogPassPipeline, pipeline)
		}
		return copyFile(inputPath, outputPath)
	})
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		const verilog = `module main__proc_worker(
  input clk,
        rst
);
  reg [1:0] state_reg0;
  always @(posedge clk) begin
    case (state_reg0)
      2'b00: state_reg0 <= 2'b01;
      default: state_reg0 <= state_reg0;
    endcase
  end
endmodule
`
		return os.WriteFile(verilogOutputPath, []byte(verilog), 0o644)
	})

	out := filepath.Join(tmp, "fsm_existing.sv")
	if _, err := EmitVerilog(design, out, Options{CIRCTOptPath: opt}); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "module main__proc_worker__loop0_fsm(") {
		t.Fatalf("did not expect loop fsm fallback when FSM already exists:\n%s", text)
	}
}

func TestEmitVerilogHandlesIndexedLookupProgram(t *testing.T) {
	const source = `
package main

var out_q [16]bool

func TopModule(a [3]bool) {
	var a_val uint8 = 0
	if a[0] {
		a_val |= 1 << 0
	}
	if a[1] {
		a_val |= 1 << 1
	}
	if a[2] {
		a_val |= 1 << 2
	}

	var lut [8][16]bool = [8][16]bool{
		{false, false, false, true, false, false, true, true, false, false, true, false, false, false, true, false},
		{true, false, true, false, true, true, true, false, true, true, true, false, false, false, false, false},
		{false, false, true, false, false, true, true, true, true, true, false, true, false, true, false, false},
		{false, true, true, true, false, true, false, true, false, false, false, false, true, false, true, false},
		{false, false, true, false, false, false, false, false, false, true, true, false, false, true, true, false},
		{false, true, true, false, false, true, false, false, true, true, false, false, true, true, true, false},
		{true, true, false, false, false, true, false, true, false, false, true, false, false, true, true, false},
		{true, false, false, true, true, true, true, true, false, false, false, true, false, false, false, false},
	}

	for i := 0; i < 16; i++ {
		out_q[i] = lut[a_val][i]
	}
}
`
	design := buildBackendDesignFromSource(t, source, "TopModule")
	tmp := t.TempDir()
	opt := touchFakeBinary(t, tmp)
	stubRunPipeline(t, func(binary, pipeline, inputPath, outputPath string) error {
		return copyFile(inputPath, outputPath)
	})
	stubRunExport(t, func(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
		if err := copyFile(inputPath, mlirOutputPath); err != nil {
			return err
		}
		return os.WriteFile(verilogOutputPath, []byte("// indexed lookup export\n"), 0o644)
	})
	out := filepath.Join(tmp, "indexed.sv")
	if _, err := EmitVerilog(design, out, Options{CIRCTOptPath: opt}); err != nil {
		t.Fatalf("EmitVerilog failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "// indexed lookup export") {
		t.Fatalf("expected stub export output, got:\n%s", data)
	}
}

func TestStripUnsupportedAutomaticLifetimeHoistsDeclsToModuleScope(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "auto.sv")
	const src = `module TopModule(
  input clk
);
  always @(posedge clk) begin
    out_q <= 1'b0;
    automatic logic [7:0] _GEN_0 = in_data;
    automatic logic [7:0] _GEN_1 =
      in_data + 8'h1; // keep comment
    out_q <= _GEN_0[0];
    out_q <= _GEN_1[0];
  end
endmodule
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := stripUnsupportedAutomaticLifetime(path); err != nil {
		t.Fatalf("stripUnsupportedAutomaticLifetime() error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "automatic logic") {
		t.Fatalf("expected automatic lifetimes to be removed, got:\n%s", text)
	}
	if !strings.Contains(text, "logic [7:0] _GEN_0;") || !strings.Contains(text, "logic [7:0] _GEN_1;") {
		t.Fatalf("expected hoisted module declarations, got:\n%s", text)
	}
	if !strings.Contains(text, "_GEN_0 = in_data;") || !strings.Contains(text, "_GEN_1 =\n      in_data + 8'h1; // keep comment") {
		t.Fatalf("expected in-block assignments to remain, got:\n%s", text)
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

func buildBackendDesignFromSource(t *testing.T, source string, target string) *ir.Design {
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
	design, err := ir.BuildDesign(prog, reporter, target)
	if err != nil {
		t.Fatalf("build design: %v", err)
	}
	passMgr := passes.NewManager()
	passMgr.Add(passes.NewWidthInference(reporter))
	if err := passMgr.Run(design); err != nil {
		t.Fatalf("run passes: %v", err)
	}
	return design
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

func testDesignWithDynamicLoopProcess() *ir.Design {
	cond := &ir.Signal{Name: "loop_cond", Type: &ir.SignalType{Width: 1}, Kind: ir.Wire}

	entry := &ir.BasicBlock{Label: "entry"}
	check := &ir.BasicBlock{Label: "check"}
	body := &ir.BasicBlock{Label: "body"}
	update := &ir.BasicBlock{Label: "update"}
	exit := &ir.BasicBlock{Label: "exit"}

	entry.Terminator = &ir.JumpTerminator{Target: check}
	check.Terminator = &ir.BranchTerminator{Cond: cond, True: body, False: exit}
	body.Terminator = &ir.JumpTerminator{Target: update}
	update.Terminator = &ir.JumpTerminator{Target: check}
	exit.Terminator = &ir.ReturnTerminator{}

	entry.Successors = []*ir.BasicBlock{check}
	check.Predecessors = []*ir.BasicBlock{entry, update}
	check.Successors = []*ir.BasicBlock{body, exit}
	body.Predecessors = []*ir.BasicBlock{check}
	body.Successors = []*ir.BasicBlock{update}
	update.Predecessors = []*ir.BasicBlock{body}
	update.Successors = []*ir.BasicBlock{check}
	exit.Predecessors = []*ir.BasicBlock{check}

	proc := &ir.Process{
		Name:        "worker",
		Sensitivity: ir.Sequential,
		Blocks:      []*ir.BasicBlock{entry, check, body, update, exit},
	}

	mod := &ir.Module{
		Name: "main",
		Ports: []ir.Port{
			{Name: "clk", Direction: ir.Input, Type: &ir.SignalType{Width: 1}},
			{Name: "rst", Direction: ir.Input, Type: &ir.SignalType{Width: 1}},
		},
		Signals:   map[string]*ir.Signal{"loop_cond": cond},
		Channels:  map[string]*ir.Channel{},
		Processes: []*ir.Process{proc},
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
