package backend

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"mygo/internal/ir"
	"mygo/internal/mlir"
)

var (
	runPipeline = runCirctPipeline
	runExport   = runCirctExportVerilog
)

// Options configures how the CIRCT backend is invoked.
type Options struct {
	// CIRCTOptPath optionally overrides the circt-opt binary. When empty the
	// backend looks it up on PATH.
	CIRCTOptPath string
	// PassPipeline holds the circt-opt --pass-pipeline string that runs before
	// --export-verilog.
	PassPipeline string
	// LoweringOptions holds the comma-separated string passed to
	// --lowering-options to control ExportVerilog lowering behavior.
	LoweringOptions string
	// DumpMLIRPath writes the MLIR handed to CIRCT to the provided path when
	// non-empty.
	DumpMLIRPath string
	// KeepTemps preserves the intermediate directory on disk for debugging.
	KeepTemps bool
	// TempRoot, when non-empty, scopes backend temp dirs under the provided
	// path instead of the system temp location.
	TempRoot string
	// FIFOSource is deprecated. FIFO implementations are now generated inline.
	FIFOSource string
}

// Result lists the artifacts produced during Verilog emission.
type Result struct {
	MainPath string
	AuxPaths []string
}

// EmitVerilog lowers the design to MLIR, runs circt-opt (optionally with a pass
// pipeline) and invokes --export-verilog to produce SystemVerilog at
// outputPath.
func EmitVerilog(design *ir.Design, outputPath string, opts Options) (Result, error) {
	if design == nil {
		return Result{}, fmt.Errorf("backend: design is nil")
	}
	if outputPath == "" || outputPath == "-" {
		return Result{}, fmt.Errorf("backend: verilog emission requires -o")
	}

	fifoInfos := collectFifoDescriptors(design)

	optPath, err := resolveBinary(opts.CIRCTOptPath, "circt-opt")
	if err != nil {
		return Result{}, fmt.Errorf("backend: resolve circt-opt: %w", err)
	}

	tempDir, err := os.MkdirTemp(opts.TempRoot, ".mygo-circt-*")
	if err != nil {
		return Result{}, fmt.Errorf("backend: create temp dir: %w", err)
	}
	if !opts.KeepTemps {
		defer os.RemoveAll(tempDir)
	}

	mlirPath := filepath.Join(tempDir, "design.mlir")
	if err := mlir.Emit(design, mlirPath); err != nil {
		return Result{}, fmt.Errorf("backend: emit mlir: %w", err)
	}

	currentInput := mlirPath
	if opts.PassPipeline != "" {
		pipelineOutput := filepath.Join(tempDir, "design.pipeline.mlir")
		if err := runPipeline(optPath, opts.PassPipeline, currentInput, pipelineOutput); err != nil {
			return Result{}, err
		}
		currentInput = pipelineOutput
	}
	exportOutput := filepath.Join(tempDir, "design.export.mlir")
	if err := runExport(optPath, "", opts.LoweringOptions, currentInput, exportOutput, outputPath); err != nil {
		return Result{}, err
	}
	currentInput = exportOutput

	if opts.DumpMLIRPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.DumpMLIRPath), 0o755); err != nil {
			return Result{}, fmt.Errorf("backend: create circt-mlir dir: %w", err)
		}
		if err := copyFile(currentInput, opts.DumpMLIRPath); err != nil {
			return Result{}, fmt.Errorf("backend: dump mlir: %w", err)
		}
	}

	if err := inlineGeneratedFifos(outputPath, fifoInfos); err != nil {
		return Result{}, err
	}
	if err := applySignedVerilog(design, outputPath); err != nil {
		return Result{}, err
	}
	return Result{MainPath: outputPath}, nil
}

func runCirctExportVerilog(binary, pipeline, loweringOptions, inputPath, mlirOutputPath, verilogOutputPath string) error {
	args := []string{inputPath, "-o", mlirOutputPath}
	if loweringOptions != "" {
		args = append(args, "--test-apply-lowering-options=options="+loweringOptions)
	}
	args = append(args, "--export-verilog")
	if pipeline != "" {
		args = append(args, "--pass-pipeline="+pipeline)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr

	if err := os.MkdirAll(filepath.Dir(mlirOutputPath), 0o755); err != nil {
		return fmt.Errorf("backend: create circt-opt output dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(verilogOutputPath), 0o755); err != nil {
		return fmt.Errorf("backend: create verilog output dir: %w", err)
	}
	outFile, err := os.Create(verilogOutputPath)
	if err != nil {
		return fmt.Errorf("backend: create verilog output file: %w", err)
	}
	defer outFile.Close()
	cmd.Stdout = outFile

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backend: circt-opt --export-verilog failed: %w", err)
	}
	return nil
}

func runCirctPipeline(binary, pipeline, inputPath, outputPath string) error {
	args := []string{inputPath, "-o", outputPath, "--pass-pipeline=" + pipeline}
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backend: circt-opt --pass-pipeline failed: %w", err)
	}
	return nil
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
	name            string
	width           int
	depth           int
	isAsyncReset    bool
	almostFullLevel int
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
			elem := signalTypeString(ch.Type)
			name := fifoModuleName(elem, depth)
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = fifoDescriptor{
				name:            name,
				width:           width,
				depth:           depth,
				isAsyncReset:    false,
				almostFullLevel: defaultAlmostFullLevel(depth),
			}
		}
	}
	result := make([]fifoDescriptor, 0, len(seen))
	for _, desc := range seen {
		result = append(result, desc)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].name < result[j].name
	})
	return result
}

func inlineGeneratedFifos(mainPath string, fifos []fifoDescriptor) error {
	if len(fifos) == 0 {
		return nil
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("backend: read verilog output: %w", err)
	}
	updated := string(data)
	generated := make([]string, 0, len(fifos))
	for _, fifo := range fifos {
		var ok bool
		updated, ok = removeModuleBlock(updated, fifo.name)
		if !ok {
			return fmt.Errorf("backend: module %s not found in generated Verilog", fifo.name)
		}
		generated = append(generated, GenerateFIFOVerilog(
			fifo.name,
			fifo.width,
			fifo.depth,
			fifo.isAsyncReset,
			fifo.almostFullLevel,
		))
	}
	if len(generated) > 0 {
		if !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += "\n" + strings.Join(generated, "\n\n") + "\n"
	}
	if err := os.WriteFile(mainPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("backend: update main verilog: %w", err)
	}
	return nil
}

func removeModuleBlock(content, moduleName string) (string, bool) {
	marker := "module " + moduleName
	start := strings.Index(content, marker)
	if start == -1 {
		return content, false
	}
	tail := content[start:]
	endIdx := strings.Index(tail, "endmodule")
	if endIdx == -1 {
		return content, false
	}
	end := start + endIdx + len("endmodule")
	for end < len(content) && content[end] != '\n' && content[end] != '\r' {
		end++
	}
	for end < len(content) && (content[end] == '\n' || content[end] == '\r') {
		end++
	}
	return content[:start] + content[end:], true
}

func fifoModuleName(elemType string, depth int) string {
	return fmt.Sprintf("mygo_fifo_%s_d%d", sanitize(elemType), depth)
}

func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("backend: create copy dest dir: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("backend: open copy source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("backend: create copy dest: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("backend: copy data: %w", err)
	}
	return nil
}

func signalWidth(t *ir.SignalType) int {
	if t == nil || t.Width <= 0 {
		return 1
	}
	return t.Width
}

func signalTypeString(t *ir.SignalType) string {
	return fmt.Sprintf("i%d", signalWidth(t))
}

func defaultAlmostFullLevel(depth int) int {
	if depth <= 1 {
		return 1
	}
	return depth - 1
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
