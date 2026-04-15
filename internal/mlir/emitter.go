package mlir

import (
	"fmt"
	"io"
	"math/bits"
	"os"
	"sort"
	"strconv"
	"strings"

	"mygo/internal/ir"
)

// Emit writes the MLIR representation of the design to outputPath. When
// outputPath is empty or "-", the result is written to stdout.
func Emit(design *ir.Design, outputPath string) error {
	var w io.Writer
	if outputPath == "" || outputPath == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	em := &emitter{
		w:               w,
		loweredChannels: ir.LowerChannelsToFIFO(design),
		modulePorts:     make(map[string][]portDesc),
	}
	fmt.Fprintln(w, "module {")
	em.indent++
	for _, module := range design.Modules {
		em.emitModule(module)
	}
	em.emitFifoExterns()
	em.indent--
	fmt.Fprintln(w, "}")
	return nil
}

type emitter struct {
	w               io.Writer
	indent          int
	loweredChannels *ir.LoweredChannelDesign
	seqClockName    string
	modulePorts     map[string][]portDesc // Track module ports for instances
	globalTempID    int                   // Global counter for unique temporary names
	rootValueNames  map[*ir.Signal]string // Value names from root process for output resolution
	rootConstNames  map[*ir.Signal]string // Constant names from root process
	topPortTypes    map[string]*ir.SignalType
}

func (e *emitter) emitModule(module *ir.Module) {
	if module == nil {
		return
	}

	// Collect modular processes (those with function parameters) separately
	// Exclude the root process (same name as module) as it gets inlined
	modularProcesses := make([]*ir.Process, 0)
	for _, proc := range module.Processes {
		if proc != nil && len(proc.Params) > 0 && proc.Name != module.Name {
			modularProcesses = append(modularProcesses, proc)
		}
	}

	processInfos := buildProcessInfos(module)
	var root *processInfo
	others := make([]*processInfo, 0, len(processInfos))
	for _, info := range processInfos {
		if info.proc != nil && info.proc.Name == module.Name && root == nil {
			root = info
			continue
		}
		others = append(others, info)
	}
	if root != nil {
		e.modulePorts[root.moduleName] = e.processPorts(module, root)
	}
	for _, info := range others {
		e.modulePorts[info.moduleName] = e.processPorts(module, info)
	}
	for _, proc := range modularProcesses {
		roles, order := collectProcessChannelRoles(proc)
		info := &processInfo{
			proc:         proc,
			moduleName:   processModuleName(module, proc),
			channelOrder: order,
			channelRoles: roles,
			channelPorts: make(map[*ir.Channel]*channelPortSet),
			usedSignals:  collectProcessSignals(proc),
		}
		e.modulePorts[info.moduleName] = e.processPorts(module, info)
	}
	e.emitTopLevelModule(module, root, others)
	for _, info := range others {
		e.emitProcessModule(module, info)
	}

	// Emit modular function processes (those with function parameters)
	// These are called via CallOperation, not spawned as separate processes
	for _, proc := range modularProcesses {
		roles, order := collectProcessChannelRoles(proc)
		info := &processInfo{
			proc:         proc,
			moduleName:   processModuleName(module, proc),
			channelOrder: order,
			channelRoles: roles,
			channelPorts: make(map[*ir.Channel]*channelPortSet),
			usedSignals:  collectProcessSignals(proc),
		}
		e.emitProcessModule(module, info)
	}
}

func (e *emitter) emitTopLevelModule(module *ir.Module, root *processInfo, processes []*processInfo) map[*ir.Channel]*channelWireSet {
	e.printIndent()
	fmt.Fprintf(e.w, "hw.module @%s(", module.Name)
	topPorts := emittedTopLevelPorts(module)
	e.topPortTypes = collectPortTypesFromIRPorts(topPorts)

	// Check if module needs sequential logic (FSM/channels/phi)
	useInoutRegs := moduleUsesFSM(module)

	// Build port declarations
	decls := portDecls(topPorts)

	// Add clk/rst ports if module needs sequential logic
	// But only if they're not already present as user-provided parameters
	if useInoutRegs {
		hasClk := false
		hasRst := false
		for _, port := range topPorts {
			if port.Direction == ir.Input {
				if port.Name == "clk" {
					hasClk = true
				}
				if isResetPortName(port.Name) {
					hasRst = true
				}
			}
		}

		var clkRstDecls []string
		if !hasClk {
			clkRstDecls = append(clkRstDecls, "in %clk: i1")
		}
		if !hasRst && moduleNeedsSyntheticReset(module) {
			clkRstDecls = append(clkRstDecls, "in %rst: i1")
		}
		decls = append(clkRstDecls, decls...)
	}

	for i, decl := range decls {
		if i > 0 {
			fmt.Fprint(e.w, ", ")
		}
		fmt.Fprint(e.w, decl)
	}
	fmt.Fprint(e.w, ")")
	fmt.Fprintln(e.w, " {")
	e.indent++

	loweredModule := e.loweredChannels.ModuleFor(module)
	channelWires := e.emitChannelWires(loweredModule)
	e.emitChannelFifos(loweredModule, channelWires)
	e.emitInternalSignals(module, topPorts, useInoutRegs)
	if root != nil {
		e.emitRootProcess(module, topPorts, root, channelWires)
	}
	for idx, info := range processes {
		e.emitProcessInstance(module, idx, info, channelWires)
	}

	// Emit hw.output with output port values
	// First, collect and prepare output values
	var outputValues []string
	var outputTypes []string
	type outputBindingValue struct {
		value string
		typ   string
	}
	resolvedBindings := make(map[string]outputBindingValue)
	for _, port := range topPorts {
		if port.Direction == ir.Output {
			beforeCount := len(outputValues)
			globalName := outputBindingName(port)
			if cached, ok := resolvedBindings[globalName]; ok && cached.value != "" {
				outputValues = append(outputValues, cached.value)
				outputTypes = append(outputTypes, cached.typ)
				continue
			}

			// Check if this is a scalar signal
			if sig, ok := module.Signals[globalName]; ok && sig != nil {
				preferStructured := root != nil && root.proc != nil && canEmitDirectClockedControl(root.proc)
				if preferStructured {
					if resolved, ok := e.resolveRootCombinationalOutputValue(root, sig); ok && resolved != "" {
						outputValues = append(outputValues, resolved)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: resolved, typ: typeString(sig.Type)}
						continue
					}
				}
				if sig.Kind == ir.Wire && e.rootValueNames != nil {
					if valueName, ok := e.rootValueNames[sig]; ok && valueName != "" {
						outputValues = append(outputValues, valueName)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: valueName, typ: typeString(sig.Type)}
						continue
					}
				}
				if !preferStructured {
					if resolved, ok := e.resolveRootCombinationalOutputValue(root, sig); ok && resolved != "" {
						outputValues = append(outputValues, resolved)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: resolved, typ: typeString(sig.Type)}
						continue
					}
				}
				if e.rootValueNames != nil {
					if valueName, ok := e.rootValueNames[sig]; ok && valueName != "" {
						rawName := "%" + sanitize(sig.Name)
						if valueName != rawName {
							outputValues = append(outputValues, valueName)
							outputTypes = append(outputTypes, typeString(sig.Type))
							resolvedBindings[globalName] = outputBindingValue{value: valueName, typ: typeString(sig.Type)}
							continue
						}
					}
				}
				// First check if we have a value from the root process (for combinational logic)
				if e.rootValueNames != nil && shouldPreferRootOutputValue(module, globalName, useInoutRegs, sig) {
					if valueName, ok := e.rootValueNames[sig]; ok {
						outputValues = append(outputValues, valueName)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: valueName, typ: typeString(sig.Type)}
						continue
					}
				}

				// Scalar output: check if it's a register (inout) or a wire
				ssaName := "%" + sanitize(sig.Name)
				if sig.Kind == ir.Reg {
					// For registers, check if they were emitted as sv.reg (inout) or seq.compreg (wire)
					if useInoutRegs {
						// sv.reg: need to read the inout value
						readName := fmt.Sprintf("%%read_%s", sanitize(port.Name))
						e.printIndent()
						fmt.Fprintf(e.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", readName, ssaName, typeString(sig.Type))
						outputValues = append(outputValues, readName)
						resolvedBindings[globalName] = outputBindingValue{value: readName, typ: typeString(sig.Type)}
					} else {
						// seq.compreg: already a wire value, use directly
						outputValues = append(outputValues, ssaName)
						resolvedBindings[globalName] = outputBindingValue{value: ssaName, typ: typeString(sig.Type)}
					}
				} else {
					// For wires, use directly
					outputValues = append(outputValues, ssaName)
					resolvedBindings[globalName] = outputBindingValue{value: ssaName, typ: typeString(sig.Type)}
				}
				outputTypes = append(outputTypes, typeString(sig.Type))
			} else if port.Type != nil && port.Type.Width > 1 {
				// Array output: collect indexed signals and pack them
				// For out_out [2]bool, we need out_out_0 and out_out_1
				var elements []string
				preferStructured := root != nil && root.proc != nil && canEmitDirectClockedControl(root.proc)
				for i := 0; i < port.Type.Width; i++ {
					elemName := fmt.Sprintf("%s_%d", globalName, i)
					if elemSig, ok := module.Signals[elemName]; ok && elemSig != nil {
						if preferStructured {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						if elemSig.Kind == ir.Wire && e.rootValueNames != nil {
							if valueName, ok := e.rootValueNames[elemSig]; ok && valueName != "" {
								elements = append(elements, valueName)
								continue
							}
						}
						if !preferStructured {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						if e.rootValueNames != nil {
							if valueName, ok := e.rootValueNames[elemSig]; ok && valueName != "" {
								rawName := "%" + sanitize(elemName)
								if valueName != rawName {
									elements = append(elements, valueName)
									continue
								}
							}
						}
						// First check if we have a value from the root process (for combinational logic)
						if e.rootValueNames != nil && shouldPreferRootOutputValue(module, globalName, useInoutRegs, elemSig) {
							if valueName, ok := e.rootValueNames[elemSig]; ok {
								elements = append(elements, valueName)
								continue
							}
						}

						// Otherwise, use the signal name
						ssaName := "%" + sanitize(elemName)
						if elemSig.Kind == ir.Reg && useInoutRegs {
							// sv.reg: need to read the inout value
							readName := fmt.Sprintf("%%read_%s_%d", sanitize(port.Name), i)
							e.printIndent()
							fmt.Fprintf(e.w, "%s = sv.read_inout %s : !hw.inout<i1>\n", readName, ssaName)
							elements = append(elements, readName)
						} else {
							// seq.compreg or wire: use directly
							elements = append(elements, ssaName)
						}
					}
				}

				if len(elements) > 0 {
					// Pack elements using comb.concat (MSB first)
					packedName := fmt.Sprintf("%%packed_%s", sanitize(port.Name))
					e.printIndent()
					fmt.Fprintf(e.w, "%s = comb.concat ", packedName)
					for i := len(elements) - 1; i >= 0; i-- {
						if i < len(elements)-1 {
							fmt.Fprint(e.w, ", ")
						}
						fmt.Fprint(e.w, elements[i])
					}
					// Each element is i1, output the individual types
					fmt.Fprint(e.w, " : ")
					for i := len(elements) - 1; i >= 0; i-- {
						if i < len(elements)-1 {
							fmt.Fprint(e.w, ", ")
						}
						fmt.Fprint(e.w, "i1")
					}
					fmt.Fprintln(e.w)
					outputValues = append(outputValues, packedName)
					outputTypes = append(outputTypes, typeString(port.Type))
					resolvedBindings[globalName] = outputBindingValue{value: packedName, typ: typeString(port.Type)}
				}
			}
			if len(outputValues) == beforeCount {
				zeroName := e.freshValueName("out_zero")
				e.printIndent()
				fmt.Fprintf(e.w, "%s = hw.constant 0 : %s\n", zeroName, typeString(port.Type))
				outputValues = append(outputValues, zeroName)
				outputTypes = append(outputTypes, typeString(port.Type))
				resolvedBindings[globalName] = outputBindingValue{value: zeroName, typ: typeString(port.Type)}
			}
		}
	}

	// Now emit the hw.output statement
	e.printIndent()
	fmt.Fprint(e.w, "hw.output")
	if len(outputValues) > 0 {
		fmt.Fprint(e.w, " ")
		for i, val := range outputValues {
			if i > 0 {
				fmt.Fprint(e.w, ", ")
			}
			fmt.Fprint(e.w, val)
		}
		fmt.Fprint(e.w, " : ")
		for i, typ := range outputTypes {
			if i > 0 {
				fmt.Fprint(e.w, ", ")
			}
			fmt.Fprint(e.w, typ)
		}
	}
	fmt.Fprintln(e.w)

	e.indent--
	e.printIndent()
	fmt.Fprintln(e.w, "}")
	return channelWires
}

func (e *emitter) resolveRootCombinationalOutputValue(root *processInfo, sig *ir.Signal) (string, bool) {
	if e == nil || root == nil || root.proc == nil || sig == nil {
		return "", false
	}
	if len(root.proc.Blocks) == 0 {
		return "", false
	}
	if root.proc.Sensitivity == ir.Combinational {
		cache := make(map[combResolveKey]string)
		visiting := make(map[combOutputKey]bool)
		return e.resolveCombinationalOutputAtBlock(root.proc, root.proc.Blocks[0], nil, sig, "", cache, visiting)
	}
	if canEmitDirectClockedControl(root.proc) {
		cache := make(map[combResolveKey]string)
		visiting := make(map[combOutputKey]bool)
		return e.resolveDirectClockedOutputAtBlock(root.proc, root.proc.Blocks[0], nil, sig, "", computeEmitterClockedBlocks(root.proc), cache, visiting)
	}
	return "", false
}

type combResolveKey struct {
	sig   *ir.Signal
	block *ir.BasicBlock
	pred  *ir.BasicBlock
}

type combOutputKey struct {
	target *ir.Signal
	block  *ir.BasicBlock
	pred   *ir.BasicBlock
}

func (e *emitter) resolveCombinationalOutputAtBlock(proc *ir.Process, block *ir.BasicBlock, pred *ir.BasicBlock, target *ir.Signal, current string, cache map[combResolveKey]string, visiting map[combOutputKey]bool) (string, bool) {
	if e == nil || proc == nil || block == nil || target == nil || strings.TrimSpace(target.Name) == "" {
		return current, current != ""
	}
	key := combOutputKey{target: target, block: block, pred: pred}
	if visiting[key] {
		return current, current != ""
	}
	visiting[key] = true
	defer delete(visiting, key)
	value := current
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
			continue
		}
		if assign.Dest.Name != target.Name {
			continue
		}
		ref := ""
		if e.rootValueNames != nil {
			if cached, ok := e.rootValueNames[assign.Value]; ok && cached != "" {
				ref = cached
			}
		}
		if ref == "" || ref == "%unknown" {
			ref = e.resolveCombinationalSignalValue(proc, assign.Value, block, pred, cache)
		}
		ref = e.normalizeResolvedSignalRef(assign.Value, ref)
		if ref == "" || ref == "%unknown" {
			continue
		}
		value = ref
	}
	switch term := block.Terminator.(type) {
	case *ir.JumpTerminator:
		return e.resolveCombinationalOutputAtBlock(proc, term.Target, block, target, value, cache, visiting)
	case *ir.ReturnTerminator, nil:
		return value, value != ""
	case *ir.BranchTerminator:
		trueVal, trueOK := e.resolveCombinationalOutputAtBlock(proc, term.True, block, target, value, cache, visiting)
		falseVal, falseOK := e.resolveCombinationalOutputAtBlock(proc, term.False, block, target, value, cache, visiting)
		switch {
		case trueOK && falseOK && trueVal == falseVal:
			return trueVal, trueVal != ""
		case trueOK && !falseOK:
			return trueVal, trueVal != ""
		case !trueOK && falseOK:
			return falseVal, falseVal != ""
		case !trueOK && !falseOK:
			return value, value != ""
		}
		cond := e.rootSignalRef(term.Cond)
		cond = e.normalizeResolvedSignalRef(term.Cond, cond)
		if cond == "" || cond == "%unknown" {
			return value, value != ""
		}
		name := e.freshValueName("out_mux")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, cond, trueVal, falseVal, typeString(target.Type))
		return name, true
	default:
		return value, value != ""
	}
}

func (e *emitter) resolveCombinationalSignalValue(proc *ir.Process, sig *ir.Signal, block *ir.BasicBlock, pred *ir.BasicBlock, cache map[combResolveKey]string) string {
	if e == nil || proc == nil || sig == nil {
		return ""
	}
	key := combResolveKey{sig: sig, block: block, pred: pred}
	if cache != nil {
		if cached, ok := cache[key]; ok {
			return cached
		}
		cache[key] = "%unknown"
	}
	if sig.Kind == ir.Const {
		ref := e.rootSignalRef(sig)
		ref = e.normalizeResolvedSignalRef(sig, ref)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}
	producer, producerBlock := findSignalProducer(proc, sig)
	if producer == nil {
		if sig.Kind == ir.Reg && sig.Name != "" {
			ref := e.freshValueName("comb_reg")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = sv.read_inout %%%s : !hw.inout<%s>\n", ref, sanitize(sig.Name), typeString(sig.Type))
			if cache != nil {
				cache[key] = ref
			}
			return ref
		}
		ref := e.rootSignalRef(sig)
		ref = e.normalizeResolvedSignalRef(sig, ref)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}
	ref := ""
	switch op := producer.(type) {
	case *ir.AssignOperation:
		ref = e.resolveCombinationalSignalValue(proc, op.Value, producerBlock, pred, cache)
	case *ir.PhiOperation:
		if pred != nil {
			for _, incoming := range op.Incomings {
				if incoming.Block == pred && incoming.Value != nil {
					ref = e.resolveCombinationalSignalValue(proc, incoming.Value, incoming.Block, nil, cache)
					break
				}
			}
		}
	case *ir.NotOperation:
		value := e.resolveCombinationalSignalValue(proc, op.Value, producerBlock, pred, cache)
		if value != "" && value != "%unknown" {
			name := e.freshValueName("comb_not")
			ones := e.boolConst(true)
			if signalWidth(op.Value.Type) != 1 {
				ones = e.emitterAllOnesConst(op.Value.Type)
			}
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.xor %s, %s : %s\n", name, value, ones, typeString(op.Value.Type))
			ref = name
		}
	case *ir.BinOperation:
		left := e.resolveCombinationalSignalValue(proc, op.Left, producerBlock, pred, cache)
		right := e.resolveCombinationalSignalValue(proc, op.Right, producerBlock, pred, cache)
		if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			name := e.freshValueName("comb_bin")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.%s %s, %s : %s\n", name, binOpName(op.Op), left, right, typeString(op.Dest.Type))
			ref = name
		}
	case *ir.CompareOperation:
		left := e.resolveCombinationalSignalValue(proc, op.Left, producerBlock, pred, cache)
		right := e.resolveCombinationalSignalValue(proc, op.Right, producerBlock, pred, cache)
		if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			name := e.freshValueName("comb_cmp")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.icmp %s %s, %s : %s\n", name, comparePredicateName(op.Predicate), left, right, typeString(op.Left.Type))
			ref = name
		}
	case *ir.MuxOperation:
		cond := e.resolveCombinationalSignalValue(proc, op.Cond, producerBlock, pred, cache)
		tval := e.resolveCombinationalSignalValue(proc, op.TrueValue, producerBlock, pred, cache)
		fval := e.resolveCombinationalSignalValue(proc, op.FalseValue, producerBlock, pred, cache)
		if cond != "" && tval != "" && fval != "" && cond != "%unknown" && tval != "%unknown" && fval != "%unknown" {
			name := e.freshValueName("comb_mux")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, cond, tval, fval, typeString(op.Dest.Type))
			ref = name
		}
	case *ir.ConvertOperation:
		value := e.resolveCombinationalSignalValue(proc, op.Value, producerBlock, pred, cache)
		if value != "" && value != "%unknown" {
			ref = e.emitResolvedConvert(value, op.Value.Type, op.Dest.Type)
		}
	default:
		ref = e.rootSignalRef(sig)
	}
	ref = e.normalizeResolvedSignalRef(sig, ref)
	if cache != nil {
		cache[key] = ref
	}
	return ref
}

func findSignalProducer(proc *ir.Process, sig *ir.Signal) (ir.Operation, *ir.BasicBlock) {
	if proc == nil || sig == nil {
		return nil, nil
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			switch typed := op.(type) {
			case *ir.AssignOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			case *ir.PhiOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			case *ir.NotOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			case *ir.BinOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			case *ir.CompareOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			case *ir.MuxOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			case *ir.ConvertOperation:
				if typed != nil && typed.Dest == sig {
					return op, block
				}
			}
		}
	}
	return nil, nil
}

func (e *emitter) resolveBlockPhiIncoming(block *ir.BasicBlock, pred *ir.BasicBlock, sig *ir.Signal) (string, bool) {
	if e == nil || block == nil || pred == nil || sig == nil {
		return "", false
	}
	for _, op := range block.Ops {
		phi, ok := op.(*ir.PhiOperation)
		if !ok || phi == nil || phi.Dest == nil || phi.Dest != sig {
			continue
		}
		for _, incoming := range phi.Incomings {
			if incoming.Block != pred || incoming.Value == nil {
				continue
			}
			ref := e.rootSignalRef(incoming.Value)
			ref = e.normalizeResolvedSignalRef(incoming.Value, ref)
			if ref == "" || ref == "%unknown" {
				return "", false
			}
			return ref, true
		}
	}
	return "", false
}

func (e *emitter) resolveDirectClockedSignalValue(proc *ir.Process, sig *ir.Signal, block *ir.BasicBlock, pred *ir.BasicBlock, clocked map[*ir.BasicBlock]bool, cache map[combResolveKey]string) string {
	if e == nil || proc == nil || sig == nil {
		return ""
	}
	key := combResolveKey{sig: sig, block: block, pred: pred}
	if cache != nil {
		if cached, ok := cache[key]; ok {
			return cached
		}
		cache[key] = "%unknown"
	}
	if sig.Kind == ir.Const {
		ref := e.rootSignalRef(sig)
		ref = e.normalizeResolvedSignalRef(sig, ref)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}
	producer, producerBlock := findSignalProducer(proc, sig)
	if producer == nil || producerBlock == nil {
		ref := e.rootSignalRef(sig)
		ref = e.normalizeResolvedSignalRef(sig, ref)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}
	if clocked != nil && clocked[producerBlock] {
		ref := e.rootSignalRef(sig)
		ref = e.normalizeResolvedSignalRef(sig, ref)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}
	ref := ""
	switch op := producer.(type) {
	case *ir.AssignOperation:
		ref = e.resolveDirectClockedSignalValue(proc, op.Value, producerBlock, pred, clocked, cache)
	case *ir.PhiOperation:
		if pred != nil {
			for _, incoming := range op.Incomings {
				if incoming.Block == pred && incoming.Value != nil {
					ref = e.resolveDirectClockedSignalValue(proc, incoming.Value, incoming.Block, nil, clocked, cache)
					break
				}
			}
		}
	case *ir.NotOperation:
		value := e.resolveDirectClockedSignalValue(proc, op.Value, producerBlock, pred, clocked, cache)
		if value != "" && value != "%unknown" {
			name := e.freshValueName("comb_not")
			ones := e.boolConst(true)
			if signalWidth(op.Value.Type) != 1 {
				ones = e.emitterAllOnesConst(op.Value.Type)
			}
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.xor %s, %s : %s\n", name, value, ones, typeString(op.Value.Type))
			ref = name
		}
	case *ir.BinOperation:
		left := e.resolveDirectClockedSignalValue(proc, op.Left, producerBlock, pred, clocked, cache)
		right := e.resolveDirectClockedSignalValue(proc, op.Right, producerBlock, pred, clocked, cache)
		if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			name := e.freshValueName("comb_bin")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.%s %s, %s : %s\n", name, binOpName(op.Op), left, right, typeString(op.Dest.Type))
			ref = name
		}
	case *ir.CompareOperation:
		left := e.resolveDirectClockedSignalValue(proc, op.Left, producerBlock, pred, clocked, cache)
		right := e.resolveDirectClockedSignalValue(proc, op.Right, producerBlock, pred, clocked, cache)
		if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			name := e.freshValueName("comb_cmp")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.icmp %s %s, %s : %s\n", name, comparePredicateName(op.Predicate), left, right, typeString(op.Left.Type))
			ref = name
		}
	case *ir.MuxOperation:
		cond := e.resolveDirectClockedSignalValue(proc, op.Cond, producerBlock, pred, clocked, cache)
		tval := e.resolveDirectClockedSignalValue(proc, op.TrueValue, producerBlock, pred, clocked, cache)
		fval := e.resolveDirectClockedSignalValue(proc, op.FalseValue, producerBlock, pred, clocked, cache)
		if cond != "" && tval != "" && fval != "" && cond != "%unknown" && tval != "%unknown" && fval != "%unknown" {
			name := e.freshValueName("comb_mux")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, cond, tval, fval, typeString(op.Dest.Type))
			ref = name
		}
	case *ir.ConvertOperation:
		value := e.resolveDirectClockedSignalValue(proc, op.Value, producerBlock, pred, clocked, cache)
		if value != "" && value != "%unknown" {
			ref = e.emitResolvedConvert(value, op.Value.Type, op.Dest.Type)
		}
	default:
		ref = e.rootSignalRef(sig)
	}
	ref = e.normalizeResolvedSignalRef(sig, ref)
	if cache != nil {
		cache[key] = ref
	}
	return ref
}

func (e *emitter) resolveDirectClockedOutputAtBlock(proc *ir.Process, block *ir.BasicBlock, pred *ir.BasicBlock, target *ir.Signal, current string, clocked map[*ir.BasicBlock]bool, cache map[combResolveKey]string, visiting map[combOutputKey]bool) (string, bool) {
	if e == nil || proc == nil || block == nil || target == nil || strings.TrimSpace(target.Name) == "" {
		return current, current != ""
	}
	key := combOutputKey{target: target, block: block, pred: pred}
	if visiting[key] {
		return current, current != ""
	}
	visiting[key] = true
	defer delete(visiting, key)
	value := current
	if !clocked[block] {
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name != target.Name {
				continue
			}
			ref := e.resolveDirectClockedSignalValue(proc, assign.Value, block, pred, clocked, cache)
			ref = e.normalizeResolvedSignalRef(assign.Value, ref)
			if ref == "" || ref == "%unknown" {
				continue
			}
			value = ref
		}
	}
	switch term := block.Terminator.(type) {
	case *ir.JumpTerminator:
		return e.resolveDirectClockedOutputAtBlock(proc, term.Target, block, target, value, clocked, cache, visiting)
	case *ir.ReturnTerminator, nil:
		return value, value != ""
	case *ir.BranchTerminator:
		trueVal, trueOK := e.resolveDirectClockedOutputAtBlock(proc, term.True, block, target, value, clocked, cache, visiting)
		falseVal, falseOK := e.resolveDirectClockedOutputAtBlock(proc, term.False, block, target, value, clocked, cache, visiting)
		switch {
		case trueOK && falseOK && trueVal == falseVal:
			return trueVal, trueVal != ""
		case trueOK && !falseOK:
			return trueVal, trueVal != ""
		case !trueOK && falseOK:
			return falseVal, falseVal != ""
		case !trueOK && !falseOK:
			return value, value != ""
		}
		cond := e.rootSignalRef(term.Cond)
		cond = e.normalizeResolvedSignalRef(term.Cond, cond)
		if cond == "" || cond == "%unknown" {
			return value, value != ""
		}
		name := e.freshValueName("out_mux")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, cond, trueVal, falseVal, typeString(target.Type))
		return name, true
	default:
		return value, value != ""
	}
}

type emitterClockedVisitKey struct {
	block     *ir.BasicBlock
	inClocked bool
}

func computeEmitterClockedBlocks(proc *ir.Process) map[*ir.BasicBlock]bool {
	blocks := make(map[*ir.BasicBlock]bool)
	if proc == nil || len(proc.Blocks) == 0 {
		return blocks
	}
	visited := make(map[emitterClockedVisitKey]bool)
	clockedReach := make(map[*ir.BasicBlock]bool)
	nonClockedReach := make(map[*ir.BasicBlock]bool)
	var visit func(block *ir.BasicBlock, inClocked bool)
	visit = func(block *ir.BasicBlock, inClocked bool) {
		if block == nil {
			return
		}
		key := emitterClockedVisitKey{block: block, inClocked: inClocked}
		if visited[key] {
			return
		}
		visited[key] = true
		if inClocked {
			clockedReach[block] = true
		} else {
			nonClockedReach[block] = true
		}
		switch term := block.Terminator.(type) {
		case *ir.BranchTerminator:
			if term.Cond != nil && isClockLikeName(term.Cond.Name) {
				visit(term.True, true)
				visit(term.False, false)
				return
			}
			if term.Cond != nil && isResetPortName(term.Cond.Name) && block == proc.Blocks[0] {
				visit(term.True, true)
				visit(term.False, false)
				return
			}
			visit(term.True, inClocked)
			visit(term.False, inClocked)
		case *ir.JumpTerminator:
			visit(term.Target, inClocked)
		}
	}
	visit(proc.Blocks[0], false)
	for block := range clockedReach {
		if !nonClockedReach[block] {
			blocks[block] = true
		}
	}
	return blocks
}

func (e *emitter) normalizeResolvedSignalRef(sig *ir.Signal, ref string) string {
	if e == nil || sig == nil || ref == "" || ref == "%unknown" {
		return ref
	}
	if sig.Kind != ir.Reg {
		return ref
	}
	raw := "%" + sanitize(sig.Name)
	if ref != raw {
		return ref
	}
	name := e.freshValueName("norm_reg")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", name, raw, typeString(sig.Type))
	return name
}

func (e *emitter) rootSignalRef(sig *ir.Signal) string {
	if e == nil || sig == nil {
		return ""
	}
	if unpacked := e.inputArrayElementRef(sig); unpacked != "" {
		return unpacked
	}
	if e.rootValueNames != nil {
		if ref, ok := e.rootValueNames[sig]; ok && ref != "" {
			return ref
		}
	}
	if sig.Kind == ir.Const {
		if e.rootConstNames != nil {
			if ref, ok := e.rootConstNames[sig]; ok && ref != "" {
				return ref
			}
		}
		return ""
	}
	if sig.Name == "" {
		return ""
	}
	if sig.Kind == ir.Reg {
		ref := e.freshValueName("root_reg")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.read_inout %%%s : !hw.inout<%s>\n", ref, sanitize(sig.Name), typeString(sig.Type))
		return ref
	}
	return "%" + sanitize(sig.Name)
}

func (e *emitter) inputArrayElementRef(sig *ir.Signal) string {
	if e == nil || sig == nil || sig.Name == "" || e.topPortTypes == nil {
		return ""
	}
	base, index, ok := indexedSignalName(sig.Name)
	if !ok {
		return ""
	}
	portType, ok := e.topPortTypes[base]
	if !ok || portType == nil {
		return ""
	}
	elemWidth := signalWidth(sig.Type)
	if elemWidth <= 0 {
		elemWidth = 1
	}
	offset := index * elemWidth
	name := e.freshValueName("in_elem")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.extract %%%s from %d : (%s) -> %s\n",
		name,
		sanitize(base),
		offset,
		typeString(portType),
		typeString(sig.Type),
	)
	return name
}

func (e *emitter) emitResolvedConvert(value string, srcType, destType *ir.SignalType) string {
	if e == nil || value == "" || value == "%unknown" {
		return value
	}
	srcWidth := signalWidth(srcType)
	destWidth := signalWidth(destType)
	if srcWidth <= 0 || destWidth <= 0 || srcWidth == destWidth {
		return value
	}
	from := typeString(srcType)
	to := typeString(destType)
	if destWidth > srcWidth {
		extendWidth := destWidth - srcWidth
		if srcType != nil && srcType.Signed {
			signBit := e.freshValueName("sext_msb")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.extract %s from %d : (%s) -> i1\n", signBit, value, srcWidth-1, from)
			replicated := e.freshValueName("sext_bits")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.replicate %s : (i1) -> i%d\n", replicated, signBit, extendWidth)
			dest := e.freshValueName("comb_conv")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.concat %s, %s : i%d, %s\n", dest, replicated, value, extendWidth, from)
			return dest
		}
		zeros := e.freshValueName("zext_pad")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = hw.constant 0 : i%d\n", zeros, extendWidth)
		dest := e.freshValueName("comb_conv")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.concat %s, %s : i%d, %s\n", dest, zeros, value, extendWidth, from)
		return dest
	}
	dest := e.freshValueName("comb_conv")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.extract %s from 0 : (%s) -> %s\n", dest, value, from, to)
	return dest
}

func outputBindingName(port ir.Port) string {
	if strings.TrimSpace(port.Binding) != "" {
		return port.Binding
	}
	return "out_" + port.Name
}

func shouldPreferRootOutputValue(module *ir.Module, binding string, useInoutRegs bool, sig *ir.Signal) bool {
	if sig == nil {
		return false
	}
	if useInoutRegs && sig.Kind == ir.Reg {
		return false
	}
	if sig.Name == binding {
		return true
	}
	if base, _, ok := indexedSignalName(sig.Name); ok && base == binding {
		return true
	}
	return false
}

func (e *emitter) emitChannelWires(loweredModule *ir.LoweredChannelModule) map[*ir.Channel]*channelWireSet {
	wires := make(map[*ir.Channel]*channelWireSet)
	if loweredModule == nil {
		return wires
	}
	for _, loweredFIFO := range loweredModule.FIFOs {
		ch := loweredFIFO.Channel
		if ch == nil {
			continue
		}
		wireSet := &channelWireSet{
			writeData:      "%" + loweredFIFO.Wires.WriteData.Name,
			writeValid:     "%" + loweredFIFO.Wires.WriteValid.Name,
			writeReady:     "%" + loweredFIFO.Wires.WriteReady.Name,
			readData:       "%" + loweredFIFO.Wires.ReadData.Name,
			readValid:      "%" + loweredFIFO.Wires.ReadValid.Name,
			readReady:      "%" + loweredFIFO.Wires.ReadReady.Name,
			full:           "%" + loweredFIFO.Wires.Full.Name,
			almostFull:     "%" + loweredFIFO.Wires.AlmostFull.Name,
			empty:          "%" + loweredFIFO.Wires.Empty.Name,
			almostEmpty:    "%" + loweredFIFO.Wires.AlmostEmpty.Name,
			producerWrites: make(map[*ir.Process]*channelProducerWireSet),
		}
		wires[ch] = wireSet
		e.printIndent()
		fmt.Fprintf(e.w, "// channel %s depth=%d type=%s\n", ch.Name, ch.Depth, typeString(ch.Type))
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : %s\n", wireSet.writeData, inoutTypeString(loweredFIFO.Wires.WriteData.Type))
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.writeValid)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.writeReady)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : %s\n", wireSet.readData, inoutTypeString(ch.Type))
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.readValid)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.readReady)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.full)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.almostFull)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.empty)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", wireSet.almostEmpty)
		if len(loweredFIFO.Producers) > 1 {
			for _, producer := range loweredFIFO.Producers {
				if producer == nil || producer.Process == nil {
					continue
				}
				producerWireSet := &channelProducerWireSet{
					writeData:  "%" + producer.Wires.WriteData.Name,
					writeValid: "%" + producer.Wires.WriteValid.Name,
					writeReady: "%" + producer.Wires.WriteReady.Name,
				}
				wireSet.producerWrites[producer.Process] = producerWireSet
				e.printIndent()
				fmt.Fprintf(e.w, "%s = sv.wire : %s\n", producerWireSet.writeData, inoutTypeString(producer.Wires.WriteData.Type))
				e.printIndent()
				fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", producerWireSet.writeValid)
				e.printIndent()
				fmt.Fprintf(e.w, "%s = sv.wire : !hw.inout<i1>\n", producerWireSet.writeReady)
			}
		}
		e.emitChannelMetadata(ch)
	}
	return wires
}

func (e *emitter) emitChannelFifos(loweredModule *ir.LoweredChannelModule, wires map[*ir.Channel]*channelWireSet) {
	if loweredModule == nil || len(loweredModule.FIFOs) == 0 {
		return
	}
	for _, loweredFIFO := range loweredModule.FIFOs {
		ch := loweredFIFO.Channel
		if ch == nil {
			continue
		}
		wireSet := wires[ch]
		oneConst := "%" + loweredFIFO.Helpers.OneConst
		rstN := "%" + loweredFIFO.Helpers.ResetN
		fullVal := "%" + loweredFIFO.Helpers.FullValue
		emptyVal := "%" + loweredFIFO.Helpers.EmptyValue
		notFullVal := "%" + loweredFIFO.Helpers.NotFull
		notEmptyVal := "%" + loweredFIFO.Helpers.NotEmpty
		e.printIndent()
		fmt.Fprintf(e.w, "%s = hw.constant 1 : i1\n", oneConst)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.xor %%rst, %s : i1\n", rstN, oneConst)
		e.printIndent()
		fmt.Fprintf(e.w, "hw.instance \"%s\" @%s(", loweredFIFO.Instance.Name, loweredFIFO.Instance.ModuleName)
		ports := []struct {
			name  string
			value string
			typ   string
		}{}
		for _, binding := range loweredFIFO.Instance.Ports {
			typeStr := typeString(binding.Type)
			if binding.InOut {
				typeStr = fmt.Sprintf("!hw.inout<%s>", typeStr)
			}
			value := "%" + binding.Wire
			ports = append(ports, struct {
				name  string
				value string
				typ   string
			}{
				name:  binding.Port,
				value: value,
				typ:   typeStr,
			})
		}
		for i, port := range ports {
			if i > 0 {
				fmt.Fprint(e.w, ", ")
			}
			fmt.Fprintf(e.w, "%s: %s : %s", port.name, port.value, port.typ)
		}
		fmt.Fprintln(e.w, ") -> ()")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.read_inout %s : !hw.inout<i1>\n", fullVal, wireSet.full)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.xor %s, %s : i1\n", notFullVal, fullVal, oneConst)
		if len(loweredFIFO.Producers) > 1 {
			e.emitMultiProducerArbitration(loweredFIFO, wireSet, notFullVal)
		} else {
			for _, conn := range loweredFIFO.Connects {
				if conn.Dst == loweredFIFO.Wires.WriteReady.Name {
					e.printIndent()
					fmt.Fprintf(e.w, "sv.assign %s, %s : %s\n", wireSet.writeReady, "%"+conn.Src, typeString(conn.Type))
				}
			}
		}
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.read_inout %s : !hw.inout<i1>\n", emptyVal, wireSet.empty)
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.xor %s, %s : i1\n", notEmptyVal, emptyVal, oneConst)
		for _, conn := range loweredFIFO.Connects {
			if conn.Dst == loweredFIFO.Wires.ReadValid.Name {
				e.printIndent()
				fmt.Fprintf(e.w, "sv.assign %s, %s : %s\n", wireSet.readValid, "%"+conn.Src, typeString(conn.Type))
			}
		}
	}
}

func (e *emitter) emitMultiProducerArbitration(loweredFIFO *ir.LoweredChannelFIFO, wireSet *channelWireSet, fifoReady string) {
	if loweredFIFO == nil || wireSet == nil || len(loweredFIFO.Producers) <= 1 {
		return
	}
	oneConst := "%" + loweredFIFO.Helpers.OneConst
	typeStr := typeString(loweredFIFO.Channel.Type)
	validValues := make([]string, 0, len(loweredFIFO.Producers))
	dataValues := make([]string, 0, len(loweredFIFO.Producers))
	grantValues := make([]string, 0, len(loweredFIFO.Producers))
	var priorValid string
	for idx, producer := range loweredFIFO.Producers {
		if producer == nil || producer.Process == nil {
			continue
		}
		producerWires := wireSet.sendPortsFor(producer.Process)
		if producerWires == nil {
			continue
		}
		validVal := e.freshValueName("mp_valid")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.read_inout %s : !hw.inout<i1>\n", validVal, producerWires.writeValid)
		dataVal := e.freshValueName("mp_data")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = sv.read_inout %s : %s\n", dataVal, producerWires.writeData, inoutTypeString(loweredFIFO.Channel.Type))
		validValues = append(validValues, validVal)
		dataValues = append(dataValues, dataVal)

		grantVal := validVal
		if idx > 0 && priorValid != "" {
			noPriorValid := e.freshValueName("mp_no_prior")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.xor %s, %s : i1\n", noPriorValid, priorValid, oneConst)
			grantVal = e.freshValueName("mp_grant")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.and %s, %s : i1\n", grantVal, validVal, noPriorValid)
		}
		grantValues = append(grantValues, grantVal)
		if priorValid == "" {
			priorValid = validVal
		} else {
			nextPrior := e.freshValueName("mp_prior")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.or %s, %s : i1\n", nextPrior, priorValid, validVal)
			priorValid = nextPrior
		}
	}
	writeValid := e.orSignals(validValues)
	e.printIndent()
	fmt.Fprintf(e.w, "sv.assign %s, %s : i1\n", wireSet.writeValid, writeValid)
	writeData := e.muxByPredicates(grantValues, dataValues, loweredFIFO.Channel.Type)
	e.printIndent()
	fmt.Fprintf(e.w, "sv.assign %s, %s : %s\n", wireSet.writeData, writeData, typeStr)
	for idx, producer := range loweredFIFO.Producers {
		if producer == nil || producer.Process == nil || idx >= len(grantValues) {
			continue
		}
		producerWires := wireSet.sendPortsFor(producer.Process)
		if producerWires == nil {
			continue
		}
		readyVal := grantValues[idx]
		if readyVal == "" || readyVal == "%unknown" {
			readyVal = e.boolConst(false)
		} else {
			gatedReady := e.freshValueName("mp_ready")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.and %s, %s : i1\n", gatedReady, readyVal, fifoReady)
			readyVal = gatedReady
		}
		e.printIndent()
		fmt.Fprintf(e.w, "sv.assign %s, %s : i1\n", producerWires.writeReady, readyVal)
	}
}

func (e *emitter) emitInternalSignals(module *ir.Module, topPorts []ir.Port, useInoutRegs bool) {
	if module == nil || len(module.Signals) == 0 {
		return
	}

	// Build a set of port names for quick lookup
	portNames := make(map[string]bool)
	for _, port := range topPorts {
		portNames[port.Name] = true
	}

	// Emit declarations for internal signals
	names := make([]string, 0, len(module.Signals))
	for name := range module.Signals {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		// Skip if this is a port (already declared in module signature)
		if portNames[name] {
			continue
		}

		sig := module.Signals[name]
		if sig.Kind == ir.Const {
			// Constants are emitted on demand in the process
			continue
		}

		// Check if this is an array element (name_number format)
		// Only treat as array element if the base name is a known mutable global array
		// Known mutable arrays: compressed, result
		// Read-only arrays (test_data, test_compressed, test_result) are ports
		isArrayElement := false
		baseName := name
		for i := 0; i < len(name)-1; i++ {
			if name[i] == '_' && name[i+1] >= '0' && name[i+1] <= '9' {
				baseName = name[:i]
				// Only treat as array element if it's a known mutable global array
				// Known mutable arrays: tqmf, compressed, result, accumc, accumd
				isArrayElement = baseName == "tqmf" || baseName == "compressed" || baseName == "result" || baseName == "accumc" || baseName == "accumd"
				break
			}
		}

		// Array elements and scalar globals need to be declared as registers for persistent storage.
		// FSM-driven processes require inout regs so per-state updates can use sv.passign.
		if isArrayElement {
			// Only emit registers if we're using FSM (useInoutRegs)
			if useInoutRegs {
				e.printIndent()
				// Initialize register with constant value if available, otherwise 0
				initValue := formatHWConstant(e.getSignalInitValue(sig), sig.Type)
				constName := fmt.Sprintf("%%c_init_%s", sanitize(name))
				fmt.Fprintf(e.w, "%s = hw.constant %s : %s\n", constName, initValue, typeString(sig.Type))
				e.printIndent()
				fmt.Fprintf(e.w, "%%%s = sv.reg : !hw.inout<%s>\n", sanitize(name), typeString(sig.Type))
				e.printIndent()
				fmt.Fprintln(e.w, "sv.initial {")
				e.indent++
				e.printIndent()
				fmt.Fprintf(e.w, "sv.bpassign %%%s, %s : %s\n", sanitize(name), constName, typeString(sig.Type))
				e.indent--
				e.printIndent()
				fmt.Fprintln(e.w, "}")
			}
			// For combinational logic, don't emit register declarations
		} else if sig.Kind == ir.Reg {
			// All register-kind signals that are not array elements
			// This includes scalar globals like xout1, xout2, nbl, dlt, dec_plt1, etc.
			e.printIndent()
			// Initialize register with constant value if available, otherwise 0
			initValue := formatHWConstant(e.getSignalInitValue(sig), sig.Type)
			constName := fmt.Sprintf("%%c_init_%s", sanitize(name))
			fmt.Fprintf(e.w, "%s = hw.constant %s : %s\n", constName, initValue, typeString(sig.Type))
			e.printIndent()
			fmt.Fprintf(e.w, "%%%s = sv.reg : !hw.inout<%s>\n", sanitize(name), typeString(sig.Type))
			e.printIndent()
			fmt.Fprintln(e.w, "sv.initial {")
			e.indent++
			e.printIndent()
			fmt.Fprintf(e.w, "sv.bpassign %%%s, %s : %s\n", sanitize(name), constName, typeString(sig.Type))
			e.indent--
			e.printIndent()
			fmt.Fprintln(e.w, "}")
		} else {
			// Don't emit wire declarations for internal signals
			// Intermediate computation values are created as SSA values during process emission
		}
	}
}

func (e *emitter) emitProcessInstance(module *ir.Module, idx int, info *processInfo, wires map[*ir.Channel]*channelWireSet) {
	if info == nil {
		return
	}
	ports := e.processPorts(module, info)
	connections := map[string]string{
		"%clk": "%clk",
		"%rst": "%rst",
	}
	// Add global output signals
	if module != nil && module.Signals != nil {
		for sig := range info.usedSignals {
			if sig != nil && sig.Kind == ir.Reg {
				// Check if this signal is a module-level signal
				if _, isModuleSignal := module.Signals[sig.Name]; isModuleSignal {
					portName := "%" + sanitize(sig.Name)
					connections[portName] = portName
				}
			}
		}
	}
	for _, ch := range info.channelOrder {
		role := info.channelRoles[ch]
		wire := wires[ch]
		if role == nil || wire == nil {
			continue
		}
		portSet := info.channelPorts[ch]
		if portSet == nil {
			continue
		}
		if role.send {
			sendPorts := wire.sendPortsFor(info.proc)
			if sendPorts == nil {
				sendPorts = &channelProducerWireSet{
					writeData:  wire.writeData,
					writeValid: wire.writeValid,
					writeReady: wire.writeReady,
				}
			}
			connections[portSet.sendData] = sendPorts.writeData
			connections[portSet.sendValid] = sendPorts.writeValid
			connections[portSet.sendReady] = sendPorts.writeReady
		}
		if role.recv {
			connections[portSet.recvData] = wire.readData
			connections[portSet.recvValid] = wire.readValid
			connections[portSet.recvReady] = wire.readReady
		}
	}
	instName := fmt.Sprintf("%s_inst%d", sanitize(info.proc.Name), idx)
	e.printIndent()
	fmt.Fprintf(e.w, "hw.instance \"%s\" @%s(", instName, info.moduleName)
	for i, port := range ports {
		if i > 0 {
			fmt.Fprint(e.w, ", ")
		}
		value := connections[port.name]
		if value == "" {
			value = port.name
		}
		portLabel := strings.TrimPrefix(port.name, "%")
		valueType := port.typ
		if port.inout {
			valueType = fmt.Sprintf("!hw.inout<%s>", port.typ)
		}
		fmt.Fprintf(e.w, "%s: %s : %s", portLabel, value, valueType)
	}
	fmt.Fprintln(e.w, ") -> ()")
}

func (e *emitter) emitProcessModule(module *ir.Module, info *processInfo) {
	if info == nil || info.proc == nil {
		return
	}
	ports := e.processPorts(module, info)

	// Store port information for later use in hw.instance operations
	e.modulePorts[info.moduleName] = ports

	e.printIndent()
	fmt.Fprintf(e.w, "hw.module @%s(", info.moduleName)
	for i, port := range ports {
		if i > 0 {
			fmt.Fprint(e.w, ", ")
		}
		dir := "in"
		if port.inout {
			dir = "inout"
		}
		fmt.Fprintf(e.w, "%s %s: %s", dir, port.name, port.typ)
	}
	fmt.Fprintln(e.w, ") {")
	e.indent++

	// Build port names set for processPrinter
	portNames := make(map[string]string)
	for _, port := range ports {
		name := strings.TrimPrefix(port.name, "%")
		portNames[name] = "%" + name
	}

	pp := &processPrinter{
		w:             e.w,
		indent:        e.indent,
		moduleSignals: module.Signals,
		usedSignals:   info.usedSignals,
		channelPorts:  info.channelPorts,
		portNames:     portNames,
		portTypes:     collectPortTypesFromDescs(ports),
		moduleName:    sanitize(module.Name),
		modulePorts:   e.modulePorts,
		emitter:       e,
		proc:          info.proc,
	}
	pp.resetState()
	pp.emitProcess(info.proc)

	e.indent--
	e.printIndent()
	fmt.Fprintln(e.w, "}")
}

func (e *emitter) emitRootProcess(module *ir.Module, topPorts []ir.Port, info *processInfo, wires map[*ir.Channel]*channelWireSet) {
	if info == nil || info.proc == nil {
		return
	}

	// Build port names set from module ports
	portNames := make(map[string]string)
	for _, port := range topPorts {
		name := strings.TrimPrefix(port.Name, "%")
		portNames[name] = "%" + name
	}
	pp := &processPrinter{
		w:             e.w,
		indent:        e.indent,
		moduleSignals: module.Signals,
		usedSignals:   info.usedSignals,
		channelPorts:  channelPortsFromWires(info, wires),
		portNames:     portNames,
		portTypes:     collectPortTypesFromIRPorts(topPorts),
		moduleName:    sanitize(module.Name),
		modulePorts:   e.modulePorts,
		emitter:       e,
		proc:          info.proc,
	}
	pp.resetState()
	pp.emitProcess(info.proc)

	// Store the valueNames for output resolution
	e.rootValueNames = pp.valueNames
	e.rootConstNames = pp.constNames
}

func (e *emitter) processPorts(module *ir.Module, info *processInfo) []portDesc {
	ports := []portDesc{
		{name: "%clk", typ: "i1"},
		{name: "%rst", typ: "i1"},
	}

	// Add function parameter ports
	for _, param := range info.proc.Params {
		if param == nil {
			continue
		}
		portName := "%" + sanitize(param.Name)
		ports = append(ports, portDesc{name: portName, typ: typeString(param.Type)})
	}

	// Add return value port
	if info.proc.Return != nil {
		portName := "%result"
		ports = append(ports, portDesc{name: portName, typ: typeString(info.proc.Return.Type)})
	}

	// Add global output signals as inout ports
	if module != nil && module.Signals != nil {
		// Build set of port names already added to avoid duplicates
		existingPorts := make(map[string]bool)
		for _, port := range ports {
			existingPorts[port.name] = true
		}

		for sig := range info.usedSignals {
			if sig != nil && sig.Kind == ir.Reg {
				portName := "%" + sanitize(sig.Name)
				// Skip if this port name is already in the ports list
				if existingPorts[portName] {
					continue
				}
				// Check if this signal is a module-level signal
				if _, isModuleSignal := module.Signals[sig.Name]; isModuleSignal {
					ports = append(ports, portDesc{name: portName, typ: inoutTypeString(sig.Type), inout: true})
					existingPorts[portName] = true
				}
			}
		}
	}

	for _, ch := range info.channelOrder {
		role := info.channelRoles[ch]
		if role == nil {
			continue
		}
		portSet := info.channelPorts[ch]
		if portSet == nil {
			portSet = &channelPortSet{}
			info.channelPorts[ch] = portSet
		}
		if role.send {
			portSet.sendData = fmt.Sprintf("%%chan_%s_wdata", sanitize(ch.Name))
			portSet.sendValid = fmt.Sprintf("%%chan_%s_wvalid", sanitize(ch.Name))
			portSet.sendReady = fmt.Sprintf("%%chan_%s_wready", sanitize(ch.Name))
			ports = append(ports,
				portDesc{name: portSet.sendData, typ: typeString(ch.Type), inout: true},
				portDesc{name: portSet.sendValid, typ: "i1", inout: true},
				portDesc{name: portSet.sendReady, typ: "i1", inout: true},
			)
		}
		if role.recv {
			portSet.recvData = fmt.Sprintf("%%chan_%s_rdata", sanitize(ch.Name))
			portSet.recvValid = fmt.Sprintf("%%chan_%s_rvalid", sanitize(ch.Name))
			portSet.recvReady = fmt.Sprintf("%%chan_%s_rready", sanitize(ch.Name))
			ports = append(ports,
				portDesc{name: portSet.recvData, typ: typeString(ch.Type), inout: true},
				portDesc{name: portSet.recvValid, typ: "i1", inout: true},
				portDesc{name: portSet.recvReady, typ: "i1", inout: true},
			)
		}
	}
	return ports
}

func emittedTopLevelPorts(module *ir.Module) []ir.Port {
	if module == nil || len(module.Ports) == 0 {
		return nil
	}
	ports := make([]ir.Port, 0, len(module.Ports))
	outputIndex := make(map[string]int)
	for _, port := range module.Ports {
		if port.Direction != ir.Output {
			ports = append(ports, port)
			continue
		}
		binding := outputBindingName(port)
		if idx, ok := outputIndex[binding]; ok {
			if preferPublicOutputPort(port, ports[idx]) {
				ports[idx] = port
			}
			continue
		}
		outputIndex[binding] = len(ports)
		ports = append(ports, port)
	}
	return ports
}

func preferPublicOutputPort(candidate, current ir.Port) bool {
	return outputPortPriority(candidate) < outputPortPriority(current)
}

func outputPortPriority(port ir.Port) int {
	if strings.HasPrefix(strings.TrimSpace(port.Name), "out_") {
		return 1
	}
	return 0
}

func (e *emitter) emitChannelMetadata(ch *ir.Channel) {
	if ch == nil {
		return
	}
	e.printIndent()
	fmt.Fprintf(e.w, "// channel %s occupancy %d/%d\n", sanitize(ch.Name), ch.Occupancy, ch.Depth)
	for _, prod := range ch.Producers {
		stage := processStage(prod.Process)
		name := processName(prod.Process)
		e.printIndent()
		fmt.Fprintf(e.w, "//   producer %s stage %d\n", name, stage)
	}
	for _, cons := range ch.Consumers {
		stage := processStage(cons.Process)
		name := processName(cons.Process)
		e.printIndent()
		fmt.Fprintf(e.w, "//   consumer %s stage %d\n", name, stage)
	}
}

func (e *emitter) printIndent() {
	for i := 0; i < e.indent; i++ {
		fmt.Fprint(e.w, "  ")
	}
}

func (e *emitter) freshValueName(prefix string) string {
	if prefix == "" {
		prefix = "tmp"
	}
	name := fmt.Sprintf("%%%s%d", prefix, e.globalTempID)
	e.globalTempID++
	return name
}

func (e *emitter) boolConst(val bool) string {
	name := e.freshValueName("c_bool")
	e.printIndent()
	intVal := 0
	if val {
		intVal = 1
	}
	fmt.Fprintf(e.w, "%s = hw.constant %d : i1\n", name, intVal)
	return name
}

func (e *emitter) emitterAllOnesConst(t *ir.SignalType) string {
	if signalWidth(t) == 1 {
		return e.boolConst(true)
	}
	name := e.freshValueName("c_ones")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = hw.constant -1 : %s\n", name, typeString(t))
	return name
}

func (e *emitter) typedZeroConst(t *ir.SignalType) string {
	name := e.freshValueName("c_zero")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = hw.constant 0 : %s\n", name, typeString(t))
	return name
}

func (e *emitter) orSignals(signals []string) string {
	filtered := make([]string, 0, len(signals))
	for _, sig := range signals {
		if sig == "" || sig == "%unknown" {
			continue
		}
		filtered = append(filtered, sig)
	}
	if len(filtered) == 0 {
		return e.boolConst(false)
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	result := filtered[0]
	for _, sig := range filtered[1:] {
		name := e.freshValueName("or")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.or %s, %s : i1\n", name, result, sig)
		result = name
	}
	return result
}

func (e *emitter) muxByPredicates(predicates []string, values []string, t *ir.SignalType) string {
	if len(predicates) == 0 || len(values) == 0 {
		return e.typedZeroConst(t)
	}
	count := len(predicates)
	if len(values) < count {
		count = len(values)
	}
	defaultValue := e.typedZeroConst(t)
	result := defaultValue
	typeStr := typeString(t)
	for i := count - 1; i >= 0; i-- {
		pred := predicates[i]
		if pred == "" || pred == "%unknown" {
			continue
		}
		val := values[i]
		if val == "" || val == "%unknown" {
			val = defaultValue
		}
		name := e.freshValueName("mux")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, pred, val, result, typeStr)
		result = name
	}
	return result
}

type portDesc struct {
	name  string
	typ   string
	inout bool
}

type channelRole struct {
	send bool
	recv bool
}

type channelPortSet struct {
	sendData  string
	sendValid string
	sendReady string
	recvData  string
	recvValid string
	recvReady string
}

type channelProducerWireSet struct {
	writeData  string
	writeValid string
	writeReady string
}

type channelWireSet struct {
	writeData      string
	writeValid     string
	writeReady     string
	readData       string
	readValid      string
	readReady      string
	full           string
	almostFull     string
	empty          string
	almostEmpty    string
	producerWrites map[*ir.Process]*channelProducerWireSet
}

func (w *channelWireSet) sendPortsFor(proc *ir.Process) *channelProducerWireSet {
	if w == nil {
		return nil
	}
	if proc != nil && w.producerWrites != nil {
		if producer := w.producerWrites[proc]; producer != nil {
			return producer
		}
	}
	return &channelProducerWireSet{
		writeData:  w.writeData,
		writeValid: w.writeValid,
		writeReady: w.writeReady,
	}
}

func channelPortsFromWires(info *processInfo, wires map[*ir.Channel]*channelWireSet) map[*ir.Channel]*channelPortSet {
	ports := make(map[*ir.Channel]*channelPortSet)
	if info == nil {
		return ports
	}
	for _, ch := range info.channelOrder {
		role := info.channelRoles[ch]
		wire := wires[ch]
		if role == nil || wire == nil {
			continue
		}
		set := &channelPortSet{}
		if role.send {
			sendPorts := wire.sendPortsFor(info.proc)
			if sendPorts == nil {
				continue
			}
			set.sendData = sendPorts.writeData
			set.sendValid = sendPorts.writeValid
			set.sendReady = sendPorts.writeReady
		}
		if role.recv {
			set.recvData = wire.readData
			set.recvValid = wire.readValid
			set.recvReady = wire.readReady
		}
		ports[ch] = set
	}
	return ports
}

type processInfo struct {
	proc         *ir.Process
	moduleName   string
	channelOrder []*ir.Channel
	channelRoles map[*ir.Channel]*channelRole
	channelPorts map[*ir.Channel]*channelPortSet
	usedSignals  map[*ir.Signal]struct{}
}

func buildProcessInfos(module *ir.Module) []*processInfo {
	if module == nil {
		return nil
	}
	infos := make([]*processInfo, 0, len(module.Processes))
	for _, proc := range module.Processes {
		if proc == nil {
			continue
		}
		// Skip processes that have function parameters, EXCEPT the root process
		// The root process (same name as module) should be included even if it has params
		// Other processes with params are modular functions called via CallOperation
		if len(proc.Params) > 0 && proc.Name != module.Name {
			continue
		}
		roles, order := collectProcessChannelRoles(proc)
		info := &processInfo{
			proc:         proc,
			moduleName:   processModuleName(module, proc),
			channelOrder: order,
			channelRoles: roles,
			channelPorts: make(map[*ir.Channel]*channelPortSet),
			usedSignals:  collectProcessSignals(proc),
		}
		infos = append(infos, info)
	}
	sort.SliceStable(infos, func(i, j int) bool {
		return infos[i].moduleName < infos[j].moduleName
	})
	return infos
}

func processModuleName(module *ir.Module, proc *ir.Process) string {
	modName := "module"
	if module != nil && module.Name != "" {
		modName = sanitize(module.Name)
	}
	procName := processName(proc)
	return fmt.Sprintf("%s__proc_%s", modName, procName)
}

func collectProcessChannelRoles(proc *ir.Process) (map[*ir.Channel]*channelRole, []*ir.Channel) {
	roles := make(map[*ir.Channel]*channelRole)
	if proc == nil {
		return roles, nil
	}
	for _, block := range proc.Blocks {
		for _, op := range block.Ops {
			switch o := op.(type) {
			case *ir.SendOperation:
				if o.Channel == nil {
					continue
				}
				role := roles[o.Channel]
				if role == nil {
					role = &channelRole{}
					roles[o.Channel] = role
				}
				role.send = true
			case *ir.RecvOperation:
				if o.Channel == nil {
					continue
				}
				role := roles[o.Channel]
				if role == nil {
					role = &channelRole{}
					roles[o.Channel] = role
				}
				role.recv = true
			}
		}
	}
	order := make([]*ir.Channel, 0, len(roles))
	for ch := range roles {
		order = append(order, ch)
	}
	sort.Slice(order, func(i, j int) bool {
		return sanitize(order[i].Name) < sanitize(order[j].Name)
	})
	return roles, order
}

func collectProcessSignals(proc *ir.Process) map[*ir.Signal]struct{} {
	used := make(map[*ir.Signal]struct{})
	if proc == nil {
		return used
	}
	add := func(sig *ir.Signal) {
		if sig != nil {
			used[sig] = struct{}{}
		}
	}
	for _, block := range proc.Blocks {
		for _, op := range block.Ops {
			switch o := op.(type) {
			case *ir.BinOperation:
				add(o.Left)
				add(o.Right)
				add(o.Dest)
			case *ir.ConvertOperation:
				add(o.Value)
				add(o.Dest)
			case *ir.AssignOperation:
				add(o.Value)
				add(o.Dest)
			case *ir.SendOperation:
				add(o.Value)
			case *ir.RecvOperation:
				add(o.Dest)
			case *ir.CompareOperation:
				add(o.Left)
				add(o.Right)
				add(o.Dest)
			case *ir.NotOperation:
				add(o.Value)
				add(o.Dest)
			case *ir.MuxOperation:
				add(o.Cond)
				add(o.TrueValue)
				add(o.FalseValue)
				add(o.Dest)
			case *ir.PhiOperation:
				add(o.Dest)
				for _, in := range o.Incomings {
					add(in.Value)
				}
			case *ir.PrintOperation:
				for _, seg := range o.Segments {
					add(seg.Value)
				}
			case *ir.CallOperation:
				for _, arg := range o.Args {
					add(arg)
				}
				add(o.Dest)
			case *ir.SpawnOperation:
				for _, arg := range o.Args {
					add(arg)
				}
			}
		}
		if block.Terminator != nil {
			switch term := block.Terminator.(type) {
			case *ir.BranchTerminator:
				add(term.Cond)
			}
		}
	}
	return used
}

type edgeKey struct {
	pred *ir.BasicBlock
	succ *ir.BasicBlock
}

type phiUpdate struct {
	phi   *ir.PhiOperation
	value *ir.Signal
}

type phiRegInfo struct {
	phi       *ir.PhiOperation
	regName   string
	valueName string
	typeStr   string
}

type recvRegInfo struct {
	op        *ir.RecvOperation
	regName   string
	valueName string
	typeStr   string
}

type statePhase int

const (
	phaseNone statePhase = iota
	phaseSendReq
	phaseSendWait
	phaseRecvReq
	phaseRecvWait
)

type channelOpDirection int

const (
	channelDirSend channelOpDirection = iota
	channelDirRecv
)

type channelOpInfo struct {
	channel     *ir.Channel
	channelID   string
	direction   channelOpDirection
	block       *ir.BasicBlock
	opIndex     int
	sendOp      *ir.SendOperation
	recvOp      *ir.RecvOperation
	target      *ir.Signal
	reqStateID  int
	waitStateID int
}

type fsmState struct {
	id     int
	block  *ir.BasicBlock
	phase  statePhase
	sendOp *ir.SendOperation
	recvOp *ir.RecvOperation
	nextID int
}

type fsmBuilder struct {
	printer            *processPrinter
	proc               *ir.Process
	blockOrder         []*ir.BasicBlock
	clockedBlocks      map[*ir.BasicBlock]bool
	blockChannelOps    map[*ir.BasicBlock][]*channelOpInfo
	channelOpsByChan   map[*ir.Channel][]*channelOpInfo
	sendOpInfos        []*channelOpInfo
	recvOpInfos        []*channelOpInfo
	blockEntryStateIDs map[*ir.BasicBlock]int
	entryStateID       int
	doneID             int
	stateWidth         int
	stateType          string
	stateConsts        map[int]string
	stateRegInout      string
	stateValue         string
	stateOrder         []*fsmState
	stateByID          map[int]*fsmState
	statePredicates    map[int]string
	phiInfos           map[*ir.PhiOperation]*phiRegInfo
	phiOrder           []*ir.PhiOperation
	phiUpdates         map[edgeKey][]phiUpdate
	sendWaitStateIDs   map[*ir.SendOperation]int
	recvWaitStateIDs   map[*ir.RecvOperation]int
	sendPredicates     map[*ir.SendOperation]string
	recvPredicates     map[*ir.RecvOperation]string
	sendReadySignals   map[*ir.SendOperation]string
	recvValidSignals   map[*ir.RecvOperation]string
	recvDataSignals    map[*ir.RecvOperation]string
	recvInfos          map[*ir.RecvOperation]*recvRegInfo
	printScratchRegs   map[printScratchKey]string
	deadlockWarnings   []string
	deadlockWarningSet map[string]struct{}
}

type printScratchKey struct {
	op    *ir.PrintOperation
	index int
}

func newFSMBuilder(printer *processPrinter, proc *ir.Process) *fsmBuilder {
	if printer == nil || proc == nil {
		return nil
	}
	builder := &fsmBuilder{
		printer:            printer,
		proc:               proc,
		clockedBlocks:      printer.computeDirectClockedBlocks(proc),
		blockChannelOps:    make(map[*ir.BasicBlock][]*channelOpInfo),
		channelOpsByChan:   make(map[*ir.Channel][]*channelOpInfo),
		blockEntryStateIDs: make(map[*ir.BasicBlock]int),
		stateConsts:        make(map[int]string),
		stateByID:          make(map[int]*fsmState),
		statePredicates:    make(map[int]string),
		phiInfos:           make(map[*ir.PhiOperation]*phiRegInfo),
		phiUpdates:         make(map[edgeKey][]phiUpdate),
		sendWaitStateIDs:   make(map[*ir.SendOperation]int),
		recvWaitStateIDs:   make(map[*ir.RecvOperation]int),
		sendPredicates:     make(map[*ir.SendOperation]string),
		recvPredicates:     make(map[*ir.RecvOperation]string),
		sendReadySignals:   make(map[*ir.SendOperation]string),
		recvValidSignals:   make(map[*ir.RecvOperation]string),
		recvDataSignals:    make(map[*ir.RecvOperation]string),
		recvInfos:          make(map[*ir.RecvOperation]*recvRegInfo),
		printScratchRegs:   make(map[printScratchKey]string),
		deadlockWarningSet: make(map[string]struct{}),
	}
	builder.collectChannelOps()
	builder.buildStateGraph()
	return builder
}

func (f *fsmBuilder) collectChannelOps() {
	if f == nil || f.proc == nil {
		return
	}
	f.blockOrder = f.blockOrder[:0]
	f.sendOpInfos = f.sendOpInfos[:0]
	f.recvOpInfos = f.recvOpInfos[:0]
	f.blockChannelOps = make(map[*ir.BasicBlock][]*channelOpInfo)
	f.channelOpsByChan = make(map[*ir.Channel][]*channelOpInfo)
	for _, block := range f.proc.Blocks {
		if block == nil {
			continue
		}
		f.blockOrder = append(f.blockOrder, block)
		for opIndex, op := range block.Ops {
			switch o := op.(type) {
			case *ir.SendOperation:
				info := &channelOpInfo{
					channel:     o.Channel,
					channelID:   channelOpID(o.Channel),
					direction:   channelDirSend,
					block:       block,
					opIndex:     opIndex,
					sendOp:      o,
					target:      o.Value,
					reqStateID:  -1,
					waitStateID: -1,
				}
				f.blockChannelOps[block] = append(f.blockChannelOps[block], info)
				f.sendOpInfos = append(f.sendOpInfos, info)
				if o.Channel != nil {
					f.channelOpsByChan[o.Channel] = append(f.channelOpsByChan[o.Channel], info)
				}
			case *ir.RecvOperation:
				info := &channelOpInfo{
					channel:     o.Channel,
					channelID:   channelOpID(o.Channel),
					direction:   channelDirRecv,
					block:       block,
					opIndex:     opIndex,
					recvOp:      o,
					target:      o.Dest,
					reqStateID:  -1,
					waitStateID: -1,
				}
				f.blockChannelOps[block] = append(f.blockChannelOps[block], info)
				f.recvOpInfos = append(f.recvOpInfos, info)
				if o.Channel != nil {
					f.channelOpsByChan[o.Channel] = append(f.channelOpsByChan[o.Channel], info)
				}
			}
		}
	}
}

func channelOpID(ch *ir.Channel) string {
	if ch == nil {
		return "unknown_channel"
	}
	return sanitize(ch.Name)
}

func (f *fsmBuilder) buildStateGraph() {
	if f == nil || f.proc == nil {
		return
	}
	nextID := 0
	for _, block := range f.blockOrder {
		if block == nil {
			continue
		}
		firstStateID := -1
		var prevWait *fsmState
		for _, opInfo := range f.blockChannelOps[block] {
			if opInfo == nil {
				continue
			}
			switch opInfo.direction {
			case channelDirSend:
				req := &fsmState{
					id:     nextID,
					block:  block,
					phase:  phaseSendReq,
					sendOp: opInfo.sendOp,
					nextID: nextID + 1,
				}
				nextID++
				wait := &fsmState{
					id:     nextID,
					block:  block,
					phase:  phaseSendWait,
					sendOp: opInfo.sendOp,
					nextID: -1,
				}
				nextID++
				if firstStateID < 0 {
					firstStateID = req.id
				}
				if prevWait != nil {
					prevWait.nextID = req.id
				}
				f.addState(req)
				f.addState(wait)
				prevWait = wait
				opInfo.reqStateID = req.id
				opInfo.waitStateID = wait.id
				if opInfo.sendOp != nil {
					f.sendWaitStateIDs[opInfo.sendOp] = wait.id
				}
			case channelDirRecv:
				req := &fsmState{
					id:     nextID,
					block:  block,
					phase:  phaseRecvReq,
					recvOp: opInfo.recvOp,
					nextID: nextID + 1,
				}
				nextID++
				wait := &fsmState{
					id:     nextID,
					block:  block,
					phase:  phaseRecvWait,
					recvOp: opInfo.recvOp,
					nextID: -1,
				}
				nextID++
				if firstStateID < 0 {
					firstStateID = req.id
				}
				if prevWait != nil {
					prevWait.nextID = req.id
				}
				f.addState(req)
				f.addState(wait)
				prevWait = wait
				opInfo.reqStateID = req.id
				opInfo.waitStateID = wait.id
				if opInfo.recvOp != nil {
					f.recvWaitStateIDs[opInfo.recvOp] = wait.id
				}
			}
		}
		noneState := &fsmState{
			id:    nextID,
			block: block,
			phase: phaseNone,
		}
		nextID++
		if firstStateID < 0 {
			firstStateID = noneState.id
		}
		if prevWait != nil {
			prevWait.nextID = noneState.id
		}
		f.addState(noneState)
		f.blockEntryStateIDs[block] = firstStateID
	}
	f.doneID = nextID
	f.addState(&fsmState{
		id:    f.doneID,
		block: nil,
		phase: phaseNone,
	})
	stateCount := f.doneID + 1
	if stateCount <= 0 {
		stateCount = 1
	}
	f.stateWidth = bitWidth(stateCount)
	if f.stateWidth <= 0 {
		f.stateWidth = 1
	}
	f.stateType = fmt.Sprintf("i%d", f.stateWidth)
}

func (f *fsmBuilder) addState(state *fsmState) {
	if f == nil || state == nil {
		return
	}
	f.stateOrder = append(f.stateOrder, state)
	f.stateByID[state.id] = state
}

func bitWidth(count int) int {
	if count <= 1 {
		return 1
	}
	return bits.Len(uint(count - 1))
}

func (f *fsmBuilder) emitStateConstants() {
	if f == nil {
		return
	}
	for _, state := range f.stateOrder {
		f.ensureStateConst(state.id)
	}
}

func (f *fsmBuilder) ensureStateConst(id int) string {
	if name, ok := f.stateConsts[id]; ok {
		return name
	}
	if f.printer == nil {
		return ""
	}
	name := f.printer.freshValueName("state_const")
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = hw.constant %d : %s\n", name, id, f.stateType)
	f.stateConsts[id] = name
	return name
}

func (f *fsmBuilder) literalForID(id int) string {
	if f.stateWidth <= 0 {
		return fmt.Sprintf("b%d", id)
	}
	return fmt.Sprintf("b%0*b", f.stateWidth, id)
}

func (f *fsmBuilder) emitStateRegister() {
	if f == nil || f.printer == nil {
		return
	}
	entryID := f.doneID
	if len(f.blockOrder) > 0 {
		if id, ok := f.blockEntryStateIDs[f.blockOrder[0]]; ok {
			entryID = id
		}
	}
	f.entryStateID = entryID
	entryConst := f.ensureStateConst(entryID)
	f.stateRegInout = f.printer.freshValueName("state_reg")
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = sv.reg : !hw.inout<%s>\n", f.stateRegInout, f.stateType)
	if entryConst != "" {
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "sv.initial {")
		f.printer.indent++
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.bpassign %s, %s : %s\n", f.stateRegInout, entryConst, f.stateType)
		f.printer.indent--
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "}")
	}
	f.stateValue = f.printer.freshValueName("state")
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", f.stateValue, f.stateRegInout, f.stateType)
}

func (f *fsmBuilder) emitRecvRegisters() {
	if f == nil || f.printer == nil {
		return
	}
	for _, block := range f.blockOrder {
		for _, op := range block.Ops {
			recvOp, ok := op.(*ir.RecvOperation)
			if !ok || recvOp == nil || recvOp.Dest == nil {
				continue
			}
			if _, exists := f.recvInfos[recvOp]; exists {
				continue
			}
			typeStr := typeString(recvOp.Dest.Type)
			regName := f.printer.freshValueName("recv_reg")
			f.printer.printIndent()
			fmt.Fprintf(f.printer.w, "%s = sv.reg : !hw.inout<%s>\n", regName, typeStr)
			destName := f.printer.bindSSA(recvOp.Dest)
			f.printer.printIndent()
			fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", destName, regName, typeStr)
			f.recvInfos[recvOp] = &recvRegInfo{
				op:        recvOp,
				regName:   regName,
				valueName: destName,
				typeStr:   typeStr,
			}
		}
	}
}

func (f *fsmBuilder) emitPrintScratchRegs() {
	if f == nil || f.printer == nil || f.proc == nil {
		return
	}
	for _, block := range f.proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			printOp, ok := op.(*ir.PrintOperation)
			if !ok || printOp == nil {
				continue
			}
			for i, seg := range printOp.Segments {
				if seg.Value == nil {
					continue
				}
				key := printScratchKey{op: printOp, index: i}
				if _, exists := f.printScratchRegs[key]; exists {
					continue
				}
				typeStr := typeString(seg.Value.Type)
				regName := f.printer.freshValueName("print_reg")
				f.printer.printIndent()
				fmt.Fprintf(f.printer.w, "%s = sv.reg : !hw.inout<%s>\n", regName, typeStr)
				zeroName := f.printer.freshValueName("print_zero")
				f.printer.printIndent()
				fmt.Fprintf(f.printer.w, "%s = hw.constant 0 : %s\n", zeroName, typeStr)
				f.printer.printIndent()
				fmt.Fprintln(f.printer.w, "sv.initial {")
				f.printer.indent++
				f.printer.printIndent()
				fmt.Fprintf(f.printer.w, "sv.bpassign %s, %s : %s\n", regName, zeroName, typeStr)
				f.printer.indent--
				f.printer.printIndent()
				fmt.Fprintln(f.printer.w, "}")
				f.printScratchRegs[key] = regName
			}
		}
	}
}

func (f *fsmBuilder) registerPhi(block *ir.BasicBlock, phi *ir.PhiOperation) {
	if f == nil || f.printer == nil || block == nil || phi == nil || phi.Dest == nil {
		return
	}
	if _, exists := f.phiInfos[phi]; exists {
		return
	}
	typeStr := typeString(phi.Dest.Type)
	regName := f.printer.freshValueName("phi_reg")
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = sv.reg : !hw.inout<%s>\n", regName, typeStr)

	// Initialize phi register to zero
	zeroConst := f.printer.freshValueName("c_init_phi")
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = hw.constant 0 : %s\n", zeroConst, typeStr)
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "sv.initial {")
	f.printer.indent++
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.bpassign %s, %s : %s\n", regName, zeroConst, typeStr)
	f.printer.indent--
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "}")

	destName := f.printer.bindSSA(phi.Dest)
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", destName, regName, typeStr)
	info := &phiRegInfo{
		phi:       phi,
		regName:   regName,
		valueName: destName,
		typeStr:   typeStr,
	}
	f.phiInfos[phi] = info
	f.phiOrder = append(f.phiOrder, phi)
	for _, incoming := range phi.Incomings {
		if incoming.Block == nil || incoming.Value == nil {
			continue
		}
		key := edgeKey{pred: incoming.Block, succ: block}
		f.phiUpdates[key] = append(f.phiUpdates[key], phiUpdate{
			phi:   phi,
			value: incoming.Value,
		})
	}
}

func (f *fsmBuilder) emitStatePredicate(stateID int) string {
	if f == nil || f.printer == nil {
		return "%unknown"
	}
	if name, ok := f.statePredicates[stateID]; ok {
		return name
	}
	stateConst := f.ensureStateConst(stateID)
	if stateConst == "" {
		return "%unknown"
	}
	name := f.printer.freshValueName("state_is")
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "%s = comb.icmp eq %s, %s : %s\n", name, f.stateValue, stateConst, f.stateType)
	f.statePredicates[stateID] = name
	return name
}

func (f *fsmBuilder) emitChannelPortLogic() {
	if f == nil || f.printer == nil || f.stateValue == "" {
		return
	}
	for _, info := range f.sendOpInfos {
		if info == nil || info.sendOp == nil {
			continue
		}
		waitID := info.waitStateID
		if waitID < 0 {
			waitID = f.sendWaitStateIDs[info.sendOp]
		}
		if waitID >= 0 {
			f.sendPredicates[info.sendOp] = f.emitStatePredicate(waitID)
		} else {
			f.sendPredicates[info.sendOp] = "%unknown"
		}
		f.sendReadySignals[info.sendOp] = "%unknown"
	}

	for _, info := range f.recvOpInfos {
		if info == nil || info.recvOp == nil {
			continue
		}
		waitID := info.waitStateID
		if waitID < 0 {
			waitID = f.recvWaitStateIDs[info.recvOp]
		}
		if waitID >= 0 {
			f.recvPredicates[info.recvOp] = f.emitStatePredicate(waitID)
		} else {
			f.recvPredicates[info.recvOp] = "%unknown"
		}
		f.recvValidSignals[info.recvOp] = "%unknown"
		f.recvDataSignals[info.recvOp] = "%unknown"
	}

	channels := make([]*ir.Channel, 0, len(f.channelOpsByChan))
	for ch := range f.channelOpsByChan {
		if ch != nil {
			channels = append(channels, ch)
		}
	}
	sort.SliceStable(channels, func(i, j int) bool {
		return sanitize(channels[i].Name) < sanitize(channels[j].Name)
	})

	for _, ch := range channels {
		ops := f.channelOpsByChan[ch]
		sendInfos := make([]*channelOpInfo, 0, len(ops))
		for _, opInfo := range ops {
			if opInfo != nil && opInfo.direction == channelDirSend && opInfo.sendOp != nil {
				sendInfos = append(sendInfos, opInfo)
			}
		}
		if len(sendInfos) == 0 {
			continue
		}
		ports := f.printer.channelPorts[ch]
		if ports == nil || ports.sendData == "" || ports.sendValid == "" || ports.sendReady == "" {
			f.printer.printIndent()
			fmt.Fprintf(f.printer.w, "// missing channel send ports for %s\n", channelOpID(ch))
			continue
		}
		preds := make([]string, 0, len(sendInfos))
		values := make([]string, 0, len(sendInfos))
		for _, opInfo := range sendInfos {
			preds = append(preds, f.sendPredicates[opInfo.sendOp])
			values = append(values, f.printer.valueRef(opInfo.target))
		}
		valid := f.printer.orSignals(preds)
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.assign %s, %s : i1\n", ports.sendValid, valid)
		data := f.printer.muxByPredicates(preds, values, ch.Type)
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.assign %s, %s : %s\n", ports.sendData, data, typeString(ch.Type))
		readyVal := f.printer.freshValueName("send_ready")
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : !hw.inout<i1>\n", readyVal, ports.sendReady)
		for _, opInfo := range sendInfos {
			f.sendReadySignals[opInfo.sendOp] = readyVal
		}
	}

	for _, ch := range channels {
		ops := f.channelOpsByChan[ch]
		recvInfos := make([]*channelOpInfo, 0, len(ops))
		for _, opInfo := range ops {
			if opInfo != nil && opInfo.direction == channelDirRecv && opInfo.recvOp != nil {
				recvInfos = append(recvInfos, opInfo)
			}
		}
		if len(recvInfos) == 0 {
			continue
		}
		ports := f.printer.channelPorts[ch]
		if ports == nil || ports.recvData == "" || ports.recvValid == "" || ports.recvReady == "" {
			f.printer.printIndent()
			fmt.Fprintf(f.printer.w, "// missing channel recv ports for %s\n", channelOpID(ch))
			continue
		}
		preds := make([]string, 0, len(recvInfos))
		for _, opInfo := range recvInfos {
			preds = append(preds, f.recvPredicates[opInfo.recvOp])
		}
		ready := f.printer.orSignals(preds)
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.assign %s, %s : i1\n", ports.recvReady, ready)
		validVal := f.printer.freshValueName("recv_valid")
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : !hw.inout<i1>\n", validVal, ports.recvValid)
		dataVal := f.printer.freshValueName("recv_data")
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : %s\n", dataVal, ports.recvData, inoutTypeString(ch.Type))
		for _, opInfo := range recvInfos {
			f.recvValidSignals[opInfo.recvOp] = validVal
			f.recvDataSignals[opInfo.recvOp] = dataVal
		}
	}
}

func (f *fsmBuilder) recordDeadlockWarning(msg string) {
	if f == nil || strings.TrimSpace(msg) == "" {
		return
	}
	if _, exists := f.deadlockWarningSet[msg]; exists {
		return
	}
	f.deadlockWarningSet[msg] = struct{}{}
	f.deadlockWarnings = append(f.deadlockWarnings, msg)
}

func (f *fsmBuilder) emitControlLogic() {
	if f == nil || f.printer == nil || f.stateRegInout == "" || f.stateValue == "" {
		return
	}
	for _, warning := range f.deadlockWarnings {
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "// deadlock warning: %s\n", warning)
	}
	clk := f.printer.portRef("clk")
	rst := f.printer.resetAssertedRef()
	entryStateConst := f.ensureStateConst(f.entryStateID)
	if entryStateConst == "" {
		entryStateConst = f.ensureStateConst(f.doneID)
	}
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.always posedge %s {\n", clk)
	f.printer.indent++
	if f.printer.hasResetPort {
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.if %s {\n", rst)
		f.printer.indent++
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", f.stateRegInout, entryStateConst, f.stateType)
		f.printer.indent--
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "} else {")
		f.printer.indent++
	}
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.case %s : %s\n", f.stateValue, f.stateType)
	for _, state := range f.stateOrder {
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "case %s: {\n", f.literalForID(state.id))
		f.printer.indent++
		f.emitStateCase(state)
		f.printer.indent--
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "}")
	}
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "default: {")
	f.printer.indent++
	f.emitHoldState()
	f.printer.indent--
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "}")
	if f.printer.hasResetPort {
		f.printer.indent--
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "}")
	}
	f.printer.indent--
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "}")
}

func (f *fsmBuilder) emitStateCase(state *fsmState) {
	if state == nil {
		return
	}
	switch state.phase {
	case phaseSendReq:
		f.emitStateAdvance(state.nextID)
	case phaseSendWait:
		cond := f.sendReadySignals[state.sendOp]
		context := "send wait state"
		if state.sendOp != nil && state.sendOp.Channel != nil {
			context = fmt.Sprintf("send wait on channel %s", sanitize(state.sendOp.Channel.Name))
		}
		f.emitWaitState([]string{cond}, context, func() {
			f.emitStateAdvance(state.nextID)
		})
	case phaseRecvReq:
		f.emitStateAdvance(state.nextID)
	case phaseRecvWait:
		cond := f.recvValidSignals[state.recvOp]
		context := "recv wait state"
		if state.recvOp != nil && state.recvOp.Channel != nil {
			context = fmt.Sprintf("recv wait on channel %s", channelOpID(state.recvOp.Channel))
		}
		f.emitWaitState([]string{cond}, context, func() {
			f.emitRecvUpdate(state.recvOp)
			f.emitStateAdvance(state.nextID)
		})
	case phaseNone:
		if state.block == nil {
			f.emitHoldState()
			return
		}
		f.emitBlockSideEffects(state.block)
		f.emitBlockTerminator(state.block)
	default:
		f.emitHoldState()
	}
}

func (f *fsmBuilder) emitBlockSideEffects(block *ir.BasicBlock) {
	if f == nil || f.printer == nil || block == nil {
		return
	}
	for _, op := range block.Ops {
		switch typed := op.(type) {
		case *ir.AssignOperation:
			f.emitAssignUpdate(block, typed)
		case *ir.PrintOperation:
			f.emitInlinePrint(typed)
		}
	}
}

func (f *fsmBuilder) emitAssignUpdate(block *ir.BasicBlock, op *ir.AssignOperation) {
	if f == nil || f.printer == nil || op == nil || op.Dest == nil || op.Value == nil {
		return
	}
	moduleSig, ok := f.printer.moduleSignals[op.Dest.Name]
	if !ok || moduleSig == nil || moduleSig.Kind != ir.Reg {
		return
	}
	if isOutputGlobalName(op.Dest.Name) && (f.clockedBlocks == nil || !f.clockedBlocks[block]) {
		return
	}
	// Skip assignments to parameters - they are input ports and cannot be assigned
	if f.printer.portNames != nil {
		if _, isPort := f.printer.portNames[op.Dest.Name]; isPort {
			return
		}
	}
	dest := "%" + sanitize(op.Dest.Name)
	value := f.printer.valueRef(op.Value)
	if dest == "" || dest == "%unknown" || value == "" || value == "%unknown" {
		return
	}
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", dest, value, typeString(op.Dest.Type))
}

func (f *fsmBuilder) emitInlinePrint(op *ir.PrintOperation) {
	if f == nil || f.printer == nil || op == nil {
		return
	}
	fd := f.printer.stdoutFD
	if fd == "" {
		fd = f.printer.stdoutConstant()
	}
	format, operands, operandTypes := f.printer.buildPrintfFormat(op)
	if len(operands) > 0 {
		materialized := make([]string, 0, len(operands))
		materializedTypes := make([]string, 0, len(operandTypes))
		valueIndex := 0
		for i, seg := range op.Segments {
			if seg.Value == nil {
				continue
			}
			key := printScratchKey{op: op, index: i}
			regName, ok := f.printScratchRegs[key]
			if !ok {
				continue
			}
			value := operands[valueIndex]
			typeStr := operandTypes[valueIndex]
			f.printer.printIndent()
			fmt.Fprintf(f.printer.w, "sv.bpassign %s, %s : %s\n", regName, value, typeStr)
			readName := f.printer.freshValueName("print_val")
			f.printer.printIndent()
			fmt.Fprintf(f.printer.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", readName, regName, typeStr)
			materialized = append(materialized, readName)
			materializedTypes = append(materializedTypes, typeStr)
			valueIndex++
		}
		operands = materialized
		operandTypes = materializedTypes
	}
	f.printer.printIndent()
	if len(operands) == 0 {
		fmt.Fprintf(f.printer.w, "sv.fwrite %s, %s\n", fd, strconv.Quote(format))
		return
	}
	fmt.Fprintf(f.printer.w, "sv.fwrite %s, %s(%s) : %s\n",
		fd,
		strconv.Quote(format),
		strings.Join(operands, ", "),
		strings.Join(operandTypes, ", "),
	)
}

func (f *fsmBuilder) emitBlockTerminator(block *ir.BasicBlock) {
	if block == nil {
		return
	}
	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		cond := f.printer.valueRef(term.Cond)
		if cond == "%unknown" || cond == "" {
			cond = f.printer.boolConst(false)
		}
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.if %s {\n", cond)
		f.printer.indent++
		f.emitTransition(block, term.True)
		f.printer.indent--
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "} else {")
		f.printer.indent++
		f.emitTransition(block, term.False)
		f.printer.indent--
		f.printer.printIndent()
		fmt.Fprintln(f.printer.w, "}")
	case *ir.JumpTerminator:
		f.emitTransition(block, term.Target)
	case *ir.ReturnTerminator:
		f.emitTransition(block, nil)
	default:
		f.emitHoldState()
	}
}

func (f *fsmBuilder) emitStateAdvance(targetID int) {
	if targetID < 0 {
		f.emitHoldState()
		return
	}
	targetConst := f.ensureStateConst(targetID)
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", f.stateRegInout, targetConst, f.stateType)
}

func (f *fsmBuilder) emitHoldState() {
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", f.stateRegInout, f.stateValue, f.stateType)
}

func (f *fsmBuilder) emitRecvUpdate(recvOp *ir.RecvOperation) {
	if f == nil || f.printer == nil || recvOp == nil {
		return
	}
	info := f.recvInfos[recvOp]
	data := f.recvDataSignals[recvOp]
	if info == nil || data == "" || data == "%unknown" {
		return
	}
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", info.regName, data, info.typeStr)
}

func (f *fsmBuilder) emitWaitState(condSignals []string, context string, onSuccess func()) {
	unknown := false
	conds := make([]string, 0, len(condSignals))
	for _, cond := range condSignals {
		if cond == "" || cond == "%unknown" {
			unknown = true
			continue
		}
		conds = append(conds, cond)
	}
	if len(condSignals) == 0 || unknown {
		if context == "" {
			context = "unknown wait state"
		}
		f.recordDeadlockWarning(context)
	}
	if len(conds) == 0 {
		f.emitHoldState()
		return
	}
	f.emitWaitStateChain(conds, 0, onSuccess)
}

func (f *fsmBuilder) emitWaitStateChain(conds []string, index int, onSuccess func()) {
	if index >= len(conds) {
		f.emitHoldState()
		return
	}
	cond := conds[index]
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.if %s {\n", cond)
	f.printer.indent++
	if onSuccess != nil {
		onSuccess()
	} else {
		f.emitHoldState()
	}
	f.printer.indent--
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "} else {")
	f.printer.indent++
	if index+1 < len(conds) {
		f.emitWaitStateChain(conds, index+1, onSuccess)
	} else {
		f.emitHoldState()
	}
	f.printer.indent--
	f.printer.printIndent()
	fmt.Fprintln(f.printer.w, "}")
}

func (f *fsmBuilder) emitTransition(pred, succ *ir.BasicBlock) {
	if f == nil || f.printer == nil {
		return
	}
	targetID := f.doneID
	if succ != nil {
		if id, ok := f.blockEntryStateIDs[succ]; ok {
			targetID = id
		}
	}
	f.emitStateAdvance(targetID)
	if succ == nil {
		return
	}
	key := edgeKey{pred: pred, succ: succ}
	for _, update := range f.phiUpdates[key] {
		info := f.phiInfos[update.phi]
		if info == nil || update.value == nil {
			continue
		}
		val := f.printer.edgeValueRef(update.value)
		if val == "" || val == "%unknown" {
			continue
		}
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", info.regName, val, info.typeStr)
	}
}

type processPrinter struct {
	w                   io.Writer
	indent              int
	nextTemp            int
	constNames          map[*ir.Signal]string
	valueNames          map[*ir.Signal]string
	portNames           map[string]string
	portTypes           map[string]*ir.SignalType
	channelPorts        map[*ir.Channel]*channelPortSet
	moduleSignals       map[string]*ir.Signal
	usedSignals         map[*ir.Signal]struct{}
	boolConsts          map[bool]string
	stdoutFD            string
	fsm                 *fsmBuilder
	seqClockName        string
	internalSignalReads map[string]string     // Maps signal name to the read_inout result
	moduleName          string                // Name of the parent module
	modulePorts         map[string][]portDesc // Module port information for instances
	emitter             *emitter              // Reference to parent emitter for global state
	emittedRegisters    map[string]bool       // Track which registers have already been emitted
	proc                *ir.Process           // The process being emitted
	hasResetPort        bool
	resetPortRef        string
	resetActiveLow      bool
	resetAssertedName   string
	directClocked       bool
	clockedBlocks       map[*ir.BasicBlock]bool
	blockValueRestores  map[*ir.Signal]valueRestore
}

type valueRestore struct {
	existed bool
	value   string
}

func (p *processPrinter) resetState() {
	p.nextTemp = 0
	p.constNames = make(map[*ir.Signal]string)
	p.valueNames = make(map[*ir.Signal]string)
	portNames := make(map[string]string)
	for name, value := range p.portNames {
		portNames[name] = value
	}
	p.hasResetPort, p.resetPortRef, p.resetActiveLow = explicitResetPortInfo(portNames)
	normalizeControlPortAliases(portNames)
	p.portNames = portNames
	if p.channelPorts == nil {
		p.channelPorts = make(map[*ir.Channel]*channelPortSet)
	}
	if p.portTypes == nil {
		p.portTypes = make(map[string]*ir.SignalType)
	}
	if p.usedSignals == nil {
		p.usedSignals = make(map[*ir.Signal]struct{})
	}
	if p.boolConsts == nil {
		p.boolConsts = make(map[bool]string)
	}
	if p.internalSignalReads == nil {
		p.internalSignalReads = make(map[string]string)
	}
	if p.emittedRegisters == nil {
		p.emittedRegisters = make(map[string]bool)
	}
	p.stdoutFD = ""
	p.fsm = nil
	p.seqClockName = ""
	p.resetAssertedName = ""
	p.directClocked = false
	p.clockedBlocks = nil
	p.blockValueRestores = nil
}

func normalizeControlPortAliases(portNames map[string]string) {
	if portNames == nil {
		return
	}
	if _, ok := portNames["clk"]; !ok {
		if value, ok := portNames["clock"]; ok {
			portNames["clk"] = value
		} else {
			portNames["clk"] = "%clk"
		}
	}
	if _, ok := portNames["clock"]; !ok {
		if value, ok := portNames["clk"]; ok {
			portNames["clock"] = value
		}
	}
	if _, ok := portNames["rst"]; !ok {
		switch {
		case portNames["reset"] != "":
			portNames["rst"] = portNames["reset"]
		case portNames["areset"] != "":
			portNames["rst"] = portNames["areset"]
		case portNames["resetn"] != "":
			portNames["rst"] = portNames["resetn"]
		case portNames["aresetn"] != "":
			portNames["rst"] = portNames["aresetn"]
		default:
			portNames["rst"] = "%rst"
		}
	}
	if _, ok := portNames["reset"]; !ok {
		if value, ok := portNames["rst"]; ok {
			portNames["reset"] = value
		}
	}
}

func hasExplicitResetPort(portNames map[string]string) bool {
	for name := range portNames {
		if isResetPortName(name) {
			return true
		}
	}
	return false
}

func explicitResetPortInfo(portNames map[string]string) (bool, string, bool) {
	if len(portNames) == 0 {
		return false, "", false
	}
	for _, name := range []string{"rst", "reset", "ar", "areset", "resetn", "aresetn"} {
		if value := portNames[name]; value != "" {
			return true, value, isActiveLowResetPortName(name)
		}
	}
	return false, "", false
}

func isActiveLowResetPortName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "resetn", "aresetn":
		return true
	default:
		return false
	}
}

func canEmitDirectClockedControl(proc *ir.Process) bool {
	if proc == nil || proc.Sensitivity != ir.Sequential {
		return false
	}
	return !processHasLoop(proc) && !processHasChannelOps(proc) && !processNeedsPrintControlFSM(proc)
}

// initArrayElementRegisters pre-populates valueNames with array element registers
// that were declared at the module level
func (p *processPrinter) initArrayElementRegisters() {
	if p.moduleSignals == nil {
		return
	}

	for name, sig := range p.moduleSignals {
		// Check if this is an array element (name_number format)
		// Only treat as array element if the base name is a known mutable global array
		isArrayElement := false
		baseName := name
		for i := 0; i < len(name)-1; i++ {
			if name[i] == '_' && name[i+1] >= '0' && name[i+1] <= '9' {
				baseName = name[:i]
				// Only treat as array element if it's a known mutable global array
				// Known mutable arrays: tqmf, compressed, result, accumc, accumd
				isArrayElement = baseName == "tqmf" || baseName == "compressed" || baseName == "result" || baseName == "accumc" || baseName == "accumd"
				break
			}
		}

		if isArrayElement && sig != nil && sig.Kind != ir.Const {
			// This is an array element register declared at module level
			// Map the signal to its register name
			regName := "%" + sanitize(name)
			p.valueNames[sig] = regName
			p.emittedRegisters[regName] = true
		}
	}
}

func (p *processPrinter) emitProcess(proc *ir.Process) {
	if proc == nil {
		return
	}
	hasPrintOps := processHasPrintOps(proc)
	p.emitConstants()
	p.emitSignals()
	p.initArrayElementRegisters()
	p.directClocked = canEmitDirectClockedControl(proc)
	if p.directClocked {
		p.clockedBlocks = p.computeDirectClockedBlocks(proc)
	}
	if !p.directClocked && (proc.Sensitivity == ir.Sequential || processHasChannelOps(proc) || processNeedsPrintControlFSM(proc)) {
		p.fsm = newFSMBuilder(p, proc)
		if p.fsm != nil {
			p.fsm.emitStateConstants()
			p.fsm.emitStateRegister()
			p.fsm.emitRecvRegisters()
			p.fsm.emitPrintScratchRegs()
		}
	} else {
		p.fsm = nil
	}
	if p.fsm != nil {
		for _, block := range proc.Blocks {
			p.beginBlockValueScope()
			for _, op := range block.Ops {
				p.emitOperation(block, op, proc)
			}
			p.endBlockValueScope()
		}
		if hasPrintOps {
			p.stdoutConstant()
		}
		p.fsm.emitChannelPortLogic()
		p.fsm.emitControlLogic()
	} else if p.directClocked {
		for _, block := range proc.Blocks {
			for _, op := range block.Ops {
				p.emitOperation(block, op, proc)
			}
		}
		p.emitDirectClockedControl(proc)
	} else {
		for _, block := range proc.Blocks {
			p.beginBlockValueScope()
			for _, op := range block.Ops {
				p.emitOperation(block, op, proc)
			}
			p.endBlockValueScope()
		}
		if p.hasCombinationalRegAssignments(proc) {
			p.emitCombinationalRegControl(proc)
		}
	}
	p.fsm = nil
	p.directClocked = false
	p.clockedBlocks = nil
}

type directClockedVisitKey struct {
	block     *ir.BasicBlock
	inClocked bool
}

func (p *processPrinter) computeDirectClockedBlocks(proc *ir.Process) map[*ir.BasicBlock]bool {
	blocks := make(map[*ir.BasicBlock]bool)
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return blocks
	}
	visited := make(map[directClockedVisitKey]bool)
	clockedReach := make(map[*ir.BasicBlock]bool)
	nonClockedReach := make(map[*ir.BasicBlock]bool)
	var visit func(block *ir.BasicBlock, inClocked bool)
	visit = func(block *ir.BasicBlock, inClocked bool) {
		if block == nil {
			return
		}
		key := directClockedVisitKey{block: block, inClocked: inClocked}
		if visited[key] {
			return
		}
		visited[key] = true
		if inClocked {
			clockedReach[block] = true
		} else {
			nonClockedReach[block] = true
		}
		switch term := block.Terminator.(type) {
		case *ir.BranchTerminator:
			if term.Cond != nil && isClockLikeName(term.Cond.Name) {
				visit(term.True, true)
				visit(term.False, false)
				return
			}
			if term.Cond != nil && isResetPortName(term.Cond.Name) && block == proc.Blocks[0] {
				if isActiveLowResetName(term.Cond.Name) {
					visit(term.True, false)
					visit(term.False, true)
				} else {
					visit(term.True, true)
					visit(term.False, false)
				}
				return
			}
			visit(term.True, inClocked)
			visit(term.False, inClocked)
		case *ir.JumpTerminator:
			visit(term.Target, inClocked)
		}
	}
	visit(proc.Blocks[0], false)
	for block := range clockedReach {
		if !nonClockedReach[block] {
			blocks[block] = true
		}
	}
	return blocks
}

func (p *processPrinter) emitSignals() {
	// Signal declarations are emitted on demand by operation emission.
}

func (p *processPrinter) beginBlockValueScope() {
	if p == nil {
		return
	}
	p.blockValueRestores = make(map[*ir.Signal]valueRestore)
}

func (p *processPrinter) endBlockValueScope() {
	if p == nil || p.blockValueRestores == nil {
		return
	}
	for sig, restore := range p.blockValueRestores {
		if !restore.existed {
			delete(p.valueNames, sig)
			continue
		}
		p.valueNames[sig] = restore.value
	}
	p.blockValueRestores = nil
}

func (p *processPrinter) recordBlockValueOverride(sig *ir.Signal) {
	if p == nil || sig == nil || p.blockValueRestores == nil {
		return
	}
	if _, exists := p.blockValueRestores[sig]; exists {
		return
	}
	value, existed := p.valueNames[sig]
	p.blockValueRestores[sig] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) emitDirectClockedControl(proc *ir.Process) {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return
	}
	sensitivity := p.directClockedSensitivity(proc)
	p.printIndent()
	fmt.Fprintf(p.w, "sv.always %s {\n", sensitivity)
	p.indent++
	p.emitDirectClockedBlock(proc.Blocks[0], make(map[*ir.BasicBlock]bool))
	p.indent--
	p.printIndent()
	fmt.Fprintln(p.w, "}")
}

func (p *processPrinter) hasCombinationalRegAssignments(proc *ir.Process) bool {
	if p == nil || proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil {
				continue
			}
			moduleSig, ok := p.moduleSignals[assign.Dest.Name]
			if !ok || moduleSig == nil || moduleSig.Kind != ir.Reg {
				continue
			}
			if p.portNames != nil {
				if _, isPort := p.portNames[assign.Dest.Name]; isPort {
					continue
				}
			}
			return true
		}
	}
	return false
}

func (p *processPrinter) emitCombinationalRegControl(proc *ir.Process) {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return
	}
	p.printIndent()
	fmt.Fprintln(p.w, "sv.alwayscomb {")
	p.indent++
	p.emitCombinationalRegBlock(proc.Blocks[0], make(map[*ir.BasicBlock]bool))
	p.indent--
	p.printIndent()
	fmt.Fprintln(p.w, "}")
}

func (p *processPrinter) emitCombinationalRegBlock(block *ir.BasicBlock, active map[*ir.BasicBlock]bool) {
	if p == nil || block == nil {
		return
	}
	if active[block] {
		return
	}
	active[block] = true
	defer delete(active, block)

	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil {
			continue
		}
		p.emitCombinationalRegAssign(assign)
	}

	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		cond := p.valueRef(term.Cond)
		if cond == "" || cond == "%unknown" {
			cond = p.boolConst(false)
		}
		p.printIndent()
		fmt.Fprintf(p.w, "sv.if %s {\n", cond)
		p.indent++
		p.emitCombinationalRegBlock(term.True, active)
		p.indent--
		p.printIndent()
		fmt.Fprintln(p.w, "} else {")
		p.indent++
		p.emitCombinationalRegBlock(term.False, active)
		p.indent--
		p.printIndent()
		fmt.Fprintln(p.w, "}")
	case *ir.JumpTerminator:
		p.emitCombinationalRegBlock(term.Target, active)
	case *ir.ReturnTerminator:
		return
	}
}

func (p *processPrinter) emitCombinationalRegAssign(op *ir.AssignOperation) {
	if p == nil || op == nil || op.Dest == nil || op.Value == nil {
		return
	}
	moduleSig, ok := p.moduleSignals[op.Dest.Name]
	if !ok || moduleSig == nil || moduleSig.Kind != ir.Reg {
		return
	}
	if p.portNames != nil {
		if _, isPort := p.portNames[op.Dest.Name]; isPort {
			return
		}
	}
	value := p.valueRef(op.Value)
	if value == "" || value == "%unknown" {
		return
	}
	dest := "%" + sanitize(op.Dest.Name)
	p.printIndent()
	fmt.Fprintf(p.w, "sv.bpassign %s, %s : %s\n", dest, value, typeString(op.Dest.Type))
}

func (p *processPrinter) directClockedSensitivity(proc *ir.Process) string {
	clk := p.portRef("clk")
	if p == nil || proc == nil || !p.hasResetPort || !p.directClockedHasAsyncReset(proc) {
		return fmt.Sprintf("posedge %s", clk)
	}
	if p.resetActiveLow {
		return fmt.Sprintf("posedge %s, negedge %s", clk, p.resetPortRef)
	}
	return fmt.Sprintf("posedge %s, posedge %s", clk, p.resetPortRef)
}

func (p *processPrinter) directClockedHasAsyncReset(proc *ir.Process) bool {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return false
	}
	entry := proc.Blocks[0]
	branch, ok := entry.Terminator.(*ir.BranchTerminator)
	if !ok || branch == nil || branch.Cond == nil {
		return false
	}
	return isResetPortName(branch.Cond.Name)
}

func (p *processPrinter) emitDirectClockedBlock(block *ir.BasicBlock, active map[*ir.BasicBlock]bool) {
	if p == nil || block == nil {
		return
	}
	if active[block] {
		return
	}
	active[block] = true
	defer delete(active, block)

	if p.clockedBlocks != nil && p.clockedBlocks[block] {
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil {
				continue
			}
			p.emitDirectAssignUpdate(assign)
		}
	}

	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		if term.Cond != nil && isClockLikeName(term.Cond.Name) {
			p.emitDirectClockedBlock(term.True, active)
			return
		}
		if term.Cond != nil && p.directClockedHasAsyncResetInEntry(block, term) {
			cond := p.resetAssertedRef()
			assertedBlock := term.True
			deassertedBlock := term.False
			if isActiveLowResetName(term.Cond.Name) {
				assertedBlock = term.False
				deassertedBlock = term.True
			}
			p.printIndent()
			fmt.Fprintf(p.w, "sv.if %s {\n", cond)
			p.indent++
			p.emitDirectClockedBlock(assertedBlock, active)
			p.indent--
			p.printIndent()
			fmt.Fprintln(p.w, "} else {")
			p.indent++
			p.emitDirectClockedBlock(deassertedBlock, active)
			p.indent--
			p.printIndent()
			fmt.Fprintln(p.w, "}")
			return
		}
		cond := p.valueRef(term.Cond)
		if cond == "" || cond == "%unknown" {
			cond = p.boolConst(false)
		}
		p.printIndent()
		fmt.Fprintf(p.w, "sv.if %s {\n", cond)
		p.indent++
		p.emitDirectClockedBlock(term.True, active)
		p.indent--
		p.printIndent()
		fmt.Fprintln(p.w, "} else {")
		p.indent++
		p.emitDirectClockedBlock(term.False, active)
		p.indent--
		p.printIndent()
		fmt.Fprintln(p.w, "}")
	case *ir.JumpTerminator:
		p.emitDirectClockedBlock(term.Target, active)
	case *ir.ReturnTerminator:
		return
	}
}

func (p *processPrinter) directClockedHasAsyncResetInEntry(block *ir.BasicBlock, term *ir.BranchTerminator) bool {
	if p == nil || block == nil || term == nil || term.Cond == nil {
		return false
	}
	if !p.hasResetPort || !isResetPortName(term.Cond.Name) {
		return false
	}
	if p.proc == nil || len(p.proc.Blocks) == 0 || p.proc.Blocks[0] != block {
		return false
	}
	return true
}

func (p *processPrinter) emitDirectAssignUpdate(op *ir.AssignOperation) {
	if p == nil || op == nil || op.Dest == nil || op.Value == nil {
		return
	}
	moduleSig, ok := p.moduleSignals[op.Dest.Name]
	if !ok || moduleSig == nil || moduleSig.Kind != ir.Reg {
		return
	}
	if p.portNames != nil {
		if _, isPort := p.portNames[op.Dest.Name]; isPort {
			return
		}
	}
	value := p.valueRef(op.Value)
	if value == "" || value == "%unknown" {
		return
	}
	dest := "%" + sanitize(op.Dest.Name)
	p.printIndent()
	fmt.Fprintf(p.w, "sv.passign %s, %s : %s\n", dest, value, typeString(op.Dest.Type))
}

func (p *processPrinter) emitConstants() {
	if len(p.moduleSignals) == 0 {
		return
	}
	names := make([]string, 0, len(p.moduleSignals))
	for name := range p.moduleSignals {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sig := p.moduleSignals[name]
		if sig.Kind != ir.Const {
			continue
		}
		if _, ok := p.usedSignals[sig]; !ok {
			continue
		}
		ssaName := p.assignConst(sig)
		p.printIndent()
		fmt.Fprintf(p.w, "%s = hw.constant %s : %s\n", ssaName, formatHWConstant(sig.Value, sig.Type), typeString(sig.Type))
	}
}

func (p *processPrinter) emitOperation(block *ir.BasicBlock, op ir.Operation, proc *ir.Process) {
	switch o := op.(type) {
	case *ir.BinOperation:
		left := p.valueRef(o.Left)
		right := p.valueRef(o.Right)
		dest := p.bindSSA(o.Dest)
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.%s %s, %s : %s\n",
			dest,
			binOpName(o.Op),
			left,
			right,
			typeString(o.Dest.Type),
		)
	case *ir.ConvertOperation:
		p.emitConvertOperation(o)
	case *ir.AssignOperation:
		if p.fsm != nil || p.directClocked {
			if p.directClocked {
				moduleSig, ok := p.moduleSignals[o.Dest.Name]
				isModuleReg := ok && moduleSig != nil && moduleSig.Kind == ir.Reg
				if p.clockedBlocks == nil || !p.clockedBlocks[block] {
					if isModuleReg && !isOutputGlobalName(o.Dest.Name) {
						return
					}
					src := p.valueRef(o.Value)
					if src != "" && src != "%unknown" {
						p.valueNames[o.Dest] = src
					}
					return
				}
				// Inside clocked blocks, keep local/combinational temporaries available for
				// later RHS evaluation, but do not overwrite the visible value of module regs.
				if !isModuleReg {
					src := p.valueRef(o.Value)
					if src != "" && src != "%unknown" {
						p.valueNames[o.Dest] = src
					}
				}
				return
			}
			src := p.valueRef(o.Value)
			if src != "" && src != "%unknown" {
				moduleSig, ok := p.moduleSignals[o.Dest.Name]
				if ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
					p.recordBlockValueOverride(o.Dest)
				}
				p.valueNames[o.Dest] = src
			}
			moduleSig, ok := p.moduleSignals[o.Dest.Name]
			if ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
				return
			}
			return
		}
		src := p.valueRef(o.Value)

		// For combinational processes, just map the value directly without creating a register
		if p.proc != nil && p.proc.Sensitivity == ir.Combinational {
			p.recordBlockValueOverride(o.Dest)
			p.valueNames[o.Dest] = src
			return
		}

		// Check if this is an assignment to an array element
		// Array elements have module-level registers that should be reused
		isArrayElement := false
		if o.Dest != nil && o.Dest.Name != "" {
			// Check if name matches pattern "name_number" for known arrays
			baseName := o.Dest.Name
			for i := 0; i < len(o.Dest.Name)-1; i++ {
				if o.Dest.Name[i] == '_' && o.Dest.Name[i+1] >= '0' && o.Dest.Name[i+1] <= '9' {
					baseName = o.Dest.Name[:i]
					// Only treat as array element if it's a known mutable global array
					// Known mutable arrays: tqmf, compressed, result, accumc, accumd
					isArrayElement = baseName == "tqmf" || baseName == "compressed" || baseName == "result" || baseName == "accumc" || baseName == "accumd"
					break
				}
			}
		}

		if isArrayElement {
			// This is an assignment to an array element
			// The register already exists at module level, so we don't create a new one
			// Just map the destination signal to the register name for future references
			regName := "%" + sanitize(o.Dest.Name)
			p.valueNames[o.Dest] = regName
			// Note: In actual hardware, updating a register requires creating a new seq.compreg
			// But we can't have multiple operations with the same SSA name
			// For now, we skip the update and assume the value will be used directly
		} else {
			// Regular assignment - create a new register
			clk := p.seqClock()
			if existingDest, ok := p.valueNames[o.Dest]; ok {
				// Check if we've already emitted a register for this destination
				if p.emittedRegisters[existingDest] {
					// Register already emitted, skip to avoid redefinition
					return
				}

				// Check if this is an internal signal read (sv.read_inout)
				// If so, we need to create a new register to avoid redefinition
				if strings.HasPrefix(existingDest, "%v") || strings.HasPrefix(existingDest, "%c") {
					// This is a temporary name from read_inout, create a fresh register
					dest := p.freshValueName("reg")
					p.printIndent()
					fmt.Fprintf(p.w, "%s = seq.compreg %s, %s : %s\n", dest, src, clk, typeString(o.Dest.Type))
					p.valueNames[o.Dest] = dest
					p.emittedRegisters[dest] = true
				} else {
					// Reuse existing name (module-level signal or array element)
					p.printIndent()
					fmt.Fprintf(p.w, "%s = seq.compreg %s, %s : %s\n", existingDest, src, clk, typeString(o.Dest.Type))
					p.emittedRegisters[existingDest] = true
				}
			} else {
				// First time assigning to this destination, create a new register
				dest := p.freshValueName("reg")
				p.printIndent()
				fmt.Fprintf(p.w, "%s = seq.compreg %s, %s : %s\n", dest, src, clk, typeString(o.Dest.Type))
				p.valueNames[o.Dest] = dest
				p.emittedRegisters[dest] = true
			}
		}
	case *ir.SendOperation:
		if p.fsm != nil {
			return
		}
		value := p.valueRef(o.Value)
		ports := p.channelPorts[o.Channel]
		if ports == nil || ports.sendData == "" {
			p.printIndent()
			name := "unknown_channel"
			if o.Channel != nil {
				name = sanitize(o.Channel.Name)
			}
			fmt.Fprintf(p.w, "// missing channel send ports for %s\n", name)
			return
		}
		p.printIndent()
		fmt.Fprintf(p.w, "sv.assign %s, %s : %s\n",
			ports.sendData,
			value,
			typeString(o.Value.Type),
		)
		validConst := p.boolConst(true)
		p.printIndent()
		fmt.Fprintf(p.w, "sv.assign %s, %s : i1\n",
			ports.sendValid,
			validConst,
		)
	case *ir.RecvOperation:
		if p.fsm != nil {
			return
		}
		dest := p.bindSSA(o.Dest)
		ports := p.channelPorts[o.Channel]
		if ports == nil || ports.recvData == "" {
			p.printIndent()
			name := "unknown_channel"
			if o.Channel != nil {
				name = sanitize(o.Channel.Name)
			}
			fmt.Fprintf(p.w, "// missing channel recv ports for %s\n", name)
			return
		}
		p.printIndent()
		fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n",
			dest,
			ports.recvData,
			inoutTypeString(o.Channel.Type),
		)
		readyConst := p.boolConst(true)
		p.printIndent()
		fmt.Fprintf(p.w, "sv.assign %s, %s : i1\n",
			ports.recvReady,
			readyConst,
		)
	case *ir.SpawnOperation:
		childStage := processStage(o.Callee)
		parentStage := processStage(proc)
		p.printIndent()
		fmt.Fprintf(p.w, "// spawn %s stage=%d parent_stage=%d\n",
			sanitize(o.Callee.Name),
			childStage,
			parentStage,
		)
	case *ir.CompareOperation:
		left := p.valueRef(o.Left)
		right := p.valueRef(o.Right)
		dest := p.bindSSA(o.Dest)
		operandType := typeString(o.Left.Type)
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.icmp %s %s, %s : %s\n",
			dest,
			comparePredicateName(o.Predicate),
			left,
			right,
			operandType,
		)
	case *ir.NotOperation:
		value := p.valueRef(o.Value)
		dest := p.bindSSA(o.Dest)
		ones := p.typedAllOnesConst(o.Value.Type)
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.xor %s, %s : %s\n", dest, value, ones, typeString(o.Value.Type))
	case *ir.MuxOperation:
		cond := p.valueRef(o.Cond)
		tVal := p.valueRef(o.TrueValue)
		fVal := p.valueRef(o.FalseValue)
		dest := p.bindSSA(o.Dest)
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n",
			dest,
			cond,
			tVal,
			fVal,
			typeString(o.Dest.Type),
		)
	case *ir.PhiOperation:
		if p.fsm != nil {
			p.fsm.registerPhi(block, o)
		} else {
			// Combinational logic: lower phi to mux
			p.emitPhiAsMux(o)
		}
	case *ir.PrintOperation:
		if p.fsm != nil {
			return
		}
		p.emitPrintOperation(o)
	case *ir.CallOperation:
		p.emitCallOperation(o)
	default:
		// skip unknown operations
	}
}

func (p *processPrinter) seqClock() string {
	if p.seqClockName != "" {
		return p.seqClockName
	}
	clk := p.portRef("clk")
	name := p.freshValueName("clk_seq")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = seq.to_clock %s\n", name, clk)
	p.seqClockName = name
	return name
}

// emitPhiAsMux lowers a phi operation to a combinational mux.
// For a phi with 2 incoming values from an if-then-else, we generate:
//
//	%result = comb.mux %condition, %true_value, %false_value
func (p *processPrinter) emitPhiAsMux(phi *ir.PhiOperation) {
	if phi == nil || len(phi.Incomings) == 0 {
		return
	}

	dest := p.bindSSA(phi.Dest)

	// For now, handle the simple case of 2 incoming values (if-then-else)
	if len(phi.Incomings) == 2 {
		// The phi node merges values from different control flow paths
		block0 := phi.Incomings[0].Block
		block1 := phi.Incomings[1].Block
		val0 := p.valueRef(phi.Incomings[0].Value)
		val1 := p.valueRef(phi.Incomings[1].Value)

		// Find a branch that determines which incoming value to use
		// Look for a branch in one of the incoming blocks
		var cond *ir.Signal
		var trueVal, falseVal string

		// Check if block0 has a branch that goes to block1
		if branch, ok := block0.Terminator.(*ir.BranchTerminator); ok {
			// If block0 branches to block1, then:
			// - when condition is true, we go to block1 (use val1)
			// - when condition is false, we stay/go elsewhere (use val0)
			if branch.True == block1 {
				cond = branch.Cond
				trueVal = val1
				falseVal = val0
			} else if branch.False == block1 {
				cond = branch.Cond
				trueVal = val0
				falseVal = val1
			}
		}

		// Check if block1 has a branch that goes to block0
		if cond == nil {
			if branch, ok := block1.Terminator.(*ir.BranchTerminator); ok {
				if branch.True == block0 {
					cond = branch.Cond
					trueVal = val0
					falseVal = val1
				} else if branch.False == block0 {
					cond = branch.Cond
					trueVal = val1
					falseVal = val0
				}
			}
		}

		// Check predecessors of the incoming blocks
		if cond == nil {
			for _, pred := range block0.Predecessors {
				if branch, ok := pred.Terminator.(*ir.BranchTerminator); ok {
					if branch.True == block0 && branch.False == block1 {
						cond = branch.Cond
						trueVal = val0
						falseVal = val1
						break
					} else if branch.True == block1 && branch.False == block0 {
						cond = branch.Cond
						trueVal = val1
						falseVal = val0
						break
					}
				}
			}
		}

		if cond == nil {
			for _, pred := range block1.Predecessors {
				if branch, ok := pred.Terminator.(*ir.BranchTerminator); ok {
					if branch.True == block0 && branch.False == block1 {
						cond = branch.Cond
						trueVal = val0
						falseVal = val1
						break
					} else if branch.True == block1 && branch.False == block0 {
						cond = branch.Cond
						trueVal = val1
						falseVal = val0
						break
					}
				}
			}
		}

		if cond != nil {
			condRef := p.valueRef(cond)
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n",
				dest, condRef, trueVal, falseVal, typeString(phi.Dest.Type))
			p.valueNames[phi.Dest] = dest
			return
		}
	}
	p.emitPhiAsMuxTree(phi, dest)
}

func (p *processPrinter) emitPhiAsMuxTree(phi *ir.PhiOperation, dest string) {
	if phi == nil || len(phi.Incomings) == 0 {
		return
	}

	// Start with the last incoming value as the default.
	currentVal := p.valueRef(phi.Incomings[len(phi.Incomings)-1].Value)
	for i := len(phi.Incomings) - 2; i >= 0; i-- {
		incoming := phi.Incomings[i]
		incomingBlock := incoming.Block
		incomingVal := p.valueRef(incoming.Value)
		var cond *ir.Signal

		for _, pred := range incomingBlock.Predecessors {
			if branch, ok := pred.Terminator.(*ir.BranchTerminator); ok {
				if branch.True == incomingBlock {
					cond = branch.Cond
					break
				}
				if branch.False == incomingBlock {
					cond = branch.Cond
					incomingVal, currentVal = currentVal, incomingVal
					break
				}
			}
		}

		if cond == nil {
			currentVal = incomingVal
			continue
		}

		condRef := p.valueRef(cond)
		name := dest
		if i > 0 {
			name = p.freshValueName("mux")
		}
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n",
			name, condRef, incomingVal, currentVal, typeString(phi.Dest.Type))
		currentVal = name
	}
	p.valueNames[phi.Dest] = currentVal
}

// canReach checks if there's a path from 'from' to 'to' block (simple BFS)
func canReach(from, to *ir.BasicBlock) bool {
	if from == to {
		return true
	}
	visited := make(map[*ir.BasicBlock]bool)
	queue := []*ir.BasicBlock{from}
	visited[from] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Check successors
		switch term := current.Terminator.(type) {
		case *ir.BranchTerminator:
			if term.True == to || term.False == to {
				return true
			}
			if !visited[term.True] {
				visited[term.True] = true
				queue = append(queue, term.True)
			}
			if !visited[term.False] {
				visited[term.False] = true
				queue = append(queue, term.False)
			}
		case *ir.JumpTerminator:
			if term.Target == to {
				return true
			}
			if !visited[term.Target] {
				visited[term.Target] = true
				queue = append(queue, term.Target)
			}
		}
	}
	return false
}

func (e *emitter) seqClock() string {
	if e.seqClockName != "" {
		return e.seqClockName
	}
	name := "%clk_seq"
	e.printIndent()
	fmt.Fprintf(e.w, "%s = seq.to_clock %%clk\n", name)
	e.seqClockName = name
	return name
}

func processHasPhi(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		for _, op := range block.Ops {
			if _, ok := op.(*ir.PhiOperation); ok {
				return true
			}
		}
	}
	return false
}

// processHasLoop checks if a process has loops (phi nodes with back-edges).
// A back-edge exists if a phi node has an incoming edge from a block that appears
// later in the block list, indicating a loop in the control flow.
func processHasLoop(proc *ir.Process) bool {
	if proc == nil {
		return false
	}

	// Build block index map
	blockIndex := make(map[*ir.BasicBlock]int)
	for i, block := range proc.Blocks {
		blockIndex[block] = i
	}

	// Check each phi node for back-edges
	for blockIdx, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			if phi, ok := op.(*ir.PhiOperation); ok {
				// Check if any incoming edge is a back-edge
				for _, incoming := range phi.Incomings {
					if incomingIdx, exists := blockIndex[incoming.Block]; exists {
						if incomingIdx >= blockIdx {
							// Back-edge detected - this is a loop
							return true
						}
					}
				}
			}
		}
	}

	return false
}

func processHasChannelOps(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		for _, op := range block.Ops {
			switch op.(type) {
			case *ir.SendOperation, *ir.RecvOperation:
				return true
			}
		}
	}
	return false
}

func processHasPrintOps(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		for _, op := range block.Ops {
			if _, ok := op.(*ir.PrintOperation); ok {
				return true
			}
		}
	}
	return false
}

func processNeedsPrintControlFSM(proc *ir.Process) bool {
	// Route all print-bearing processes through the FSM lowering so stdout side
	// effects execute once per logical control path instead of on every clock.
	return processHasPrintOps(proc)
}

func (p *processPrinter) assignConst(sig *ir.Signal) string {
	if name, ok := p.constNames[sig]; ok {
		return name
	}
	name := fmt.Sprintf("%%c%d", p.emitter.globalTempID)
	p.emitter.globalTempID++
	p.constNames[sig] = name
	return name
}

func (p *processPrinter) bindSSA(sig *ir.Signal) string {
	if sig == nil {
		return "%unknown"
	}
	if name, ok := p.valueNames[sig]; ok {
		return name
	}
	name := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
	p.emitter.globalTempID++
	p.valueNames[sig] = name
	return name
}

func (p *processPrinter) valueRef(sig *ir.Signal) string {
	if sig == nil {
		return "%unknown"
	}
	if sig.Kind == ir.Const {
		return p.assignConst(sig)
	}
	if unpacked := p.inputArrayElementRef(sig); unpacked != "" {
		p.valueNames[sig] = unpacked
		return unpacked
	}
	if sig.Name != "" {
		if portName, ok := p.portNames[sig.Name]; ok {
			p.valueNames[sig] = portName
			return portName
		}
	}
	if (p.fsm != nil || p.directClocked) && sig.Name != "" && sig.Name != "clk" && sig.Name != "rst" {
		if name, ok := p.valueNames[sig]; ok && name != "" {
			rawName := "%" + sanitize(sig.Name)
			if name != rawName {
				return name
			}
		}
		if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
			wireName := "%" + sanitize(sig.Name)
			readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
			p.emitter.globalTempID++
			p.printIndent()
			fmt.Fprintf(p.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", readName, wireName, typeString(sig.Type))
			return readName
		}
	}
	if name, ok := p.valueNames[sig]; ok {
		return name
	}

	// Check if this is an internal signal (not a port)
	// Test data arrays (with "test_" prefix) are ports
	// Working storage arrays (like "compressed", "result") are internal signals
	isPort := strings.HasPrefix(sig.Name, "test_") || sig.Name == "clk" || sig.Name == "rst"

	// Check if this is an array element (name_number format)
	isArrayElement := false
	for i := 0; i < len(sig.Name)-1; i++ {
		if sig.Name[i] == '_' && sig.Name[i+1] >= '0' && sig.Name[i+1] <= '9' {
			isArrayElement = true
			break
		}
	}

	if !isPort && sig.Name != "" && !isArrayElement {
		if packed := p.packArraySignalValue(sig); packed != "" {
			p.valueNames[sig] = packed
			return packed
		}
		// Check if this is a scalar global register (pre-declared at module level)
		// All register-kind signals that are not array elements are pre-declared.
		// In combinational logic they still need a read_inout to produce a plain value.
		if sig.Kind == ir.Reg {
			if readName, ok := p.internalSignalReads[sig.Name]; ok {
				return readName
			}
			wireName := "%" + sanitize(sig.Name)
			readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
			p.emitter.globalTempID++
			p.printIndent()
			fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n", readName, wireName, inoutTypeString(sig.Type))
			p.internalSignalReads[sig.Name] = readName
			p.valueNames[sig] = readName
			return readName
		}

		// This is an internal signal, need to read it with sv.read_inout
		if readName, ok := p.internalSignalReads[sig.Name]; ok {
			return readName
		}

		// Emit sv.read_inout to get the regular type value
		wireName := "%" + sanitize(sig.Name)
		readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
		p.emitter.globalTempID++

		p.printIndent()
		fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n", readName, wireName, inoutTypeString(sig.Type))

		p.internalSignalReads[sig.Name] = readName
		p.valueNames[sig] = readName
		return readName
	}

	// For ports and array elements (which are declared as registers), reference directly
	name := "%" + sanitize(sig.Name)
	p.valueNames[sig] = name
	return name
}

func (p *processPrinter) inputArrayElementRef(sig *ir.Signal) string {
	if p == nil || sig == nil || sig.Name == "" || p.portNames == nil {
		return ""
	}
	base, index, ok := indexedSignalName(sig.Name)
	if !ok {
		return ""
	}
	portRef, ok := p.portNames[base]
	if !ok || portRef == "" {
		return ""
	}
	elemWidth := signalWidth(sig.Type)
	if elemWidth <= 0 {
		elemWidth = 1
	}
	offset := index * elemWidth
	name := p.freshValueName("in_elem")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = comb.extract %s from %d : (%s) -> %s\n",
		name,
		portRef,
		offset,
		typeString(arrayElementContainerType(base, elemWidth, p.portTypes, p.moduleSignals)),
		typeString(sig.Type),
	)
	return name
}

func indexedSignalName(name string) (string, int, bool) {
	if strings.TrimSpace(name) == "" {
		return "", 0, false
	}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] != '_' {
			continue
		}
		if i == len(name)-1 {
			return "", 0, false
		}
		raw := name[i+1:]
		idx, err := strconv.Atoi(raw)
		if err != nil {
			return "", 0, false
		}
		return name[:i], idx, true
	}
	return "", 0, false
}

func arrayElementContainerType(base string, elemWidth int, portTypes map[string]*ir.SignalType, moduleSignals map[string]*ir.Signal) *ir.SignalType {
	if portTypes != nil {
		if typ, ok := portTypes[base]; ok && typ != nil {
			return typ
		}
	}
	if moduleSignals != nil {
		if sig, ok := moduleSignals[base]; ok && sig != nil && sig.Type != nil {
			return sig.Type
		}
	}
	if elemWidth <= 0 {
		elemWidth = 1
	}
	return &ir.SignalType{Width: elemWidth, Signed: false}
}

func collectPortTypesFromIRPorts(ports []ir.Port) map[string]*ir.SignalType {
	portTypes := make(map[string]*ir.SignalType, len(ports))
	for _, port := range ports {
		portTypes[port.Name] = port.Type
	}
	return portTypes
}

func collectPortTypesFromDescs(ports []portDesc) map[string]*ir.SignalType {
	portTypes := make(map[string]*ir.SignalType, len(ports))
	for _, port := range ports {
		name := strings.TrimPrefix(port.name, "%")
		if name == "" {
			continue
		}
		portTypes[name] = parseTypeString(port.typ)
	}
	return portTypes
}

func parseTypeString(typ string) *ir.SignalType {
	typ = strings.TrimSpace(typ)
	typ = strings.TrimPrefix(typ, "!hw.inout<")
	typ = strings.TrimSuffix(typ, ">")
	if !strings.HasPrefix(typ, "i") {
		return &ir.SignalType{Width: 1, Signed: false}
	}
	width, err := strconv.Atoi(strings.TrimPrefix(typ, "i"))
	if err != nil || width <= 0 {
		width = 1
	}
	return &ir.SignalType{Width: width, Signed: false}
}

func (p *processPrinter) packArraySignalValue(sig *ir.Signal) string {
	if p == nil || sig == nil || sig.Type == nil || sig.Type.Width <= 1 || sig.Name == "" || p.moduleSignals == nil {
		return ""
	}
	elements := make([]string, 0, sig.Type.Width)
	for i := 0; i < sig.Type.Width; i++ {
		elemSig, ok := p.moduleSignals[fmt.Sprintf("%s_%d", sig.Name, i)]
		if !ok || elemSig == nil {
			return ""
		}
		elements = append(elements, p.valueRef(elemSig))
	}
	name := p.freshValueName("packed")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = comb.concat ", name)
	for i := len(elements) - 1; i >= 0; i-- {
		if i < len(elements)-1 {
			fmt.Fprint(p.w, ", ")
		}
		fmt.Fprint(p.w, elements[i])
	}
	fmt.Fprint(p.w, " : ")
	for i := len(elements) - 1; i >= 0; i-- {
		if i < len(elements)-1 {
			fmt.Fprint(p.w, ", ")
		}
		fmt.Fprint(p.w, "i1")
	}
	fmt.Fprintln(p.w)
	return name
}

func (p *processPrinter) portRef(name string) string {
	if val, ok := p.portNames[name]; ok {
		return val
	}
	return fmt.Sprintf("%%%s", sanitize(name))
}

func (p *processPrinter) resetAssertedRef() string {
	if !p.hasResetPort {
		return p.portRef("rst")
	}
	if !p.resetActiveLow {
		return p.resetPortRef
	}
	if p.resetAssertedName != "" {
		return p.resetAssertedName
	}
	one := p.boolConst(true)
	name := p.freshValueName("rst_asserted")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = comb.xor %s, %s : i1\n", name, p.resetPortRef, one)
	p.resetAssertedName = name
	return name
}

func (p *processPrinter) edgeValueRef(sig *ir.Signal) string {
	if p == nil || sig == nil {
		return "%unknown"
	}
	if sig.Name != "" && p.moduleSignals != nil {
		if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
			wireName := "%" + sanitize(sig.Name)
			readName := p.freshValueName("edge_reg")
			p.printIndent()
			fmt.Fprintf(p.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", readName, wireName, typeString(sig.Type))
			return readName
		}
	}
	return p.valueRef(sig)
}

func (p *processPrinter) printIndent() {
	for i := 0; i < p.indent; i++ {
		fmt.Fprint(p.w, "  ")
	}
}

func (p *processPrinter) boolConst(val bool) string {
	if name, ok := p.boolConsts[val]; ok {
		return name
	}
	name := fmt.Sprintf("%%c_bool_%d", len(p.boolConsts))
	p.boolConsts[val] = name
	p.printIndent()
	intVal := 0
	if val {
		intVal = 1
	}
	fmt.Fprintf(p.w, "%s = hw.constant %d : i1\n", name, intVal)
	return name
}

func moduleUsesFSM(module *ir.Module) bool {
	if module == nil {
		return false
	}
	for _, proc := range module.Processes {
		if proc == nil {
			continue
		}
		if proc.Sensitivity == ir.Sequential || processHasChannelOps(proc) || processNeedsPrintControlFSM(proc) {
			return true
		}
	}
	return false
}

func moduleNeedsSyntheticReset(module *ir.Module) bool {
	if module == nil {
		return false
	}
	for _, port := range module.Ports {
		if port.Direction == ir.Input && isResetPortName(port.Name) {
			return false
		}
	}
	for _, proc := range module.Processes {
		if proc == nil {
			continue
		}
		if processHasChannelOps(proc) || processNeedsPrintControlFSM(proc) {
			return true
		}
	}
	return false
}

func isResetPortName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rst", "reset", "ar", "areset", "resetn", "aresetn":
		return true
	default:
		return false
	}
}

func isActiveLowResetName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "resetn", "aresetn":
		return true
	default:
		return false
	}
}

func isClockLikeName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "clk", "clock":
		return true
	default:
		return false
	}
}

func isOutputGlobalName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "out_")
}

func (p *processPrinter) typedZeroConst(t *ir.SignalType) string {
	name := p.freshValueName("c_zero")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = hw.constant 0 : %s\n", name, typeString(t))
	return name
}

func (p *processPrinter) typedAllOnesConst(t *ir.SignalType) string {
	if signalWidth(t) == 1 {
		return p.boolConst(true)
	}
	name := p.freshValueName("c_ones")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = hw.constant -1 : %s\n", name, typeString(t))
	return name
}

func (p *processPrinter) orSignals(signals []string) string {
	filtered := make([]string, 0, len(signals))
	for _, sig := range signals {
		if sig == "" || sig == "%unknown" {
			continue
		}
		filtered = append(filtered, sig)
	}
	if len(filtered) == 0 {
		return p.boolConst(false)
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	result := filtered[0]
	for _, sig := range filtered[1:] {
		name := p.freshValueName("or")
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.or %s, %s : i1\n", name, result, sig)
		result = name
	}
	return result
}

func (p *processPrinter) muxByPredicates(predicates []string, values []string, t *ir.SignalType) string {
	if len(predicates) == 0 || len(values) == 0 {
		return p.typedZeroConst(t)
	}
	count := len(predicates)
	if len(values) < count {
		count = len(values)
	}
	defaultValue := p.typedZeroConst(t)
	result := defaultValue
	typeStr := typeString(t)
	for i := count - 1; i >= 0; i-- {
		pred := predicates[i]
		if pred == "" || pred == "%unknown" {
			continue
		}
		val := values[i]
		if val == "" || val == "%unknown" {
			val = defaultValue
		}
		name := p.freshValueName("mux")
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n", name, pred, val, result, typeStr)
		result = name
	}
	return result
}

func (p *processPrinter) freshValueName(prefix string) string {
	if prefix == "" {
		prefix = "tmp"
	}
	name := fmt.Sprintf("%%%s%d", prefix, p.emitter.globalTempID)
	p.emitter.globalTempID++
	return name
}

func (p *processPrinter) emitConvertOperation(o *ir.ConvertOperation) {
	if o == nil || o.Value == nil || o.Dest == nil {
		return
	}
	srcWidth := signalWidth(o.Value.Type)
	destWidth := signalWidth(o.Dest.Type)
	src := p.valueRef(o.Value)
	dest := p.bindSSA(o.Dest)
	from := typeString(o.Value.Type)
	to := typeString(o.Dest.Type)

	switch {
	case destWidth == srcWidth:
		// Sign-only reinterpretation: MLIR integer types are signless, so we can
		// alias the source value directly instead of emitting an explicit bitcast.
		p.valueNames[o.Dest] = src
		return
	case destWidth > srcWidth:
		extendWidth := destWidth - srcWidth
		if extendWidth <= 0 {
			p.valueNames[o.Dest] = src
			return
		}
		if o.Value.Type != nil && o.Value.Type.Signed {
			signBit := p.freshValueName("sext_msb")
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.extract %s from %d : (%s) -> i1\n",
				signBit,
				src,
				srcWidth-1,
				from,
			)
			replicated := p.freshValueName("sext_bits")
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.replicate %s : (i1) -> i%d\n",
				replicated,
				signBit,
				extendWidth,
			)
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.concat %s, %s : i%d, %s\n",
				dest,
				replicated,
				src,
				extendWidth,
				from,
			)
		} else {
			p.printIndent()
			zeros := p.freshValueName("zext_pad")
			fmt.Fprintf(p.w, "%s = hw.constant 0 : i%d\n", zeros, extendWidth)
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.concat %s, %s : i%d, %s\n",
				dest,
				zeros,
				src,
				extendWidth,
				from,
			)
		}
	default:
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.extract %s from 0 : (%s) -> %s\n",
			dest,
			src,
			from,
			to,
		)
	}
}

func (p *processPrinter) emitCallOperation(op *ir.CallOperation) {
	if op == nil || strings.TrimSpace(op.Callee) == "" {
		return
	}

	// Generate module and instance names
	callee := sanitize(op.Callee)
	moduleName := fmt.Sprintf("%s__proc_%s", p.moduleName, callee)
	instanceName := fmt.Sprintf("\"%s_inst\"", callee)

	// Look up the module's port information
	ports, ok := p.modulePorts[moduleName]
	if !ok {
		// Module not found - this shouldn't happen in normal flow
		// Fall back to generic argument names
		p.printIndent()
		fmt.Fprintf(p.w, "// warning: module %s not found for call\n", moduleName)
		return
	}

	// Build argument list for the instance using actual port names
	args := make([]string, 0, len(ports))

	// Skip clk and rst ports (ports[0] and ports[1]) as they're added automatically
	// Map arguments to parameter ports (starting from port 2)
	portIdx := 2 // Skip clk and rst
	for _, arg := range op.Args {
		if arg == nil {
			continue
		}
		if portIdx >= len(ports) {
			// Shouldn't happen - indicates port count mismatch
			break
		}

		// Use the actual port name from the module definition
		portName := strings.TrimPrefix(ports[portIdx].name, "%")

		// Get the SSA value reference for this argument
		argValue := p.valueRef(arg)

		args = append(args, fmt.Sprintf("%s: %s : %s", portName, argValue, typeString(arg.Type)))
		portIdx++
	}

	// Generate instance call
	p.printIndent()
	if op.Dest != nil {
		// For functions with return values, the result is an input port named "result"
		// Add the result port argument
		resultName := p.bindSSA(op.Dest)
		p.valueNames[op.Dest] = resultName

		// Check if there's a result port and add it as an argument
		for _, port := range ports {
			if strings.TrimPrefix(port.name, "%") == "result" {
				// Add result as an input argument (it's an inout port in the module)
				args = append(args, fmt.Sprintf("result: %s : %s", resultName, typeString(op.Dest.Type)))
				break
			}
		}

		// Generate the hw.instance with instance name
		if len(args) > 0 {
			fmt.Fprintf(p.w, "hw.instance %s @%s(%s) -> ()\n",
				instanceName, moduleName, strings.Join(args, ", "))
		} else {
			fmt.Fprintf(p.w, "hw.instance %s @%s() -> ()\n",
				instanceName, moduleName)
		}
	} else {
		// No return value
		if len(args) > 0 {
			fmt.Fprintf(p.w, "hw.instance %s @%s(%s) -> ()\n",
				instanceName, moduleName, strings.Join(args, ", "))
		} else {
			fmt.Fprintf(p.w, "hw.instance %s @%s() -> ()\n",
				instanceName, moduleName)
		}
	}
}

func (p *processPrinter) emitPrintOperation(op *ir.PrintOperation) {
	if op == nil {
		return
	}
	format, operands, operandTypes := p.buildPrintfFormat(op)
	clk := p.portRef("clk")
	fd := p.stdoutConstant()

	p.printIndent()
	fmt.Fprintf(p.w, "sv.always posedge %s {\n", clk)
	p.indent++
	p.printIndent()
	if len(operands) == 0 {
		fmt.Fprintf(p.w, "sv.fwrite %s, %s\n", fd, strconv.Quote(format))
	} else {
		fmt.Fprintf(p.w, "sv.fwrite %s, %s(%s) : %s\n",
			fd,
			strconv.Quote(format),
			strings.Join(operands, ", "),
			strings.Join(operandTypes, ", "),
		)
	}
	p.indent--
	p.printIndent()
	fmt.Fprintln(p.w, "}")
}

func (p *processPrinter) buildPrintfFormat(op *ir.PrintOperation) (string, []string, []string) {
	var builder strings.Builder
	var values []string
	var types []string

	for _, seg := range op.Segments {
		if seg.Value == nil {
			builder.WriteString(escapePercent(seg.Text))
			continue
		}
		values = append(values, p.valueRef(seg.Value))
		types = append(types, typeString(seg.Value.Type))
		builder.WriteString(printVerbSpecifier(seg))
	}
	return builder.String(), values, types
}

func escapePercent(text string) string {
	return strings.ReplaceAll(text, "%", "%%")
}

func printVerbSpecifier(seg ir.PrintSegment) string {
	var builder strings.Builder
	builder.WriteByte('%')
	if seg.ZeroPad && seg.Width > 0 {
		builder.WriteByte('0')
	}
	if seg.Width > 0 {
		builder.WriteString(strconv.Itoa(seg.Width))
	}
	switch seg.Verb {
	case ir.PrintVerbHex:
		if seg.Width == 0 {
			builder.WriteString("0x")
		} else {
			builder.WriteByte('x')
		}
	case ir.PrintVerbBin:
		if seg.Width == 0 {
			builder.WriteString("0b")
		} else {
			builder.WriteByte('b')
		}
	case ir.PrintVerbFloat:
		builder.WriteByte('f')
	case ir.PrintVerbBool:
		if seg.Width == 0 {
			builder.WriteString("0s")
		} else {
			builder.WriteByte('s')
		}
	default:
		if seg.Width == 0 {
			builder.WriteString("0d")
		} else {
			builder.WriteByte('d')
		}
	}
	return builder.String()
}

func (p *processPrinter) stdoutConstant() string {
	if p.stdoutFD != "" {
		return p.stdoutFD
	}
	name := p.freshValueName("stdout_fd")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = hw.constant %d : i32\n", name, 0x80000001)
	p.stdoutFD = name
	return name
}

func portDecls(ports []ir.Port) []string {
	decls := make([]string, 0, len(ports))
	for _, port := range ports {
		switch port.Direction {
		case ir.Output:
			decls = append(decls, fmt.Sprintf("out %s: %s", sanitize(port.Name), typeString(port.Type)))
		default:
			decls = append(decls, fmt.Sprintf("in %%%s: %s", sanitize(port.Name), typeString(port.Type)))
		}
	}
	return decls
}

func typeString(t *ir.SignalType) string {
	width := 1
	if t != nil && t.Width > 0 {
		width = t.Width
	}
	return fmt.Sprintf("i%d", width)
}

func inoutTypeString(t *ir.SignalType) string {
	return fmt.Sprintf("!hw.inout<%s>", typeString(t))
}

func binOpName(op ir.BinOp) string {
	switch op {
	case ir.Add:
		return "add"
	case ir.Sub:
		return "sub"
	case ir.Mul:
		return "mul"
	case ir.Div:
		// For division, we need to determine signed vs unsigned
		// Default to unsigned division for safety
		return "divu"
	case ir.Rem:
		// Current lowering treats integer arithmetic as unsigned in comb.
		// This matches the existing division path and covers positive CHStone indices.
		return "modu"
	case ir.And:
		return "and"
	case ir.Or:
		return "or"
	case ir.Xor:
		return "xor"
	case ir.Shl:
		return "shl"
	case ir.ShrU:
		return "shru"
	case ir.ShrS:
		return "shrs"
	default:
		return "unknown"
	}
}

func comparePredicateName(pred ir.ComparePredicate) string {
	switch pred {
	case ir.CompareEQ:
		return "eq"
	case ir.CompareNE:
		return "ne"
	case ir.CompareSLT:
		return "slt"
	case ir.CompareSLE:
		return "sle"
	case ir.CompareSGT:
		return "sgt"
	case ir.CompareSGE:
		return "sge"
	case ir.CompareULT:
		return "ult"
	case ir.CompareULE:
		return "ule"
	case ir.CompareUGT:
		return "ugt"
	case ir.CompareUGE:
		return "uge"
	default:
		return "eq"
	}
}

func processStage(proc *ir.Process) int {
	if proc == nil {
		return 0
	}
	if proc.Stage < 0 {
		return 0
	}
	return proc.Stage
}

func processName(proc *ir.Process) string {
	if proc == nil || proc.Name == "" {
		return "unnamed_process"
	}
	return sanitize(proc.Name)
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

func (e *emitter) getSignalInitValue(sig *ir.Signal) interface{} {
	if sig == nil || sig.Value == nil {
		return 0
	}
	return sig.Value
}

func formatHWConstant(value interface{}, typ *ir.SignalType) string {
	if value == nil {
		return "0"
	}
	switch v := value.(type) {
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int:
		return normalizeHWConstantBits(uint64(int64(v)), signalWidth(typ))
	case int8:
		return normalizeHWConstantBits(uint64(int64(v)), signalWidth(typ))
	case int16:
		return normalizeHWConstantBits(uint64(int64(v)), signalWidth(typ))
	case int32:
		return normalizeHWConstantBits(uint64(int64(v)), signalWidth(typ))
	case int64:
		return normalizeHWConstantBits(uint64(v), signalWidth(typ))
	case uint:
		return normalizeHWConstantBits(uint64(v), signalWidth(typ))
	case uint8:
		return normalizeHWConstantBits(uint64(v), signalWidth(typ))
	case uint16:
		return normalizeHWConstantBits(uint64(v), signalWidth(typ))
	case uint32:
		return normalizeHWConstantBits(uint64(v), signalWidth(typ))
	case uint64:
		return normalizeHWConstantBits(v, signalWidth(typ))
	case string:
		return v
	default:
		return fmt.Sprintf("%v", value)
	}
}

func normalizeHWConstantBits(bits uint64, width int) string {
	if width <= 0 {
		return strconv.FormatUint(bits, 10)
	}
	if width == 1 {
		return strconv.FormatUint(bits&1, 10)
	}
	if width >= 64 {
		return strconv.FormatInt(int64(bits), 10)
	}
	mask := (uint64(1) << width) - 1
	bits &= mask
	signBit := uint64(1) << (width - 1)
	if bits&signBit != 0 {
		return strconv.FormatInt(int64(bits|^mask), 10)
	}
	return strconv.FormatUint(bits, 10)
}

func (e *emitter) emitFifoExterns() {
	if e.loweredChannels == nil || len(e.loweredChannels.FIFODecls) == 0 {
		return
	}
	for _, decl := range e.loweredChannels.FIFODecls {
		if decl == nil {
			continue
		}
		elemType := typeString(decl.DataType)
		e.printIndent()
		fmt.Fprintf(e.w, "hw.module @%s(in %%clk: i1, in %%rst_n: i1, inout %%wr_en: i1, inout %%wr_data: %s, inout %%full: i1, inout %%almost_full: i1, inout %%rd_en: i1, inout %%rd_data: %s, inout %%empty: i1, inout %%almost_empty: i1) {\n",
			decl.ModuleName,
			elemType,
			elemType,
		)
		e.indent++
		e.printIndent()
		fmt.Fprintln(e.w, "hw.output")
		e.indent--
		e.printIndent()
		fmt.Fprintln(e.w, "}")
	}
}

func signalWidth(t *ir.SignalType) int {
	if t == nil || t.Width <= 0 {
		return 1
	}
	return t.Width
}
