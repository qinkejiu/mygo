package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"

	"mygo/internal/backend"
	"mygo/internal/constdata"
	"mygo/internal/diag"
	"mygo/internal/frontend"
	"mygo/internal/ir"
	"mygo/internal/mlir"
	"mygo/internal/passes"
	"mygo/internal/validate"
)

var emitVerilog = backend.EmitVerilog

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printGlobalUsage()
		return fmt.Errorf("missing command")
	}

	switch args[0] {
	case "compile":
		return runCompile(args[1:])
	case "sim":
		return runSim(args[1:])
	case "lint":
		return runLint(args[1:])
	default:
		printGlobalUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runCompile(args []string) error {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	emit := fs.String("emit", "mlir", "output format (ssa|ir|mlir|verilog)")
	output := fs.String("o", "", "output file path (stdout when omitted, except verilog)")
	target := fs.String("target", "", "target function or module (default: auto-detect TopModule, else main)")
	diagFormat := fs.String("diag-format", "text", "diagnostic output format (text|json)")
	circtOpt := fs.String("circt-opt", "", "path to circt-opt (optional, falls back to PATH lookup)")
	circtPipeline := fs.String("circt-pipeline", "", "circt-opt --pass-pipeline string (optional)")
	circtLowering := fs.String("circt-lowering-options", "", "comma-separated circt-opt --lowering-options string (optional)")
	circtMLIR := fs.String("circt-mlir", "", "path to dump the MLIR handed to CIRCT (optional)")
	benchmarkRefPath := fs.String("benchmark-ref-path", "", "path to benchmark reference Verilog used for interface wrapping (optional)")
	fifoSrc := fs.String("fifo-src", "", "deprecated: external FIFO source path (ignored; FIFOs are generated inline)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("compile command requires at least one Go source file")
	}

	inputs := fs.Args()
	tempRoot := artifactTempRoot(inputs)
	result, err := prepareProgram(inputs, *diagFormat)
	if err != nil {
		return err
	}

	if *emit == "ssa" {
		return emitSSAProgram(result.program, *output)
	}

	if err := validateProgram(result); err != nil {
		return err
	}

	design, err := ir.BuildDesign(result.program, result.reporter, *target)
	if err != nil {
		return err
	}

	if err := runDefaultPasses(design, result.reporter); err != nil {
		return err
	}
	hasChannels := designHasChannels(design)

	switch *emit {
	case "ir":
		return emitIRDesign(design, *output)
	case "mlir":
		if err := ensureHardwareLowerableDesign(design); err != nil {
			return err
		}
		return mlir.Emit(design, *output)
	case "verilog":
		if err := ensureHardwareLowerableDesign(design); err != nil {
			return err
		}
		if *output == "" || *output == "-" {
			return fmt.Errorf("verilog emission requires -o")
		}
		opts := backend.Options{
			CIRCTOptPath:     *circtOpt,
			PassPipeline:     *circtPipeline,
			LoweringOptions:  *circtLowering,
			DumpMLIRPath:     *circtMLIR,
			TempRoot:         tempRoot,
			BenchmarkRefPath: *benchmarkRefPath,
			FIFOSource:       *fifoSrc,
		}
		res, err := emitVerilog(design, *output, opts)
		if err != nil {
			return err
		}
		if hasChannels && *fifoSrc != "" {
			fmt.Fprintln(os.Stderr, "warning: --fifo-src is deprecated and ignored; FIFOs are generated inline")
		}
		if len(res.AuxPaths) > 0 {
			fmt.Fprintf(os.Stderr, "additional sources written: %s\n", strings.Join(res.AuxPaths, ", "))
		}
		return nil
	default:
		return fmt.Errorf("unknown emit format: %s", *emit)
	}

}

func printGlobalUsage() {
	fmt.Fprintf(os.Stderr, "MyGO compiler (phase 1 scaffold)\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  mygo <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  compile    Compile Go source to SSA, IR, MLIR, or Verilog\n")
	fmt.Fprintf(os.Stderr, "  sim        Compile to Verilog and run a simulator\n")
	fmt.Fprintf(os.Stderr, "  lint       Run validation-only checks (e.g. concurrency rules)\n")
}

func runLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	concurrency := fs.Bool("concurrency", true, "enable concurrency validation rules")
	diagFormat := fs.String("diag-format", "text", "diagnostic output format (text|json)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("lint requires at least one Go source file")
	}

	result, err := prepareProgram(fs.Args(), *diagFormat)
	if err != nil {
		return err
	}

	if *concurrency {
		if err := validateProgram(result); err != nil {
			return err
		}
	}

	return nil
}

type frontendResult struct {
	reporter *diag.Reporter
	program  *ssa.Program
	ssaPkgs  []*ssa.Package
	pkgs     []*packages.Package
}

func prepareProgram(sources []string, diagFormat string) (*frontendResult, error) {
	reporter := diag.NewReporter(os.Stderr, diagFormat)
	cfg := frontend.LoadConfig{Sources: sources}
	pkgs, _, err := frontend.LoadPackages(cfg, reporter)
	if err != nil {
		return nil, err
	}
	if reporter.HasErrors() {
		return nil, fmt.Errorf("errors reported while loading packages")
	}
	prog, ssaPkgs, err := frontend.BuildSSA(pkgs, reporter)
	if err != nil {
		return nil, err
	}
	if reporter.HasErrors() {
		return nil, fmt.Errorf("errors reported during SSA construction")
	}
	return &frontendResult{
		reporter: reporter,
		program:  prog,
		ssaPkgs:  ssaPkgs,
		pkgs:     pkgs,
	}, nil
}

func runDefaultPasses(design *ir.Design, reporter *diag.Reporter) error {
	passMgr := passes.NewManager()
	passMgr.Add(passes.NewWidthInference(reporter))
	if err := passMgr.Run(design); err != nil {
		return err
	}
	if reporter != nil && reporter.HasErrors() {
		return fmt.Errorf("analysis passes reported errors")
	}
	return nil
}

func validateProgram(result *frontendResult) error {
	if result == nil || result.program == nil {
		return fmt.Errorf("no program available for validation")
	}
	if err := validate.CheckProgram(result.program, result.ssaPkgs, result.pkgs, result.reporter); err != nil {
		return err
	}
	return nil
}

func runSim(args []string) error {
	fs := flag.NewFlagSet("sim", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	target := fs.String("target", "", "target function or module (default: auto-detect TopModule, else main)")
	diagFormat := fs.String("diag-format", "text", "diagnostic output format (text|json)")
	circtOpt := fs.String("circt-opt", "", "path to circt-opt (optional)")
	circtPipeline := fs.String("circt-pipeline", "", "circt-opt --pass-pipeline string (optional)")
	circtLowering := fs.String("circt-lowering-options", "", "comma-separated circt-opt --lowering-options string (optional)")
	circtMLIR := fs.String("circt-mlir", "", "path to dump the MLIR handed to CIRCT (optional)")
	benchmarkRefPath := fs.String("benchmark-ref-path", "", "path to benchmark reference Verilog used for interface wrapping (optional)")
	verilogOut := fs.String("verilog-out", "", "path to write the emitted Verilog bundle (optional)")
	keepArtifacts := fs.Bool("keep-artifacts", true, "keep temporary artifacts generated during simulation")
	simulator := fs.String("simulator", "", "simulator executable to run (e.g. a Verilator wrapper script)")
	simArgs := fs.String("sim-args", "", "additional simulator arguments (space-separated)")
	expectPath := fs.String("expect", "", "path to file containing expected simulator stdout (optional)")
	fifoSrc := fs.String("fifo-src", "", "deprecated: external FIFO source path (ignored; FIFOs are generated inline)")
	simMaxCycles := fs.Int("sim-max-cycles", 64, "maximum clock cycles to run when using the default Verilator simulator")
	simResetCycles := fs.Int("sim-reset-cycles", 2, "number of initial cycles to hold reset asserted for the default simulator")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return fmt.Errorf("sim requires at least one Go source file")
	}

	inputs := fs.Args()

	if *expectPath == "" && len(inputs) == 1 {
		if candidate := defaultSimExpectPath(inputs[0]); candidate != "" {
			if _, err := os.Stat(candidate); err == nil {
				*expectPath = candidate
			}
		}
	}
	result, err := prepareProgram(inputs, *diagFormat)
	if err != nil {
		return err
	}
	// Keep simulation output focused on runtime behavior; simulation callers care
	// about hard failures, not frontend advisory noise.
	result.reporter.SetMinSeverity(diag.Error)

	if err := validateProgram(result); err != nil {
		return err
	}

	design, err := ir.BuildDesign(result.program, result.reporter, *target)
	if err != nil {
		return err
	}

	if err := runDefaultPasses(design, result.reporter); err != nil {
		return err
	}
	if err := ensureHardwareLowerableDesign(design); err != nil {
		return err
	}

	hasChannels := designHasChannels(design)
	tempRoot := artifactTempRoot(inputs)

	var tempDir string
	if *verilogOut == "" {
		var err error
		tempDir, err = os.MkdirTemp(tempRoot, ".mygo-sim-*")
		if err != nil {
			return err
		}
		if !*keepArtifacts {
			defer os.RemoveAll(tempDir)
		}
	}

	svPath := *verilogOut
	if svPath == "" {
		svPath = filepath.Join(tempDir, "design.sv")
	} else if err := os.MkdirAll(filepath.Dir(svPath), 0o755); err != nil {
		return err
	}

	opts := backend.Options{
		CIRCTOptPath:     *circtOpt,
		PassPipeline:     *circtPipeline,
		LoweringOptions:  *circtLowering,
		DumpMLIRPath:     *circtMLIR,
		KeepTemps:        *keepArtifacts,
		TempRoot:         tempRoot,
		BenchmarkRefPath: *benchmarkRefPath,
		FIFOSource:       *fifoSrc,
	}
	if hasChannels && *fifoSrc != "" {
		fmt.Fprintln(os.Stderr, "warning: --fifo-src is deprecated and ignored; FIFOs are generated inline")
	}

	res, err := emitVerilog(design, svPath, opts)
	if err != nil {
		return err
	}
	svPath = res.MainPath
	auxFiles := append([]string{}, res.AuxPaths...)

	if *simulator == "" {
		constants := []constdata.ArrayConstant{}
		for _, input := range inputs {
			consts, err := constdata.ExtractConstants(input)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not extract constants from %s: %v\n", input, err)
				continue
			}
			constants = append(constants, consts...)
		}
		return runBuiltinVerilator(svPath, auxFiles, *expectPath, *simMaxCycles, *simResetCycles, tempRoot, *keepArtifacts, constants)
	}

	simulatorArgs := parseSimArgs(*simArgs)
	simulatorArgs = append(simulatorArgs, svPath)
	simulatorArgs = append(simulatorArgs, auxFiles...)
	cmd := exec.Command(*simulator, simulatorArgs...)

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("simulator failed: %w", err)
	}

	output := normalizeSimulatorStdout(stdoutBuf.Bytes())

	if *expectPath != "" {
		if err := compareSimulatorOutput(*expectPath, output); err != nil {
			return err
		}
	}

	if _, err := os.Stdout.Write(output); err != nil {
		return fmt.Errorf("write simulator stdout: %w", err)
	}

	return nil
}

func parseSimArgs(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.Fields(raw)
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

func ensureHardwareLowerableDesign(design *ir.Design) error {
	if err := ir.EnsureHardwareLowerableChannels(design); err != nil {
		return fmt.Errorf("unsupported hardware lowering:\n%s", err)
	}
	return nil
}

func designHasChannels(design *ir.Design) bool {
	if design == nil {
		return false
	}
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		if len(module.Channels) > 0 {
			return true
		}
	}
	return false
}

func designHasConcurrentPrints(design *ir.Design) bool {
	if design == nil {
		return false
	}
	printProcesses := 0
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		for _, proc := range module.Processes {
			if !processHasPrints(proc) {
				continue
			}
			printProcesses++
			if proc != nil && proc.Spawned {
				return true
			}
			if printProcesses > 1 {
				return true
			}
		}
	}
	return false
}

func processHasPrints(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			if _, ok := op.(*ir.PrintOperation); ok {
				return true
			}
		}
	}
	return false
}

func defaultSimExpectPath(input string) string {
	if input == "" {
		return ""
	}
	cleaned := filepath.Clean(input)
	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		return filepath.Join(cleaned, "expected.sim")
	}
	dir := filepath.Dir(cleaned)
	return filepath.Join(dir, "expected.sim")
}

func artifactTempRoot(inputs []string) string {
	for _, in := range inputs {
		if dir := resolveInputDir(in); dir != "" {
			return ensureArtifactRoot(dir)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return ensureArtifactRoot(cwd)
	}
	return os.TempDir()
}

func resolveInputDir(input string) string {
	if input == "" {
		return ""
	}
	cleaned := filepath.Clean(input)
	if dir := existingDirectory(cleaned); dir != "" {
		return dir
	}
	parent := filepath.Dir(cleaned)
	return existingDirectory(parent)
}

func existingDirectory(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func ensureArtifactRoot(base string) string {
	if base == "" {
		return os.TempDir()
	}
	root := filepath.Join(base, ".mygo-tmp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return base
	}
	return root
}

func runBuiltinVerilator(mainPath string, auxPaths []string, expectPath string, maxCycles, resetCycles int, tempRoot string, keepArtifacts bool, constants []constdata.ArrayConstant) error {
	if maxCycles <= 0 {
		return fmt.Errorf("default simulator requires --sim-max-cycles > 0 (got %d)", maxCycles)
	}
	if resetCycles < 0 {
		return fmt.Errorf("default simulator requires --sim-reset-cycles >= 0 (got %d)", resetCycles)
	}
	verilatorPath, err := exec.LookPath("verilator")
	if err != nil {
		return fmt.Errorf("resolve verilator: %w", err)
	}

	hasClock, hasReset, err := detectTopModuleClockReset(mainPath)
	if err != nil {
		return fmt.Errorf("detect top module ports: %w", err)
	}
	driver, err := renderVerilatorDriver(maxCycles, resetCycles, hasClock, hasReset, constants)
	if err != nil {
		return fmt.Errorf("render verilator driver: %w", err)
	}

	var tempDir string
	if keepArtifacts {
		tempDir, err = cachedVerilatorTempDir(tempRoot, mainPath, auxPaths, driver)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			return fmt.Errorf("create verilator cache dir: %w", err)
		}
	} else {
		tempDir, err = os.MkdirTemp(tempRoot, ".mygo-verilator-*")
		if err != nil {
			return fmt.Errorf("create verilator temp dir: %w", err)
		}
		defer os.RemoveAll(tempDir)
	}

	buildDir := filepath.Join(tempDir, "verilator")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return fmt.Errorf("create verilator build dir: %w", err)
	}
	driverPath := filepath.Join(buildDir, "sim_main.cpp")
	if err := os.WriteFile(driverPath, []byte(driver), 0o644); err != nil {
		return fmt.Errorf("write verilator driver: %w", err)
	}
	if _, err := installXargsShim(buildDir); err != nil {
		return err
	}

	objDir := filepath.Join(buildDir, "obj_dir")
	simPath := filepath.Join(objDir, "mygo_sim")
	if _, err := os.Stat(simPath); err == nil {
		return runBuiltVerilatorBinary(simPath, expectPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat cached verilator binary: %w", err)
	}
	args := []string{
		"--cc", "--exe", "--build",
		"--sv",
		"-Wno-CMPCONST",
		"-Wno-UNSIGNED",
		"-CFLAGS", "-O0",
		"-CFLAGS", "-g0",
		"--Mdir", objDir,
		"--top-module", "main",
		"-o", "mygo_sim",
	}
	args = append(args, mainPath)
	args = append(args, auxPaths...)
	args = append(args, driverPath)

	cmd := exec.Command(verilatorPath, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = verilatorBuildEnv(buildDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("verilator build failed: %w", err)
	}

	return runBuiltVerilatorBinary(simPath, expectPath)
}

func runBuiltVerilatorBinary(simPath, expectPath string) error {
	simCmd := exec.Command(simPath)
	var stdoutBuf bytes.Buffer
	simCmd.Stdout = &stdoutBuf
	simCmd.Stderr = os.Stderr
	if err := simCmd.Run(); err != nil {
		return fmt.Errorf("verilator simulation failed: %w", err)
	}
	normalizedStdout := normalizeSimulatorStdout(stdoutBuf.Bytes())
	if _, err := os.Stdout.Write(normalizedStdout); err != nil {
		return fmt.Errorf("write simulator stdout: %w", err)
	}
	if expectPath != "" {
		if err := compareSimulatorOutput(expectPath, normalizedStdout); err != nil {
			return err
		}
	}
	return nil
}

func cachedVerilatorTempDir(tempRoot, mainPath string, auxPaths []string, driver string) (string, error) {
	hash := sha256.New()
	addFile := func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read verilator input %s: %w", path, err)
		}
		if _, err := io.WriteString(hash, filepath.Base(path)); err != nil {
			return err
		}
		if _, err := hash.Write([]byte{0}); err != nil {
			return err
		}
		if _, err := hash.Write(data); err != nil {
			return err
		}
		_, err = hash.Write([]byte{0})
		return err
	}
	if err := addFile(mainPath); err != nil {
		return "", err
	}
	for _, aux := range auxPaths {
		if err := addFile(aux); err != nil {
			return "", err
		}
	}
	if _, err := io.WriteString(hash, driver); err != nil {
		return "", err
	}
	sum := fmt.Sprintf("%x", hash.Sum(nil))
	return filepath.Join(tempRoot, ".mygo-verilator-cache-"+sum[:16]), nil
}

func detectTopModuleClockReset(mainPath string) (bool, bool, error) {
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return false, false, err
	}
	text := string(data)
	start := strings.Index(text, "module main(")
	if start < 0 {
		return false, false, nil
	}
	rest := text[start:]
	end := strings.Index(rest, ");")
	if end < 0 {
		return false, false, nil
	}
	header := rest[:end]
	hasClock := strings.Contains(header, " clk") || strings.Contains(header, "(clk") || strings.Contains(header, "\tclk")
	hasReset := strings.Contains(header, " rst") || strings.Contains(header, "(rst") || strings.Contains(header, "\trst")
	return hasClock, hasReset, nil
}

func normalizeSimulatorStdout(data []byte) []byte {
	replacer := strings.NewReplacer(
		"(nan)", "(NaN)",
		"(-nan)", "(NaN)",
		"(inf)", "(+Inf)",
		"(-inf)", "(-Inf)",
	)
	text := replacer.Replace(string(data))
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = normalizeHexByteRunLine(line)
	}
	return []byte(strings.Join(lines, "\n"))
}

func verilatorBuildEnv(buildDir string) []string {
	env := prependPathToEnv(buildDir)
	jobs := runtime.NumCPU()
	if jobs < 1 {
		jobs = 1
	}
	if jobs > 8 {
		jobs = 8
	}
	env = append(env, fmt.Sprintf("MAKEFLAGS=-j%d", jobs))
	return env
}

func outputsDifferOnlyByLineOrder(want, got []byte) bool {
	wantLines := normalizedOutputLines(want)
	gotLines := normalizedOutputLines(got)
	if len(wantLines) == 0 || len(wantLines) != len(gotLines) {
		return false
	}
	if sameStringSlices(wantLines, gotLines) {
		return false
	}
	wantSorted := append([]string(nil), wantLines...)
	gotSorted := append([]string(nil), gotLines...)
	sort.Strings(wantSorted)
	sort.Strings(gotSorted)
	return sameStringSlices(wantSorted, gotSorted)
}

func normalizedOutputLines(data []byte) []string {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func sameStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeHexByteRunLine(line string) string {
	tab := strings.IndexByte(line, '\t')
	if tab < 0 || tab+1 >= len(line) {
		return line
	}
	body := strings.TrimSpace(line[tab+1:])
	if body == "" {
		return line
	}
	for _, ch := range body {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return line
		}
	}
	normalized, ok := decodeZeroPaddedHexBytes(body)
	if !ok {
		return line
	}
	return line[:tab+1] + normalized
}

func decodeZeroPaddedHexBytes(body string) (string, bool) {
	if len(body) < 8 {
		return "", false
	}
	var out strings.Builder
	for i := 0; i < len(body); {
		switch {
		case i+9 <= len(body) && body[i:i+8] == "00000000":
			out.WriteByte('0')
			out.WriteByte(asciiLowerHex(body[i+8]))
			i += 9
		case i+8 <= len(body) && body[i:i+6] == "000000":
			out.WriteByte(asciiLowerHex(body[i+6]))
			out.WriteByte(asciiLowerHex(body[i+7]))
			i += 8
		default:
			return "", false
		}
	}
	return out.String(), true
}

func asciiLowerHex(ch byte) byte {
	if ch >= 'A' && ch <= 'F' {
		return ch + ('a' - 'A')
	}
	return ch
}

func compareSimulatorOutput(expectPath string, got []byte) error {
	want, err := os.ReadFile(expectPath)
	if err != nil {
		return fmt.Errorf("read expect file: %w", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		return fmt.Errorf("simulator output mismatch\nexpected:\n%s\nactual:\n%s", string(want), string(got))
	}
	return nil
}

func prependPathToEnv(dir string) []string {
	env := os.Environ()
	newPath := dir
	currentPath := os.Getenv("PATH")
	if currentPath != "" {
		newPath = dir + string(os.PathListSeparator) + currentPath
	}
	replaced := false
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + newPath
			replaced = true
			break
		}
	}
	if !replaced {
		env = append(env, "PATH="+newPath)
	}
	return env
}

func emitSSAProgram(prog *ssa.Program, outputPath string) error {
	return withOutputWriter(outputPath, func(w io.Writer) error {
		pkgs := sortedSSAPackages(prog)
		if len(pkgs) == 0 {
			return fmt.Errorf("no SSA packages available to emit")
		}
		for i, pkg := range pkgs {
			if i > 0 {
				fmt.Fprintln(w)
			}
			if _, err := pkg.WriteTo(w); err != nil {
				return err
			}
		}
		return nil
	})
}

func emitIRDesign(design *ir.Design, outputPath string) error {
	if design == nil {
		return fmt.Errorf("no IR design available to emit")
	}
	return withOutputWriter(outputPath, func(w io.Writer) error {
		ir.Dump(design, w)
		return nil
	})
}

func sortedSSAPackages(prog *ssa.Program) []*ssa.Package {
	if prog == nil {
		return nil
	}
	all := prog.AllPackages()
	pkgs := make([]*ssa.Package, 0, len(all))
	for _, pkg := range all {
		if pkg == nil {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return packageSortKey(pkgs[i]) < packageSortKey(pkgs[j])
	})
	return pkgs
}

func packageSortKey(pkg *ssa.Package) string {
	if pkg == nil {
		return ""
	}
	if pkg.Pkg != nil {
		return pkg.Pkg.Path()
	}
	return pkg.String()
}

func withOutputWriter(path string, fn func(io.Writer) error) error {
	w, cleanup, err := outputWriter(path)
	if err != nil {
		return err
	}
	if cleanup == nil {
		return fn(w)
	}
	err = fn(w)
	if closeErr := cleanup(); err == nil && closeErr != nil {
		err = closeErr
	}
	return err
}

func outputWriter(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, nil, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
