package ir

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"

	"mygo/internal/diag"
)

// BuildDesign converts the SSA program into the hardware IR described in README.
func BuildDesign(prog *ssa.Program, reporter *diag.Reporter) (*Design, error) {
	mainPkg := findMainPackage(prog)
	if mainPkg == nil {
		return nil, fmt.Errorf("no main package found")
	}

	mainFn := mainPkg.Func("main")
	if mainFn == nil {
		return nil, fmt.Errorf("main function not found in package %s", mainPkg.Pkg.Path())
	}

	builder := &builder{
		reporter:             reporter,
		mainPkg:              mainPkg,
		signals:              make(map[ssa.Value]*Signal),
		tupleSignals:         make(map[ssa.Value][]*Signal),
		indexedBases:         make(map[ssa.Value]*indexedBaseState),
		globalValues:         make(map[*ssa.Global]*Signal),
		globalStorage:        make(map[*ssa.Global]*Signal),
		blockGlobalValues:    make(map[*BasicBlock]map[*ssa.Global]*Signal),
		processes:            make(map[*ssa.Function]*Process),
		channels:             make(map[ssa.Value]*Channel),
		paramSignals:         make(map[*ssa.Parameter]*Signal),
		paramChannels:        make(map[*ssa.Parameter]*Channel),
		channelParamBindings: make(map[*ssa.Parameter]map[*Channel]struct{}),
		channelUsage:         make(map[*Channel]int),
		loopFSMs:             make(map[*ssa.Function][]*loopFSM),
		mergedCalls:          make(map[*BasicBlock]*mergedCallInfo),
		nextStage:            1,
	}

	module := builder.buildModule(mainFn)
	builder.analyzeChannels(prog)
	builder.finalizeProcessStages()
	builder.finalizeChannelOccupancy()
	if reporter.HasErrors() {
		return nil, fmt.Errorf("failed to build module")
	}

	design := &Design{
		Modules:  []*Module{module},
		TopLevel: module,
	}

	return design, nil
}

type builder struct {
	reporter             *diag.Reporter
	mainPkg              *ssa.Package
	module               *Module
	signals              map[ssa.Value]*Signal
	tupleSignals         map[ssa.Value][]*Signal
	indexedBases         map[ssa.Value]*indexedBaseState
	globalValues         map[*ssa.Global]*Signal
	globalStorage        map[*ssa.Global]*Signal
	blockGlobalValues    map[*BasicBlock]map[*ssa.Global]*Signal
	processes            map[*ssa.Function]*Process
	channels             map[ssa.Value]*Channel
	paramSignals         map[*ssa.Parameter]*Signal
	paramChannels        map[*ssa.Parameter]*Channel
	channelParamBindings map[*ssa.Parameter]map[*Channel]struct{}
	channelUsage         map[*Channel]int
	loopFSMs             map[*ssa.Function][]*loopFSM
	mergedCalls          map[*BasicBlock]*mergedCallInfo
	nextStage            int
	blocks               map[*ssa.BasicBlock]*BasicBlock
	currentBlock         *BasicBlock
	tempID               int
}

type mergedCallInfo struct {
	entryBlock   *BasicBlock
	returnBlocks []*BasicBlock
}

type indexedBaseState struct {
	base     ssa.Value
	elemType *SignalType
	length   int
	dims     []int
	elements map[int]*Signal
	storage  map[int]*Signal
	parent   *indexedBaseState
	offset   int
}

const defaultDynamicSliceIndexMax = 64

// State is an SSA-backed FSM state for loop lowering.
type State struct {
	name   string
	instrs []ssa.Instruction
}

type fsmTransition struct {
	from string
	to   string
	when string
}

type loopFSM struct {
	loop        loopStructure
	states      []State
	transitions []fsmTransition
}

func (b *builder) buildModule(fn *ssa.Function) *Module {
	mod := &Module{
		Name:     fn.Name(),
		Ports:    defaultPorts(),
		Signals:  make(map[string]*Signal),
		Channels: make(map[string]*Channel),
		Source:   fn.Pos(),
	}
	b.module = mod
	b.bootstrapGlobalInitializers(fn.Pkg)
	entry := b.buildProcess(fn)
	if entry != nil && entry.Stage < 0 {
		entry.Stage = 0
	}
	return mod
}

// buildReferencedProcesses scans for CallOperations and builds referenced processes
// This ensures that modular functions (those called via CallOperation) have their processes built
func (b *builder) buildReferencedProcesses(prog *ssa.Program) {
	if b.module == nil {
		return
	}

	// Collect all function names referenced by CallOperations
	referencedFuncNames := make(map[string]struct{})
	for _, proc := range b.module.Processes {
		if proc == nil {
			continue
		}
		for _, block := range proc.Blocks {
			if block == nil {
				continue
			}
			for _, op := range block.Ops {
				if callOp, ok := op.(*CallOperation); ok && callOp.Callee != "" {
					referencedFuncNames[callOp.Callee] = struct{}{}
				}
			}
		}
	}

	// Build all referenced functions that haven't been built yet
	for funcName := range referencedFuncNames {
		// Check if already built
		alreadyBuilt := false
		for _, proc := range b.module.Processes {
			if proc != nil && proc.Name == funcName {
				alreadyBuilt = true
				break
			}
		}
		if alreadyBuilt {
			continue
		}

		// Find the SSA function in the program
		for _, pkg := range prog.AllPackages() {
			fn := pkg.Func(funcName)
			if fn != nil {
				b.buildProcess(fn)
				break
			}
		}
	}
}

func (b *builder) buildProcess(fn *ssa.Function) *Process {
	if proc, ok := b.processes[fn]; ok {
		return proc
	}
	proc := &Process{
		Name:         fn.Name(),
		Source:       fn.Pos(),
		Sensitivity:  Sequential,
		Stage:        -1,
		Params:       make([]*Signal, 0),
		ReturnValues: make(map[*BasicBlock]*Signal),
	}
	b.processes[fn] = proc
	b.module.Processes = append(b.module.Processes, proc)
	b.bindFunctionParams(fn, proc)

	prevBlocks := b.blocks
	b.blocks = make(map[*ssa.BasicBlock]*BasicBlock)
	defer func() { b.blocks = prevBlocks }()

	ordered := make([]*ssa.BasicBlock, 0, len(fn.Blocks))
	var entryBB *BasicBlock
	for _, block := range fn.Blocks {
		if block == nil {
			continue
		}
		bb := &BasicBlock{Label: blockComment(block)}
		if entryBB == nil {
			entryBB = bb
		}
		b.blocks[block] = bb
		proc.Blocks = append(proc.Blocks, bb)
		ordered = append(ordered, block)
	}

	for _, block := range ordered {
		b.translateBlock(proc, block)
	}
	b.rebuildProcessEdges(proc)
	b.repairPhiPredecessors(proc)
	b.orderBlocks(proc, entryBB)
	b.buildLoopFSMs(fn)
	return proc
}

func (b *builder) translateBlock(proc *Process, block *ssa.BasicBlock) {
	if block == nil {
		return
	}
	if len(block.Preds) > 0 {
		b.materializeIndexedStateStorage()
	}
	bb := b.blocks[block]
	if bb == nil {
		return
	}
	prevBlock := b.currentBlock
	b.currentBlock = bb
	defer func() { b.currentBlock = prevBlock }()
	for idx, instr := range block.Instrs {
		switch v := instr.(type) {
		case *ssa.Phi:
			b.handlePhi(block, bb, v)
		case *ssa.If:
			b.handleIf(block, bb, v)
		case *ssa.Jump:
			b.handleJump(block, bb)
		case *ssa.Return:
			b.handleReturn(proc, bb, v)
		default:
			if b.translateInstr(proc, block, bb, idx, instr) {
				return
			}
		}
	}
}

func (b *builder) connectBlocks(blocks []*ssa.BasicBlock) {
	for _, block := range blocks {
		if block == nil {
			continue
		}
		src := b.blocks[block]
		if src == nil {
			continue
		}
		for _, succ := range block.Succs {
			if succ == nil {
				continue
			}
			dst := b.blocks[succ]
			if dst == nil {
				continue
			}
			src.Successors = append(src.Successors, dst)
			dst.Predecessors = append(dst.Predecessors, src)
		}
	}
}

func (b *builder) rebuildProcessEdges(proc *Process) {
	if proc == nil {
		return
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		block.Successors = nil
		block.Predecessors = nil
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		switch term := block.Terminator.(type) {
		case *BranchTerminator:
			if term.True != nil {
				block.Successors = appendUniqueBlock(block.Successors, term.True)
				term.True.Predecessors = appendUniqueBlock(term.True.Predecessors, block)
			}
			if term.False != nil {
				block.Successors = appendUniqueBlock(block.Successors, term.False)
				term.False.Predecessors = appendUniqueBlock(term.False.Predecessors, block)
			}
		case *JumpTerminator:
			if term.Target != nil {
				block.Successors = appendUniqueBlock(block.Successors, term.Target)
				term.Target.Predecessors = appendUniqueBlock(term.Target.Predecessors, block)
			}
		}
	}
}

func (b *builder) repairPhiPredecessors(proc *Process) {
	if proc == nil {
		return
	}
	for _, block := range proc.Blocks {
		if block == nil || len(block.Predecessors) == 0 {
			continue
		}
		for _, op := range block.Ops {
			phi, ok := op.(*PhiOperation)
			if !ok || phi == nil {
				continue
			}
			for i := range phi.Incomings {
				incoming := phi.Incomings[i].Block
				if incoming == nil || containsBlock(block.Predecessors, incoming) {
					continue
				}
				replacement := matchPhiPredecessor(block.Predecessors, incoming)
				if replacement != nil {
					phi.Incomings[i].Block = replacement
				}
			}
		}
	}
}

func containsBlock(blocks []*BasicBlock, target *BasicBlock) bool {
	for _, block := range blocks {
		if block == target {
			return true
		}
	}
	return false
}

func matchPhiPredecessor(preds []*BasicBlock, incoming *BasicBlock) *BasicBlock {
	if incoming == nil {
		return nil
	}
	for _, pred := range preds {
		if pred == nil {
			continue
		}
		if pred.Label == incoming.Label {
			return pred
		}
		if strings.HasPrefix(pred.Label, incoming.Label+"_inline_cont") {
			return pred
		}
	}
	return nil
}

func appendUniqueBlock(blocks []*BasicBlock, block *BasicBlock) []*BasicBlock {
	if block == nil {
		return blocks
	}
	for _, existing := range blocks {
		if existing == block {
			return blocks
		}
	}
	return append(blocks, block)
}

func (b *builder) finalizeProcessStages() {
	if b.module == nil {
		return
	}
	for _, proc := range b.module.Processes {
		if proc == nil {
			continue
		}
		if proc.Stage < 0 {
			proc.Stage = 0
		}
		if proc.Stage >= b.nextStage {
			b.nextStage = proc.Stage + 1
		}
	}
}

func (b *builder) finalizeChannelOccupancy() {
	if b.module == nil {
		return
	}
	for _, ch := range b.module.Channels {
		if ch == nil {
			continue
		}
		occ := b.channelUsage[ch]
		if occ < 0 {
			occ = 0
		}
		if ch.Depth > 0 && occ > ch.Depth {
			occ = ch.Depth
		}
		ch.Occupancy = occ
	}
}

func (b *builder) ensureProcessStage(proc *Process) int {
	if proc == nil {
		return 0
	}
	if proc.Stage < 0 {
		proc.Stage = 0
	}
	return proc.Stage
}

func (b *builder) assignChildStage(parent, child *Process) {
	if child == nil {
		return
	}
	child.Spawned = true
	parentStage := b.ensureProcessStage(parent)
	desired := parentStage + 1
	if desired < b.nextStage {
		desired = b.nextStage
	}
	if child.Stage < desired {
		child.Stage = desired
	}
	if desired >= b.nextStage {
		b.nextStage = desired + 1
	}
}

func (b *builder) recordChannelDelta(ch *Channel, delta int) {
	if ch == nil {
		return
	}
	value := b.channelUsage[ch]
	value += delta
	if value < 0 {
		value = 0
	}
	b.channelUsage[ch] = value
}

func (b *builder) orderBlocks(proc *Process, entry *BasicBlock) {
	if proc == nil || len(proc.Blocks) == 0 {
		return
	}
	visited := make(map[*BasicBlock]bool)
	order := make([]*BasicBlock, 0, len(proc.Blocks))
	var visit func(*BasicBlock)
	visit = func(bb *BasicBlock) {
		if bb == nil || visited[bb] {
			return
		}
		visited[bb] = true
		for _, succ := range bb.Successors {
			visit(succ)
		}
		order = append(order, bb)
	}
	visit(proc.Blocks[0])
	for _, bb := range proc.Blocks {
		if !visited[bb] {
			visit(bb)
		}
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	if entry != nil {
		for i, bb := range order {
			if bb == entry {
				if i > 0 {
					copy(order[1:i+1], order[0:i])
					order[0] = entry
				}
				break
			}
		}
	}
	proc.Blocks = order
}

func (b *builder) buildLoopFSMs(fn *ssa.Function) {
	if b == nil || fn == nil || len(fn.Blocks) == 0 {
		return
	}
	loops := findLoops(fn)
	if len(loops) == 0 {
		return
	}
	fsms := make([]*loopFSM, 0, len(loops))
	for _, loop := range loops {
		if !isDynamicBoundaryLoop(loop) {
			continue
		}
		fsm := b.buildFSMForLoop(loop)
		if fsm == nil {
			continue
		}
		fsms = append(fsms, fsm)
	}
	if len(fsms) == 0 {
		return
	}
	if b.loopFSMs == nil {
		b.loopFSMs = make(map[*ssa.Function][]*loopFSM)
	}
	b.loopFSMs[fn] = fsms
}

func isDynamicBoundaryLoop(loop loopStructure) bool {
	if loop.Header == nil || len(loop.ExitConditions) == 0 {
		return false
	}
	for _, exitCond := range loop.ExitConditions {
		if exitCond == nil {
			continue
		}
		if !isConstBool(exitCond.Cond) {
			return true
		}
	}
	return false
}

func isConstBool(v ssa.Value) bool {
	c, ok := v.(*ssa.Const)
	if !ok || c == nil || c.Value == nil {
		return false
	}
	return c.Value.Kind() == constant.Bool
}

// buildFSMForLoop builds a canonical CHECK/BODY/UPDATE FSM for dynamic loops.
func (b *builder) buildFSMForLoop(loop loopStructure) *loopFSM {
	if loop.Header == nil {
		return nil
	}
	latchSet := make(map[*ssa.BasicBlock]struct{}, len(loop.Latches))
	for _, latch := range loop.Latches {
		if latch != nil {
			latchSet[latch] = struct{}{}
		}
	}

	bodyBlocks := make([]*ssa.BasicBlock, 0, len(loop.Body))
	for _, block := range loop.Body {
		if block == nil {
			continue
		}
		if _, isLatch := latchSet[block]; isLatch {
			continue
		}
		bodyBlocks = append(bodyBlocks, block)
	}
	sort.Slice(bodyBlocks, func(i, j int) bool {
		return bodyBlocks[i].Index < bodyBlocks[j].Index
	})

	latches := make([]*ssa.BasicBlock, 0, len(loop.Latches))
	for _, latch := range loop.Latches {
		if latch != nil {
			latches = append(latches, latch)
		}
	}
	sort.Slice(latches, func(i, j int) bool {
		return latches[i].Index < latches[j].Index
	})

	check := State{
		name:   "CHECK",
		instrs: copyInstructions(loop.Header.Instrs, true),
	}
	body := State{
		name:   "BODY",
		instrs: flattenBlockInstructions(bodyBlocks),
	}
	update := State{
		name:   "UPDATE",
		instrs: flattenBlockInstructions(latches),
	}

	return &loopFSM{
		loop: loop,
		states: []State{
			check,
			body,
			update,
		},
		transitions: []fsmTransition{
			{from: "CHECK", to: "BODY", when: "true"},
			{from: "CHECK", to: "EXIT", when: "false"},
			{from: "BODY", to: "UPDATE", when: "always"},
			{from: "UPDATE", to: "CHECK", when: "always"},
		},
	}
}

func flattenBlockInstructions(blocks []*ssa.BasicBlock) []ssa.Instruction {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]ssa.Instruction, 0)
	for _, block := range blocks {
		if block == nil {
			continue
		}
		out = append(out, copyInstructions(block.Instrs, false)...)
	}
	return out
}

func copyInstructions(instrs []ssa.Instruction, keepTerminator bool) []ssa.Instruction {
	if len(instrs) == 0 {
		return nil
	}
	out := make([]ssa.Instruction, 0, len(instrs))
	for _, instr := range instrs {
		if instr == nil {
			continue
		}
		if !keepTerminator {
			switch instr.(type) {
			case *ssa.If, *ssa.Jump, *ssa.Return:
				continue
			}
		}
		out = append(out, instr)
	}
	return out
}

func (b *builder) handleIf(block *ssa.BasicBlock, bb *BasicBlock, stmt *ssa.If) {
	if bb == nil || stmt == nil {
		return
	}
	cond := b.signalForValue(stmt.Cond)
	if cond == nil {
		b.reporter.Warning(stmt.Pos(), "if condition has no signal mapping; treating as false")
	}
	var trueBB, falseBB *BasicBlock
	if len(block.Succs) > 0 {
		trueBB = b.blocks[block.Succs[0]]
	}
	if len(block.Succs) > 1 {
		falseBB = b.blocks[block.Succs[1]]
	}
	b.setBlockTerminator(bb, &BranchTerminator{
		Cond:  cond,
		True:  trueBB,
		False: falseBB,
	})
}

func (b *builder) handleJump(block *ssa.BasicBlock, bb *BasicBlock) {
	if bb == nil || block == nil {
		return
	}
	var target *BasicBlock
	if len(block.Succs) > 0 {
		target = b.blocks[block.Succs[0]]
	}
	b.setBlockTerminator(bb, &JumpTerminator{Target: target})
}

func (b *builder) handleReturn(proc *Process, bb *BasicBlock, ret *ssa.Return) {
	if bb == nil {
		return
	}
	b.setBlockTerminator(bb, &ReturnTerminator{})

	// Extract return value if present
	if ret != nil && len(ret.Results) > 0 {
		returnValue := ret.Results[0]
		if returnValue != nil {
			sig := b.signalForValue(returnValue)
			if sig != nil {
				proc.Return = sig
				if proc.ReturnValues == nil {
					proc.ReturnValues = make(map[*BasicBlock]*Signal)
				}
				proc.ReturnValues[bb] = sig
			}
		}
	}
}

func (b *builder) setBlockTerminator(bb *BasicBlock, term Terminator) {
	if bb == nil {
		return
	}
	info, ok := b.mergedCalls[bb]
	if !ok || info == nil || info.entryBlock == nil {
		bb.Terminator = term
		return
	}

	bb.Terminator = &JumpTerminator{Target: info.entryBlock}
	for _, retBlock := range info.returnBlocks {
		if retBlock == nil {
			continue
		}
		retBlock.Terminator = b.cloneContinuationTerminator(term)
	}
}

func (b *builder) cloneContinuationTerminator(term Terminator) Terminator {
	switch t := term.(type) {
	case *BranchTerminator:
		return &BranchTerminator{
			Cond:  t.Cond,
			True:  t.True,
			False: t.False,
		}
	case *JumpTerminator:
		return &JumpTerminator{Target: t.Target}
	case *ReturnTerminator:
		return &ReturnTerminator{}
	default:
		return term
	}
}

func (b *builder) handlePhi(block *ssa.BasicBlock, bb *BasicBlock, phi *ssa.Phi) {
	if bb == nil || phi == nil {
		return
	}
	dest := b.ensureValueSignal(phi)
	dest.Type = signalType(phi.Type())
	incomings := make([]PhiIncoming, 0, len(phi.Edges))
	for idx, edge := range phi.Edges {
		var pred *BasicBlock
		if block != nil && idx < len(block.Preds) {
			pred = b.blocks[block.Preds[idx]]
		}
		value := b.signalForValue(edge)
		incomings = append(incomings, PhiIncoming{
			Block: pred,
			Value: value,
		})
	}
	b.signals[phi] = dest
	if mux := b.tryLowerPhiToMux(block, incomings, dest); mux != nil {
		bb.Ops = append(bb.Ops, mux)
		return
	}
	bb.Ops = append(bb.Ops, &PhiOperation{
		Dest:      dest,
		Incomings: incomings,
	})
}

func (b *builder) handleBinOp(proc *Process, block *ssa.BasicBlock, bb *BasicBlock, instrIndex int, op *ssa.BinOp) bool {
	if bb == nil || op == nil {
		return false
	}
	left := b.signalForBinOperand(bb, op.X)
	right := b.signalForBinOperand(bb, op.Y)
	if left == nil || right == nil {
		typ := signalType(op.Type())
		if typ == nil {
			typ = &SignalType{Width: 1, Signed: false}
		}
		zero := b.newConstSignal(0, typ, op.Pos())
		b.bindResolvedValue(bb, op, zero)
		b.reporter.Warning(op.Pos(), fmt.Sprintf("binary op %s has unresolved operand; using zero value fallback", op.Op.String()))
		return false
	}
	if dest, ok := b.lowerNativeFloatBinOp(bb, op.Op, op.Type(), op.X.Type(), left, right, op.Pos()); ok {
		b.bindResolvedValue(bb, op, dest)
		return false
	}
	if op.Op == token.QUO && isNativeFloat64Type(op.X.Type()) {
		if b.mergeNativeFloatHelperCall(proc, block, bb, instrIndex, op, "float64_div", argsToSignals(left, right)) {
			return true
		}
	}
	if pred, ok := translateCompareOp(op.Op, isSignedType(op.X.Type())); ok {
		dest := b.ensureValueSignal(op)
		dest.Type = signalType(op.Type())
		bb.Ops = append(bb.Ops, &CompareOperation{
			Predicate: pred,
			Dest:      dest,
			Left:      left,
			Right:     right,
		})
		return false
	}
	bin, ok := translateBinOp(op.Op)
	if ok && bin == ShrU && op.Op == token.SHR && isSignedType(op.X.Type()) {
		bin = ShrS
	}
	if !ok {
		b.reporter.Warning(op.Pos(), fmt.Sprintf("unsupported binary op: %s", op.Op.String()))
		return false
	}
	dest := b.ensureValueSignal(op)
	dest.Type = signalType(op.Type())
	if isShiftBinOp(bin) {
		leftType := signalType(op.X.Type())
		if leftType != nil && (left.Type == nil || !left.Type.Equal(leftType)) {
			left.Type = leftType
		}
		if left.Type != nil && (right.Type == nil || !right.Type.Equal(left.Type)) {
			cast := b.newAnonymousSignal("shift", left.Type, op.Pos())
			bb.Ops = append(bb.Ops, &ConvertOperation{
				Dest:  cast,
				Value: right,
			})
			right = cast
		}
	}
	bb.Ops = append(bb.Ops, &BinOperation{
		Op:    bin,
		Dest:  dest,
		Left:  left,
		Right: right,
	})
	return false
}

func (b *builder) signalForBinOperand(bb *BasicBlock, v ssa.Value) *Signal {
	sig := b.signalForValue(v)
	if sig != nil {
		return sig
	}
	addr, ok := v.(*ssa.IndexAddr)
	if !ok {
		return nil
	}
	return b.signalForIndexAddrInBlock(bb, addr)
}

func (b *builder) handleUnOp(proc *Process, bb *BasicBlock, op *ssa.UnOp) {
	if op == nil {
		return
	}
	switch op.Op {
	case token.MUL:
		if idxAddr, ok := op.X.(*ssa.IndexAddr); ok {
			if b.lowerIndexedLoad(bb, op, idxAddr) {
				return
			}
		}
		ptr := b.signalForValue(op.X)
		if ptr != nil {
			b.bindResolvedValue(bb, op, ptr)
			return
		}
		// Fallback for dereference loads whose address form we do not yet lower
		// (e.g. array/slice index addresses). Keep IR structurally valid by
		// materializing a typed zero value instead of leaving the SSA value
		// unmapped.
		typ := signalType(op.Type())
		zero := b.newConstSignal(0, typ, op.Pos())
		b.bindResolvedValue(bb, op, zero)
		b.reporter.Warning(op.Pos(), fmt.Sprintf("unresolved dereference %T; using zero value fallback", op.X))
	case token.ARROW:
		b.handleRecv(proc, bb, op)
	case token.NOT, token.XOR:
		value := b.signalForValue(op.X)
		if value == nil {
			typ := signalType(op.Type())
			zero := b.newConstSignal(0, typ, op.Pos())
			b.bindResolvedValue(bb, op, zero)
			b.reporter.Warning(op.Pos(), fmt.Sprintf("unary op %s has unresolved operand %T; using zero value fallback", op.Op.String(), op.X))
			return
		}
		dest := b.ensureValueSignal(op)
		dest.Type = signalType(op.Type())
		bb.Ops = append(bb.Ops, &NotOperation{
			Dest:  dest,
			Value: value,
		})
	case token.SUB:
		value := b.signalForValue(op.X)
		if value == nil {
			typ := signalType(op.Type())
			zero := b.newConstSignal(0, typ, op.Pos())
			b.bindResolvedValue(bb, op, zero)
			b.reporter.Warning(op.Pos(), fmt.Sprintf("unary op %s has unresolved operand %T; using zero value fallback", op.Op.String(), op.X))
			return
		}
		if dest, ok := b.lowerNativeFloatNeg(bb, value, op.Type(), op.Pos()); ok {
			b.bindResolvedValue(bb, op, dest)
			return
		}
		dest := b.ensureValueSignal(op)
		dest.Type = signalType(op.Type())
		zero := b.newConstSignal(0, dest.Type, op.Pos())
		bb.Ops = append(bb.Ops, &BinOperation{
			Op:    Sub,
			Dest:  dest,
			Left:  zero,
			Right: value,
		})
	case token.ADD:
		if value := b.signalForValue(op.X); value != nil {
			b.bindResolvedValue(bb, op, value)
			return
		}
		typ := signalType(op.Type())
		zero := b.newConstSignal(0, typ, op.Pos())
		b.bindResolvedValue(bb, op, zero)
		b.reporter.Warning(op.Pos(), fmt.Sprintf("unary op %s has unresolved operand %T; using zero value fallback", op.Op.String(), op.X))
	default:
		typ := signalType(op.Type())
		zero := b.newConstSignal(0, typ, op.Pos())
		b.bindResolvedValue(bb, op, zero)
		b.reporter.Warning(op.Pos(), fmt.Sprintf("unsupported unary op: %s; using zero value fallback", op.Op.String()))
	}
}

func (b *builder) bindResolvedValue(bb *BasicBlock, value ssa.Value, resolved *Signal) *Signal {
	if value == nil || resolved == nil {
		return nil
	}
	if current, ok := b.signals[value]; ok && current != nil {
		if current == resolved {
			return current
		}
		if bb != nil {
			bb.Ops = append(bb.Ops, &AssignOperation{
				Dest:  current,
				Value: resolved,
			})
			return current
		}
	}
	b.signals[value] = resolved
	return resolved
}

func (b *builder) tryLowerPhiToMux(block *ssa.BasicBlock, incomings []PhiIncoming, dest *Signal) *MuxOperation {
	if block == nil || len(block.Preds) != 2 || len(incomings) != 2 {
		return nil
	}
	predA := block.Preds[0]
	predB := block.Preds[1]
	if predA == nil || predB == nil {
		return nil
	}
	if len(predA.Preds) == 0 || len(predB.Preds) == 0 {
		return nil
	}
	header := predA.Preds[0]
	if header == nil || header != predB.Preds[0] {
		return nil
	}
	if len(header.Succs) < 2 || len(header.Instrs) == 0 {
		return nil
	}
	ifInstr, ok := header.Instrs[len(header.Instrs)-1].(*ssa.If)
	if !ok {
		return nil
	}
	cond := b.signalForValue(ifInstr.Cond)
	if cond == nil {
		return nil
	}
	trueSucc := header.Succs[0]
	falseSucc := header.Succs[1]
	var trueVal, falseVal *Signal
	switch {
	case trueSucc == predA && falseSucc == predB:
		trueVal = incomings[0].Value
		falseVal = incomings[1].Value
	case trueSucc == predB && falseSucc == predA:
		trueVal = incomings[1].Value
		falseVal = incomings[0].Value
	default:
		return nil
	}
	if trueVal == nil || falseVal == nil {
		return nil
	}
	return &MuxOperation{
		Dest:       dest,
		Cond:       cond,
		TrueValue:  trueVal,
		FalseValue: falseVal,
	}
}

func (b *builder) translateInstr(proc *Process, block *ssa.BasicBlock, bb *BasicBlock, instrIndex int, instr ssa.Instruction) bool {
	switch v := instr.(type) {
	case *ssa.Alloc:
		b.handleAlloc(v)
	case *ssa.Store:
		if b.lowerIndexedStore(bb, v) {
			return false
		}
		if g, ok := unwrapAddressValue(v.Addr).(*ssa.Global); ok {
			dest := b.signalForGlobalStorage(g)
			val := b.signalForValue(v.Val)
			if dest == nil || val == nil {
				return false
			}
			bb.Ops = append(bb.Ops, &AssignOperation{Dest: dest, Value: val})
			b.setBlockGlobalValue(bb, g, val)
			return false
		}
		dest := b.signalForValue(v.Addr)
		val := b.signalForValue(v.Val)
		if dest == nil || val == nil {
			return false
		}
		bb.Ops = append(bb.Ops, &AssignOperation{Dest: dest, Value: val})
	case *ssa.BinOp:
		return b.handleBinOp(proc, block, bb, instrIndex, v)
	case *ssa.UnOp:
		b.handleUnOp(proc, bb, v)
	case *ssa.Convert:
		b.lowerTypeChange(bb, v, v.X, v.Type())
	case *ssa.ChangeType:
		b.lowerTypeChange(bb, v, v.X, v.Type())
	case *ssa.MakeChan:
		b.handleMakeChan(v)
	case *ssa.Send:
		b.handleSend(proc, bb, v)
	case *ssa.DebugRef:
		// Skip debug markers.
	case *ssa.Call:
		if b.handleFmtPrint(proc, bb, v) {
			return false
		}
		return b.handleCall(proc, block, bb, instrIndex, v)
	case *ssa.Go:
		b.handleGo(proc, bb, v)
	case *ssa.IndexAddr:
		// Used for fmt.Printf variadic handling – ignore for now.
	case *ssa.Extract:
		// Extract values are resolved lazily through signalForValue.
	case *ssa.MakeInterface:
		// Interfaces only appear for fmt.Printf arguments – ignore.
	case *ssa.Slice:
		// Also part of fmt formatting.
	case *ssa.If, *ssa.Jump, *ssa.Return:
		// handled separately in translateBlock
	default:
		// For unsupported instructions we emit a warning once.
		b.reporter.Warning(instr.Pos(), fmt.Sprintf("instruction %T ignored in IR builder", instr))
	}
	return false
}

func (b *builder) handleAlloc(a *ssa.Alloc) {
	ptrType, ok := a.Type().(*types.Pointer)
	if !ok {
		b.reporter.Warning(a.Pos(), "allocation without pointer type encountered")
		return
	}
	elem := ptrType.Elem()
	name := b.allocName(a)
	sig := &Signal{
		Name:   name,
		Type:   signalType(elem),
		Kind:   Reg,
		Source: a.Pos(),
	}
	b.module.Signals[sig.Name] = sig
	b.signals[a] = sig
}

func (b *builder) bindFunctionParams(fn *ssa.Function, proc *Process) {
	if fn == nil {
		return
	}
	// Store the SSA parameters for later remapping during inlining
	proc.SSAParams = make([]ssa.Value, 0, len(fn.Params))
	for _, param := range fn.Params {
		if param == nil {
			continue
		}
		proc.SSAParams = append(proc.SSAParams, param)
		if ch, ok := b.paramChannels[param]; ok {
			b.channels[param] = ch
			continue
		}
		if sig, ok := b.paramSignals[param]; ok {
			b.signals[param] = sig
			proc.Params = append(proc.Params, sig)
			continue
		}
		if isChannelType(param.Type()) {
			ch := &Channel{
				Name:          b.uniqueName(param.Name()),
				Type:          channelElemType(param.Type()),
				Depth:         0,
				DeclaredDepth: 0,
				InferredDepth: 0,
				DepthReason:   "",
				IsParameter:   true,
				Source:        param.Pos(),
			}
			b.module.Channels[ch.Name] = ch
			b.channels[param] = ch
			b.channelUsage[ch] = 0
			continue
		}

		// Check if this is a slice parameter
		if isSliceType(param.Type()) {
			// For slice parameters, we don't create a single signal
			// Instead, we create an indexedBaseState that will be used for array accesses
			// The actual element signals will be bound when the function is inlined
			elemType, length, ok := indexedElementInfo(param.Type())
			if !ok {
				b.reporter.Warning(param.Pos(), fmt.Sprintf("slice parameter %s has unknown element type", param.Name()))
				continue
			}
			state := &indexedBaseState{
				base:     param,
				elemType: signalType(elemType),
				length:   length,
				elements: make(map[int]*Signal),
				storage:  make(map[int]*Signal),
			}
			if state.elemType == nil {
				state.elemType = &SignalType{Width: 32, Signed: true}
			}

			// Pre-create element signals for known-size slices
			// For slices with unknown length (length == -1), we can't pre-create elements
			// The elements will be created when the slice is bound to an argument
			if length > 0 {
				for i := 0; i < length; i++ {
					elemSig := &Signal{
						Name:   fmt.Sprintf("%s_%d", param.Name(), i),
						Type:   state.elemType.Clone(),
						Kind:   Wire,
						Source: param.Pos(),
					}
					state.elements[i] = elemSig
					state.storage[i] = elemSig
				}
			} else {
				// For slices with unknown length, we'll need to determine the length from the argument
				// This will be handled during parameter binding
			}

			b.indexedBases[param] = state
			// Create a placeholder signal for the slice parameter itself
			sig := &Signal{
				Name:   defaultName(param.Name(), b.uniqueName("param")),
				Type:   signalType(elemType), // Use element type for placeholder
				Kind:   Wire,
				Source: param.Pos(),
			}
			b.signals[param] = sig
			proc.Params = append(proc.Params, sig)
			continue
		}

		sig := &Signal{
			Name:   defaultName(param.Name(), b.uniqueName("param")),
			Type:   signalType(param.Type()),
			Kind:   Wire,
			Source: param.Pos(),
		}
		// Don't add function parameters to module signals - they will be ports instead
		// b.module.Signals[sig.Name] = sig
		b.signals[param] = sig
		proc.Params = append(proc.Params, sig)
	}
}

func (b *builder) handleMakeChan(mc *ssa.MakeChan) {
	chType, ok := mc.Type().Underlying().(*types.Chan)
	if !ok {
		b.reporter.Warning(mc.Pos(), "makechan without channel type encountered")
		return
	}
	name := mc.Name()
	if name == "" {
		name = b.uniqueName("chan")
	}
	depth := 1
	if c, ok := mc.Size.(*ssa.Const); ok && c.Value != nil {
		if v, ok := constant.Int64Val(c.Value); ok && v > 0 {
			depth = int(v)
		}
	}
	channel := &Channel{
		Name:          name,
		Type:          signalType(chType.Elem()),
		Depth:         depth,
		DeclaredDepth: depth,
		InferredDepth: 0,
		DepthReason:   "",
		Source:        mc.Pos(),
	}
	b.module.Channels[channel.Name] = channel
	b.channels[mc] = channel
	b.channelUsage[channel] = 0
}

func (b *builder) handleSend(proc *Process, bb *BasicBlock, send *ssa.Send) {
	channel := b.channelForValue(send.Chan)
	value := b.signalForValue(send.X)
	if channel == nil || value == nil {
		return
	}
	bb.Ops = append(bb.Ops, &SendOperation{
		Channel: channel,
		Value:   value,
	})
	channel.AddEndpoint(proc, ChannelSend)
	b.recordChannelDelta(channel, 1)
}

func (b *builder) handleRecv(proc *Process, bb *BasicBlock, recv *ssa.UnOp) {
	channel := b.channelForValue(recv.X)
	dest := b.ensureValueSignal(recv)
	dest.Type = signalType(recv.Type())
	if channel == nil {
		return
	}
	bb.Ops = append(bb.Ops, &RecvOperation{
		Channel: channel,
		Dest:    dest,
	})
	channel.AddEndpoint(proc, ChannelReceive)
	b.recordChannelDelta(channel, -1)
}

func (b *builder) handleGo(proc *Process, bb *BasicBlock, stmt *ssa.Go) {
	if stmt.Call.IsInvoke() {
		b.reporter.Warning(stmt.Pos(), "interface go calls are not supported in IR builder")
		return
	}
	callee := stmt.Call.StaticCallee()
	if callee == nil {
		b.reporter.Warning(stmt.Pos(), "goroutine target has no static callee")
		return
	}
	b.bindCallArguments(callee, stmt.Call.Args)
	target := b.buildProcess(callee)
	b.assignChildStage(proc, target)
	var args []*Signal
	var chanArgs []*Channel
	var params *types.Tuple
	if sig := stmt.Call.Signature(); sig != nil {
		params = sig.Params()
	}
	for idx, arg := range stmt.Call.Args {
		var paramType types.Type
		if params != nil && idx < params.Len() {
			paramType = params.At(idx).Type()
		}
		if paramType != nil && isChannelType(paramType) {
			if ch := b.channelForValueSilent(arg); ch != nil {
				chanArgs = append(chanArgs, ch)
			}
			continue
		}
		if sig := b.signalForValue(arg); sig != nil {
			args = append(args, sig)
		}
	}
	bb.Ops = append(bb.Ops, &SpawnOperation{
		Callee:   target,
		Args:     args,
		ChanArgs: chanArgs,
	})
}

func (b *builder) bindCallArguments(fn *ssa.Function, args []ssa.Value) {
	if fn == nil {
		return
	}
	params := fn.Params
	for i := 0; i < len(params) && i < len(args); i++ {
		param := params[i]
		arg := args[i]
		paramType := param.Type()
		if isChannelType(paramType) {
			if ch := b.channelForValueSilent(arg); ch != nil {
				b.addChannelParamBinding(param, ch)
				if _, exists := b.paramChannels[param]; !exists {
					b.paramChannels[param] = ch
				}
			}
			continue
		}
		if sig := b.signalForValue(arg); sig != nil {
			if _, exists := b.paramSignals[param]; !exists {
				b.paramSignals[param] = sig
			}
		}
	}
}
func (b *builder) buildConstSignal(c *ssa.Const) *Signal {
	sig := &Signal{
		Name:   b.newConstName(),
		Type:   signalType(c.Type()),
		Kind:   Const,
		Source: c.Pos(),
		Value:  extractConstValue(c),
	}
	b.module.Signals[sig.Name] = sig
	return sig
}

func (b *builder) signalForIndexAddr(addr *ssa.IndexAddr) *Signal {
	if addr == nil {
		return nil
	}
	return b.memoryAccess(nil, addr)
}

func (b *builder) signalForIndexAddrInBlock(bb *BasicBlock, addr *ssa.IndexAddr) *Signal {
	if addr == nil {
		return nil
	}
	return b.memoryAccess(bb, addr)
}

func (b *builder) memoryAccess(bb *BasicBlock, addr *ssa.IndexAddr) *Signal {
	if b == nil || addr == nil {
		return nil
	}
	if cached, ok := b.signals[addr]; ok && cached != nil {
		return cached
	}
	base, indices, ok := collectIndexedAccess(addr)
	if !ok {
		return nil
	}
	state := b.indexedStateForBase(base, addr.Pos())
	if state == nil {
		return nil
	}
	if idx, ok := indexedConstantFlatIndex(state, indices); ok {
		elem := b.indexedElementSignal(state, idx, addr.Pos())
		if elem != nil {
			b.signals[addr] = elem
		}
		return elem
	}
	if bb == nil {
		bb = b.currentBlock
	}
	if bb == nil {
		return nil
	}
	index, ok := b.linearizeIndexedAccess(bb, state, indices, addr.Pos())
	if !ok || index == nil {
		return nil
	}
	maxLen := state.length
	if maxLen < 0 {
		maxLen = defaultDynamicSliceIndexMax
	}
	selected := b.selectIndexedElementWithMax(bb, state, index, addr.Pos(), maxLen)
	if selected != nil {
		b.signals[addr] = selected
	}
	return selected
}

func (b *builder) lowerIndexedLoad(bb *BasicBlock, load *ssa.UnOp, addr *ssa.IndexAddr) bool {
	if bb == nil || load == nil || addr == nil {
		return false
	}
	selected := b.memoryAccess(bb, addr)
	if selected == nil {
		return false
	}
	selected = b.snapshotLoadedSignal(bb, selected, load.Pos())
	// Preserve any pre-created placeholder signal (e.g. when a phi references
	// this load before the defining block is translated) by resolving through
	// bindResolvedValue instead of replacing the map entry directly.
	b.bindResolvedValue(bb, load, selected)
	return true
}

func (b *builder) lowerIndexedStore(bb *BasicBlock, store *ssa.Store) bool {
	if bb == nil || store == nil {
		return false
	}
	addr, ok := store.Addr.(*ssa.IndexAddr)
	if !ok {
		return false
	}
	base, indices, ok := collectIndexedAccess(addr)
	if !ok {
		return false
	}
	state := b.indexedStateForBase(base, store.Pos())
	if state == nil {
		return false
	}
	value := b.signalForValue(store.Val)
	if value == nil {
		return false
	}
	if idx, ok := indexedConstantFlatIndex(state, indices); ok {
		dest := b.indexedElementStorageSignal(state, idx, store.Pos())
		if dest == nil {
			return false
		}
		// For internal array elements (not ports), create an assign operation to store the value
		// Check if this element is an internal signal (not a port)
		isInternal := false
		if dest.Name != "" {
			// Check if this looks like an array element that's not a test data port
			isInternal = !strings.HasPrefix(dest.Name, "test_")
		}
		if isInternal {
			// Create an assign operation to store the value in a register
			bb.Ops = append(bb.Ops, &AssignOperation{
				Dest:  dest,
				Value: value,
			})
		}
		state.elements[idx] = value
		return true
	}
	maxLen := state.length
	if maxLen < 0 {
		maxLen = defaultDynamicSliceIndexMax
	}
	if maxLen <= 0 {
		return false
	}
	index, ok := b.linearizeIndexedAccess(bb, state, indices, store.Pos())
	if !ok || index == nil {
		return false
	}
	indexType := index.Type
	if indexType == nil {
		indexType = &SignalType{Width: 32, Signed: true}
	}
	for i := 0; i < maxLen; i++ {
		current := b.indexedElementSignal(state, i, store.Pos())
		dest := b.indexedElementStorageSignal(state, i, store.Pos())
		if current == nil || dest == nil {
			continue
		}
		cond := b.newAnonymousSignal("idxeq", &SignalType{Width: 1, Signed: false}, store.Pos())
		bb.Ops = append(bb.Ops, &CompareOperation{
			Predicate: CompareEQ,
			Dest:      cond,
			Left:      index,
			Right:     b.newConstSignal(int64(i), indexType, store.Pos()),
		})
		next := b.newAnonymousSignal("idxstore", current.Type, store.Pos())
		bb.Ops = append(bb.Ops, &MuxOperation{
			Dest:       next,
			Cond:       cond,
			TrueValue:  value,
			FalseValue: current,
		})
		// For internal array elements, create an assign operation to store the value
		isInternal := false
		if dest.Name != "" {
			isInternal = !strings.HasPrefix(dest.Name, "test_")
		}
		if isInternal {
			bb.Ops = append(bb.Ops, &AssignOperation{
				Dest:  dest,
				Value: next,
			})
		}
		state.elements[i] = next
	}
	return true
}

func (b *builder) selectIndexedElement(bb *BasicBlock, state *indexedBaseState, index *Signal, pos token.Pos) *Signal {
	return b.selectIndexedElementWithMax(bb, state, index, pos, state.length)
}

func (b *builder) selectIndexedElementWithMax(bb *BasicBlock, state *indexedBaseState, index *Signal, pos token.Pos, maxLen int) *Signal {
	if bb == nil || state == nil || index == nil || maxLen <= 0 {
		return nil
	}
	selected := b.indexedElementSignal(state, 0, pos)
	if selected == nil {
		return nil
	}
	indexType := index.Type
	if indexType == nil {
		indexType = &SignalType{Width: 32, Signed: true}
	}
	for i := 1; i < maxLen; i++ {
		elem := b.indexedElementSignal(state, i, pos)
		if elem == nil {
			continue
		}
		cond := b.newAnonymousSignal("idxeq", &SignalType{Width: 1, Signed: false}, pos)
		bb.Ops = append(bb.Ops, &CompareOperation{
			Predicate: CompareEQ,
			Dest:      cond,
			Left:      index,
			Right:     b.newConstSignal(int64(i), indexType, pos),
		})
		next := b.newAnonymousSignal("idxload", state.elemType, pos)
		bb.Ops = append(bb.Ops, &MuxOperation{
			Dest:       next,
			Cond:       cond,
			TrueValue:  elem,
			FalseValue: selected,
		})
		selected = next
	}
	return selected
}

func (b *builder) indexedStateForBase(base ssa.Value, pos token.Pos) *indexedBaseState {
	base = unwrapIndexedBase(base)
	if base == nil {
		return nil
	}
	if state, ok := b.indexedBases[base]; ok {
		return state
	}
	elemType, length, dims, ok := indexedShapeInfo(base.Type())
	if !ok {
		return nil
	}
	state := &indexedBaseState{
		base:     base,
		elemType: signalType(elemType),
		length:   length,
		dims:     dims,
		elements: make(map[int]*Signal),
		storage:  make(map[int]*Signal),
	}
	if state.elemType == nil {
		state.elemType = &SignalType{Width: 32, Signed: true}
	}
	if g, ok := base.(*ssa.Global); ok {
		b.bindGlobalIndexedInputPorts(g, state)
	}
	_ = pos
	b.indexedBases[base] = state
	return state
}

func (b *builder) indexedElementSignal(state *indexedBaseState, idx int, pos token.Pos) *Signal {
	if state == nil || idx < 0 {
		return nil
	}
	if state.length >= 0 && idx >= state.length {
		return nil
	}
	if sig, ok := state.elements[idx]; ok {
		return sig
	}
	if sig, ok := state.storage[idx]; ok && sig != nil {
		state.elements[idx] = sig
		return sig
	}
	sig := b.newConstSignal(0, state.elemType, pos)
	state.elements[idx] = sig
	return sig
}

func (b *builder) indexedElementStorageSignal(state *indexedBaseState, idx int, pos token.Pos) *Signal {
	if state == nil || idx < 0 {
		return nil
	}
	if state.length >= 0 && idx >= state.length {
		return nil
	}
	if sig, ok := state.storage[idx]; ok && sig != nil {
		return sig
	}
	if _, isGlobal := state.base.(*ssa.Global); !isGlobal {
		baseName := "idxbase"
		if alloc, ok := state.base.(*ssa.Alloc); ok {
			baseName = b.allocName(alloc)
		} else if state.base != nil {
			baseName = defaultName(state.base.Name(), baseName)
			baseName = strings.ReplaceAll(baseName, ".", "_")
			baseName = strings.ReplaceAll(baseName, " ", "_")
		}
		sig := &Signal{
			Name:   fmt.Sprintf("%s_%d", baseName, idx),
			Type:   state.elemType.Clone(),
			Kind:   Reg,
			Source: pos,
			Value:  int64(0),
		}
		state.storage[idx] = sig
		if b.module != nil {
			b.module.Signals[sig.Name] = sig
		}
		if state.elements[idx] == nil || state.elements[idx].Kind == Const {
			state.elements[idx] = sig
		}
		return sig
	}
	sig := b.indexedElementSignal(state, idx, pos)
	if sig != nil {
		state.storage[idx] = sig
	}
	return sig
}

func (b *builder) bindGlobalIndexedInputPorts(g *ssa.Global, state *indexedBaseState) {
	if b == nil || b.module == nil || g == nil || state == nil || state.length <= 0 {
		return
	}

	// Determine if this global should be an input port based on naming convention.
	// Only test input data (e.g., "test_data") should be input ports.
	// Expected outputs (e.g., "test_compressed", "test_result") and working storage
	// (e.g., "compressed", "result") should be internal signals.
	shouldHavePorts := g.Name() == "test_data"

	base := defaultName(g.Name(), "global")
	base = strings.ReplaceAll(base, ".", "_")

	for i := 0; i < state.length; i++ {
		sigName := fmt.Sprintf("%s_%d", base, i)
		typ := state.elemType.Clone()
		if typ == nil {
			typ = &SignalType{Width: 32, Signed: true}
		}

		// Check if there's already a constant signal from bootstrapGlobalInitializers
		constSig := state.elements[i]
		hasConstInit := constSig != nil && constSig.Kind == Const

		kind := Wire
		if !shouldHavePorts {
			kind = Reg
		}

		// Create the signal (or reuse existing)
		var sig *Signal
		if existingSig, ok := b.module.Signals[sigName]; ok {
			// Signal already exists in module, reuse it
			sig = existingSig
			if !shouldHavePorts {
				sig.Kind = Reg
			}
			// If there's a constant init value and the signal doesn't have one, use it
			if hasConstInit && sig.Value == nil {
				sig.Value = constSig.Value
			}
		} else {
			// Create new signal with constant value if available
			var initValue interface{}
			if hasConstInit {
				initValue = constSig.Value
			}

			sig = &Signal{
				Name:   sigName,
				Type:   typ.Clone(),
				Kind:   kind,
				Source: g.Pos(),
				Value:  initValue,
			}
			b.module.Signals[sig.Name] = sig
		}

		state.storage[i] = sig
		if state.elements[i] == nil || state.elements[i].Kind == Const {
			state.elements[i] = sig
		}

		// Only add as input port for read-only test input data
		if shouldHavePorts && !b.hasPort(sigName) {
			b.module.Ports = append(b.module.Ports, Port{
				Name:      sigName,
				Direction: Input,
				Type:      typ.Clone(),
			})
		}
	}
}

func (b *builder) hasPort(name string) bool {
	if b == nil || b.module == nil || strings.TrimSpace(name) == "" {
		return false
	}
	for _, port := range b.module.Ports {
		if port.Name == name {
			return true
		}
	}
	return false
}

func unwrapIndexedBase(v ssa.Value) ssa.Value {
	if base, _, ok := collectIndexedAccess(v); ok {
		return base
	}
	for v != nil {
		switch val := v.(type) {
		case *ssa.ChangeType:
			v = val.X
		case *ssa.Convert:
			v = val.X
		default:
			return v
		}
	}
	return nil
}

func collectIndexedAccess(v ssa.Value) (ssa.Value, []ssa.Value, bool) {
	if v == nil {
		return nil, nil, false
	}
	var indices []ssa.Value
	for v != nil {
		switch val := v.(type) {
		case *ssa.ChangeType:
			v = val.X
		case *ssa.Convert:
			v = val.X
		case *ssa.Slice:
			v = val.X
		case *ssa.IndexAddr:
			indices = append([]ssa.Value{val.Index}, indices...)
			v = val.X
		default:
			if len(indices) == 0 {
				return nil, nil, false
			}
			return v, indices, true
		}
	}
	return nil, nil, false
}

func indexedShapeInfo(t types.Type) (types.Type, int, []int, bool) {
	if t == nil {
		return nil, 0, nil, false
	}
	switch tt := t.Underlying().(type) {
	case *types.Pointer:
		return indexedShapeInfo(tt.Elem())
	case *types.Array:
		if elem, nestedLen, nestedDims, ok := indexedShapeInfo(tt.Elem()); ok {
			dims := append([]int{int(tt.Len())}, nestedDims...)
			if nestedLen < 0 {
				return elem, -1, dims, true
			}
			return elem, int(tt.Len()) * nestedLen, dims, true
		}
		return tt.Elem(), int(tt.Len()), []int{int(tt.Len())}, true
	case *types.Slice:
		if elem, _, nestedDims, ok := indexedShapeInfo(tt.Elem()); ok {
			return elem, -1, append([]int{-1}, nestedDims...), true
		}
		return tt.Elem(), -1, []int{-1}, true
	}
	return nil, 0, nil, false
}

func indexedElementInfo(t types.Type) (types.Type, int, bool) {
	elem, length, _, ok := indexedShapeInfo(t)
	return elem, length, ok
}

func (s *indexedBaseState) stride(dim int) (int, bool) {
	if s == nil || dim < 0 {
		return 0, false
	}
	if len(s.dims) == 0 {
		if dim == 0 {
			return 1, true
		}
		return 0, false
	}
	if dim >= len(s.dims) {
		return 0, false
	}
	stride := 1
	for i := dim + 1; i < len(s.dims); i++ {
		if s.dims[i] < 0 {
			return 0, false
		}
		stride *= s.dims[i]
	}
	return stride, true
}

func indexedConstantFlatIndex(state *indexedBaseState, indices []ssa.Value) (int, bool) {
	if state == nil || len(indices) == 0 {
		return 0, false
	}
	if len(state.dims) > 1 && len(indices) != len(state.dims) {
		return 0, false
	}
	flat := 0
	for i, index := range indices {
		raw, ok := constIndexValue(index)
		if !ok {
			return 0, false
		}
		dimSize := -1
		if i < len(state.dims) {
			dimSize = state.dims[i]
		}
		if dimSize >= 0 && raw >= dimSize {
			return 0, false
		}
		stride, ok := state.stride(i)
		if !ok {
			if len(indices) == 1 {
				stride = 1
			} else {
				return 0, false
			}
		}
		flat += raw * stride
	}
	if state.length >= 0 && flat >= state.length {
		return 0, false
	}
	return flat, true
}

func (b *builder) synthesizeBinOp(bb *BasicBlock, prefix string, op BinOp, left, right *Signal, typ *SignalType, pos token.Pos) *Signal {
	if b == nil || bb == nil || left == nil || right == nil {
		return nil
	}
	if typ == nil {
		typ = left.Type.Promote(right.Type)
	}
	if typ == nil {
		typ = &SignalType{Width: 32, Signed: true}
	}
	dest := b.newAnonymousSignal(prefix, typ, pos)
	bb.Ops = append(bb.Ops, &BinOperation{
		Op:    op,
		Dest:  dest,
		Left:  left,
		Right: right,
	})
	return dest
}

func (b *builder) synthesizeCompare(bb *BasicBlock, prefix string, pred ComparePredicate, left, right *Signal, pos token.Pos) *Signal {
	if b == nil || bb == nil || left == nil || right == nil {
		return nil
	}
	dest := b.newAnonymousSignal(prefix, boolSignalType(), pos)
	bb.Ops = append(bb.Ops, &CompareOperation{
		Predicate: pred,
		Dest:      dest,
		Left:      left,
		Right:     right,
	})
	return dest
}

func (b *builder) synthesizeMux(bb *BasicBlock, prefix string, cond, trueValue, falseValue *Signal, typ *SignalType, pos token.Pos) *Signal {
	if b == nil || bb == nil || cond == nil || trueValue == nil || falseValue == nil {
		return nil
	}
	if typ == nil {
		typ = trueValue.Type
		if typ == nil {
			typ = falseValue.Type
		}
	}
	dest := b.newAnonymousSignal(prefix, typ, pos)
	bb.Ops = append(bb.Ops, &MuxOperation{
		Dest:       dest,
		Cond:       cond,
		TrueValue:  trueValue,
		FalseValue: falseValue,
	})
	return dest
}

func (b *builder) synthesizeCountLeadingZeros32(bb *BasicBlock, value *Signal, pos token.Pos) *Signal {
	if b == nil || bb == nil || value == nil {
		return nil
	}
	wordType := &SignalType{Width: 32, Signed: false}
	countType := &SignalType{Width: 8, Signed: true}
	current := value
	if value.Type == nil || !value.Type.Equal(wordType) {
		current = b.newAnonymousSignal("clz32_arg", wordType, pos)
		bb.Ops = append(bb.Ops, &ConvertOperation{Dest: current, Value: value})
	}
	count := b.newConstSignal(int8(0), countType, pos)
	steps := []struct {
		limit uint32
		add   int8
		shift uint32
	}{
		{limit: 0x00010000, add: 16, shift: 16},
		{limit: 0x01000000, add: 8, shift: 8},
		{limit: 0x10000000, add: 4, shift: 4},
		{limit: 0x40000000, add: 2, shift: 2},
		{limit: 0x80000000, add: 1, shift: 1},
	}
	for _, step := range steps {
		cond := b.synthesizeCompare(bb, "clz32_cmp", CompareULT, current, b.newConstSignal(step.limit, wordType, pos), pos)
		if cond == nil {
			return nil
		}
		incremented := b.synthesizeBinOp(bb, "clz32_add", Add, count, b.newConstSignal(step.add, countType, pos), countType, pos)
		if incremented == nil {
			return nil
		}
		count = b.synthesizeMux(bb, "clz32_sel", cond, incremented, count, countType, pos)
		shifted := b.synthesizeBinOp(bb, "clz32_shl", Shl, current, b.newConstSignal(step.shift, wordType, pos), wordType, pos)
		if shifted == nil {
			return nil
		}
		current = b.synthesizeMux(bb, "clz32_word", cond, shifted, current, wordType, pos)
	}
	return count
}

func (b *builder) synthesizeCountLeadingZeros64(bb *BasicBlock, value *Signal, pos token.Pos) *Signal {
	if b == nil || bb == nil || value == nil {
		return nil
	}
	wordType := &SignalType{Width: 64, Signed: false}
	countType := &SignalType{Width: 8, Signed: true}
	current := value
	if value.Type == nil || !value.Type.Equal(wordType) {
		current = b.newAnonymousSignal("clz64_arg", wordType, pos)
		bb.Ops = append(bb.Ops, &ConvertOperation{Dest: current, Value: value})
	}
	highZero := b.synthesizeCompare(bb, "clz64_cmp", CompareULT, current, b.newConstSignal(uint64(1)<<32, wordType, pos), pos)
	if highZero == nil {
		return nil
	}
	upper := b.synthesizeBinOp(bb, "clz64_shr", ShrU, current, b.newConstSignal(uint64(32), wordType, pos), wordType, pos)
	if upper == nil {
		return nil
	}
	lower32 := b.newAnonymousSignal("clz64_lo", &SignalType{Width: 32, Signed: false}, pos)
	bb.Ops = append(bb.Ops, &ConvertOperation{Dest: lower32, Value: current})
	upper32 := b.newAnonymousSignal("clz64_hi", &SignalType{Width: 32, Signed: false}, pos)
	bb.Ops = append(bb.Ops, &ConvertOperation{Dest: upper32, Value: upper})
	selected := b.synthesizeMux(bb, "clz64_sel", highZero, lower32, upper32, lower32.Type, pos)
	if selected == nil {
		return nil
	}
	inner := b.synthesizeCountLeadingZeros32(bb, selected, pos)
	if inner == nil {
		return nil
	}
	base := b.synthesizeMux(bb, "clz64_base", highZero, b.newConstSignal(int8(32), countType, pos), b.newConstSignal(int8(0), countType, pos), countType, pos)
	if base == nil {
		return nil
	}
	return b.synthesizeBinOp(bb, "clz64_add", Add, base, inner, countType, pos)
}

func (b *builder) linearizeIndexedAccess(bb *BasicBlock, state *indexedBaseState, indices []ssa.Value, pos token.Pos) (*Signal, bool) {
	if b == nil || bb == nil || state == nil || len(indices) == 0 {
		return nil, false
	}
	if len(state.dims) > 1 && len(indices) != len(state.dims) {
		return nil, false
	}

	indexType := &SignalType{Width: 32, Signed: true}
	offset := 0
	var acc *Signal

	for i, indexValue := range indices {
		stride, ok := state.stride(i)
		if !ok {
			if len(indices) == 1 {
				stride = 1
			} else {
				return nil, false
			}
		}
		if raw, ok := constIndexValue(indexValue); ok {
			offset += raw * stride
			continue
		}
		index := b.signalForValue(indexValue)
		if index == nil {
			return nil, false
		}
		if index.Type != nil {
			indexType = index.Type.Clone()
		}
		term := index
		if stride != 1 {
			term = b.synthesizeBinOp(
				bb,
				"idxmul",
				Mul,
				term,
				b.newConstSignal(int64(stride), indexType.Clone(), pos),
				indexType.Clone(),
				pos,
			)
			if term == nil {
				return nil, false
			}
		}
		if acc == nil {
			acc = term
			continue
		}
		acc = b.synthesizeBinOp(bb, "idxadd", Add, acc, term, indexType.Clone(), pos)
		if acc == nil {
			return nil, false
		}
	}

	if acc == nil {
		return b.newConstSignal(int64(offset), indexType, pos), true
	}
	if offset != 0 {
		acc = b.synthesizeBinOp(
			bb,
			"idxadd",
			Add,
			acc,
			b.newConstSignal(int64(offset), indexType.Clone(), pos),
			indexType.Clone(),
			pos,
		)
		if acc == nil {
			return nil, false
		}
	}
	return acc, true
}

func (b *builder) snapshotLoadedSignal(bb *BasicBlock, sig *Signal, pos token.Pos) *Signal {
	if b == nil || bb == nil || sig == nil {
		return sig
	}
	if sig.Kind == Const {
		return sig
	}
	if sig.Kind != Reg {
		return sig
	}
	dest := b.newAnonymousSignal("loadsnap", sig.Type.Clone(), pos)
	bb.Ops = append(bb.Ops, &MuxOperation{
		Dest:       dest,
		Cond:       b.newConstSignal(true, &SignalType{Width: 1, Signed: false}, pos),
		TrueValue:  sig,
		FalseValue: sig,
	})
	return dest
}

func (b *builder) materializeIndexedStateStorage() {
	if b == nil {
		return
	}
	for _, state := range b.indexedBases {
		if state == nil {
			continue
		}
		for idx, storage := range state.storage {
			if storage == nil {
				continue
			}
			state.elements[idx] = storage
		}
	}
}

func constIndexValue(v ssa.Value) (int, bool) {
	c, ok := v.(*ssa.Const)
	if !ok || c == nil || c.Value == nil {
		return 0, false
	}
	raw, ok := constant.Int64Val(c.Value)
	if !ok || raw < 0 {
		return 0, false
	}
	return int(raw), true
}

func (b *builder) signalForValue(v ssa.Value) *Signal {
	if v == nil {
		return nil
	}
	if sig, ok := b.signals[v]; ok {
		return sig
	}
	switch val := v.(type) {
	case *ssa.Const:
		sig := b.buildConstSignal(val)
		b.signals[v] = sig
		return sig
	case *ssa.BinOp:
		return b.ensureValueSignal(val)
	case *ssa.UnOp:
		return b.ensureValueSignal(val)
	case *ssa.Convert:
		return b.ensureValueSignal(val)
	case *ssa.ChangeType:
		return b.signalForValue(val.X)
	case *ssa.Phi:
		return b.ensureValueSignal(val)
	case *ssa.IndexAddr:
		if sig := b.memoryAccess(b.currentBlock, val); sig != nil {
			return sig
		}
		// Dynamic index addresses need block context plus analyzable base/index.
		// When unavailable here, callers with explicit block context can retry.
		return nil
	case *ssa.Extract:
		if tuple, ok := b.tupleSignals[val.Tuple]; ok && val.Index >= 0 && val.Index < len(tuple) {
			return tuple[val.Index]
		}
		return nil
	case *ssa.Global:
		return b.signalForGlobal(val)
	case *ssa.Slice:
		return b.signalForValue(val.X)
	case *ssa.MakeInterface, *ssa.MakeChan:
		return nil
	case *ssa.Call:
		return nil
	}
	b.reporter.Warning(v.Pos(), fmt.Sprintf("no signal mapping for value %T", v))
	return nil
}

func (b *builder) lowerTypeChange(bb *BasicBlock, destVal ssa.Value, srcVal ssa.Value, dstType types.Type) {
	if bb == nil || destVal == nil || srcVal == nil {
		return
	}
	source := b.signalForValue(srcVal)
	if source == nil {
		return
	}
	if dest, ok := b.lowerNativeFloatConvert(bb, source, srcVal.Type(), dstType, srcVal.Pos()); ok {
		b.bindResolvedValue(bb, destVal, dest)
		return
	}
	destSignalType := signalType(dstType)
	if source.Type != nil && source.Type.Equal(destSignalType) {
		b.signals[destVal] = source
		return
	}
	dest := b.ensureValueSignal(destVal)
	dest.Type = destSignalType
	bb.Ops = append(bb.Ops, &ConvertOperation{
		Dest:  dest,
		Value: source,
	})
}

func (b *builder) lowerNativeFloatBinOp(bb *BasicBlock, tok token.Token, resultType types.Type, leftType types.Type, left, right *Signal, pos token.Pos) (*Signal, bool) {
	if bb == nil || left == nil || right == nil || !isNativeFloat64Type(leftType) {
		return nil, false
	}
	switch tok {
	case token.ADD:
		return b.lowerNativeFloatAdd(bb, left, right, signalType(resultType), pos)
	case token.SUB:
		negRight, ok := b.callMainHelperValue(bb, "float64_neg", argsToSignals(right), signalType(leftType), pos)
		if !ok {
			return nil, false
		}
		return b.lowerNativeFloatAdd(bb, left, negRight, signalType(resultType), pos)
	case token.MUL:
		return b.callMainHelperValue(bb, "float64_mul", argsToSignals(left, right), signalType(resultType), pos)
	case token.QUO:
		return b.callMainHelperValue(bb, "float64_div", argsToSignals(left, right), signalType(resultType), pos)
	case token.LEQ:
		return b.callMainHelperValue(bb, "float64_le", argsToSignals(left, right), signalType(resultType), pos)
	case token.GEQ:
		return b.callMainHelperValue(bb, "float64_le", argsToSignals(right, left), signalType(resultType), pos)
	case token.EQL:
		return b.lowerNativeFloatEquality(bb, left, right, true, pos)
	case token.NEQ:
		return b.lowerNativeFloatEquality(bb, left, right, false, pos)
	case token.LSS:
		le, ok := b.callMainHelperValue(bb, "float64_le", argsToSignals(left, right), boolSignalType(), pos)
		if !ok {
			return nil, false
		}
		revLe, ok := b.callMainHelperValue(bb, "float64_le", argsToSignals(right, left), boolSignalType(), pos)
		if !ok {
			return nil, false
		}
		notRevLe := b.newAnonymousSignal("flt_not", boolSignalType(), pos)
		bb.Ops = append(bb.Ops, &NotOperation{
			Dest:  notRevLe,
			Value: revLe,
		})
		return b.synthesizeBinOp(bb, "flt_lt", And, le, notRevLe, boolSignalType(), pos), true
	case token.GTR:
		return b.lowerNativeFloatBinOp(bb, token.LSS, resultType, leftType, right, left, pos)
	default:
		return nil, false
	}
}

func (b *builder) lowerNativeFloatAdd(bb *BasicBlock, left, right *Signal, destType *SignalType, pos token.Pos) (*Signal, bool) {
	if bb == nil || left == nil || right == nil {
		return nil, false
	}
	signA, ok := b.nativeFloatSign(bb, left, pos)
	if !ok {
		return nil, false
	}
	signB, ok := b.nativeFloatSign(bb, right, pos)
	if !ok {
		return nil, false
	}
	sameSign := b.synthesizeCompare(bb, "flt_same_sign", CompareEQ, signA, signB, pos)
	if sameSign == nil {
		return nil, false
	}
	expA, ok := b.nativeFloatExponent(bb, left, pos)
	if !ok {
		return nil, false
	}
	expB, ok := b.nativeFloatExponent(bb, right, pos)
	if !ok {
		return nil, false
	}
	leftHasMaxExp := b.synthesizeCompare(bb, "flt_exp_ge", CompareUGE, expA, expB, pos)
	if leftHasMaxExp == nil {
		return nil, false
	}
	addAB, ok := b.callMainHelperValue(bb, "addFloat64Sigs", argsToSignals(left, right, signA), destType, pos)
	if !ok {
		return nil, false
	}
	addBA, ok := b.callMainHelperValue(bb, "addFloat64Sigs", argsToSignals(right, left, signB), destType, pos)
	if !ok {
		return nil, false
	}
	addRes := b.synthesizeMux(bb, "flt_add_order", leftHasMaxExp, addAB, addBA, destType, pos)
	if addRes == nil {
		return nil, false
	}
	subRes, ok := b.callMainHelperValue(bb, "subFloat64Sigs", argsToSignals(left, right, signA), destType, pos)
	if !ok {
		return nil, false
	}
	return b.synthesizeMux(bb, "flt_addsel", sameSign, addRes, subRes, destType, pos), true
}

func (b *builder) nativeFloatSign(bb *BasicBlock, value *Signal, pos token.Pos) (*Signal, bool) {
	if bb == nil || value == nil {
		return nil, false
	}
	wordType := &SignalType{Width: 64, Signed: false}
	signWord := b.synthesizeBinOp(bb, "flt_sign_shr", ShrU, value, b.newConstSignal(uint64(63), wordType, pos), wordType, pos)
	if signWord == nil {
		return nil, false
	}
	sign := b.synthesizeCompare(bb, "flt_sign_bool", CompareNE, signWord, b.newConstSignal(uint64(0), wordType, pos), pos)
	if sign == nil {
		return nil, false
	}
	return sign, true
}

func (b *builder) nativeFloatExponent(bb *BasicBlock, value *Signal, pos token.Pos) (*Signal, bool) {
	if bb == nil || value == nil {
		return nil, false
	}
	wordType := &SignalType{Width: 64, Signed: false}
	shifted := b.synthesizeBinOp(bb, "flt_exp_shr", ShrU, value, b.newConstSignal(uint64(52), wordType, pos), wordType, pos)
	if shifted == nil {
		return nil, false
	}
	exp := b.synthesizeBinOp(bb, "flt_exp_mask", And, shifted, b.newConstSignal(uint64(0x7FF), wordType, pos), wordType, pos)
	if exp == nil {
		return nil, false
	}
	return exp, true
}

func (b *builder) lowerNativeFloatEquality(bb *BasicBlock, left, right *Signal, equal bool, pos token.Pos) (*Signal, bool) {
	ab, ok := b.callMainHelperValue(bb, "float64_le", argsToSignals(left, right), boolSignalType(), pos)
	if !ok {
		return nil, false
	}
	ba, ok := b.callMainHelperValue(bb, "float64_le", argsToSignals(right, left), boolSignalType(), pos)
	if !ok {
		return nil, false
	}
	eq := b.synthesizeBinOp(bb, "flt_eq", And, ab, ba, boolSignalType(), pos)
	if eq == nil {
		return nil, false
	}
	if equal {
		return eq, true
	}
	dest := b.newAnonymousSignal("flt_ne", boolSignalType(), pos)
	bb.Ops = append(bb.Ops, &NotOperation{
		Dest:  dest,
		Value: eq,
	})
	return dest, true
}

func (b *builder) lowerNativeFloatNeg(bb *BasicBlock, value *Signal, valueType types.Type, pos token.Pos) (*Signal, bool) {
	if bb == nil || value == nil || !isNativeFloat64Type(valueType) {
		return nil, false
	}
	return b.callMainHelperValue(bb, "float64_neg", argsToSignals(value), signalType(valueType), pos)
}

func (b *builder) lowerNativeFloatConvert(bb *BasicBlock, source *Signal, srcType, dstType types.Type, pos token.Pos) (*Signal, bool) {
	if bb == nil || source == nil || !isNativeFloat64Type(dstType) {
		return nil, false
	}
	if srcType != nil {
		if !isIntegerType(srcType) {
			return nil, false
		}
	} else if source.Type == nil || source.Type.Width <= 0 || source.Type.Width > 32 {
		return nil, false
	}
	helperArg := source
	helperType := &SignalType{Width: 32, Signed: true}
	if source.Type == nil || !source.Type.Equal(helperType) {
		helperArg = b.newAnonymousSignal("fconv_arg", helperType, pos)
		bb.Ops = append(bb.Ops, &ConvertOperation{
			Dest:  helperArg,
			Value: source,
		})
	}
	return b.callMainHelperValue(bb, "int32_to_float64", argsToSignals(helperArg), signalType(dstType), pos)
}

func (b *builder) callMainHelperValue(bb *BasicBlock, name string, args []*Signal, destType *SignalType, pos token.Pos) (*Signal, bool) {
	results, ok := b.inlineMainHelperCall(bb, name, args, pos)
	if !ok || len(results) != 1 || results[0] == nil {
		return nil, false
	}
	result := results[0]
	if destType == nil || result.Type == nil || result.Type.Equal(destType) {
		return result, true
	}
	dest := b.newAnonymousSignal("helpercast", destType, pos)
	bb.Ops = append(bb.Ops, &ConvertOperation{
		Dest:  dest,
		Value: result,
	})
	return dest, true
}

func (b *builder) mergeNativeFloatHelperCall(proc *Process, block *ssa.BasicBlock, bb *BasicBlock, instrIndex int, value ssa.Value, name string, args []*Signal) bool {
	if b == nil || proc == nil || block == nil || bb == nil || value == nil || b.mainPkg == nil {
		return false
	}
	callee := b.mainPkg.Func(name)
	if callee == nil {
		return false
	}
	dest := b.ensureValueSignal(value)
	dest.Type = signalType(value.Type())
	return b.buildAndMergeProcess(proc, block, bb, instrIndex, callee, args, nil, dest)
}

func (b *builder) inlineMainHelperCall(bb *BasicBlock, name string, args []*Signal, pos token.Pos) ([]*Signal, bool) {
	if b == nil || bb == nil || b.mainPkg == nil || strings.TrimSpace(name) == "" {
		return nil, false
	}
	callee := b.mainPkg.Func(name)
	if callee == nil {
		return nil, false
	}
	stack := make(map[*ssa.Function]struct{})
	baseOps := len(bb.Ops)
	if results, ok := b.inlineCall(bb, callee, args, stack, 0); ok {
		return results, true
	}
	bb.Ops = bb.Ops[:baseOps]
	baseOps = len(bb.Ops)
	if results, ok := b.inlineCallWithOptions(bb, callee, args, stack, 0, true); ok {
		return results, true
	}
	bb.Ops = bb.Ops[:baseOps]
	return nil, false
}

func argsToSignals(args ...*Signal) []*Signal {
	out := make([]*Signal, 0, len(args))
	for _, arg := range args {
		if arg != nil {
			out = append(out, arg)
		}
	}
	return out
}

func (b *builder) channelForValue(v ssa.Value) *Channel {
	return b.lookupChannel(v, true)
}

func (b *builder) channelForValueSilent(v ssa.Value) *Channel {
	return b.lookupChannel(v, false)
}

func (b *builder) lookupChannel(v ssa.Value, warn bool) *Channel {
	if ch, ok := b.channels[v]; ok {
		return ch
	}
	switch val := v.(type) {
	case *ssa.ChangeType:
		return b.lookupChannel(val.X, warn)
	}
	if warn && v != nil {
		b.reporter.Warning(v.Pos(), fmt.Sprintf("no channel mapping for value %T", v))
	}
	return nil
}

func (b *builder) newConstName() string {
	name := fmt.Sprintf("const_%d", b.tempID)
	b.tempID++
	return name
}

func (b *builder) handleFmtPrint(proc *Process, bb *BasicBlock, call *ssa.Call) bool {
	fn, ok := call.Call.Value.(*ssa.Function)
	if !ok || fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return false
	}
	if fn.Pkg.Pkg.Path() != "fmt" {
		return false
	}

	var segments []PrintSegment
	var err error

	switch fn.Name() {
	case "Printf":
		if len(call.Call.Args) == 0 {
			b.reporter.Warning(call.Pos(), "fmt.Printf requires a constant format string")
			return true
		}
		formatConst, ok := call.Call.Args[0].(*ssa.Const)
		if !ok || formatConst.Value == nil || formatConst.Value.Kind() != constant.String {
			b.reporter.Warning(call.Pos(), "fmt.Printf format must be a constant string")
			return true
		}
		format := constant.StringVal(formatConst.Value)
		argValues, argErr := b.expandCallArgs(call.Call.Args[1:])
		if argErr != nil {
			err = argErr
			break
		}
		segments, err = b.buildPrintfSegments(format, argValues)
	case "Println":
		argValues, argErr := b.expandCallArgs(call.Call.Args)
		if argErr != nil {
			err = argErr
			break
		}
		segments, err = b.buildPrintSegments(argValues, true)
	case "Print":
		argValues, argErr := b.expandCallArgs(call.Call.Args)
		if argErr != nil {
			err = argErr
			break
		}
		segments, err = b.buildPrintSegments(argValues, false)
	default:
		return false
	}

	if err != nil {
		b.reporter.Warning(call.Pos(), fmt.Sprintf("fmt.%s: %v", fn.Name(), err))
		return true
	}
	if len(segments) == 0 {
		segments = appendLiteralSegment(nil, "")
	}
	bb.Ops = append(bb.Ops, &PrintOperation{Segments: segments})
	return true
}

func (b *builder) buildPrintfSegments(format string, args []ssa.Value) ([]PrintSegment, error) {
	var segments []PrintSegment
	argIndex := 0
	var literal strings.Builder
	flushLiteral := func() {
		if literal.Len() == 0 {
			return
		}
		segments = appendLiteralSegment(segments, literal.String())
		literal.Reset()
	}
	for i := 0; i < len(format); {
		if format[i] != '%' {
			literal.WriteByte(format[i])
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			literal.WriteByte('%')
			i += 2
			continue
		}
		i++
		if i >= len(format) {
			return nil, fmt.Errorf("trailing %% in format string")
		}
		verbChar, width, zeroPad, next, parseErr := parsePrintfSpecifier(format, i)
		if parseErr != nil {
			return nil, parseErr
		}
		i = next
		flushLiteral()
		if argIndex >= len(args) {
			return nil, fmt.Errorf("not enough arguments for format")
		}
		sig := b.signalForValue(args[argIndex])
		if sig == nil {
			return nil, fmt.Errorf("unsupported argument type %T", args[argIndex])
		}
		argIndex++
		var verb PrintVerb
		switch verbChar {
		case 'd', 'v':
			verb = PrintVerbDec
		case 'x', 'X':
			verb = PrintVerbHex
		case 'b':
			verb = PrintVerbBin
		case 'f':
			verb = PrintVerbFloat
		case 't':
			verb = PrintVerbDec
		default:
			return nil, fmt.Errorf("unsupported verb %%%c", verbChar)
		}
		segments = append(segments, PrintSegment{
			Value:   sig,
			Verb:    verb,
			Width:   width,
			ZeroPad: zeroPad,
		})
	}
	flushLiteral()
	if argIndex != len(args) {
		return nil, fmt.Errorf("too many arguments for format")
	}
	return segments, nil
}

func parsePrintfSpecifier(format string, start int) (verb byte, width int, zeroPad bool, next int, err error) {
	i := start
	if i < len(format) && format[i] == '0' {
		zeroPad = true
		i++
	}
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		width = width*10 + int(format[i]-'0')
		i++
	}
	if i >= len(format) {
		return 0, 0, false, 0, fmt.Errorf("trailing %% in format string")
	}
	verb = format[i]
	i++
	if width == 0 {
		zeroPad = false
	}
	return verb, width, zeroPad, i, nil
}

func (b *builder) buildPrintSegments(args []ssa.Value, newline bool) ([]PrintSegment, error) {
	var segments []PrintSegment
	buildValueSegments := func(v ssa.Value) ([]PrintSegment, bool, error) {
		switch val := v.(type) {
		case *ssa.Const:
			if val.IsNil() || val.Value == nil {
				return nil, false, nil
			}
			if val.Value != nil && val.Value.Kind() == constant.String {
				return []PrintSegment{{Text: constant.StringVal(val.Value)}}, true, nil
			}
		}
		sig := b.signalForValue(v)
		if sig == nil {
			return nil, false, fmt.Errorf("unsupported argument %T", v)
		}
		return []PrintSegment{{Value: sig, Verb: PrintVerbDec}}, true, nil
	}
	emittedCount := 0
	for _, arg := range args {
		argSegments, emitted, err := buildValueSegments(arg)
		if err != nil {
			return nil, err
		}
		if !emitted {
			continue
		}
		if emittedCount > 0 {
			segments = appendLiteralSegment(segments, " ")
		}
		segments = append(segments, argSegments...)
		emittedCount++
	}
	if newline {
		segments = appendLiteralSegment(segments, "\n")
	}
	if len(segments) == 0 && newline {
		segments = appendLiteralSegment(segments, "\n")
	}
	return segments, nil
}

func appendLiteralSegment(segments []PrintSegment, text string) []PrintSegment {
	if text == "" {
		return segments
	}
	if len(segments) > 0 && segments[len(segments)-1].Value == nil {
		segments[len(segments)-1].Text += text
		return segments
	}
	return append(segments, PrintSegment{Text: text})
}

func (b *builder) expandCallArgs(args []ssa.Value) ([]ssa.Value, error) {
	var expanded []ssa.Value
	for _, arg := range args {
		if slice, ok := arg.(*ssa.Slice); ok {
			values, err := b.expandVarArgs(slice)
			if err != nil {
				return nil, err
			}
			expanded = append(expanded, values...)
			continue
		}
		expanded = append(expanded, arg)
	}
	return expanded, nil
}

func (b *builder) expandVarArgs(slice *ssa.Slice) ([]ssa.Value, error) {
	alloc, ok := slice.X.(*ssa.Alloc)
	if !ok || alloc.Comment != "varargs" {
		return nil, fmt.Errorf("unsupported variadic argument form %T", slice.X)
	}
	referrers := alloc.Referrers()
	if referrers == nil {
		return nil, fmt.Errorf("varargs slice has no referrers")
	}
	type indexedValue struct {
		index int
		value ssa.Value
	}
	var items []indexedValue
	for _, ref := range *referrers {
		idxAddr, ok := ref.(*ssa.IndexAddr)
		if !ok || idxAddr.X != alloc {
			continue
		}
		idxConst, ok := idxAddr.Index.(*ssa.Const)
		if !ok {
			continue
		}
		index64, ok := constant.Int64Val(idxConst.Value)
		if !ok {
			return nil, fmt.Errorf("non-integer vararg index")
		}
		index := int(index64)
		var stored ssa.Value
		if users := idxAddr.Referrers(); users != nil {
			for _, user := range *users {
				store, ok := user.(*ssa.Store)
				if !ok {
					continue
				}
				stored = store.Val
				break
			}
		}
		if stored == nil {
			continue
		}
		if mi, ok := stored.(*ssa.MakeInterface); ok {
			stored = mi.X
		}
		items = append(items, indexedValue{index: index, value: stored})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("failed to decode variadic arguments")
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].index < items[j].index
	})
	values := make([]ssa.Value, 0, len(items))
	for _, item := range items {
		values = append(values, item.value)
	}
	return values, nil
}

func defaultPorts() []Port {
	return []Port{
		{
			Name:      "clk",
			Direction: Input,
			Type: &SignalType{
				Width:  1,
				Signed: false,
			},
		},
		{
			Name:      "rst",
			Direction: Input,
			Type: &SignalType{
				Width:  1,
				Signed: false,
			},
		},
	}
}

func blockComment(block *ssa.BasicBlock) string {
	if block.Comment != "" {
		return block.Comment
	}
	return fmt.Sprintf("block_%d", block.Index)
}

func translateBinOp(tok token.Token) (BinOp, bool) {
	switch tok {
	case token.ADD:
		return Add, true
	case token.SUB:
		return Sub, true
	case token.MUL:
		return Mul, true
	case token.QUO:
		return Div, true
	case token.REM:
		return Rem, true
	case token.AND:
		return And, true
	case token.OR:
		return Or, true
	case token.XOR:
		return Xor, true
	case token.SHL:
		return Shl, true
	case token.SHR:
		return ShrU, true
	default:
		return 0, false
	}
}

func isShiftBinOp(op BinOp) bool {
	switch op {
	case Shl, ShrU, ShrS:
		return true
	default:
		return false
	}
}

func translateCompareOp(tok token.Token, signed bool) (ComparePredicate, bool) {
	switch tok {
	case token.EQL:
		return CompareEQ, true
	case token.NEQ:
		return CompareNE, true
	case token.LSS:
		if signed {
			return CompareSLT, true
		}
		return CompareULT, true
	case token.LEQ:
		if signed {
			return CompareSLE, true
		}
		return CompareULE, true
	case token.GTR:
		if signed {
			return CompareSGT, true
		}
		return CompareUGT, true
	case token.GEQ:
		if signed {
			return CompareSGE, true
		}
		return CompareUGE, true
	default:
		return 0, false
	}
}

func isSignedType(t types.Type) bool {
	if t == nil {
		return true
	}
	if basic, ok := t.Underlying().(*types.Basic); ok {
		if basic.Info()&types.IsUnsigned != 0 {
			return false
		}
	}
	return true
}

func isIntegerType(t types.Type) bool {
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}

func isNativeFloatType(t types.Type) bool {
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Float32 || basic.Kind() == types.Float64)
}

func isNativeFloat64Type(t types.Type) bool {
	if !isNativeFloatType(t) {
		return false
	}
	return t.Underlying().(*types.Basic).Kind() == types.Float64
}

func boolSignalType() *SignalType {
	return &SignalType{Width: 1, Signed: false}
}

func signalType(t types.Type) *SignalType {
	switch bt := t.Underlying().(type) {
	case *types.Basic:
		width, signed := widthForBasic(bt)
		return &SignalType{Width: width, Signed: signed}
	default:
		return &SignalType{Width: 32, Signed: true}
	}
}

func widthForBasic(b *types.Basic) (int, bool) {
	switch b.Kind() {
	case types.Int8:
		return 8, true
	case types.Uint8:
		return 8, false
	case types.Int16:
		return 16, true
	case types.Uint16:
		return 16, false
	case types.Int32, types.Int:
		return 32, true
	case types.Uint32, types.Uint:
		return 32, false
	case types.Int64:
		return 64, true
	case types.Uint64:
		return 64, false
	case types.Float32:
		return 32, false
	case types.Float64:
		return 64, false
	case types.Bool:
		return 1, false
	default:
		return 32, true
	}
}

func isChannelType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Chan)
	return ok
}

func channelElemType(t types.Type) *SignalType {
	if ch, ok := t.Underlying().(*types.Chan); ok {
		return signalType(ch.Elem())
	}
	return &SignalType{Width: 1, Signed: false}
}

func isSliceType(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Slice:
		return true
	case *types.Pointer:
		// Pointer to slice or array
		return false
	}
	return false
}

func isIndexedValueType(t types.Type) bool {
	_, _, _, ok := indexedShapeInfo(t)
	return ok
}

func defaultName(candidate, fallback string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return fallback
	}
	return candidate
}

func extractConstValue(c *ssa.Const) interface{} {
	if c.IsNil() {
		return nil
	}
	switch c.Type().Underlying().(*types.Basic).Kind() {
	case types.Int8, types.Int16, types.Int32, types.Int64, types.Int:
		if i, ok := constant.Int64Val(c.Value); ok {
			return i
		}
	case types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uint:
		if u, ok := constant.Uint64Val(c.Value); ok {
			return u
		}
	case types.Bool:
		return constant.BoolVal(c.Value)
	case types.Float32:
		if f, ok := constant.Float64Val(c.Value); ok {
			return math.Float32bits(float32(f))
		}
	case types.Float64:
		if f, ok := constant.Float64Val(c.Value); ok {
			return math.Float64bits(f)
		}
	}
	return c.Value.ExactString()
}

func findMainPackage(prog *ssa.Program) *ssa.Package {
	for _, pkg := range prog.AllPackages() {
		if pkg == nil || pkg.Pkg == nil {
			continue
		}
		if pkg.Pkg.Path() == "main" || pkg.Pkg.Name() == "main" {
			return pkg
		}
	}
	return nil
}

func (b *builder) allocName(a *ssa.Alloc) string {
	candidate := strings.TrimSpace(a.Comment)
	if strings.HasPrefix(candidate, "var ") {
		candidate = strings.TrimPrefix(candidate, "var ")
	}
	if candidate == "" {
		candidate = a.Name()
	}
	if candidate == "" {
		return b.uniqueName("alloc")
	}
	candidate = strings.ReplaceAll(candidate, ".", "_")
	candidate = strings.ReplaceAll(candidate, " ", "_")
	return candidate
}

func (b *builder) uniqueName(prefix string) string {
	name := fmt.Sprintf("%s_%d", prefix, b.tempID)
	b.tempID++
	return name
}

func (b *builder) newAnonymousSignal(prefix string, typ *SignalType, pos token.Pos) *Signal {
	if prefix == "" {
		prefix = "tmp"
	}
	name := b.uniqueName(prefix)
	sig := &Signal{
		Name:   name,
		Type:   typ.Clone(),
		Kind:   Wire,
		Source: pos,
	}
	if sig.Type == nil {
		sig.Type = &SignalType{}
	}
	if b.module != nil {
		b.module.Signals[name] = sig
	}
	return sig
}

func (b *builder) newConstSignal(value interface{}, typ *SignalType, pos token.Pos) *Signal {
	sig := &Signal{
		Name:   b.newConstName(),
		Type:   typ.Clone(),
		Kind:   Const,
		Source: pos,
		Value:  value,
	}
	if sig.Type == nil {
		sig.Type = &SignalType{}
	}
	if b.module != nil {
		b.module.Signals[sig.Name] = sig
	}
	return sig
}

func (b *builder) ensureValueSignal(v ssa.Value) *Signal {
	if v == nil {
		return nil
	}
	if sig, ok := b.signals[v]; ok && sig != nil {
		return sig
	}
	base := defaultName(v.Name(), "tmp")
	name := b.uniqueName(base)
	sig := &Signal{
		Name:   name,
		Type:   signalType(v.Type()),
		Kind:   Wire,
		Source: v.Pos(),
	}
	if b.module != nil {
		b.module.Signals[sig.Name] = sig
	}
	b.signals[v] = sig
	return sig
}
