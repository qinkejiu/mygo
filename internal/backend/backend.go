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

const defaultVerilogPassPipeline = "builtin.module(lower-seq-to-sv,hw.module(lower-hw-to-sv))"

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
	if err := applyLoopFSMVerilog(design, outputPath); err != nil {
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
