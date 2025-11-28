package backend

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mygo/internal/ir"
	"mygo/internal/mlir"
)

// Options configures how the CIRCT backend is invoked.
type Options struct {
	// CIRCTOptPath optionally overrides the circt-opt binary. When empty the
	// backend looks it up on PATH if needed.
	CIRCTOptPath string
	// CIRCTTranslatePath optionally overrides the circt-translate binary. When
	// empty the backend looks it up on PATH.
	CIRCTTranslatePath string
	// PassPipeline holds the circt-opt --pass-pipeline string (empty skips
	// circt-opt unless CIRCTOptPath is explicitly set).
	PassPipeline string
	// DumpMLIRPath writes the MLIR handed to CIRCT to the provided path when
	// non-empty.
	DumpMLIRPath string
	// KeepTemps preserves the intermediate directory on disk for debugging.
	KeepTemps bool
}

// EmitVerilog lowers the design to MLIR, optionally runs circt-opt, and invokes
// circt-translate --export-verilog to produce SystemVerilog at outputPath. When
// outputPath is empty or "-", stdout is used.
func EmitVerilog(design *ir.Design, outputPath string, opts Options) error {
	if design == nil {
		return fmt.Errorf("backend: design is nil")
	}

	fifoInfos := collectFifoDescriptors(design)

	translatePath, err := resolveBinary(opts.CIRCTTranslatePath, "circt-translate")
	if err != nil {
		return fmt.Errorf("backend: resolve circt-translate: %w", err)
	}

	var optPath string
	if needsOpt(opts) {
		if optPath, err = resolveBinary(opts.CIRCTOptPath, "circt-opt"); err != nil {
			return fmt.Errorf("backend: resolve circt-opt: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "mygo-circt-*")
	if err != nil {
		return fmt.Errorf("backend: create temp dir: %w", err)
	}
	if !opts.KeepTemps {
		defer os.RemoveAll(tempDir)
	}

	mlirPath := opts.DumpMLIRPath
	if mlirPath == "" {
		mlirPath = filepath.Join(tempDir, "design.mlir")
	} else if err := os.MkdirAll(filepath.Dir(mlirPath), 0o755); err != nil {
		return fmt.Errorf("backend: create circt-mlir dir: %w", err)
	}

	if err := mlir.Emit(design, mlirPath); err != nil {
		return fmt.Errorf("backend: emit mlir: %w", err)
	}

	currentInput := mlirPath
	if needsOpt(opts) {
		optOutput := filepath.Join(tempDir, "design.opt.mlir")
		if err := runCirctOpt(optPath, opts.PassPipeline, currentInput, optOutput); err != nil {
			return err
		}
		currentInput = optOutput
	}

	writeStdout := outputPath == "" || outputPath == "-"
	finalOutput := outputPath
	if writeStdout {
		finalOutput = filepath.Join(tempDir, "design.sv")
	}

	if err := runCirctTranslate(translatePath, currentInput, finalOutput); err != nil {
		return err
	}

	if err := injectFifoImplementations(finalOutput, fifoInfos); err != nil {
		return err
	}

	if writeStdout {
		data, err := os.ReadFile(finalOutput)
		if err != nil {
			return err
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func needsOpt(opts Options) bool {
	return opts.PassPipeline != "" || opts.CIRCTOptPath != ""
}

func runCirctOpt(binary, pipeline, inputPath, outputPath string) error {
	args := []string{inputPath, "-o", outputPath}
	if pipeline != "" {
		args = append(args, "--pass-pipeline="+pipeline)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backend: circt-opt failed: %w", err)
	}
	return nil
}

func runCirctTranslate(binary, inputPath, outputPath string) error {
	args := []string{"--export-verilog", inputPath}
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("backend: create verilog output dir: %w", err)
	}
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("backend: create verilog output file: %w", err)
	}
	defer outFile.Close()
	cmd.Stdout = outFile

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backend: circt-translate failed: %w", err)
	}
	return nil
}

func outputWriter(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("backend: create output dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("backend: create output file: %w", err)
	}
	return f, f.Close, nil
}

func resolveBinary(explicit, fallback string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	path, err := exec.LookPath(fallback)
	if err != nil {
		return "", err
	}
	return path, nil
}

type fifoDescriptor struct {
	name  string
	width int
	depth int
}

func collectFifoDescriptors(design *ir.Design) []fifoDescriptor {
	seen := make(map[string]fifoDescriptor)
	if design == nil {
		return nil
	}
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		for _, ch := range module.Channels {
			if ch == nil {
				continue
			}
			width := signalWidth(ch.Type)
			depth := ch.Depth
			if depth <= 0 {
				depth = 1
			}
			elem := typeString(ch.Type)
			name := fifoModuleName(elem, depth)
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = fifoDescriptor{
				name:  name,
				width: width,
				depth: depth,
			}
		}
	}
	result := make([]fifoDescriptor, 0, len(seen))
	for _, desc := range seen {
		result = append(result, desc)
	}
	return result
}

func injectFifoImplementations(path string, fifos []fifoDescriptor) error {
	if len(fifos) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("backend: read verilog output: %w", err)
	}
	content := string(data)
	for _, fifo := range fifos {
		replacement := fifoTemplate(fifo)
		var replErr error
		content, replErr = replaceModule(content, fifo.name, replacement)
		if replErr != nil {
			return replErr
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("backend: write patched verilog: %w", err)
	}
	return nil
}

func replaceModule(content, moduleName, replacement string) (string, error) {
	marker := "module " + moduleName
	start := strings.Index(content, marker)
	if start == -1 {
		return content, fmt.Errorf("backend: module %s not found in translated Verilog", moduleName)
	}
	tail := content[start:]
	endIdx := strings.Index(tail, "endmodule")
	if endIdx == -1 {
		return content, fmt.Errorf("backend: malformed module %s (missing endmodule)", moduleName)
	}
	endIdx += start + len("endmodule")
	// include trailing newline(s)
	for endIdx < len(content) && (content[endIdx] == '\n' || content[endIdx] == '\r') {
		endIdx++
	}
	var buf bytes.Buffer
	buf.WriteString(content[:start])
	buf.WriteString(replacement)
	if !strings.HasSuffix(replacement, "\n") {
		buf.WriteString("\n")
	}
	buf.WriteString(content[endIdx:])
	return buf.String(), nil
}

func fifoTemplate(desc fifoDescriptor) string {
	width := desc.width
	if width <= 0 {
		width = 1
	}
	depth := desc.depth
	if depth <= 0 {
		depth = 1
	}
	rangeStr := verilogRange(width)
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "module %s (\n", desc.name)
	fmt.Fprintf(builder, "  input wire clk,\n")
	fmt.Fprintf(builder, "  input wire rst,\n")
	fmt.Fprintf(builder, "  inout wire %sin_data,\n", rangeStr)
	fmt.Fprintf(builder, "  inout wire in_valid,\n")
	fmt.Fprintf(builder, "  inout wire in_ready,\n")
	fmt.Fprintf(builder, "  inout wire %sout_data,\n", rangeStr)
	fmt.Fprintf(builder, "  inout wire out_valid,\n")
	fmt.Fprintf(builder, "  inout wire out_ready\n")
	fmt.Fprintf(builder, ");\n")
	fmt.Fprintf(builder, "  localparam integer WIDTH = %d;\n", width)
	fmt.Fprintf(builder, "  localparam integer DEPTH = %d;\n", depth)
	fmt.Fprintf(builder, "  localparam integer ADDR_BITS = (DEPTH <= 1) ? 1 : $clog2(DEPTH);\n")
	fmt.Fprintf(builder, "  localparam integer COUNT_BITS = (DEPTH <= 1) ? 1 : $clog2(DEPTH + 1);\n")
	fmt.Fprintf(builder, "  reg [WIDTH-1:0] mem [0:DEPTH-1];\n")
	fmt.Fprintf(builder, "  reg [ADDR_BITS-1:0] wptr;\n")
	fmt.Fprintf(builder, "  reg [ADDR_BITS-1:0] rptr;\n")
	fmt.Fprintf(builder, "  reg [COUNT_BITS-1:0] count;\n")
	fmt.Fprintf(builder, "  wire ready_int;\n")
	fmt.Fprintf(builder, "  wire valid_int;\n")
	fmt.Fprintf(builder, "  wire push;\n")
	fmt.Fprintf(builder, "  wire pop;\n")
	fmt.Fprintf(builder, "  assign ready_int = (count < DEPTH);\n")
	fmt.Fprintf(builder, "  assign valid_int = (count != 0);\n")
	fmt.Fprintf(builder, "  assign in_ready = ready_int;\n")
	fmt.Fprintf(builder, "  assign out_valid = valid_int;\n")
	fmt.Fprintf(builder, "  assign out_data = mem[rptr];\n")
	fmt.Fprintf(builder, "  assign push = in_valid & ready_int;\n")
	fmt.Fprintf(builder, "  assign pop = valid_int & out_ready;\n")
	fmt.Fprintf(builder, "  always @(posedge clk) begin\n")
	fmt.Fprintf(builder, "    if (rst) begin\n")
	fmt.Fprintf(builder, "      wptr <= {ADDR_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "      rptr <= {ADDR_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "      count <= {COUNT_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "    end else begin\n")
	fmt.Fprintf(builder, "      if (push) begin\n")
	fmt.Fprintf(builder, "        mem[wptr] <= in_data;\n")
	fmt.Fprintf(builder, "        if (DEPTH == 1) begin\n")
	fmt.Fprintf(builder, "          wptr <= {ADDR_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "        end else if (wptr == DEPTH - 1) begin\n")
	fmt.Fprintf(builder, "          wptr <= {ADDR_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "        end else begin\n")
	fmt.Fprintf(builder, "          wptr <= wptr + 1'b1;\n")
	fmt.Fprintf(builder, "        end\n")
	fmt.Fprintf(builder, "      end\n")
	fmt.Fprintf(builder, "      if (pop) begin\n")
	fmt.Fprintf(builder, "        if (DEPTH == 1) begin\n")
	fmt.Fprintf(builder, "          rptr <= {ADDR_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "        end else if (rptr == DEPTH - 1) begin\n")
	fmt.Fprintf(builder, "          rptr <= {ADDR_BITS{1'b0}};\n")
	fmt.Fprintf(builder, "        end else begin\n")
	fmt.Fprintf(builder, "          rptr <= rptr + 1'b1;\n")
	fmt.Fprintf(builder, "        end\n")
	fmt.Fprintf(builder, "      end\n")
	fmt.Fprintf(builder, "      case ({push, pop})\n")
	fmt.Fprintf(builder, "        2'b10: count <= count + 1'b1;\n")
	fmt.Fprintf(builder, "        2'b01: count <= count - 1'b1;\n")
	fmt.Fprintf(builder, "        default: count <= count;\n")
	fmt.Fprintf(builder, "      endcase\n")
	fmt.Fprintf(builder, "    end\n")
	fmt.Fprintf(builder, "  end\n")
	fmt.Fprintf(builder, "endmodule\n")
	return builder.String()
}

func verilogRange(width int) string {
	if width <= 1 {
		return ""
	}
	return fmt.Sprintf("[%d:0] ", width-1)
}

func fifoModuleName(elemType string, depth int) string {
	return fmt.Sprintf("mygo.fifo_%s_d%d", sanitize(elemType), depth)
}

func signalWidth(t *ir.SignalType) int {
	if t == nil || t.Width <= 0 {
		return 1
	}
	return t.Width
}

func typeString(t *ir.SignalType) string {
	return fmt.Sprintf("i%d", signalWidth(t))
}

func sanitize(name string) string {
	if name == "" {
		return "unnamed"
	}
	var b strings.Builder
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (r >= '0' && r <= '9' && i > 0) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
