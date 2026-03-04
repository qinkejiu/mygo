package ir

import (
	"fmt"
	"go/token"
	"sort"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/ssa"
)

func (b *builder) analyzeChannels(prog *ssa.Program) {
	if b == nil || b.module == nil || prog == nil {
		return
	}
	bindings := b.analyzeCallGraphChannelBindings(prog)
	b.applyChannelParamBindings(bindings)
	b.buildChannelDependencyGraph()
	b.inferChannelDepths()
}

func (b *builder) analyzeCallGraphChannelBindings(prog *ssa.Program) map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{} {
	result := make(map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{})
	if prog == nil {
		return result
	}
	mainPkg := findMainPackage(prog)
	if mainPkg == nil {
		return result
	}
	mainFn := mainPkg.Func("main")
	if mainFn == nil {
		return result
	}

	for param, ch := range b.paramChannels {
		if param == nil || ch == nil {
			continue
		}
		fn := param.Parent()
		if fn == nil {
			continue
		}
		addFunctionParamBinding(result, fn, param, ch)
		b.addChannelParamBinding(param, ch)
	}

	cg := cha.CallGraph(prog)
	queue := []*ssa.Function{mainFn}
	inQueue := map[*ssa.Function]bool{mainFn: true}
	seen := make(map[*ssa.Function]bool)

	for len(queue) > 0 {
		fn := queue[0]
		queue = queue[1:]
		inQueue[fn] = false
		if fn == nil || len(fn.Blocks) == 0 {
			continue
		}
		seen[fn] = true

		current := result[fn]
		updates := b.inspectGoCalls(fn, current)
		for callee, params := range updates {
			if mergeFunctionBindings(result, callee, params) {
				if !inQueue[callee] {
					queue = append(queue, callee)
					inQueue[callee] = true
				}
			}
		}

		node := cg.Nodes[fn]
		if node == nil {
			continue
		}
		for _, edge := range node.Out {
			if edge == nil || edge.Callee == nil {
				continue
			}
			callee := edge.Callee.Func
			if callee == nil || len(callee.Blocks) == 0 {
				continue
			}
			if !seen[callee] && !inQueue[callee] {
				queue = append(queue, callee)
				inQueue[callee] = true
			}
		}
	}

	for _, fnBindings := range result {
		for param, channels := range fnBindings {
			for ch := range channels {
				b.addChannelParamBinding(param, ch)
			}
		}
	}

	return result
}

func (b *builder) inspectGoCalls(fn *ssa.Function, bindings map[*ssa.Parameter]map[*Channel]struct{}) map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{} {
	updates := make(map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{})
	if fn == nil {
		return updates
	}

	for _, block := range fn.Blocks {
		if block == nil {
			continue
		}
		for _, instr := range block.Instrs {
			goCall, ok := instr.(*ssa.Go)
			if !ok {
				continue
			}
			if goCall.Call.IsInvoke() {
				continue
			}
			callee := goCall.Call.StaticCallee()
			if callee == nil {
				continue
			}
			for idx, arg := range goCall.Call.Args {
				if idx >= len(callee.Params) {
					break
				}
				param := callee.Params[idx]
				if param == nil || !isChannelType(param.Type()) {
					continue
				}
				channels := b.resolveChannelsForValue(arg, bindings, make(map[ssa.Value]struct{}))
				if len(channels) == 0 {
					continue
				}
				for ch := range channels {
					addFunctionParamBinding(updates, callee, param, ch)
				}
			}
		}
	}

	return updates
}

func (b *builder) resolveChannelsForValue(v ssa.Value, bindings map[*ssa.Parameter]map[*Channel]struct{}, seen map[ssa.Value]struct{}) map[*Channel]struct{} {
	if v == nil {
		return nil
	}
	if _, ok := seen[v]; ok {
		return nil
	}
	seen[v] = struct{}{}

	result := make(map[*Channel]struct{})
	if ch := b.channelForValueSilent(v); ch != nil {
		result[ch] = struct{}{}
	}

	switch val := v.(type) {
	case *ssa.Parameter:
		mergeChannelSet(result, bindings[val])
		mergeChannelSet(result, b.channelParamBindings[val])
		if ch, ok := b.paramChannels[val]; ok && ch != nil {
			result[ch] = struct{}{}
		}
	case *ssa.ChangeType:
		mergeChannelSet(result, b.resolveChannelsForValue(val.X, bindings, seen))
	case *ssa.Phi:
		for _, edge := range val.Edges {
			mergeChannelSet(result, b.resolveChannelsForValue(edge, bindings, seen))
		}
	case *ssa.MakeInterface:
		mergeChannelSet(result, b.resolveChannelsForValue(val.X, bindings, seen))
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func (b *builder) applyChannelParamBindings(bindings map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{}) {
	for fn, paramBindings := range bindings {
		if fn == nil {
			continue
		}
		proc := b.processes[fn]
		if proc == nil {
			continue
		}
		usage := channelParamUsage(fn)
		for param, channels := range paramBindings {
			if param == nil {
				continue
			}
			dirs, ok := usage[param]
			if !ok {
				continue
			}
			for ch := range channels {
				if ch == nil {
					continue
				}
				if dirs.send {
					ch.AddEndpoint(proc, ChannelSend)
				}
				if dirs.recv {
					ch.AddEndpoint(proc, ChannelReceive)
				}
			}
		}
	}
}

func (b *builder) buildChannelDependencyGraph() {
	if b == nil || b.module == nil {
		return
	}
	for _, ch := range b.module.Channels {
		if ch == nil {
			continue
		}
		ch.Dependencies = ch.Dependencies[:0]
		producers := uniqueChannelProcesses(ch.Producers)
		consumers := uniqueChannelProcesses(ch.Consumers)
		seen := make(map[channelDependencyKey]struct{})
		for _, producer := range producers {
			if producer == nil {
				continue
			}
			for _, consumer := range consumers {
				if consumer == nil {
					continue
				}
				key := channelDependencyKey{producer: producer, consumer: consumer}
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				ch.Dependencies = append(ch.Dependencies, ChannelDependency{
					Producer: producer,
					Consumer: consumer,
				})
			}
		}
	}
}

func (b *builder) inferChannelDepths() {
	if b == nil || b.module == nil {
		return
	}
	for _, ch := range b.module.Channels {
		if ch == nil {
			continue
		}
		producers := uniqueChannelProcesses(ch.Producers)
		consumers := uniqueChannelProcesses(ch.Consumers)
		producerCount := len(producers)
		consumerCount := len(consumers)

		inferred := 0
		reason := ""
		switch {
		case producerCount == 1 && consumerCount == 1:
			producer := producers[0]
			consumer := consumers[0]
			if isCrossGoroutine(producer, consumer) {
				if ch.DeclaredDepth <= 1 {
					inferred = 2
					reason = "SPSC cross-goroutine: increased to 2 to avoid potential deadlock"
				} else {
					inferred = ch.DeclaredDepth
					reason = "SPSC cross-goroutine: declared depth is sufficient"
				}
			} else {
				inferred = ch.DeclaredDepth
				reason = "SPSC same goroutine: keeping declared depth"
			}
		case producerCount > 1 || consumerCount > 1:
			inferred = maxInt(ch.DeclaredDepth, 2)
			reason = "Multi-producer or multi-consumer: conservative minimum depth 2"
		default:
			inferred = ch.DeclaredDepth
			switch {
			case producerCount == 0 && consumerCount == 0:
				reason = "Warning: channel has no producers and no consumers"
			case producerCount == 0:
				reason = "Warning: channel has no producers"
			default:
				reason = "Warning: channel has no consumers"
			}
		}

		ch.InferredDepth = inferred
		ch.DepthReason = reason
		ch.Depth = maxInt(ch.DeclaredDepth, ch.InferredDepth)

		if b.reporter != nil && ch.InferredDepth != ch.DeclaredDepth {
			b.reporter.Info(ch.Source, fmt.Sprintf(
				"channel %q depth inference: declared=%d inferred=%d effective=%d (%s)",
				ch.Name,
				ch.DeclaredDepth,
				ch.InferredDepth,
				ch.Depth,
				ch.DepthReason,
			))
		}
	}
}

func isCrossGoroutine(p1, p2 *Process) bool {
	if p1 == nil || p2 == nil || p1 == p2 {
		return false
	}
	// Spawned processes originate from `go` statements and are always concurrent.
	if p1.Spawned || p2.Spawned {
		return true
	}
	// Stage/position checks are conservative fallbacks for partially annotated IR.
	if p1.Stage >= 0 && p2.Stage >= 0 && p1.Stage != p2.Stage {
		return true
	}
	if p1.Source != token.NoPos && p2.Source != token.NoPos && p1.Source != p2.Source {
		return true
	}
	return false
}

func (b *builder) addChannelParamBinding(param *ssa.Parameter, ch *Channel) {
	if b == nil || param == nil || ch == nil {
		return
	}
	if b.channelParamBindings == nil {
		b.channelParamBindings = make(map[*ssa.Parameter]map[*Channel]struct{})
	}
	set, ok := b.channelParamBindings[param]
	if !ok {
		set = make(map[*Channel]struct{})
		b.channelParamBindings[param] = set
	}
	set[ch] = struct{}{}
}

type channelParamDirection struct {
	send bool
	recv bool
}

type channelDependencyKey struct {
	producer *Process
	consumer *Process
}

type loopStructure struct {
	Header         *ssa.BasicBlock
	Body           []*ssa.BasicBlock
	Latches        []*ssa.BasicBlock
	Exits          []*ssa.BasicBlock
	ExitConditions []*ssa.If
	ExitEdges      []loopExitEdge
}

type loopExitEdge struct {
	From   *ssa.BasicBlock
	To     *ssa.BasicBlock
	If     *ssa.If
	IsTrue bool
}

func channelParamUsage(fn *ssa.Function) map[*ssa.Parameter]channelParamDirection {
	usage := make(map[*ssa.Parameter]channelParamDirection)
	if fn == nil {
		return usage
	}
	for _, block := range fn.Blocks {
		if block == nil {
			continue
		}
		for _, instr := range block.Instrs {
			switch inst := instr.(type) {
			case *ssa.Send:
				if param := channelParameterForValue(inst.Chan); param != nil {
					dirs := usage[param]
					dirs.send = true
					usage[param] = dirs
				}
			case *ssa.UnOp:
				if inst.Op != token.ARROW {
					continue
				}
				if param := channelParameterForValue(inst.X); param != nil {
					dirs := usage[param]
					dirs.recv = true
					usage[param] = dirs
				}
			}
		}
	}
	return usage
}

func channelParameterForValue(v ssa.Value) *ssa.Parameter {
	switch val := v.(type) {
	case *ssa.Parameter:
		if isChannelType(val.Type()) {
			return val
		}
	case *ssa.ChangeType:
		return channelParameterForValue(val.X)
	case *ssa.Phi:
		var candidate *ssa.Parameter
		for _, edge := range val.Edges {
			param := channelParameterForValue(edge)
			if param == nil {
				return nil
			}
			if candidate == nil {
				candidate = param
				continue
			}
			if candidate != param {
				return nil
			}
		}
		return candidate
	}
	return nil
}

func mergeFunctionBindings(dst map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{}, fn *ssa.Function, updates map[*ssa.Parameter]map[*Channel]struct{}) bool {
	if fn == nil || len(updates) == 0 {
		return false
	}
	fnMap, ok := dst[fn]
	if !ok {
		fnMap = make(map[*ssa.Parameter]map[*Channel]struct{})
		dst[fn] = fnMap
	}
	changed := false
	for param, channels := range updates {
		if param == nil || len(channels) == 0 {
			continue
		}
		set, ok := fnMap[param]
		if !ok {
			set = make(map[*Channel]struct{})
			fnMap[param] = set
		}
		for ch := range channels {
			if ch == nil {
				continue
			}
			if _, exists := set[ch]; exists {
				continue
			}
			set[ch] = struct{}{}
			changed = true
		}
	}
	return changed
}

func addFunctionParamBinding(bindings map[*ssa.Function]map[*ssa.Parameter]map[*Channel]struct{}, fn *ssa.Function, param *ssa.Parameter, ch *Channel) {
	if fn == nil || param == nil || ch == nil {
		return
	}
	fnMap, ok := bindings[fn]
	if !ok {
		fnMap = make(map[*ssa.Parameter]map[*Channel]struct{})
		bindings[fn] = fnMap
	}
	set, ok := fnMap[param]
	if !ok {
		set = make(map[*Channel]struct{})
		fnMap[param] = set
	}
	set[ch] = struct{}{}
}

func mergeChannelSet(dst map[*Channel]struct{}, src map[*Channel]struct{}) {
	if len(src) == 0 {
		return
	}
	for ch := range src {
		if ch != nil {
			dst[ch] = struct{}{}
		}
	}
}

func uniqueChannelProcesses(endpoints []*ChannelEndpoint) []*Process {
	if len(endpoints) == 0 {
		return nil
	}
	seen := make(map[*Process]struct{})
	processes := make([]*Process, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint == nil || endpoint.Process == nil {
			continue
		}
		if _, exists := seen[endpoint.Process]; exists {
			continue
		}
		seen[endpoint.Process] = struct{}{}
		processes = append(processes, endpoint.Process)
	}
	return processes
}

func findLoops(fn *ssa.Function) []loopStructure {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}

	dominators := computeDominators(fn)
	loopsByHeader := make(map[*ssa.BasicBlock]*loopStructure)
	loopNodeSets := make(map[*ssa.BasicBlock]map[*ssa.BasicBlock]struct{})

	for _, src := range fn.Blocks {
		if src == nil {
			continue
		}
		if jumpTerminator(src) == nil {
			continue
		}
		for _, dst := range src.Succs {
			if dst == nil {
				continue
			}
			if ifTerminator(dst) == nil {
				continue
			}
			if !blockDominates(dominators, dst, src) {
				continue
			}

			loop, ok := loopsByHeader[dst]
			if !ok {
				loop = &loopStructure{Header: dst}
				loopsByHeader[dst] = loop
				loopNodeSets[dst] = make(map[*ssa.BasicBlock]struct{})
			}
			addUniqueBlock(&loop.Latches, src)

			nodes := naturalLoopNodes(dst, src)
			dstSet := loopNodeSets[dst]
			for node := range nodes {
				dstSet[node] = struct{}{}
			}
		}
	}

	if len(loopsByHeader) == 0 {
		return nil
	}

	loops := make([]loopStructure, 0, len(loopsByHeader))
	for header, loop := range loopsByHeader {
		nodes := loopNodeSets[header]
		loop.Body = orderedLoopBody(header, nodes)
		loop.Exits, loop.ExitConditions, loop.ExitEdges = collectLoopExits(nodes)
		sortBasicBlocks(loop.Latches)
		sortBasicBlocks(loop.Exits)
		sortExitConditions(loop.ExitConditions)
		sort.Slice(loop.ExitEdges, func(i, j int) bool {
			a := loop.ExitEdges[i]
			b := loop.ExitEdges[j]
			if a.From != b.From {
				return blockIndex(a.From) < blockIndex(b.From)
			}
			if a.To != b.To {
				return blockIndex(a.To) < blockIndex(b.To)
			}
			if a.If != b.If {
				return ifIndex(a.If) < ifIndex(b.If)
			}
			return !a.IsTrue && b.IsTrue
		})
		loops = append(loops, *loop)
	}

	sort.Slice(loops, func(i, j int) bool {
		return blockIndex(loops[i].Header) < blockIndex(loops[j].Header)
	})
	return loops
}

func computeDominators(fn *ssa.Function) map[*ssa.BasicBlock]map[*ssa.BasicBlock]struct{} {
	dominators := make(map[*ssa.BasicBlock]map[*ssa.BasicBlock]struct{})
	if fn == nil || len(fn.Blocks) == 0 {
		return dominators
	}

	blocks := make([]*ssa.BasicBlock, 0, len(fn.Blocks))
	allBlocks := make(map[*ssa.BasicBlock]struct{})
	for _, block := range fn.Blocks {
		if block == nil {
			continue
		}
		blocks = append(blocks, block)
		allBlocks[block] = struct{}{}
	}
	if len(blocks) == 0 {
		return dominators
	}

	entry := blocks[0]
	for _, block := range blocks {
		if block == entry {
			dominators[block] = map[*ssa.BasicBlock]struct{}{block: {}}
			continue
		}
		dominators[block] = cloneBlockSet(allBlocks)
	}

	changed := true
	for changed {
		changed = false
		for _, block := range blocks {
			if block == entry {
				continue
			}
			next := make(map[*ssa.BasicBlock]struct{})
			firstPred := true
			for _, pred := range block.Preds {
				if pred == nil {
					continue
				}
				predDom := dominators[pred]
				if firstPred {
					next = cloneBlockSet(predDom)
					firstPred = false
					continue
				}
				next = intersectBlockSets(next, predDom)
			}
			next[block] = struct{}{}
			if !blockSetEqual(next, dominators[block]) {
				dominators[block] = next
				changed = true
			}
		}
	}

	return dominators
}

func blockDominates(dominators map[*ssa.BasicBlock]map[*ssa.BasicBlock]struct{}, dom, block *ssa.BasicBlock) bool {
	if dom == nil || block == nil {
		return false
	}
	set, ok := dominators[block]
	if !ok {
		return false
	}
	_, ok = set[dom]
	return ok
}

func naturalLoopNodes(header, latch *ssa.BasicBlock) map[*ssa.BasicBlock]struct{} {
	nodes := make(map[*ssa.BasicBlock]struct{})
	if header == nil || latch == nil {
		return nodes
	}
	nodes[header] = struct{}{}
	nodes[latch] = struct{}{}

	stack := []*ssa.BasicBlock{latch}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, pred := range n.Preds {
			if pred == nil {
				continue
			}
			if _, seen := nodes[pred]; seen {
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

func orderedLoopBody(header *ssa.BasicBlock, nodes map[*ssa.BasicBlock]struct{}) []*ssa.BasicBlock {
	if len(nodes) == 0 {
		return nil
	}
	body := make([]*ssa.BasicBlock, 0, len(nodes))
	for block := range nodes {
		if block == nil || block == header {
			continue
		}
		body = append(body, block)
	}
	sortBasicBlocks(body)
	return body
}

func collectLoopExits(nodes map[*ssa.BasicBlock]struct{}) ([]*ssa.BasicBlock, []*ssa.If, []loopExitEdge) {
	if len(nodes) == 0 {
		return nil, nil, nil
	}
	exitSet := make(map[*ssa.BasicBlock]struct{})
	condSet := make(map[*ssa.If]struct{})
	edges := make([]loopExitEdge, 0)

	for block := range nodes {
		if block == nil {
			continue
		}
		ifTerm := ifTerminator(block)
		for succIdx, succ := range block.Succs {
			if succ == nil {
				continue
			}
			if _, inside := nodes[succ]; inside {
				continue
			}
			exitSet[succ] = struct{}{}
			edge := loopExitEdge{
				From: block,
				To:   succ,
			}
			if ifTerm != nil {
				condSet[ifTerm] = struct{}{}
				edge.If = ifTerm
				edge.IsTrue = succIdx == 0
			}
			edges = append(edges, edge)
		}
	}

	exits := make([]*ssa.BasicBlock, 0, len(exitSet))
	for block := range exitSet {
		exits = append(exits, block)
	}
	conditions := make([]*ssa.If, 0, len(condSet))
	for ifInstr := range condSet {
		conditions = append(conditions, ifInstr)
	}

	return exits, conditions, edges
}

func ifTerminator(block *ssa.BasicBlock) *ssa.If {
	if block == nil || len(block.Instrs) == 0 {
		return nil
	}
	ifInstr, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil
	}
	return ifInstr
}

func jumpTerminator(block *ssa.BasicBlock) *ssa.Jump {
	if block == nil || len(block.Instrs) == 0 {
		return nil
	}
	jump, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Jump)
	if !ok {
		return nil
	}
	return jump
}

func addUniqueBlock(blocks *[]*ssa.BasicBlock, block *ssa.BasicBlock) {
	if blocks == nil || block == nil {
		return
	}
	for _, candidate := range *blocks {
		if candidate == block {
			return
		}
	}
	*blocks = append(*blocks, block)
}

func sortBasicBlocks(blocks []*ssa.BasicBlock) {
	sort.Slice(blocks, func(i, j int) bool {
		return blockIndex(blocks[i]) < blockIndex(blocks[j])
	})
}

func sortExitConditions(conditions []*ssa.If) {
	sort.Slice(conditions, func(i, j int) bool {
		return ifIndex(conditions[i]) < ifIndex(conditions[j])
	})
}

func blockIndex(block *ssa.BasicBlock) int {
	if block == nil {
		return -1
	}
	return block.Index
}

func ifIndex(ifInstr *ssa.If) int {
	if ifInstr == nil || ifInstr.Block() == nil {
		return -1
	}
	return ifInstr.Block().Index
}

func cloneBlockSet(src map[*ssa.BasicBlock]struct{}) map[*ssa.BasicBlock]struct{} {
	dst := make(map[*ssa.BasicBlock]struct{}, len(src))
	for block := range src {
		dst[block] = struct{}{}
	}
	return dst
}

func intersectBlockSets(a, b map[*ssa.BasicBlock]struct{}) map[*ssa.BasicBlock]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return make(map[*ssa.BasicBlock]struct{})
	}
	result := make(map[*ssa.BasicBlock]struct{})
	for block := range a {
		if _, ok := b[block]; ok {
			result[block] = struct{}{}
		}
	}
	return result
}

func blockSetEqual(a, b map[*ssa.BasicBlock]struct{}) bool {
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
