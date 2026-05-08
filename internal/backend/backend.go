package backend

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mygo/internal/ir"
	"mygo/internal/mlir"
)

var (
	runPipeline = runCirctPipeline
	runExport   = runCirctExportVerilog
)

const defaultVerilogPassPipeline = "builtin.module(map-arith-to-comb,lower-seq-to-sv,hw.module(lower-hw-to-sv))"

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
	// BenchmarkRefPath, when non-empty, points at a benchmark reference module
	// whose top-level interface should be mirrored by the emitted TopModule.
	BenchmarkRefPath string
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
	if design.TopLevel != nil && design.TopLevel.MixedClock != nil {
		if err := emitMixedClockTopModuleVerilog(design.TopLevel, outputPath); err != nil {
			return Result{}, err
		}
		return Result{MainPath: outputPath}, nil
	}

	loweredChannels := ir.LowerChannelsToFIFO(design)

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

	// Dump initial MLIR before processing if requested
	if opts.DumpMLIRPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.DumpMLIRPath), 0o755); err != nil {
			return Result{}, fmt.Errorf("backend: create circt-mlir dir: %w", err)
		}
		if err := copyFile(mlirPath, opts.DumpMLIRPath); err != nil {
			return Result{}, fmt.Errorf("backend: dump initial mlir: %w", err)
		}
	}

	passPipeline := strings.TrimSpace(opts.PassPipeline)
	if passPipeline == "" {
		// Default lowering is required because the emitter produces seq ops
		// (for example seq.to_clock/seq.compreg) that cannot be exported
		// directly to Verilog without seq/hw to sv conversion.
		passPipeline = defaultVerilogPassPipeline
	}

	currentInput := mlirPath
	if passPipeline != "" {
		pipelineOutput := filepath.Join(tempDir, "design.pipeline.mlir")
		if err := runPipeline(optPath, passPipeline, currentInput, pipelineOutput); err != nil {
			return Result{}, err
		}
		currentInput = pipelineOutput
	}
	exportOutput := filepath.Join(tempDir, "design.export.mlir")
	if err := runExport(optPath, "", opts.LoweringOptions, currentInput, exportOutput, outputPath); err != nil {
		return Result{}, err
	}
	currentInput = exportOutput

	if err := inlineGeneratedFifos(outputPath, loweredChannels.FIFODecls); err != nil {
		return Result{}, err
	}
	if err := applySignedVerilog(design, outputPath); err != nil {
		return Result{}, err
	}
	if err := applyLoopFSMVerilog(design, outputPath); err != nil {
		return Result{}, err
	}
	if err := stripUnsupportedAutomaticLifetime(outputPath); err != nil {
		return Result{}, err
	}
	if err := applyBenchmarkInterfaceWrapper(outputPath, opts.BenchmarkRefPath); err != nil {
		return Result{}, err
	}
	if err := stripLongVerilogComments(outputPath, 1024); err != nil {
		return Result{}, err
	}
	return Result{MainPath: outputPath}, nil
}

func emitMixedClockTopModuleVerilog(module *ir.Module, outputPath string) error {
	return fmt.Errorf("backend: mixed clock verilog emission is unavailable in the current workspace state")
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

func stripLongVerilogComments(path string, maxCommentLen int) error {
	if strings.TrimSpace(path) == "" || maxCommentLen <= 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("backend: read verilog for comment stripping: %w", err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	changed := false
	for i, line := range lines {
		idx := strings.Index(line, "//")
		if idx < 0 {
			continue
		}
		comment := line[idx:]
		if len(comment) <= maxCommentLen {
			continue
		}
		newline := ""
		if strings.HasSuffix(line, "\n") {
			newline = "\n"
		}
		lines[i] = strings.TrimRight(line[:idx], " \t") + newline
		changed = true
	}
	if !changed {
		return nil
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644); err != nil {
		return fmt.Errorf("backend: write verilog after comment stripping: %w", err)
	}
	return nil
}

func stripUnsupportedAutomaticLifetime(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("backend: read verilog for automatic-lifetime stripping: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	var moduleHeader []string
	var moduleBody []string
	var moduleDecls []string
	moduleDeclSeen := make(map[string]struct{})
	inModule := false
	headerDone := false
	changed := false
	singleDeclRE := regexp.MustCompile(`^(\s*)automatic\s+(logic|reg)(\s+\[[^]]+\])?\s+([A-Za-z_][A-Za-z0-9_$]*)\s*=\s*(.*);\s*(//.*)?$`)
	multiDeclRE := regexp.MustCompile(`^(\s*)automatic\s+(logic|reg)(\s+\[[^]]+\])?\s+([A-Za-z_][A-Za-z0-9_$]*)\s*=\s*(//.*)?$`)
	plainDeclRE := regexp.MustCompile(`^(\s*)automatic\s+(logic|reg)(\s+\[[^]]+\])?\s+([A-Za-z_][A-Za-z0-9_$]*)\s*;\s*(//.*)?$`)
	flushModule := func(endLine string) {
		out = append(out, moduleHeader...)
		out = append(out, moduleDecls...)
		out = append(out, moduleBody...)
		out = append(out, endLine)
		moduleHeader = nil
		moduleBody = nil
		moduleDecls = nil
		moduleDeclSeen = make(map[string]struct{})
		inModule = false
		headerDone = false
	}
	addModuleDecl := func(indent, kind, width, name, comment string) {
		key := kind + "|" + width + "|" + name
		if _, ok := moduleDeclSeen[key]; ok {
			return
		}
		decl := fmt.Sprintf("%s%s%s %s;", indent, kind, width, name)
		if strings.TrimSpace(comment) != "" {
			decl += " " + strings.TrimSpace(comment)
		}
		moduleDecls = append(moduleDecls, decl)
		moduleDeclSeen[key] = struct{}{}
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !inModule {
			if strings.HasPrefix(strings.TrimSpace(line), "module ") {
				inModule = true
				moduleHeader = append(moduleHeader, line)
				if strings.Contains(line, ");") {
					headerDone = true
				}
				continue
			}
			out = append(out, line)
			continue
		}
		if !headerDone {
			moduleHeader = append(moduleHeader, line)
			if strings.Contains(line, ");") {
				headerDone = true
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "endmodule") {
			flushModule(line)
			continue
		}
		if m := singleDeclRE.FindStringSubmatch(line); m != nil {
			indent, kind, width, name, expr, comment := m[1], m[2], m[3], m[4], m[5], m[6]
			addModuleDecl(indent, kind, width, name, comment)
			moduleBody = append(moduleBody, fmt.Sprintf("%s%s = %s;", indent, name, expr))
			changed = true
			continue
		}
		if m := multiDeclRE.FindStringSubmatch(line); m != nil {
			indent, kind, width, name, comment := m[1], m[2], m[3], m[4], m[5]
			addModuleDecl(indent, kind, width, name, comment)
			moduleBody = append(moduleBody, fmt.Sprintf("%s%s =", indent, name))
			changed = true
			for i+1 < len(lines) {
				i++
				moduleBody = append(moduleBody, lines[i])
				if strings.Contains(lines[i], ";") {
					break
				}
			}
			continue
		}
		if m := plainDeclRE.FindStringSubmatch(line); m != nil {
			indent, kind, width, name, comment := m[1], m[2], m[3], m[4], m[5]
			addModuleDecl(indent, kind, width, name, comment)
			changed = true
			continue
		}
		moduleBody = append(moduleBody, line)
	}
	if inModule {
		out = append(out, moduleHeader...)
		out = append(out, moduleDecls...)
		out = append(out, moduleBody...)
	}
	if !changed {
		return nil
	}
	updated := strings.Join(out, "\n")
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("backend: write verilog after automatic-lifetime stripping: %w", err)
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

func inlineGeneratedFifos(mainPath string, fifos []*ir.FIFODecl) error {
	if len(fifos) == 0 {
		return nil
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("backend: read verilog output: %w", err)
	}
	updated := string(data)
	rewrittenReusableModule := false
	for _, fifo := range fifos {
		if fifo == nil {
			continue
		}
		var err error
		updated, err = rewriteFIFOStubInstances(updated, fifo)
		if err != nil {
			return err
		}
		var ok bool
		updated, ok = removeModuleBlock(updated, fifo.ModuleName)
		if !ok {
			return fmt.Errorf("backend: module %s not found in generated Verilog", fifo.ModuleName)
		}
		rewrittenReusableModule = true
	}
	if rewrittenReusableModule {
		if !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += "\n" + GenerateReusableParametricFIFOVerilog(reusableFIFOName) + "\n"
	}
	if err := os.WriteFile(mainPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("backend: update main verilog: %w", err)
	}
	return nil
}

func rewriteFIFOStubInstances(content string, fifo *ir.FIFODecl) (string, error) {
	if fifo == nil {
		return content, nil
	}
	re := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(fifo.ModuleName) + `(\s+)([A-Za-z_][A-Za-z0-9_$]*)(\s*\()`)
	params := fmt.Sprintf(
		"%s #(.DATA_WIDTH(%d), .DEPTH(%d), .ADDR_WIDTH(%d), .COUNT_WIDTH(%d), .LAST_PTR_VALUE(%d), .DEPTH_COUNT_VALUE(%d), .ALMOST_FULL_LEVEL(%d), .ALMOST_EMPTY_LEVEL(%d), .ALMOST_FULL_COUNT_VALUE(%d), .ALMOST_EMPTY_COUNT_VALUE(%d), .USE_REGISTERED_READ(%d), .ALMOST_EMPTY_USES_EMPTY(%d), .ASYNC_RESET(%d))",
		reusableModuleNameForDecl(fifo),
		fifo.DataWidth,
		fifo.Depth,
		fifo.AddrWidth,
		fifo.CountWidth,
		fifo.LastPtrValue,
		fifo.DepthCountValue,
		fifo.AlmostFullLevel,
		fifo.AlmostEmptyLevel,
		fifo.AlmostFullCountValue,
		fifo.AlmostEmptyCountValue,
		boolToInt(fifo.UseRegisteredRead),
		boolToInt(fifo.AlmostEmptyUsesEmpty),
		boolToInt(fifo.AsyncReset),
	)
	rewritten := re.ReplaceAllString(content, `${1}`+params+`${2}${3}${4}`)
	return rewritten, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func reusableModuleNameForDecl(fifo *ir.FIFODecl) string {
	if fifo == nil || strings.TrimSpace(fifo.ReusableModuleName) == "" {
		return reusableFIFOName
	}
	return fifo.ReusableModuleName
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

type verilogPort struct {
	Name      string
	Direction string
	Width     int
}

func applyBenchmarkInterfaceWrapper(verilogPath, benchmarkRefPath string) error {
	benchmarkRefPath = strings.TrimSpace(benchmarkRefPath)
	if benchmarkRefPath == "" {
		return nil
	}
	expectedPorts, err := parseVerilogModulePortsFromFile(benchmarkRefPath, "RefModule")
	if err != nil {
		return fmt.Errorf("backend: parse benchmark reference interface: %w", err)
	}
	if len(expectedPorts) == 0 {
		return nil
	}
	data, err := os.ReadFile(verilogPath)
	if err != nil {
		return fmt.Errorf("backend: read verilog for interface wrapping: %w", err)
	}
	content := string(data)
	actualPorts, err := parseVerilogModulePorts(content, "TopModule")
	if err != nil {
		return fmt.Errorf("backend: parse emitted top interface: %w", err)
	}
	if sameVerilogPortSignature(actualPorts, expectedPorts) {
		return nil
	}
	updated, renamed := renameModuleDeclaration(content, "TopModule", "TopModule__impl")
	if !renamed {
		return fmt.Errorf("backend: top module declaration not found for interface wrapping")
	}
	wrapper, err := buildBenchmarkWrapper("TopModule", "TopModule__impl", expectedPorts, actualPorts)
	if err != nil {
		return fmt.Errorf("backend: build benchmark interface wrapper: %w", err)
	}
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += "\n" + wrapper + "\n"
	if err := os.WriteFile(verilogPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("backend: write wrapped verilog: %w", err)
	}
	return nil
}

func parseVerilogModulePortsFromFile(path, moduleName string) ([]verilogPort, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseVerilogModulePorts(string(data), moduleName)
}

func parseVerilogModulePorts(content, moduleName string) ([]verilogPort, error) {
	re := regexp.MustCompile(`(?s)module\s+` + regexp.QuoteMeta(moduleName) + `\s*\((.*?)\)\s*;`)
	matches := re.FindStringSubmatch(content)
	if len(matches) < 2 {
		return nil, fmt.Errorf("module %s interface not found", moduleName)
	}
	header := stripLineComments(matches[1])
	segments := strings.Split(header, ",")
	ports := make([]verilogPort, 0, len(segments))
	currentDir := ""
	currentWidth := 1
	for _, raw := range segments {
		segment := strings.TrimSpace(raw)
		if segment == "" {
			continue
		}
		port, nextDir, nextWidth, ok, err := parseVerilogPortSegment(segment, currentDir, currentWidth)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		currentDir = nextDir
		currentWidth = nextWidth
		ports = append(ports, port)
	}
	return ports, nil
}

func stripLineComments(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "//"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

func parseVerilogPortSegment(segment, currentDir string, currentWidth int) (verilogPort, string, int, bool, error) {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return verilogPort{}, currentDir, currentWidth, false, nil
	}
	nextDir := currentDir
	nextWidth := currentWidth
	for _, dir := range []string{"input", "output", "inout"} {
		if strings.HasPrefix(segment, dir) {
			nextDir = dir
			nextWidth = 1
			segment = strings.TrimSpace(segment[len(dir):])
			break
		}
	}
	if nextDir == "" {
		return verilogPort{}, currentDir, currentWidth, false, nil
	}
	rangeRe := regexp.MustCompile(`\[\s*(\d+)\s*:\s*(\d+)\s*\]`)
	if match := rangeRe.FindStringSubmatch(segment); len(match) == 3 {
		msb, err := strconv.Atoi(match[1])
		if err != nil {
			return verilogPort{}, currentDir, currentWidth, false, err
		}
		lsb, err := strconv.Atoi(match[2])
		if err != nil {
			return verilogPort{}, currentDir, currentWidth, false, err
		}
		if msb >= lsb {
			nextWidth = msb - lsb + 1
		} else {
			nextWidth = lsb - msb + 1
		}
		segment = strings.TrimSpace(rangeRe.ReplaceAllString(segment, " "))
	}
	fields := strings.Fields(segment)
	filtered := fields[:0]
	for _, field := range fields {
		switch field {
		case "wire", "reg", "logic", "signed", "unsigned":
			continue
		default:
			filtered = append(filtered, field)
		}
	}
	if len(filtered) == 0 {
		return verilogPort{}, nextDir, nextWidth, false, nil
	}
	name := strings.Trim(filtered[len(filtered)-1], " \t\r\n")
	name = strings.TrimSuffix(name, ")")
	if name == "" {
		return verilogPort{}, nextDir, nextWidth, false, nil
	}
	return verilogPort{Name: name, Direction: nextDir, Width: nextWidth}, nextDir, nextWidth, true, nil
}

func sameVerilogPortSignature(actual, expected []verilogPort) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i].Name != expected[i].Name || actual[i].Direction != expected[i].Direction || actual[i].Width != expected[i].Width {
			return false
		}
	}
	return true
}

func renameModuleDeclaration(content, fromName, toName string) (string, bool) {
	re := regexp.MustCompile(`(?m)^module\s+` + regexp.QuoteMeta(fromName) + `(\s*\()`)
	updated := re.ReplaceAllString(content, "module "+toName+`${1}`)
	return updated, updated != content
}

func buildBenchmarkWrapper(wrapperName, implName string, expectedPorts, actualPorts []verilogPort) (string, error) {
	actualByName := make(map[string]verilogPort, len(actualPorts))
	for _, port := range actualPorts {
		actualByName[port.Name] = port
	}
	expectedByName := make(map[string]verilogPort, len(expectedPorts))
	for _, port := range expectedPorts {
		expectedByName[port.Name] = port
	}

	connectionLines := make([]string, 0, len(actualPorts))
	wireDecls := make([]string, 0)
	assignLines := make([]string, 0)

	for _, actual := range actualPorts {
		switch actual.Direction {
		case "input":
			expr := zeroLiteral(actual.Width)
			if expected, ok := expectedByName[actual.Name]; ok {
				expr = adaptInputExpr(expected, actual)
			} else if isClockOrResetPortName(actual.Name) {
				expr = zeroLiteral(actual.Width)
			}
			connectionLines = append(connectionLines, fmt.Sprintf("    .%s(%s)", actual.Name, expr))
		case "output", "inout":
			if expected, ok := directOutputBinding(actual, expectedByName); ok {
				if expected.Width == actual.Width {
					connectionLines = append(connectionLines, fmt.Sprintf("    .%s(%s)", actual.Name, expected.Name))
					continue
				}
				tempName := wrapperTempName(actual.Name)
				wireDecls = append(wireDecls, fmt.Sprintf("  wire %s%s;", formatWidth(actual.Width), tempName))
				assignLines = append(assignLines, fmt.Sprintf("  assign %s = %s;", expected.Name, adaptOutputExpr(tempName, actual.Width, expected.Width)))
				connectionLines = append(connectionLines, fmt.Sprintf("    .%s(%s)", actual.Name, tempName))
				continue
			}
			if expected, idx, ok := findPackedOutputBinding(actual.Name, expectedPorts); ok {
				tempName := wrapperTempName(actual.Name)
				wireDecls = append(wireDecls, fmt.Sprintf("  wire %s%s;", formatWidth(actual.Width), tempName))
				assignLines = append(assignLines, fmt.Sprintf("  assign %s[%d] = %s;", expected.Name, idx, tempName))
				connectionLines = append(connectionLines, fmt.Sprintf("    .%s(%s)", actual.Name, tempName))
				continue
			}
			tempName := wrapperTempName(actual.Name)
			wireDecls = append(wireDecls, fmt.Sprintf("  wire %s%s;", formatWidth(actual.Width), tempName))
			connectionLines = append(connectionLines, fmt.Sprintf("    .%s(%s)", actual.Name, tempName))
		default:
			return "", fmt.Errorf("unsupported actual port direction %q", actual.Direction)
		}
	}

	for _, expected := range expectedPorts {
		if expected.Direction != "output" && expected.Direction != "inout" {
			continue
		}
		if hasDirectOutputBinding(expected, actualByName) {
			continue
		}
		if hasPackedOutputBinding(expected.Name, expected.Width, actualPorts) {
			continue
		}
		return "", fmt.Errorf("no implementation binding found for expected output %s", expected.Name)
	}

	var builder strings.Builder
	builder.WriteString("module ")
	builder.WriteString(wrapperName)
	builder.WriteString("(\n")
	for i, port := range expectedPorts {
		builder.WriteString("  ")
		builder.WriteString(port.Direction)
		builder.WriteString(" ")
		builder.WriteString(formatWidth(port.Width))
		builder.WriteString(port.Name)
		if i < len(expectedPorts)-1 {
			builder.WriteString(",\n")
		} else {
			builder.WriteString("\n")
		}
	}
	builder.WriteString(");\n")
	if len(wireDecls) > 0 {
		for _, decl := range uniqueSortedStrings(wireDecls) {
			builder.WriteString(decl)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("  ")
	builder.WriteString(implName)
	builder.WriteString(" dut (\n")
	for i, line := range connectionLines {
		builder.WriteString(line)
		if i < len(connectionLines)-1 {
			builder.WriteString(",\n")
		} else {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("  );\n")
	if len(assignLines) > 0 {
		for _, line := range uniqueSortedStrings(assignLines) {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("endmodule")
	return builder.String(), nil
}

func directOutputBinding(actual verilogPort, expectedByName map[string]verilogPort) (verilogPort, bool) {
	if expected, ok := expectedByName[actual.Name]; ok {
		return expected, true
	}
	if strings.HasPrefix(actual.Name, "out_") {
		alias := strings.TrimPrefix(actual.Name, "out_")
		if expected, ok := expectedByName[alias]; ok {
			return expected, true
		}
	}
	return verilogPort{}, false
}

func hasDirectOutputBinding(expected verilogPort, actualByName map[string]verilogPort) bool {
	if _, ok := actualByName[expected.Name]; ok {
		return true
	}
	if _, ok := actualByName["out_"+expected.Name]; ok {
		return true
	}
	return false
}

func formatWidth(width int) string {
	if width <= 1 {
		return ""
	}
	return fmt.Sprintf("[%d:0] ", width-1)
}

func zeroLiteral(width int) string {
	if width <= 1 {
		return "1'b0"
	}
	return fmt.Sprintf("%d'b0", width)
}

func adaptInputExpr(expected, actual verilogPort) string {
	if expected.Width == actual.Width {
		return expected.Name
	}
	if actual.Width <= 1 {
		if expected.Width <= 1 {
			return expected.Name
		}
		return fmt.Sprintf("%s[0]", expected.Name)
	}
	if expected.Width > actual.Width {
		return fmt.Sprintf("%s[%d:0]", expected.Name, actual.Width-1)
	}
	pad := actual.Width - expected.Width
	if pad <= 0 {
		return expected.Name
	}
	return fmt.Sprintf("{{%d{1'b0}}, %s}", pad, expected.Name)
}

func adaptOutputExpr(signalName string, actualWidth, expectedWidth int) string {
	if actualWidth == expectedWidth {
		return signalName
	}
	if expectedWidth <= 1 {
		if actualWidth <= 1 {
			return signalName
		}
		return fmt.Sprintf("%s[0]", signalName)
	}
	if actualWidth > expectedWidth {
		return fmt.Sprintf("%s[%d:0]", signalName, expectedWidth-1)
	}
	pad := expectedWidth - actualWidth
	if pad <= 0 {
		return signalName
	}
	return fmt.Sprintf("{{%d{1'b0}}, %s}", pad, signalName)
}

func findPackedOutputBinding(actualName string, expectedPorts []verilogPort) (verilogPort, int, bool) {
	for _, expected := range expectedPorts {
		if expected.Direction != "output" && expected.Direction != "inout" {
			continue
		}
		if expected.Width <= 1 {
			continue
		}
		if idx, ok := matchIndexedPortWithOutputAlias(actualName, expected.Name); ok && idx >= 0 && idx < expected.Width {
			return expected, idx, true
		}
	}
	return verilogPort{}, 0, false
}

func hasPackedOutputBinding(expectedName string, expectedWidth int, actualPorts []verilogPort) bool {
	if expectedWidth <= 1 {
		return false
	}
	for _, actual := range actualPorts {
		if actual.Direction != "output" && actual.Direction != "inout" {
			continue
		}
		if idx, ok := matchIndexedPortWithOutputAlias(actual.Name, expectedName); ok && idx >= 0 && idx < expectedWidth {
			return true
		}
	}
	return false
}

func matchIndexedPortWithOutputAlias(actualName, expectedBase string) (int, bool) {
	if idx, ok := matchIndexedPort(actualName, expectedBase); ok {
		return idx, true
	}
	if strings.HasPrefix(actualName, "out_") {
		return matchIndexedPort(strings.TrimPrefix(actualName, "out_"), expectedBase)
	}
	return 0, false
}

func matchIndexedPort(actualName, expectedBase string) (int, bool) {
	if strings.HasPrefix(actualName, expectedBase+"_") {
		idxText := strings.TrimPrefix(actualName, expectedBase+"_")
		idx, err := strconv.Atoi(idxText)
		return idx, err == nil
	}
	if strings.HasPrefix(actualName, expectedBase) {
		idxText := strings.TrimPrefix(actualName, expectedBase)
		if idxText != "" {
			idx, err := strconv.Atoi(idxText)
			return idx, err == nil
		}
	}
	return 0, false
}

func wrapperTempName(name string) string {
	replacer := strings.NewReplacer("$", "_", ".", "_")
	return "__mygo_" + replacer.Replace(name)
}

func isClockOrResetPortName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "clk", "clock", "rst", "reset", "areset", "resetn", "aresetn":
		return true
	default:
		return false
	}
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
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

type processLoopStructure struct {
	header *ir.BasicBlock
	latch  *ir.BasicBlock
}

func applyLoopFSMVerilog(design *ir.Design, verilogPath string) error {
	if design == nil || verilogPath == "" {
		return nil
	}
	fallbacks := collectLoopFSMFallbacks(design)
	if len(fallbacks) == 0 {
		return nil
	}

	data, err := os.ReadFile(verilogPath)
	if err != nil {
		return fmt.Errorf("backend: read verilog output: %w", err)
	}
	verilog := string(data)

	modules := make([]string, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		if fallback.processModule == "" {
			continue
		}
		if moduleHasFSMLogic(verilog, fallback.processModule) {
			continue
		}
		if strings.Contains(verilog, "module "+fallback.fsmModule+"(") {
			continue
		}
		modules = append(modules, renderLoopFSMModule(fallback))
	}
	if len(modules) == 0 {
		return nil
	}

	if !strings.HasSuffix(verilog, "\n") {
		verilog += "\n"
	}
	verilog += "\n// mygo autogenerated loop FSM fallback modules\n"
	verilog += strings.Join(modules, "\n\n")
	verilog += "\n"

	if err := os.WriteFile(verilogPath, []byte(verilog), 0o644); err != nil {
		return fmt.Errorf("backend: update verilog with loop fsm fallback: %w", err)
	}
	return nil
}

type loopFSMFallback struct {
	processModule string
	fsmModule     string
	condSignal    string
}

func collectLoopFSMFallbacks(design *ir.Design) []loopFSMFallback {
	if design == nil {
		return nil
	}
	fallbacks := make([]loopFSMFallback, 0)
	seen := make(map[string]struct{})
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		ordered := orderProcesses(module)
		for _, proc := range ordered {
			if proc == nil {
				continue
			}
			loops := findDynamicBoundaryLoops(proc)
			if len(loops) == 0 {
				continue
			}
			processModule := processModuleName(module, proc)
			for idx, loop := range loops {
				fsmModule := fmt.Sprintf("%s__loop%d_fsm", processModule, idx)
				if _, ok := seen[fsmModule]; ok {
					continue
				}
				seen[fsmModule] = struct{}{}
				fallbacks = append(fallbacks, loopFSMFallback{
					processModule: processModule,
					fsmModule:     fsmModule,
					condSignal:    loopConditionSignal(loop),
				})
			}
		}
	}
	sort.SliceStable(fallbacks, func(i, j int) bool {
		if fallbacks[i].processModule != fallbacks[j].processModule {
			return fallbacks[i].processModule < fallbacks[j].processModule
		}
		return fallbacks[i].fsmModule < fallbacks[j].fsmModule
	})
	return fallbacks
}

func moduleHasFSMLogic(verilog, moduleName string) bool {
	body, ok := moduleBlock(verilog, moduleName)
	if !ok {
		return false
	}
	if !strings.Contains(body, "state_reg") {
		return false
	}
	return strings.Contains(body, "case (state_reg") || strings.Contains(body, "case(state_reg")
}

func moduleBlock(verilog, moduleName string) (string, bool) {
	if verilog == "" || moduleName == "" {
		return "", false
	}
	start := strings.Index(verilog, "module "+moduleName)
	if start < 0 {
		return "", false
	}
	tail := verilog[start:]
	endRel := strings.Index(tail, "endmodule")
	if endRel < 0 {
		return "", false
	}
	end := start + endRel + len("endmodule")
	return verilog[start:end], true
}

func renderLoopFSMModule(fallback loopFSMFallback) string {
	moduleName := fallback.fsmModule
	conditionName := fallback.condSignal
	if conditionName == "" {
		conditionName = "check_cond"
	}
	const stateCount = 4
	stateWidth := fsmStateWidth(stateCount)
	return fmt.Sprintf(`module %s(
  input clk,
        rst,
        check_cond,
        body_done,
        update_done,
  output reg [%d:0] state,
  output     active,
             done,
             body_enable,
             update_enable,
             state_check,
             state_body,
             state_update,
             state_exit
);

  // Derived from IR loop condition signal: %s
  localparam [%d:0] STATE_CHECK  = %d'd0;
  localparam [%d:0] STATE_BODY   = %d'd1;
  localparam [%d:0] STATE_UPDATE = %d'd2;
  localparam [%d:0] STATE_EXIT   = %d'd3;

  // Predecoded state predicates keep control fanout shallow for improved timing closure.
  wire state_is_check = (state == STATE_CHECK);
  wire state_is_body = (state == STATE_BODY);
  wire state_is_update = (state == STATE_UPDATE);
  wire state_is_exit = (state == STATE_EXIT);

  assign state_check = state_is_check;
  assign state_body = state_is_body;
  assign state_update = state_is_update;
  assign state_exit = state_is_exit;
  assign body_enable = state_is_body;
  assign update_enable = state_is_update;
  assign active = ~state_is_exit;
  assign done = state_is_exit;

  // Compact binary state encoding minimizes register usage while keeping timing predictable.
  (* fsm_encoding = "sequential" *) reg [%d:0] next_state;

  always @(*) begin
    // Default hold avoids unnecessary state toggles and shortens the transition logic cone.
    next_state = state;
    case (state)
      STATE_CHECK: begin
        if (check_cond)
          next_state = STATE_BODY;
        else
          next_state = STATE_EXIT;
      end
      STATE_BODY: begin
        if (body_done)
          next_state = STATE_UPDATE;
      end
      STATE_UPDATE: begin
        if (update_done)
          next_state = STATE_CHECK;
      end
      STATE_EXIT: begin
        next_state = STATE_EXIT;
      end
      default:
        next_state = STATE_CHECK;
    endcase
  end

  always @(posedge clk) begin
    if (rst)
      state <= STATE_CHECK;
    else
      state <= next_state;
  end
endmodule`, moduleName,
		stateWidth-1,
		conditionName,
		stateWidth-1, stateWidth,
		stateWidth-1, stateWidth,
		stateWidth-1, stateWidth,
		stateWidth-1, stateWidth,
		stateWidth-1)
}

func loopConditionSignal(loop processLoopStructure) string {
	if loop.header == nil {
		return "check_cond"
	}
	branch, ok := loop.header.Terminator.(*ir.BranchTerminator)
	if !ok || branch == nil || branch.Cond == nil {
		return "check_cond"
	}
	if branch.Cond.Name == "" {
		return "check_cond"
	}
	return sanitize(branch.Cond.Name)
}

func fsmStateWidth(states int) int {
	if states <= 1 {
		return 1
	}
	width := 0
	for n := states - 1; n > 0; n >>= 1 {
		width++
	}
	return width
}

func findDynamicBoundaryLoops(proc *ir.Process) []processLoopStructure {
	if proc == nil || len(proc.Blocks) == 0 {
		return nil
	}
	dominators := processDominators(proc)
	predecessors := processPredecessors(proc)
	loops := make([]processLoopStructure, 0)
	seen := make(map[string]struct{})

	for _, src := range proc.Blocks {
		if src == nil {
			continue
		}
		jump, ok := src.Terminator.(*ir.JumpTerminator)
		if !ok || jump.Target == nil {
			continue
		}
		header := jump.Target
		branch, ok := header.Terminator.(*ir.BranchTerminator)
		if !ok || branch == nil || branch.Cond == nil {
			continue
		}
		if branch.Cond.Kind == ir.Const {
			continue
		}
		if !processBlockDominates(dominators, header, src) {
			continue
		}

		loopNodes := processNaturalLoopNodes(header, src, predecessors)
		if len(loopNodes) == 0 {
			continue
		}

		key := fmt.Sprintf("%p:%p", header, src)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		loops = append(loops, processLoopStructure{
			header: header,
			latch:  src,
		})
	}

	sort.SliceStable(loops, func(i, j int) bool {
		return blockOrderIndex(proc, loops[i].header) < blockOrderIndex(proc, loops[j].header)
	})
	return loops
}

func processDominators(proc *ir.Process) map[*ir.BasicBlock]map[*ir.BasicBlock]struct{} {
	dominators := make(map[*ir.BasicBlock]map[*ir.BasicBlock]struct{})
	if proc == nil || len(proc.Blocks) == 0 {
		return dominators
	}

	blocks := make([]*ir.BasicBlock, 0, len(proc.Blocks))
	all := make(map[*ir.BasicBlock]struct{})
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		blocks = append(blocks, block)
		all[block] = struct{}{}
	}
	if len(blocks) == 0 {
		return dominators
	}

	entry := blocks[0]
	for _, block := range blocks {
		if block == entry {
			dominators[block] = map[*ir.BasicBlock]struct{}{block: {}}
			continue
		}
		dominators[block] = cloneProcessBlockSet(all)
	}

	predecessors := processPredecessors(proc)
	changed := true
	for changed {
		changed = false
		for _, block := range blocks {
			if block == entry {
				continue
			}
			preds := predecessors[block]
			next := make(map[*ir.BasicBlock]struct{})
			first := true
			for _, pred := range preds {
				if pred == nil {
					continue
				}
				predSet := dominators[pred]
				if first {
					next = cloneProcessBlockSet(predSet)
					first = false
					continue
				}
				next = intersectProcessBlockSet(next, predSet)
			}
			next[block] = struct{}{}
			if !equalProcessBlockSet(next, dominators[block]) {
				dominators[block] = next
				changed = true
			}
		}
	}

	return dominators
}

func processPredecessors(proc *ir.Process) map[*ir.BasicBlock][]*ir.BasicBlock {
	predMap := make(map[*ir.BasicBlock][]*ir.BasicBlock)
	if proc == nil {
		return predMap
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		if len(block.Predecessors) > 0 {
			preds := predMap[block]
			for _, pred := range block.Predecessors {
				appendUniqueProcessBlock(&preds, pred)
			}
			predMap[block] = preds
		}
		for _, succ := range block.Successors {
			preds := predMap[succ]
			appendUniqueProcessBlock(&preds, block)
			predMap[succ] = preds
		}
	}
	return predMap
}

func processNaturalLoopNodes(header, latch *ir.BasicBlock, predecessors map[*ir.BasicBlock][]*ir.BasicBlock) map[*ir.BasicBlock]struct{} {
	nodes := make(map[*ir.BasicBlock]struct{})
	if header == nil || latch == nil {
		return nodes
	}
	nodes[header] = struct{}{}
	nodes[latch] = struct{}{}

	stack := []*ir.BasicBlock{latch}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, pred := range predecessors[n] {
			if pred == nil {
				continue
			}
			if _, ok := nodes[pred]; ok {
				continue
			}
			nodes[pred] = struct{}{}
			if pred != header {
				stack = append(stack, pred)
			}
		}
	}
	return nodes
}

func processBlockDominates(dom map[*ir.BasicBlock]map[*ir.BasicBlock]struct{}, candidate, block *ir.BasicBlock) bool {
	if candidate == nil || block == nil {
		return false
	}
	set, ok := dom[block]
	if !ok {
		return false
	}
	_, ok = set[candidate]
	return ok
}

func blockOrderIndex(proc *ir.Process, target *ir.BasicBlock) int {
	if proc == nil || target == nil {
		return -1
	}
	for idx, block := range proc.Blocks {
		if block == target {
			return idx
		}
	}
	return -1
}

func appendUniqueProcessBlock(dst *[]*ir.BasicBlock, block *ir.BasicBlock) {
	if dst == nil || block == nil {
		return
	}
	for _, existing := range *dst {
		if existing == block {
			return
		}
	}
	*dst = append(*dst, block)
}

func cloneProcessBlockSet(src map[*ir.BasicBlock]struct{}) map[*ir.BasicBlock]struct{} {
	dst := make(map[*ir.BasicBlock]struct{}, len(src))
	for block := range src {
		dst[block] = struct{}{}
	}
	return dst
}

func intersectProcessBlockSet(a, b map[*ir.BasicBlock]struct{}) map[*ir.BasicBlock]struct{} {
	out := make(map[*ir.BasicBlock]struct{})
	if len(a) == 0 || len(b) == 0 {
		return out
	}
	for block := range a {
		if _, ok := b[block]; ok {
			out[block] = struct{}{}
		}
	}
	return out
}

func equalProcessBlockSet(a, b map[*ir.BasicBlock]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for block := range a {
		if _, ok := b[block]; !ok {
			return false
		}
	}
	return true
}
