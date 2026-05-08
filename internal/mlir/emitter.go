package mlir

import (
	"bufio"
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
	var flush func() error
	if outputPath == "" || outputPath == "-" {
		bw := bufio.NewWriterSize(os.Stdout, 1<<20)
		w = bw
		flush = bw.Flush
	} else {
		f, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		bw := bufio.NewWriterSize(f, 1<<20)
		w = bw
		flush = bw.Flush
	}
	if flush == nil {
		flush = func() error { return nil }
	}

	em := &emitter{
		w:               w,
		loweredChannels: ir.LowerChannelsToFIFO(design),
		modulePorts:     make(map[string][]portDesc),
		assignedSignals: make(map[*ir.Module]map[string]struct{}),
	}
	fmt.Fprintln(w, "module {")
	em.indent++
	for _, module := range design.Modules {
		em.emitModule(module)
	}
	em.emitFifoExterns()
	em.indent--
	fmt.Fprintln(w, "}")
	return flush()
}

type emitter struct {
	w               io.Writer
	indent          int
	loweredChannels *ir.LoweredChannelDesign
	seqClockName    string
	modulePorts     map[string][]portDesc     // Track module ports for instances
	globalTempID    int                       // Global counter for unique temporary names
	rootValueNames  map[*ir.Signal]string     // Value names from root process for output resolution
	rootConstNames  map[*ir.Signal]string     // Constant names from root process
	topPortTypes    map[string]*ir.SignalType // Readable top-level ports only
	assignedSignals map[*ir.Module]map[string]struct{}
	currentAssigned map[string]struct{}
	immutableConsts map[*ir.Signal]string
}

func (e *emitter) emitModule(module *ir.Module) {
	if module == nil {
		return
	}
	prevRootValues := e.rootValueNames
	prevRootConsts := e.rootConstNames
	prevTopPortTypes := e.topPortTypes
	prevCurrentAssigned := e.currentAssigned
	prevImmutableConsts := e.immutableConsts
	e.rootValueNames = nil
	e.rootConstNames = nil
	e.topPortTypes = nil
	e.currentAssigned = e.moduleAssignedSignalNames(module)
	e.immutableConsts = make(map[*ir.Signal]string)
	defer func() {
		e.rootValueNames = prevRootValues
		e.rootConstNames = prevRootConsts
		e.topPortTypes = prevTopPortTypes
		e.currentAssigned = prevCurrentAssigned
		e.immutableConsts = prevImmutableConsts
	}()

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
	e.topPortTypes = collectReadablePortTypesFromIRPorts(topPorts)

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
	allowRootValueShortcut := canUseRootValueShortcut(root)
	for _, port := range topPorts {
		if port.Direction == ir.Output {
			beforeCount := len(outputValues)
			globalName := outputBindingName(port)
			if cached, ok := resolvedBindings[globalName]; ok && cached.value != "" {
				outputValues = append(outputValues, cached.value)
				outputTypes = append(outputTypes, cached.typ)
				continue
			}
			if port.Type != nil && port.Type.Width > 1 {
				if packedRef, ok := e.resolvePackedStateWordOutput(module, port, globalName); ok {
					outputValues = append(outputValues, packedRef)
					outputTypes = append(outputTypes, typeString(port.Type))
					resolvedBindings[globalName] = outputBindingValue{value: packedRef, typ: typeString(port.Type)}
					continue
				}
				var elements []string
				preferStructured := root != nil && root.proc != nil && canEmitDirectClockedControl(root.proc)
				for i := 0; i < port.Type.Width; i++ {
					elemSig := resolveIndexedElementSignal(module.Signals, globalName, i, 1)
					if elemSig != nil {
						elemName := elemSig.Name
						if elemSig.Kind != ir.Reg && root != nil && root.proc != nil {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						structuredElem := preferStructured && elemSig.Kind != ir.Reg
						if structuredElem {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						if !structuredElem && elemSig.Kind != ir.Reg {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						if elemSig.Kind != ir.Reg && e.rootValueNames != nil {
							if valueName, ok := e.rootValueNames[elemSig]; ok && valueName != "" {
								rawName := "%" + sanitize(elemName)
								if valueName != rawName {
									elements = append(elements, valueName)
									continue
								}
							}
						}
						if allowRootValueShortcut && e.rootValueNames != nil && shouldPreferRootOutputValue(module, globalName, useInoutRegs, elemSig) {
							if valueName, ok := e.rootValueNames[elemSig]; ok {
								elements = append(elements, valueName)
								continue
							}
						}
						ssaName := "%" + sanitize(elemName)
						if elemSig.Kind == ir.Reg {
							ref := e.rootRegRef(elemSig, fmt.Sprintf("read_%s_%d", sanitize(port.Name), i))
							if ref == "" {
								continue
							}
							elements = append(elements, ref)
						} else {
							elements = append(elements, ssaName)
						}
					}
				}
				if len(elements) > 0 {
					packedName := fmt.Sprintf("%%packed_%s", sanitize(port.Name))
					e.printIndent()
					fmt.Fprintf(e.w, "%s = comb.concat ", packedName)
					for i := len(elements) - 1; i >= 0; i-- {
						if i < len(elements)-1 {
							fmt.Fprint(e.w, ", ")
						}
						fmt.Fprint(e.w, elements[i])
					}
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
					continue
				}
			}

			// Check if this is a scalar signal
			if sig, ok := module.Signals[globalName]; ok && sig != nil {
				if sig.Kind != ir.Reg && root != nil && root.proc != nil {
					if resolved, ok := e.resolveRootCombinationalOutputValue(root, sig); ok && resolved != "" {
						outputValues = append(outputValues, resolved)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: resolved, typ: typeString(sig.Type)}
						continue
					}
				}
				preferStructured := root != nil && root.proc != nil && canEmitDirectClockedControl(root.proc)
				structuredSig := preferStructured && sig.Kind != ir.Reg
				if structuredSig {
					if resolved, ok := e.resolveRootCombinationalOutputValue(root, sig); ok && resolved != "" {
						outputValues = append(outputValues, resolved)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: resolved, typ: typeString(sig.Type)}
						continue
					}
				}
				if !structuredSig && sig.Kind != ir.Reg {
					if resolved, ok := e.resolveRootCombinationalOutputValue(root, sig); ok && resolved != "" {
						outputValues = append(outputValues, resolved)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: resolved, typ: typeString(sig.Type)}
						continue
					}
				}
				if sig.Kind != ir.Reg && e.rootValueNames != nil {
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
				if allowRootValueShortcut && e.rootValueNames != nil && shouldPreferRootOutputValue(module, globalName, useInoutRegs, sig) {
					if valueName, ok := e.rootValueNames[sig]; ok {
						outputValues = append(outputValues, valueName)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: valueName, typ: typeString(sig.Type)}
						continue
					}
				}

				// For wire signals in sequential processes, resolve the assignment source
				if sig.Kind == ir.Wire && root != nil && root.proc != nil && root.proc.Sensitivity == ir.Sequential {
					if assignSrc := e.resolveWireAssignmentSource(root.proc, sig); assignSrc != "" {
						outputValues = append(outputValues, assignSrc)
						outputTypes = append(outputTypes, typeString(sig.Type))
						resolvedBindings[globalName] = outputBindingValue{value: assignSrc, typ: typeString(sig.Type)}
						continue
					}
				}

				// Scalar output: check if it's a register (inout) or a wire
				ssaName := "%" + sanitize(sig.Name)
				if sig.Kind == ir.Reg {
					ref := e.rootRegRef(sig, "read_"+sanitize(port.Name))
					if ref == "" {
						continue
					}
					outputValues = append(outputValues, ref)
					resolvedBindings[globalName] = outputBindingValue{value: ref, typ: typeString(sig.Type)}
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
					elemSig := resolveIndexedElementSignal(module.Signals, globalName, i, 1)
					if elemSig != nil {
						elemName := elemSig.Name
						if elemSig.Kind != ir.Reg && root != nil && root.proc != nil {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						structuredElem := preferStructured && elemSig.Kind != ir.Reg
						if structuredElem {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						if !structuredElem && elemSig.Kind != ir.Reg {
							if resolved, ok := e.resolveRootCombinationalOutputValue(root, elemSig); ok && resolved != "" {
								elements = append(elements, resolved)
								continue
							}
						}
						if elemSig.Kind != ir.Reg && e.rootValueNames != nil {
							if valueName, ok := e.rootValueNames[elemSig]; ok && valueName != "" {
								rawName := "%" + sanitize(elemName)
								if valueName != rawName {
									elements = append(elements, valueName)
									continue
								}
							}
						}
						// First check if we have a value from the root process (for combinational logic)
						if allowRootValueShortcut && e.rootValueNames != nil && shouldPreferRootOutputValue(module, globalName, useInoutRegs, elemSig) {
							if valueName, ok := e.rootValueNames[elemSig]; ok {
								elements = append(elements, valueName)
								continue
							}
						}

						// Otherwise, use the signal name
						ssaName := "%" + sanitize(elemName)
						if elemSig.Kind == ir.Reg {
							ref := e.rootRegRef(elemSig, fmt.Sprintf("read_%s_%d", sanitize(port.Name), i))
							if ref == "" {
								continue
							}
							elements = append(elements, ref)
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

func (e *emitter) resolvePackedStateWordOutput(module *ir.Module, port ir.Port, binding string) (string, bool) {
	if e == nil || module == nil || module.Signals == nil || port.Type == nil || port.Type.Width <= 1 {
		return "", false
	}
	if binding != "out_q" && port.Name != "q" {
		return "", false
	}
	wordRegs := findPackedStateWordRegs(module.Signals)
	if len(wordRegs) == 0 {
		return "", false
	}
	totalWidth := 0
	for _, sig := range wordRegs {
		totalWidth += signalWidth(sig.Type)
	}
	if totalWidth != port.Type.Width {
		return "", false
	}
	values := make([]string, 0, len(wordRegs))
	types := make([]string, 0, len(wordRegs))
	for _, sig := range wordRegs {
		ref := e.rootSignalRef(sig)
		ref = e.normalizeResolvedSignalRef(sig, ref)
		if ref == "" || ref == "%unknown" {
			return "", false
		}
		values = append(values, ref)
		types = append(types, typeString(sig.Type))
	}
	name := e.freshValueName("packed_state_words")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.concat ", name)
	for i := len(values) - 1; i >= 0; i-- {
		if i < len(values)-1 {
			fmt.Fprint(e.w, ", ")
		}
		fmt.Fprint(e.w, values[i])
	}
	fmt.Fprint(e.w, " : ")
	for i := len(types) - 1; i >= 0; i-- {
		if i < len(types)-1 {
			fmt.Fprint(e.w, ", ")
		}
		fmt.Fprint(e.w, types[i])
	}
	fmt.Fprintln(e.w)
	return name, true
}

func findPackedStateWordRegs(signals map[string]*ir.Signal) []*ir.Signal {
	type indexedSig struct {
		index int
		sig   *ir.Signal
	}
	var indexed []indexedSig
	for name, sig := range signals {
		if sig == nil || sig.Kind != ir.Reg || sig.Type == nil || signalWidth(sig.Type) <= 1 {
			continue
		}
		if !strings.HasPrefix(name, "state") {
			continue
		}
		raw := strings.TrimPrefix(name, "state")
		if raw == "" {
			continue
		}
		idx, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		indexed = append(indexed, indexedSig{index: idx, sig: sig})
	}
	if len(indexed) == 0 {
		return nil
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].index < indexed[j].index })
	for i := range indexed {
		if indexed[i].index != i {
			return nil
		}
	}
	out := make([]*ir.Signal, 0, len(indexed))
	for _, entry := range indexed {
		out = append(out, entry.sig)
	}
	return out
}

func (e *emitter) resolveRootCombinationalOutputValue(root *processInfo, sig *ir.Signal) (string, bool) {
	if e == nil || root == nil || root.proc == nil || sig == nil {
		return "", false
	}
	if len(root.proc.Blocks) == 0 {
		return "", false
	}
	if emitterProcessHasDualClockEdges(root.proc) {
		if resolved, ok := e.resolveDualEdgeOutputValue(root.proc); ok {
			return resolved, true
		}
	}
	if root.proc.Sensitivity == ir.Combinational {
		if isOutputGlobalName(sig.Name) && !signalReadOnRHS(root.proc, sig) {
			if countSignalAssignments(root.proc, sig) > 1 {
				cache := make(map[combResolveKey]string)
				visiting := make(map[combOutputKey]bool)
				resolved, ok := e.resolveCombinationalOutputAtBlock(root.proc, root.proc.Blocks[0], nil, sig, "", cache, visiting)
				if ok {
					if zeroGuarded, zok := e.zeroWhenNoActiveState(root.proc, sig, resolved); zok {
						return zeroGuarded, true
					}
					return resolved, true
				}
			}
			if resolved, ok := e.resolveOrderedCombinationalOutput(root.proc, sig); ok {
				if zeroGuarded, zok := e.zeroWhenNoActiveState(root.proc, sig, resolved); zok {
					return zeroGuarded, true
				}
				return resolved, true
			}
		}
		if sig.Kind == ir.Reg {
			return "", false
		}
		if processHasLoop(root.proc) {
			if resolved, ok := e.resolveCountedLoopOutputValue(root.proc, sig); ok {
				return resolved, true
			}
		}
		if sig.Kind != ir.Reg && e.rootValueNames != nil && countSignalAssignments(root.proc, sig) <= 1 {
			if ref, ok := e.rootValueNames[sig]; ok && ref != "" {
				raw := "%" + sanitize(sig.Name)
				if ref != raw {
					if zeroGuarded, zok := e.zeroWhenNoActiveState(root.proc, sig, ref); zok {
						return zeroGuarded, true
					}
					return ref, true
				}
			}
		}
		if resolved, ok := e.resolveAlwaysAssignedOutputValue(root.proc, sig, nil); ok {
			if zeroGuarded, zok := e.zeroWhenNoActiveState(root.proc, sig, resolved); zok {
				return zeroGuarded, true
			}
			return resolved, true
		}
		cache := make(map[combResolveKey]string)
		visiting := make(map[combOutputKey]bool)
		resolved, ok := e.resolveCombinationalOutputAtBlock(root.proc, root.proc.Blocks[0], nil, sig, "", cache, visiting)
		if zeroGuarded, zok := e.zeroWhenNoActiveState(root.proc, sig, resolved); zok {
			return zeroGuarded, true
		}
		return resolved, ok
	}
	if canEmitDirectClockedControl(root.proc) {
		clocked := computeEmitterClockedBlocks(root.proc)
		if !isOutputGlobalName(sig.Name) && !emitterProcessHasDualClockEdges(root.proc) {
			if resolved, ok := e.resolveDirectClockedAssignedOutput(root.proc, sig, clocked); ok {
				return resolved, true
			}
		}
		if !isOutputGlobalName(sig.Name) {
			if resolved, ok := e.resolveAlwaysAssignedOutputValue(root.proc, sig, clocked); ok {
				return resolved, true
			}
		}
		if isOutputGlobalName(sig.Name) {
			if mirror := findMirroredStateSignal(root.proc, sig); mirror != nil {
				ref := e.rootSignalRef(mirror)
				ref = e.normalizeResolvedSignalRef(mirror, ref)
				if ref != "" && ref != "%unknown" {
					return ref, true
				}
			}
			clockedAssign, nonClockedAssign := emitterSignalAssignmentKinds(root.proc, sig, clocked)
			if !signalReadOnRHS(root.proc, sig) {
				if resolved, ok := e.resolveOrderedDirectClockedOutput(root.proc, sig, clocked); ok {
					return resolved, true
				}
			}
			if nonClockedAssign && emitterProcessHasDualClockEdges(root.proc) {
				if resolved, ok := e.resolveDualEdgeHeldOutput(root.proc, sig, clocked); ok {
					return resolved, true
				}
			}
			if nonClockedAssign {
				if resolved, ok := e.resolveOrderedDirectClockedOutput(root.proc, sig, clocked); ok {
					return resolved, true
				}
			}
			if !clockedAssign {
				if resolved, ok := e.resolveDirectClockedAssignedOutput(root.proc, sig, clocked); ok {
					return resolved, true
				}
			}
			cache := make(map[combResolveKey]string)
			visiting := make(map[combOutputKey]bool)
			return e.resolveDirectClockedOutputAtBlock(root.proc, root.proc.Blocks[0], nil, sig, "", clocked, cache, visiting)
		}
		cache := make(map[combResolveKey]string)
		visiting := make(map[combOutputKey]bool)
		return e.resolveDirectClockedOutputAtBlock(root.proc, root.proc.Blocks[0], nil, sig, "", clocked, cache, visiting)
	}
	return "", false
}

func (e *emitter) resolveWireAssignmentSource(proc *ir.Process, wire *ir.Signal) string {
	if e == nil || proc == nil || wire == nil || wire.Kind != ir.Wire {
		return ""
	}
	// Find the assignment to this wire signal
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			if assign, ok := op.(*ir.AssignOperation); ok {
				if assign.Dest != nil && assign.Dest.Name == wire.Name && assign.Value != nil {
					// Found the assignment, return the source signal reference
					if assign.Value.Kind == ir.Reg {
						return e.rootRegRef(assign.Value, "read_"+sanitize(assign.Value.Name))
					}
					// For other signal types, use the SSA name directly
					return "%" + sanitize(assign.Value.Name)
				}
			}
		}
	}
	return ""
}

func (e *emitter) zeroWhenNoActiveState(proc *ir.Process, sig *ir.Signal, resolved string) (string, bool) {
	if e == nil || proc == nil || sig == nil || resolved == "" || resolved == "%unknown" {
		return "", false
	}
	if !strings.Contains(sig.Name, "next_state") {
		return "", false
	}
	// Preserve the guard only for one-shot next_state builders that assign the
	// output from a locally accumulated value. Direct multi-branch encoded FSM
	// outputs have valid state==0 semantics and must not be forced to zero.
	if countSignalAssignments(proc, sig) != 1 {
		return "", false
	}
	stateType, ok := e.topPortTypes["state"]
	if !ok || stateType == nil || signalWidth(stateType) <= 1 {
		return "", false
	}
	zeroState := e.freshValueName("state_zero")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = hw.constant 0 : %s\n", zeroState, typeString(stateType))
	stateEmpty := e.freshValueName("state_empty")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.icmp eq %%state, %s : %s\n", stateEmpty, zeroState, typeString(stateType))
	zeroOut := e.typedZeroConst(sig.Type)
	name := e.freshValueName("next_state_guard")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, stateEmpty, zeroOut, resolved, typeString(sig.Type))
	return name, true
}

func (e *emitter) resolveOrderedCombinationalOutput(proc *ir.Process, target *ir.Signal) (string, bool) {
	if e == nil || proc == nil || target == nil {
		return "", false
	}
	assigns := make([]directOutputAssign, 0)
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name != target.Name {
				continue
			}
			assigns = append(assigns, directOutputAssign{block: block, value: assign.Value})
		}
	}
	if len(assigns) == 0 {
		return "", false
	}

	cache := make(map[combResolveKey]string)
	current := ""
	for idx, assign := range assigns {
		ref := e.resolveCombinationalSignalValue(proc, assign.value, assign.block, nil, cache)
		ref = e.normalizeResolvedSignalRef(assign.value, ref)
		if ref == "" || ref == "%unknown" {
			continue
		}
		if idx == 0 || current == "" {
			current = ref
			continue
		}
		termsCache := make(map[*ir.BasicBlock][]condTerm)
		terms := emitterBlockReachabilityTerms(proc, assign.block, false, termsCache, make(map[*ir.BasicBlock]bool))
		condRef := e.emitEmitterCondTermsRef(terms)
		if condRef == "" || condRef == "%unknown" {
			current = ref
			continue
		}
		name := e.freshValueName("out_mux")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, condRef, ref, current, typeString(target.Type))
		current = name
	}
	if current == "" {
		return "", false
	}
	return current, true
}

func findMirroredStateSignal(proc *ir.Process, target *ir.Signal) *ir.Signal {
	if proc == nil || target == nil || !isOutputGlobalName(target.Name) {
		return nil
	}
	var candidate *ir.Signal
	foundAssign := false
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		var targetAssign *ir.AssignOperation
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name == target.Name {
				targetAssign = assign
				break
			}
		}
		if targetAssign == nil {
			continue
		}
		foundAssign = true
		var blockCandidate *ir.Signal
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name == target.Name || isOutputGlobalName(assign.Dest.Name) {
				continue
			}
			if assign.Value == targetAssign.Value || emitterSameSignal(assign.Value, targetAssign.Value) {
				blockCandidate = assign.Dest
				break
			}
		}
		if blockCandidate == nil {
			return nil
		}
		if candidate == nil {
			candidate = blockCandidate
			continue
		}
		if !emitterSameSignal(candidate, blockCandidate) {
			return nil
		}
	}
	if !foundAssign || candidate == nil {
		return nil
	}
	return candidate
}

func (e *emitter) resolveDualEdgeHeldOutput(proc *ir.Process, target *ir.Signal, clocked map[*ir.BasicBlock]bool) (string, bool) {
	if e == nil || proc == nil || target == nil {
		return "", false
	}
	partnerName := dualEdgeHoldPartnerName(target.Name)
	if partnerName == "" {
		return "", false
	}
	lastAssign, ok := findLastOutputAssign(proc, target.Name)
	if !ok || lastAssign.value == nil {
		return "", false
	}
	partnerRef := e.freshValueName("held_q")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = sv.read_inout %%%s : !hw.inout<%s>\n", partnerRef, sanitize(partnerName), typeString(target.Type))
	if partnerRef == "" || partnerRef == "%unknown" {
		return "", false
	}
	valueCache := make(map[combResolveKey]string)
	highRef := e.resolveDirectClockedSignalValue(proc, lastAssign.value, lastAssign.block, nil, clocked, valueCache)
	highRef = e.normalizeResolvedSignalRef(lastAssign.value, highRef)
	if highRef == "" || highRef == "%unknown" {
		return "", false
	}
	clockRef := "%clock"
	if _, ok := e.topPortTypes["clock"]; !ok {
		clockRef = "%clk"
	}
	name := e.freshValueName("out_hold")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, clockRef, highRef, partnerRef, typeString(target.Type))
	return name, true
}

func dualEdgeHoldPartnerName(name string) string {
	switch {
	case strings.HasSuffix(name, "_p"):
		return strings.TrimSuffix(name, "_p") + "_q"
	case name == "p":
		return "q"
	default:
		return ""
	}
}

func findLastOutputAssign(proc *ir.Process, targetName string) (directOutputAssign, bool) {
	if proc == nil || strings.TrimSpace(targetName) == "" {
		return directOutputAssign{}, false
	}
	var last directOutputAssign
	found := false
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name != targetName {
				continue
			}
			last = directOutputAssign{block: block, value: assign.Value}
			found = true
		}
	}
	return last, found
}

type countedLoopInfo struct {
	header           *ir.BasicBlock
	bodyEntry        *ir.BasicBlock
	done             *ir.BasicBlock
	entry            *ir.BasicBlock
	headerPhis       []*ir.PhiOperation
	entryIncoming    map[*ir.Signal]*ir.Signal
	backedgeIncoming map[*ir.Signal]*ir.Signal
	iterations       int
}

type countedLoopResolveKey struct {
	sig  *ir.Signal
	pred *ir.BasicBlock
}

func (e *emitter) resolveCountedLoopOutputValue(proc *ir.Process, target *ir.Signal) (string, bool) {
	if e == nil || proc == nil || target == nil {
		return "", false
	}
	info, ok := analyzeCountedLoop(proc)
	if !ok {
		return "", false
	}

	overrides := make(map[*ir.Signal]string, len(info.headerPhis))
	for _, phi := range info.headerPhis {
		if phi == nil || phi.Dest == nil {
			return "", false
		}
		initSig := info.entryIncoming[phi.Dest]
		if initSig == nil {
			return "", false
		}
		ref := e.resolveCountedLoopSignalValue(info, proc, initSig, nil, overrides, make(map[countedLoopResolveKey]string))
		if ref == "" || ref == "%unknown" {
			return "", false
		}
		overrides[phi.Dest] = ref
	}

	for i := 0; i < info.iterations; i++ {
		cache := make(map[countedLoopResolveKey]string)
		next := make(map[*ir.Signal]string, len(info.headerPhis))
		for _, phi := range info.headerPhis {
			if phi == nil || phi.Dest == nil {
				return "", false
			}
			backSig := info.backedgeIncoming[phi.Dest]
			if backSig == nil {
				return "", false
			}
			ref := e.resolveCountedLoopSignalValue(info, proc, backSig, nil, overrides, cache)
			if ref == "" || ref == "%unknown" {
				return "", false
			}
			next[phi.Dest] = ref
		}
		overrides = next
	}

	for _, op := range info.done.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
			continue
		}
		if assign.Dest.Name != target.Name {
			continue
		}
		ref := e.resolveCountedLoopSignalValue(info, proc, assign.Value, nil, overrides, make(map[countedLoopResolveKey]string))
		ref = e.normalizeResolvedSignalRef(assign.Value, ref)
		if ref == "" || ref == "%unknown" {
			return "", false
		}
		return ref, true
	}
	return "", false
}

func analyzeCountedLoop(proc *ir.Process) (*countedLoopInfo, bool) {
	if proc == nil || len(proc.Blocks) < 4 {
		return nil, false
	}
	entry := proc.Blocks[0]
	if entry == nil {
		return nil, false
	}
	entryJump, ok := entry.Terminator.(*ir.JumpTerminator)
	if !ok || entryJump == nil || entryJump.Target == nil {
		return nil, false
	}
	header := entryJump.Target
	headerBranch, ok := header.Terminator.(*ir.BranchTerminator)
	if !ok || headerBranch == nil || headerBranch.True == nil || headerBranch.False == nil || headerBranch.Cond == nil {
		return nil, false
	}
	done := headerBranch.False
	bodyEntry := headerBranch.True
	if done == nil || bodyEntry == nil {
		return nil, false
	}

	headerPhis := make([]*ir.PhiOperation, 0)
	entryIncoming := make(map[*ir.Signal]*ir.Signal)
	backedgeIncoming := make(map[*ir.Signal]*ir.Signal)
	blockIndex := make(map[*ir.BasicBlock]int, len(proc.Blocks))
	for i, block := range proc.Blocks {
		blockIndex[block] = i
	}
	var backedge *ir.BasicBlock
	for _, op := range header.Ops {
		phi, ok := op.(*ir.PhiOperation)
		if !ok || phi == nil || phi.Dest == nil {
			continue
		}
		headerPhis = append(headerPhis, phi)
		for _, incoming := range phi.Incomings {
			if incoming.Block == nil || incoming.Value == nil {
				return nil, false
			}
			if incoming.Block == entry {
				entryIncoming[phi.Dest] = incoming.Value
				continue
			}
			if blockIndex[incoming.Block] >= blockIndex[header] {
				if backedge == nil {
					backedge = incoming.Block
				} else if backedge != incoming.Block {
					return nil, false
				}
				backedgeIncoming[phi.Dest] = incoming.Value
			}
		}
	}
	if len(headerPhis) == 0 || len(entryIncoming) != len(headerPhis) || len(backedgeIncoming) != len(headerPhis) || backedge == nil {
		return nil, false
	}

	cmp := findCompareOp(header, headerBranch.Cond)
	if cmp == nil {
		return nil, false
	}
	if cmp.Predicate != ir.CompareSLT && cmp.Predicate != ir.CompareULT {
		return nil, false
	}
	if _, ok := entryIncoming[cmp.Left]; !ok {
		return nil, false
	}
	bound, ok := signalInt64Value(cmp.Right)
	if !ok || bound < 0 {
		return nil, false
	}
	start, ok := signalInt64Value(entryIncoming[cmp.Left])
	if !ok || start != 0 {
		return nil, false
	}
	if !isUnitIncrement(backedgeIncoming[cmp.Left], cmp.Left, proc) {
		return nil, false
	}

	return &countedLoopInfo{
		header:           header,
		bodyEntry:        bodyEntry,
		done:             done,
		entry:            entry,
		headerPhis:       headerPhis,
		entryIncoming:    entryIncoming,
		backedgeIncoming: backedgeIncoming,
		iterations:       int(bound),
	}, true
}

func findCompareOp(block *ir.BasicBlock, dest *ir.Signal) *ir.CompareOperation {
	if block == nil || dest == nil {
		return nil
	}
	for _, op := range block.Ops {
		cmp, ok := op.(*ir.CompareOperation)
		if !ok || cmp == nil || cmp.Dest != dest {
			continue
		}
		return cmp
	}
	return nil
}

func signalReadOnRHS(proc *ir.Process, target *ir.Signal) bool {
	if proc == nil || target == nil {
		return false
	}
	seen := make(map[*ir.Signal]bool)
	var usesSignal func(sig *ir.Signal) bool
	usesSignal = func(sig *ir.Signal) bool {
		if sig == nil {
			return false
		}
		if emitterSameSignal(sig, target) {
			return true
		}
		if seen[sig] {
			return false
		}
		seen[sig] = true
		producer, _ := findSignalProducer(proc, sig)
		switch op := producer.(type) {
		case *ir.AssignOperation:
			return usesSignal(op.Value)
		case *ir.NotOperation:
			return usesSignal(op.Value)
		case *ir.BinOperation:
			return usesSignal(op.Left) || usesSignal(op.Right)
		case *ir.CompareOperation:
			return usesSignal(op.Left) || usesSignal(op.Right)
		case *ir.MuxOperation:
			return usesSignal(op.Cond) || usesSignal(op.TrueValue) || usesSignal(op.FalseValue)
		case *ir.ConvertOperation:
			return usesSignal(op.Value)
		case *ir.PhiOperation:
			for _, incoming := range op.Incomings {
				if usesSignal(incoming.Value) {
					return true
				}
			}
		}
		return false
	}

	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, raw := range block.Ops {
			assign, ok := raw.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Dest.Name != target.Name || assign.Value == nil {
				continue
			}
			if usesSignal(assign.Value) {
				return true
			}
		}
	}
	return false
}

func signalInt64Value(sig *ir.Signal) (int64, bool) {
	if sig == nil || sig.Kind != ir.Const {
		return 0, false
	}
	switch v := sig.Value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	default:
		return 0, false
	}
}

func isUnitIncrement(sig *ir.Signal, induction *ir.Signal, proc *ir.Process) bool {
	if sig == nil || induction == nil || proc == nil {
		return false
	}
	producer, _ := findSignalProducer(proc, sig)
	bin, ok := producer.(*ir.BinOperation)
	if !ok || bin == nil || bin.Op != ir.Add {
		return false
	}
	if bin.Left == induction {
		step, ok := signalInt64Value(bin.Right)
		return ok && step == 1
	}
	if bin.Right == induction {
		step, ok := signalInt64Value(bin.Left)
		return ok && step == 1
	}
	return false
}

func (e *emitter) resolveCountedLoopSignalValue(info *countedLoopInfo, proc *ir.Process, sig *ir.Signal, pred *ir.BasicBlock, overrides map[*ir.Signal]string, cache map[countedLoopResolveKey]string) string {
	if e == nil || proc == nil || sig == nil {
		return ""
	}
	if ref, ok := overrides[sig]; ok && ref != "" {
		return ref
	}
	key := countedLoopResolveKey{sig: sig, pred: pred}
	if cache != nil {
		if cached, ok := cache[key]; ok {
			return cached
		}
		cache[key] = "%unknown"
	}
	if sig.Kind == ir.Const {
		ref := e.rootSignalRef(sig)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}
	if e.isReadableTopPortSignal(sig.Name) {
		ref := e.rootSignalRef(sig)
		if cache != nil {
			cache[key] = ref
		}
		return ref
	}

	producer, _ := findSignalProducer(proc, sig)
	if producer == nil {
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
		ref = e.resolveCountedLoopSignalValue(info, proc, op.Value, pred, overrides, cache)
	case *ir.PhiOperation:
		ref = e.resolveCountedLoopPhiValue(info, proc, op, pred, overrides, cache)
	case *ir.NotOperation:
		value := e.resolveCountedLoopSignalValue(info, proc, op.Value, pred, overrides, cache)
		if value != "" && value != "%unknown" {
			name := e.freshValueName("loop_not")
			ones := e.boolConst(true)
			if signalWidth(op.Value.Type) != 1 {
				ones = e.emitterAllOnesConst(op.Value.Type)
			}
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.xor %s, %s : %s\n", name, value, ones, typeString(op.Value.Type))
			ref = name
		}
	case *ir.BinOperation:
		left := e.resolveCountedLoopSignalValue(info, proc, op.Left, pred, overrides, cache)
		right := e.resolveCountedLoopSignalValue(info, proc, op.Right, pred, overrides, cache)
		if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			name := e.freshValueName("loop_bin")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.%s %s, %s : %s\n", name, binOpName(op.Op), left, right, typeString(op.Dest.Type))
			ref = name
		}
	case *ir.CompareOperation:
		left := e.resolveCountedLoopSignalValue(info, proc, op.Left, pred, overrides, cache)
		right := e.resolveCountedLoopSignalValue(info, proc, op.Right, pred, overrides, cache)
		if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			name := e.freshValueName("loop_cmp")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.icmp %s %s, %s : %s\n", name, comparePredicateName(op.Predicate), left, right, typeString(op.Left.Type))
			ref = name
		}
	case *ir.MuxOperation:
		cond := e.resolveCountedLoopSignalValue(info, proc, op.Cond, pred, overrides, cache)
		tval := e.resolveCountedLoopSignalValue(info, proc, op.TrueValue, pred, overrides, cache)
		fval := e.resolveCountedLoopSignalValue(info, proc, op.FalseValue, pred, overrides, cache)
		if cond != "" && tval != "" && fval != "" && cond != "%unknown" && tval != "%unknown" && fval != "%unknown" {
			name := e.freshValueName("loop_mux")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, cond, tval, fval, typeString(op.Dest.Type))
			ref = name
		}
	case *ir.ConvertOperation:
		value := e.resolveCountedLoopSignalValue(info, proc, op.Value, pred, overrides, cache)
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

func (e *emitter) resolveCountedLoopPhiValue(info *countedLoopInfo, proc *ir.Process, phi *ir.PhiOperation, pred *ir.BasicBlock, overrides map[*ir.Signal]string, cache map[countedLoopResolveKey]string) string {
	if e == nil || phi == nil || phi.Dest == nil {
		return ""
	}
	if pred != nil {
		for _, incoming := range phi.Incomings {
			if incoming.Block != pred || incoming.Value == nil {
				continue
			}
			return e.resolveCountedLoopSignalValue(info, proc, incoming.Value, nil, overrides, cache)
		}
	}
	if len(phi.Incomings) == 0 {
		return ""
	}

	currentIncoming := phi.Incomings[len(phi.Incomings)-1]
	current := e.resolveCountedLoopSignalValue(info, proc, currentIncoming.Value, nil, overrides, cache)
	if current == "" || current == "%unknown" {
		return current
	}
	for i := len(phi.Incomings) - 2; i >= 0; i-- {
		incoming := phi.Incomings[i]
		if incoming.Block == nil || incoming.Value == nil {
			continue
		}
		incomingVal := e.resolveCountedLoopSignalValue(info, proc, incoming.Value, nil, overrides, cache)
		if incomingVal == "" || incomingVal == "%unknown" {
			continue
		}
		termsCache := make(map[*ir.BasicBlock][]condTerm)
		terms := countedLoopPhiIncomingConditionTerms(info, incoming.Block, phiBlockForIncoming(phi, incoming.Block), termsCache, make(map[*ir.BasicBlock]bool))
		condRef := e.emitCountedLoopCondTermsRef(info, proc, terms, overrides, cache)
		if condRef == "" || condRef == "%unknown" {
			current = incomingVal
			continue
		}
		name := e.freshValueName("loop_phi")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, condRef, incomingVal, current, typeString(phi.Dest.Type))
		current = name
	}
	return current
}

func countedLoopPhiIncomingConditionTerms(info *countedLoopInfo, pred, target *ir.BasicBlock, cache map[*ir.BasicBlock][]condTerm, active map[*ir.BasicBlock]bool) []condTerm {
	if info == nil || pred == nil {
		return nil
	}
	terms := countedLoopBlockReachabilityTerms(info, pred, cache, active)
	if len(terms) == 0 {
		return nil
	}
	if pred == target || target == nil {
		return terms
	}
	switch term := pred.Terminator.(type) {
	case *ir.BranchTerminator:
		switch {
		case term.True == target:
			return appendLiteralToTerms(terms, condLiteral{sig: term.Cond, positive: true})
		case term.False == target:
			return appendLiteralToTerms(terms, condLiteral{sig: term.Cond, positive: false})
		default:
			return nil
		}
	case *ir.JumpTerminator:
		if term.Target == target {
			return terms
		}
	}
	return nil
}

func countedLoopBlockReachabilityTerms(info *countedLoopInfo, block *ir.BasicBlock, cache map[*ir.BasicBlock][]condTerm, active map[*ir.BasicBlock]bool) []condTerm {
	if info == nil || block == nil {
		return nil
	}
	if cache != nil {
		if cached, ok := cache[block]; ok {
			return cached
		}
	}
	if active[block] {
		return nil
	}
	if info.bodyEntry == block {
		terms := []condTerm{{}}
		if cache != nil {
			cache[block] = terms
		}
		return terms
	}
	active[block] = true
	defer delete(active, block)

	var terms []condTerm
	for _, pred := range block.Predecessors {
		terms = append(terms, countedLoopPhiIncomingConditionTerms(info, pred, block, cache, active)...)
	}
	terms = simplifyCondTerms(terms)
	if cache != nil {
		cache[block] = terms
	}
	return terms
}

func (e *emitter) emitCountedLoopCondTermsRef(info *countedLoopInfo, proc *ir.Process, terms []condTerm, overrides map[*ir.Signal]string, cache map[countedLoopResolveKey]string) string {
	if e == nil || len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 && len(terms[0]) == 0 {
		return e.boolConst(true)
	}
	termRefs := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) == 0 {
			return e.boolConst(true)
		}
		ref := ""
		for _, lit := range term {
			litRef := e.resolveCountedLoopSignalValue(info, proc, lit.sig, nil, overrides, cache)
			litRef = e.normalizeResolvedSignalRef(lit.sig, litRef)
			if litRef == "" || litRef == "%unknown" {
				return ""
			}
			if !lit.positive {
				one := e.boolConst(true)
				name := e.freshValueName("loop_phi_not")
				e.printIndent()
				fmt.Fprintf(e.w, "%s = comb.xor %s, %s : i1\n", name, litRef, one)
				litRef = name
			}
			if ref == "" {
				ref = litRef
				continue
			}
			name := e.freshValueName("loop_phi_and")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.and %s, %s : i1\n", name, ref, litRef)
			ref = name
		}
		if ref != "" {
			termRefs = append(termRefs, ref)
		}
	}
	if len(termRefs) == 0 {
		return ""
	}
	current := termRefs[0]
	for _, ref := range termRefs[1:] {
		name := e.freshValueName("loop_phi_or")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.or %s, %s : i1\n", name, current, ref)
		current = name
	}
	return current
}

func (e *emitter) resolveDualEdgeOutputValue(proc *ir.Process) (string, bool) {
	if e == nil || proc == nil {
		return "", false
	}
	var clockTerm *ir.BranchTerminator
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		term, ok := block.Terminator.(*ir.BranchTerminator)
		if !ok || term == nil || term.Cond == nil || !isClockLikeName(term.Cond.Name) {
			continue
		}
		trueClocked, falseClocked := emitterClockedBranchPolarity(proc, term)
		if trueClocked && falseClocked {
			clockTerm = term
			break
		}
	}
	if clockTerm == nil {
		return "", false
	}
	posSig, okPos := emitterFindUniqueStateAssign(clockTerm.True, make(map[*ir.BasicBlock]bool))
	negSig, okNeg := emitterFindUniqueStateAssign(clockTerm.False, make(map[*ir.BasicBlock]bool))
	if (!okPos || !okNeg || posSig == nil || negSig == nil || posSig.Name == negSig.Name) && proc != nil {
		posSig, negSig = emitterFindDualEdgeStatePair(proc)
		okPos = posSig != nil
		okNeg = negSig != nil
	}
	if !okPos || !okNeg || posSig == nil || negSig == nil || posSig.Name == negSig.Name {
		return "", false
	}
	posRef := e.rootSignalRef(posSig)
	negRef := e.rootSignalRef(negSig)
	if posRef == "" || negRef == "" || posRef == "%unknown" || negRef == "%unknown" {
		return "", false
	}
	name := e.freshValueName("dualedge_mux")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = comb.mux %%clk, %s, %s : %s\n", name, posRef, negRef, typeString(posSig.Type))
	return name, true
}

func emitterFindDualEdgeStatePair(proc *ir.Process) (*ir.Signal, *ir.Signal) {
	if proc == nil {
		return nil, nil
	}
	byName := make(map[string]*ir.Signal)
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || isOutputGlobalName(assign.Dest.Name) {
				continue
			}
			byName[assign.Dest.Name] = assign.Dest
		}
	}
	var posSig *ir.Signal
	var negSig *ir.Signal
	for name, sig := range byName {
		switch {
		case strings.HasSuffix(name, "_qp") || name == "qp":
			posSig = sig
		case strings.HasSuffix(name, "_qn") || name == "qn":
			negSig = sig
		}
	}
	return posSig, negSig
}

func emitterFindUniqueStateAssign(block *ir.BasicBlock, seen map[*ir.BasicBlock]bool) (*ir.Signal, bool) {
	if block == nil {
		return nil, false
	}
	if seen[block] {
		return nil, false
	}
	seen[block] = true
	defer delete(seen, block)
	var found *ir.Signal
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil || isOutputGlobalName(assign.Dest.Name) {
			continue
		}
		if found != nil && found.Name != assign.Dest.Name {
			return nil, false
		}
		found = assign.Dest
	}
	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		trueSig, trueOK := emitterFindUniqueStateAssign(term.True, seen)
		falseSig, falseOK := emitterFindUniqueStateAssign(term.False, seen)
		switch {
		case trueOK && falseOK:
			if trueSig == nil || falseSig == nil || trueSig.Name != falseSig.Name {
				return nil, false
			}
			if found != nil && found.Name != trueSig.Name {
				return nil, false
			}
			return trueSig, true
		case trueOK:
			if found != nil && trueSig != nil && found.Name != trueSig.Name {
				return nil, false
			}
			if trueSig != nil {
				return trueSig, true
			}
		case falseOK:
			if found != nil && falseSig != nil && found.Name != falseSig.Name {
				return nil, false
			}
			if falseSig != nil {
				return falseSig, true
			}
		}
	case *ir.JumpTerminator:
		if sig, ok := emitterFindUniqueStateAssign(term.Target, seen); ok {
			if found != nil && sig != nil && found.Name != sig.Name {
				return nil, false
			}
			if sig != nil {
				return sig, true
			}
		}
	}
	return found, found != nil
}

func (e *emitter) resolveAlwaysAssignedOutputValue(proc *ir.Process, sig *ir.Signal, clocked map[*ir.BasicBlock]bool) (string, bool) {
	if e == nil || proc == nil || sig == nil || !signalAssignedOnAllPaths(proc, sig, proc.Blocks[0], make(map[*ir.BasicBlock]bool)) {
		return "", false
	}
	if countSignalAssignments(proc, sig) != 1 {
		return "", false
	}
	producer, block := findSignalProducer(proc, sig)
	assign, ok := producer.(*ir.AssignOperation)
	if !ok || assign == nil || assign.Value == nil || block == nil {
		return "", false
	}
	cache := make(map[combResolveKey]string)
	var ref string
	if clocked == nil {
		ref = e.resolveCombinationalSignalValue(proc, assign.Value, block, nil, cache)
	} else {
		ref = e.resolveDirectClockedSignalValue(proc, assign.Value, block, nil, clocked, cache)
	}
	ref = e.normalizeResolvedSignalRef(assign.Value, ref)
	return ref, ref != "" && ref != "%unknown"
}

func countSignalAssignments(proc *ir.Process, sig *ir.Signal) int {
	if proc == nil || sig == nil {
		return 0
	}
	count := 0
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil {
				continue
			}
			if emitterSameSignal(assign.Dest, sig) {
				count++
			}
		}
	}
	return count
}

func signalAssignedOnAllPaths(proc *ir.Process, sig *ir.Signal, block *ir.BasicBlock, seen map[*ir.BasicBlock]bool) bool {
	if proc == nil || sig == nil || block == nil {
		return false
	}
	if seen[block] {
		return true
	}
	seen[block] = true
	defer delete(seen, block)
	if blockAssignsSignal(block, sig) {
		return true
	}
	switch term := block.Terminator.(type) {
	case *ir.ReturnTerminator, nil:
		return false
	case *ir.JumpTerminator:
		return signalAssignedOnAllPaths(proc, sig, term.Target, seen)
	case *ir.BranchTerminator:
		return signalAssignedOnAllPaths(proc, sig, term.True, seen) && signalAssignedOnAllPaths(proc, sig, term.False, seen)
	default:
		return false
	}
}

func blockAssignsSignal(block *ir.BasicBlock, sig *ir.Signal) bool {
	if block == nil || sig == nil {
		return false
	}
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil {
			continue
		}
		if emitterSameSignal(assign.Dest, sig) {
			return true
		}
	}
	return false
}

func emitterSameSignal(a, b *ir.Signal) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a == b {
		return true
	}
	return a.Name != "" && a.Name == b.Name
}

type directOutputAssign struct {
	block *ir.BasicBlock
	value *ir.Signal
}

type emitterPathConditionStep struct {
	cond     *ir.Signal
	takeTrue bool
}

func (e *emitter) resolveDirectClockedAssignedOutput(proc *ir.Process, target *ir.Signal, clocked map[*ir.BasicBlock]bool) (string, bool) {
	if e == nil || proc == nil || target == nil || len(proc.Blocks) == 0 {
		return "", false
	}
	assigns := make([]directOutputAssign, 0)
	includeClocked := isOutputGlobalName(target.Name)
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		if !includeClocked && clocked[block] {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name != target.Name {
				continue
			}
			assigns = append(assigns, directOutputAssign{block: block, value: assign.Value})
		}
	}
	if len(assigns) == 0 {
		return "", false
	}
	cache := make(map[combResolveKey]string)
	current := ""
	for i := len(assigns) - 1; i >= 0; i-- {
		ref := e.resolveDirectClockedSignalValue(proc, assigns[i].value, assigns[i].block, nil, clocked, cache)
		ref = e.normalizeResolvedSignalRef(assigns[i].value, ref)
		if ref == "" || ref == "%unknown" {
			continue
		}
		if current == "" {
			current = ref
			continue
		}
		termsCache := make(map[*ir.BasicBlock][]condTerm)
		terms := emitterBlockReachabilityTerms(proc, assigns[i].block, emitterProcessHasDualClockEdges(proc), termsCache, make(map[*ir.BasicBlock]bool))
		condRef := e.emitEmitterCondTermsRef(terms)
		if condRef == "" || condRef == "%unknown" {
			continue
		}
		name := e.freshValueName("out_mux")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, condRef, ref, current, typeString(target.Type))
		current = name
	}
	if current == "" {
		return "", false
	}
	return current, true
}

func (e *emitter) resolveOrderedDirectClockedOutput(proc *ir.Process, target *ir.Signal, clocked map[*ir.BasicBlock]bool) (string, bool) {
	if e == nil || proc == nil || target == nil {
		return "", false
	}
	assigns := make([]directOutputAssign, 0)
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil || assign.Value == nil {
				continue
			}
			if assign.Dest.Name != target.Name {
				continue
			}
			assigns = append(assigns, directOutputAssign{block: block, value: assign.Value})
		}
	}
	if len(assigns) == 0 {
		return "", false
	}

	cache := make(map[combResolveKey]string)
	current := ""
	for idx, assign := range assigns {
		ref := e.resolveDirectClockedSignalValue(proc, assign.value, assign.block, nil, clocked, cache)
		ref = e.normalizeResolvedSignalRef(assign.value, ref)
		if ref == "" || ref == "%unknown" {
			continue
		}
		if idx == 0 || current == "" {
			current = ref
			continue
		}
		termsCache := make(map[*ir.BasicBlock][]condTerm)
		terms := emitterBlockReachabilityTerms(proc, assign.block, emitterProcessHasDualClockEdges(proc), termsCache, make(map[*ir.BasicBlock]bool))
		condRef := e.emitEmitterCondTermsRef(terms)
		if condRef == "" || condRef == "%unknown" {
			current = ref
			continue
		}
		name := e.freshValueName("out_mux")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, condRef, ref, current, typeString(target.Type))
		current = name
	}
	if current == "" {
		return "", false
	}
	return current, true
}

func emitterBlockReachabilityTerms(proc *ir.Process, block *ir.BasicBlock, includeClockConds bool, cache map[*ir.BasicBlock][]condTerm, active map[*ir.BasicBlock]bool) []condTerm {
	if proc == nil || block == nil {
		return nil
	}
	if cache != nil {
		if cached, ok := cache[block]; ok {
			return cached
		}
	}
	if active[block] {
		return nil
	}
	if len(proc.Blocks) > 0 && proc.Blocks[0] == block {
		terms := []condTerm{{}}
		if cache != nil {
			cache[block] = terms
		}
		return terms
	}
	active[block] = true
	defer delete(active, block)

	var terms []condTerm
	for _, pred := range block.Predecessors {
		terms = append(terms, emitterEdgeConditionTerms(proc, pred, block, includeClockConds, cache, active)...)
	}
	terms = simplifyCondTerms(terms)
	if cache != nil {
		cache[block] = terms
	}
	return terms
}

func emitterEdgeConditionTerms(proc *ir.Process, pred, target *ir.BasicBlock, includeClockConds bool, cache map[*ir.BasicBlock][]condTerm, active map[*ir.BasicBlock]bool) []condTerm {
	if proc == nil || pred == nil {
		return nil
	}
	terms := emitterBlockReachabilityTerms(proc, pred, includeClockConds, cache, active)
	if len(terms) == 0 {
		return nil
	}
	if pred == target || target == nil {
		return terms
	}
	switch term := pred.Terminator.(type) {
	case *ir.BranchTerminator:
		switch {
		case term.True == target:
			if term.Cond != nil && (!isClockLikeName(term.Cond.Name) || includeClockConds) && !isResetPortName(term.Cond.Name) {
				return appendLiteralToTerms(terms, condLiteral{sig: term.Cond, positive: true})
			}
			return terms
		case term.False == target:
			if term.Cond != nil && (!isClockLikeName(term.Cond.Name) || includeClockConds) && !isResetPortName(term.Cond.Name) {
				return appendLiteralToTerms(terms, condLiteral{sig: term.Cond, positive: false})
			}
			return terms
		default:
			return nil
		}
	case *ir.JumpTerminator:
		if term.Target == target {
			return terms
		}
	}
	return nil
}

func (e *emitter) findEmitterPathConditions(proc *ir.Process, target *ir.BasicBlock, includeClockConds bool) ([]emitterPathConditionStep, bool) {
	if e == nil || proc == nil || len(proc.Blocks) == 0 || target == nil {
		return nil, false
	}
	visited := make(map[*ir.BasicBlock]bool)
	return e.findEmitterPathConditionsFrom(proc.Blocks[0], target, visited, includeClockConds)
}

func (e *emitter) findEmitterPathConditionsFrom(current, target *ir.BasicBlock, visited map[*ir.BasicBlock]bool, includeClockConds bool) ([]emitterPathConditionStep, bool) {
	if current == nil || target == nil {
		return nil, false
	}
	if current == target {
		return []emitterPathConditionStep{}, true
	}
	if visited[current] {
		return nil, false
	}
	visited[current] = true
	defer delete(visited, current)
	switch term := current.Terminator.(type) {
	case *ir.BranchTerminator:
		if steps, ok := e.findEmitterPathConditionsFrom(term.True, target, visited, includeClockConds); ok {
			prefix := []emitterPathConditionStep{}
			if term.Cond != nil && (!isClockLikeName(term.Cond.Name) || includeClockConds) && !isResetPortName(term.Cond.Name) {
				prefix = append(prefix, emitterPathConditionStep{cond: term.Cond, takeTrue: true})
			}
			return append(prefix, steps...), true
		}
		if steps, ok := e.findEmitterPathConditionsFrom(term.False, target, visited, includeClockConds); ok {
			prefix := []emitterPathConditionStep{}
			if term.Cond != nil && (!isClockLikeName(term.Cond.Name) || includeClockConds) && !isResetPortName(term.Cond.Name) {
				prefix = append(prefix, emitterPathConditionStep{cond: term.Cond, takeTrue: false})
			}
			return append(prefix, steps...), true
		}
	case *ir.JumpTerminator:
		return e.findEmitterPathConditionsFrom(term.Target, target, visited, includeClockConds)
	}
	return nil, false
}

func emitterProcessHasDualClockEdges(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		term, ok := block.Terminator.(*ir.BranchTerminator)
		if !ok || term == nil || term.Cond == nil || !isClockLikeName(term.Cond.Name) {
			continue
		}
		trueClocked, falseClocked := emitterClockedBranchPolarity(proc, term)
		if trueClocked && falseClocked {
			return true
		}
	}
	return false
}

func (e *emitter) emitEmitterPathConditionRef(steps []emitterPathConditionStep) string {
	if e == nil {
		return ""
	}
	if len(steps) == 0 {
		return e.boolConst(true)
	}
	current := ""
	for _, step := range steps {
		if step.cond == nil {
			return ""
		}
		lit := e.rootSignalRef(step.cond)
		lit = e.normalizeResolvedSignalRef(step.cond, lit)
		if lit == "" || lit == "%unknown" {
			return ""
		}
		if !step.takeTrue {
			one := e.boolConst(true)
			name := e.freshValueName("path_not")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.xor %s, %s : i1\n", name, lit, one)
			lit = name
		}
		if current == "" {
			current = lit
			continue
		}
		name := e.freshValueName("path_and")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.and %s, %s : i1\n", name, current, lit)
		current = name
	}
	return current
}

func (e *emitter) emitEmitterCondTermsRef(terms []condTerm) string {
	if e == nil || len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 && len(terms[0]) == 0 {
		return e.boolConst(true)
	}
	termRefs := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) == 0 {
			return e.boolConst(true)
		}
		ref := ""
		for _, lit := range term {
			litRef := e.rootSignalRef(lit.sig)
			litRef = e.normalizeResolvedSignalRef(lit.sig, litRef)
			if litRef == "" || litRef == "%unknown" {
				return ""
			}
			if !lit.positive {
				one := e.boolConst(true)
				name := e.freshValueName("path_not")
				e.printIndent()
				fmt.Fprintf(e.w, "%s = comb.xor %s, %s : i1\n", name, litRef, one)
				litRef = name
			}
			if ref == "" {
				ref = litRef
				continue
			}
			name := e.freshValueName("path_and")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.and %s, %s : i1\n", name, ref, litRef)
			ref = name
		}
		if ref != "" {
			termRefs = append(termRefs, ref)
		}
	}
	if len(termRefs) == 0 {
		return ""
	}
	current := termRefs[0]
	for _, ref := range termRefs[1:] {
		name := e.freshValueName("path_or")
		e.printIndent()
		fmt.Fprintf(e.w, "%s = comb.or %s, %s : i1\n", name, current, ref)
		current = name
	}
	return current
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
		switch typed := op.(type) {
		case *ir.AssignOperation:
			if typed == nil || typed.Dest == nil || typed.Value == nil {
				continue
			}
			if typed.Dest.Name != target.Name {
				continue
			}
			ref := e.resolveCombinationalSignalValue(proc, typed.Value, block, pred, cache)
			ref = e.normalizeResolvedSignalRef(typed.Value, ref)
			if ref == "" || ref == "%unknown" {
				continue
			}
			value = ref
		case *ir.MuxOperation:
			if typed == nil || typed.Dest == nil || value == "" || value == "%unknown" {
				continue
			}
			trueRef := e.resolveCombinationalSignalValue(proc, typed.TrueValue, block, pred, cache)
			falseRef := e.resolveCombinationalSignalValue(proc, typed.FalseValue, block, pred, cache)
			trueRef = e.normalizeResolvedSignalRef(typed.TrueValue, trueRef)
			falseRef = e.normalizeResolvedSignalRef(typed.FalseValue, falseRef)
			if trueRef != value && falseRef != value {
				continue
			}
			destRef := e.rootSignalRef(typed.Dest)
			destRef = e.normalizeResolvedSignalRef(typed.Dest, destRef)
			if destRef == "" || destRef == "%unknown" {
				continue
			}
			value = destRef
		}
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
		if e.isReadableTopPortSignal(sig.Name) {
			ref := e.rootSignalRef(sig)
			ref = e.normalizeResolvedSignalRef(sig, ref)
			if cache != nil {
				cache[key] = ref
			}
			return ref
		}
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
	if clocked != nil && clocked[producerBlock] && sig.Kind == ir.Reg && !isOutputGlobalName(sig.Name) {
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
			break
		}
		current := ""
		termsCache := make(map[*ir.BasicBlock][]condTerm)
		for i := len(op.Incomings) - 1; i >= 0; i-- {
			incoming := op.Incomings[i]
			if incoming.Value == nil {
				continue
			}
			incomingRef := e.resolveDirectClockedSignalValue(proc, incoming.Value, incoming.Block, nil, clocked, cache)
			incomingRef = e.normalizeResolvedSignalRef(incoming.Value, incomingRef)
			if incomingRef == "" || incomingRef == "%unknown" {
				continue
			}
			if current == "" || incoming.Block == nil {
				current = incomingRef
				continue
			}
			terms := emitterEdgeConditionTerms(proc, incoming.Block, producerBlock, emitterProcessHasDualClockEdges(proc), termsCache, make(map[*ir.BasicBlock]bool))
			condRef := e.emitEmitterCondTermsRef(terms)
			if condRef == "" || condRef == "%unknown" {
				current = incomingRef
				continue
			}
			name := e.freshValueName("comb_phi")
			e.printIndent()
			fmt.Fprintf(e.w, "%s = comb.mux %s, %s, %s : %s\n", name, condRef, incomingRef, current, typeString(op.Dest.Type))
			current = name
		}
		ref = current
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
		if pred == nil && op.Cond != nil && isClockLikeName(op.Cond.Name) {
			ref = e.resolveDirectClockedSignalValue(proc, op.FalseValue, producerBlock, pred, clocked, cache)
			break
		}
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
		if term.Cond != nil && isClockLikeName(term.Cond.Name) {
			trueClocked := false
			falseClocked := false
			if clocked != nil {
				trueClocked = clocked[term.True]
				falseClocked = clocked[term.False]
			}
			switch {
			case trueClocked && !falseClocked:
				return e.resolveDirectClockedOutputAtBlock(proc, term.False, block, target, value, clocked, cache, visiting)
			case falseClocked && !trueClocked:
				return e.resolveDirectClockedOutputAtBlock(proc, term.True, block, target, value, clocked, cache, visiting)
			}
		}
		trueVal, trueOK := e.resolveDirectClockedOutputAtBlock(proc, term.True, block, target, value, clocked, cache, visiting)
		falseVal, falseOK := e.resolveDirectClockedOutputAtBlock(proc, term.False, block, target, value, clocked, cache, visiting)
		if term.Cond != nil && isResetPortName(term.Cond.Name) {
			switch {
			case trueOK && falseOK && trueVal == falseVal:
				return trueVal, trueVal != ""
			case trueOK:
				return trueVal, trueVal != ""
			case falseOK:
				return falseVal, falseVal != ""
			default:
				return value, value != ""
			}
		}
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
	if processHasImplicitClockedOutputState(proc) {
		for _, block := range proc.Blocks {
			if block != nil {
				blocks[block] = true
			}
		}
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
				trueClocked, falseClocked := emitterClockedBranchPolarity(proc, term)
				visit(term.True, trueClocked)
				visit(term.False, falseClocked)
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

func processHasImplicitClockedOutputState(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	hasClock := false
	outputParams := make(map[string]struct{})
	for _, param := range proc.Params {
		if param == nil {
			continue
		}
		if isClockLikeName(param.Name) {
			hasClock = true
			continue
		}
		outputParams[param.Name] = struct{}{}
	}
	if !hasClock || len(outputParams) == 0 {
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
			if !isOutputGlobalName(assign.Dest.Name) {
				continue
			}
			if _, ok := outputParams[strings.TrimPrefix(assign.Dest.Name, "out_")]; ok {
				return true
			}
		}
	}
	return false
}

func emitterClockedBranchPolarity(proc *ir.Process, term *ir.BranchTerminator) (bool, bool) {
	if proc == nil || term == nil {
		return false, false
	}
	includeOutputs := !processHasInternalStateAssignmentsStandalone(proc)
	trueClocked := emitterBlockPathHasPersistentAssign(term.True, includeOutputs, make(map[*ir.BasicBlock]bool))
	falseClocked := emitterBlockPathHasPersistentAssign(term.False, includeOutputs, make(map[*ir.BasicBlock]bool))
	switch {
	case trueClocked && falseClocked:
		return true, true
	case trueClocked:
		return true, false
	case falseClocked:
		return false, true
	default:
		return true, false
	}
}

func emitterBlockPathHasPersistentAssign(block *ir.BasicBlock, includeOutputs bool, seen map[*ir.BasicBlock]bool) bool {
	if block == nil {
		return false
	}
	if seen[block] {
		return false
	}
	seen[block] = true
	defer delete(seen, block)
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil {
			continue
		}
		if strings.HasPrefix(assign.Dest.Name, "__mygo_state_") || (includeOutputs && isOutputGlobalName(assign.Dest.Name)) {
			return true
		}
	}
	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		return emitterBlockPathHasPersistentAssign(term.True, includeOutputs, seen) || emitterBlockPathHasPersistentAssign(term.False, includeOutputs, seen)
	case *ir.JumpTerminator:
		return emitterBlockPathHasPersistentAssign(term.Target, includeOutputs, seen)
	default:
		return false
	}
}

func processHasInternalStateAssignmentsStandalone(proc *ir.Process) bool {
	if proc == nil {
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
			if strings.HasPrefix(assign.Dest.Name, "__mygo_state_") {
				return true
			}
		}
	}
	return false
}

func (e *emitter) normalizeResolvedSignalRef(sig *ir.Signal, ref string) string {
	if e == nil || sig == nil || ref == "" || ref == "%unknown" {
		return ref
	}
	if constRef := e.immutableRegConstRef(sig); constRef != "" {
		return constRef
	}
	if sig.Kind != ir.Reg || e.isReadableTopPortSignal(sig.Name) {
		return ref
	}
	raw := "%" + sanitize(sig.Name)
	if ref != raw {
		return ref
	}
	return e.rootRegRef(sig, "norm_reg")
}

func (e *emitter) rootSignalRef(sig *ir.Signal) string {
	if e == nil || sig == nil {
		return ""
	}
	if unpacked := e.inputArrayElementRef(sig); unpacked != "" {
		return unpacked
	}
	if sig.Kind != ir.Reg && e.rootValueNames != nil {
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
	if e.isReadableTopPortSignal(sig.Name) {
		return "%" + sanitize(sig.Name)
	}
	if sig.Kind == ir.Reg {
		return e.rootRegRef(sig, "root_reg")
	}
	return "%" + sanitize(sig.Name)
}

func (e *emitter) immutableRegConstRef(sig *ir.Signal) string {
	if e == nil || sig == nil || sig.Kind != ir.Reg || sig.Name == "" || sig.Type == nil || isOutputGlobalName(sig.Name) {
		return ""
	}
	if e.isReadableTopPortSignal(sig.Name) {
		return ""
	}
	if e.currentAssigned != nil {
		if _, ok := e.currentAssigned[sig.Name]; ok {
			return ""
		}
	}
	if sig.Value == nil {
		return ""
	}
	if e.immutableConsts != nil {
		if ref, ok := e.immutableConsts[sig]; ok && ref != "" {
			return ref
		}
	}
	ref := e.freshValueName("root_init_const")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = hw.constant %s : %s\n", ref, formatHWConstant(sig.Value, sig.Type), typeString(sig.Type))
	if e.immutableConsts == nil {
		e.immutableConsts = make(map[*ir.Signal]string)
	}
	e.immutableConsts[sig] = ref
	return ref
}

func (e *emitter) rootRegRef(sig *ir.Signal, prefix string) string {
	if e == nil || sig == nil || sig.Name == "" || sig.Type == nil {
		return ""
	}
	if constRef := e.immutableRegConstRef(sig); constRef != "" {
		return constRef
	}
	if prefix == "" {
		prefix = "root_reg"
	}
	ref := e.freshValueName(prefix)
	e.printIndent()
	fmt.Fprintf(e.w, "%s = sv.read_inout %%%s : !hw.inout<%s>\n", ref, sanitize(sig.Name), typeString(sig.Type))
	return ref
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

func (e *emitter) isReadableTopPortSignal(name string) bool {
	if e == nil || name == "" || e.topPortTypes == nil {
		return false
	}
	if _, ok := e.topPortTypes[name]; ok {
		return true
	}
	base, _, ok := indexedSignalName(name)
	if !ok {
		return false
	}
	_, ok = e.topPortTypes[base]
	return ok
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
		dest := e.freshValueName("comb_conv")
		e.printIndent()
		if srcType != nil && srcType.Signed {
			fmt.Fprintf(e.w, "%s = arith.extsi %s : %s to %s\n", dest, value, from, to)
			return dest
		}
		fmt.Fprintf(e.w, "%s = arith.extui %s : %s to %s\n", dest, value, from, to)
		return dest
	}
	dest := e.freshValueName("comb_conv")
	e.printIndent()
	fmt.Fprintf(e.w, "%s = arith.trunci %s : %s to %s\n", dest, value, from, to)
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
	if sig.Kind == ir.Reg {
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

func canUseRootValueShortcut(root *processInfo) bool {
	if root == nil || root.proc == nil {
		return false
	}
	return processHasLoop(root.proc) || len(root.proc.Blocks) <= 1
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

	// Build a set of non-output port names for quick lookup.
	// Top-level output ports are lowered via hw.output and still need backing
	// internal signals when they correspond to out_* globals.
	portNames := make(map[string]bool)
	for _, port := range topPorts {
		if port.Direction == ir.Output {
			continue
		}
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
		if e.shouldSkipImmutableRegDeclaration(module, name, sig) {
			continue
		}

		// Check if this is a materialized array element backed by a base signal.
		isArrayElement := false
		for i := 0; i < len(name)-1; i++ {
			if name[i] == '_' && name[i+1] >= '0' && name[i+1] <= '9' {
				isArrayElement = portNames[name[:i]]
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
		} else if sig.Kind == ir.Reg || (useInoutRegs && e.moduleSignalNeedsStorage(module, sig)) {
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

func (e *emitter) moduleSignalNeedsStorage(module *ir.Module, sig *ir.Signal) bool {
	if e == nil || module == nil || sig == nil || sig.Name == "" {
		return false
	}
	if sig.Kind == ir.Reg {
		return true
	}
	if sig.Kind != ir.Wire {
		return false
	}
	assigned := e.moduleAssignedSignalNames(module)
	if _, ok := assigned[sig.Name]; !ok {
		return false
	}
	for _, proc := range module.Processes {
		if proc == nil {
			continue
		}
		if signalReadOnRHS(proc, sig) {
			return true
		}
	}
	return false
}

func (e *emitter) moduleAssignedSignalNames(module *ir.Module) map[string]struct{} {
	if e == nil || module == nil {
		return nil
	}
	if cached, ok := e.assignedSignals[module]; ok {
		return cached
	}
	assigned := make(map[string]struct{})
	for _, proc := range module.Processes {
		if proc == nil {
			continue
		}
		for _, block := range proc.Blocks {
			if block == nil {
				continue
			}
			for _, op := range block.Ops {
				assign, ok := op.(*ir.AssignOperation)
				if !ok || assign == nil || assign.Dest == nil || assign.Dest.Name == "" {
					continue
				}
				assigned[assign.Dest.Name] = struct{}{}
			}
		}
	}
	e.assignedSignals[module] = assigned
	return assigned
}

func (e *emitter) shouldSkipImmutableRegDeclaration(module *ir.Module, name string, sig *ir.Signal) bool {
	if e == nil || module == nil || sig == nil || sig.Kind != ir.Reg || sig.Value == nil || name == "" || isOutputGlobalName(name) {
		return false
	}
	assigned := e.moduleAssignedSignalNames(module)
	if _, ok := assigned[name]; ok {
		return false
	}
	return true
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
				if sig.Name == "varargs" {
					continue
				}
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

	// Only input ports are readable SSA operands inside the root process.
	// Top-level output ports are materialized by hw.output, not as value-carrying
	// block arguments, so treating them as readable ports causes mixed i1/inout
	// uses for out_* globals.
	portNames := make(map[string]string)
	for _, port := range topPorts {
		if port.Direction == ir.Output {
			continue
		}
		name := strings.TrimPrefix(port.Name, "%")
		portNames[name] = "%" + name
	}
	if moduleUsesFSM(module) {
		if _, ok := portNames["clk"]; !ok {
			portNames["clk"] = "%clk"
		}
		if moduleNeedsSyntheticReset(module) {
			portNames["rst"] = "%rst"
		}
	}
	portTypes := collectPortTypesFromIRPorts(topPorts)
	if moduleUsesFSM(module) {
		if _, ok := portTypes["clk"]; !ok {
			portTypes["clk"] = &ir.SignalType{Width: 1}
		}
		if moduleNeedsSyntheticReset(module) {
			portTypes["rst"] = &ir.SignalType{Width: 1}
		}
	}
	pp := &processPrinter{
		w:             e.w,
		indent:        e.indent,
		moduleSignals: module.Signals,
		usedSignals:   info.usedSignals,
		channelPorts:  channelPortsFromWires(info, wires),
		portNames:     portNames,
		portTypes:     portTypes,
		moduleName:    sanitize(module.Name),
		modulePorts:   e.modulePorts,
		emitter:       e,
		proc:          info.proc,
	}
	pp.resetState()
	pp.emitProcess(info.proc)

	// Store the valueNames for output resolution
	rootValues := make(map[*ir.Signal]string, len(pp.valueNames)+len(pp.persistentValues))
	for sig, value := range pp.valueNames {
		rootValues[sig] = value
	}
	for sig, value := range pp.persistentValues {
		rootValues[sig] = value
	}
	e.rootValueNames = rootValues
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
				if sig.Name == "varargs" {
					continue
				}
				portName := "%" + sanitize(sig.Name)
				// Skip if this port name is already in the ports list
				if existingPorts[portName] {
					continue
				}
				// Check if this signal is a module-level signal
				if _, isModuleSignal := module.Signals[sig.Name]; isModuleSignal {
					ports = append(ports, portDesc{name: portName, typ: typeString(sig.Type), inout: true})
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
	for _, port := range module.Ports {
		ports = append(ports, port)
	}
	return ports
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
	clk := f.printer.clockPortRef()
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
	// Hoist shared bool constants outside the state arms so later case blocks do
	// not reference SSA values defined in earlier case regions.
	f.printer.boolConst(false)
	f.printer.boolConst(true)
	f.printer.printIndent()
	fmt.Fprintf(f.printer.w, "sv.case %s : %s\n", f.stateValue, f.stateType)
	for _, state := range f.stateOrder {
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "case %s: {\n", f.literalForID(state.id))
		f.printer.indent++
		f.printer.clearTransientStateCaseValues()
		f.printer.beginBlockValueScope()
		f.emitStateCase(state)
		f.printer.endBlockValueScope()
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
	if !ok || moduleSig == nil || !f.printer.moduleSignalNeedsStorage(op.Dest.Name) {
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
	f.printer.emitIndexedAggregateAssignMirrors(op.Dest, value, false)
	f.printer.recordBlockValueOverride(op.Dest)
	f.printer.valueNames[op.Dest] = value
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
		val := f.printer.edgeValueRefForPred(update.value, pred)
		if val == "" || val == "%unknown" {
			continue
		}
		f.printer.printIndent()
		fmt.Fprintf(f.printer.w, "sv.passign %s, %s : %s\n", info.regName, val, info.typeStr)
	}
}

type processPrinter struct {
	w                    io.Writer
	indent               int
	nextTemp             int
	constNames           map[*ir.Signal]string
	literalConstNames    map[string]string
	valueNames           map[*ir.Signal]string
	portNames            map[string]string
	portTypes            map[string]*ir.SignalType
	channelPorts         map[*ir.Channel]*channelPortSet
	moduleSignals        map[string]*ir.Signal
	usedSignals          map[*ir.Signal]struct{}
	boolConsts           map[bool]string
	zeroConsts           map[string]string
	onesConsts           map[string]string
	stdoutFD             string
	fsm                  *fsmBuilder
	seqClockName         string
	internalSignalReads  map[string]string     // Maps signal name to the read_inout result
	moduleName           string                // Name of the parent module
	modulePorts          map[string][]portDesc // Module port information for instances
	emitter              *emitter              // Reference to parent emitter for global state
	emittedRegisters     map[string]bool       // Track which registers have already been emitted
	proc                 *ir.Process           // The process being emitted
	hasResetPort         bool
	resetPortRef         string
	resetActiveLow       bool
	resetAssertedName    string
	directClocked        bool
	clockedBlocks        map[*ir.BasicBlock]bool
	currentPhiPred       *ir.BasicBlock
	blockValueRestores   []map[*ir.Signal]valueRestore
	blockReadRestores    []map[string]valueRestore
	blockZeroRestores    []map[string]valueRestore
	blockOnesRestores    []map[string]valueRestore
	blockConvertRestores []map[convertCacheKey]valueRestore
	blockExprRestores    []map[exprCacheKey]valueRestore
	persistentValues     map[*ir.Signal]string
	indexedAggregates    map[string][]indexedAggregateElement
	producerCache        map[*ir.Signal]cachedProducer
	producerCacheSeeded  bool
	blockCondTermsCache  map[*ir.BasicBlock][]condTerm
	phiCondTermsCache    map[phiCondCacheKey][]condTerm
	convertCache         map[convertCacheKey]string
	exprCache            map[exprCacheKey]string
	persistPureOpCaches  bool
}

type valueRestore struct {
	existed bool
	value   string
}

type cachedProducer struct {
	op    ir.Operation
	block *ir.BasicBlock
}

type phiCondCacheKey struct {
	pred   *ir.BasicBlock
	target *ir.BasicBlock
}

type convertCacheKey struct {
	value string
	from  string
	to    string
}

type exprCacheKey struct {
	kind string
	a    string
	b    string
	c    string
	typ  string
}

type clockEdgeMode int

const (
	clockEdgePos clockEdgeMode = iota
	clockEdgeNeg
)

func (p *processPrinter) resetState() {
	p.nextTemp = 0
	p.constNames = make(map[*ir.Signal]string)
	p.literalConstNames = make(map[string]string)
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
	if p.zeroConsts == nil {
		p.zeroConsts = make(map[string]string)
	} else {
		for key := range p.zeroConsts {
			delete(p.zeroConsts, key)
		}
	}
	if p.onesConsts == nil {
		p.onesConsts = make(map[string]string)
	} else {
		for key := range p.onesConsts {
			delete(p.onesConsts, key)
		}
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
	p.currentPhiPred = nil
	p.blockValueRestores = nil
	p.blockReadRestores = nil
	p.blockZeroRestores = nil
	p.blockOnesRestores = nil
	p.blockConvertRestores = nil
	p.blockExprRestores = nil
	if p.indexedAggregates == nil {
		p.indexedAggregates = make(map[string][]indexedAggregateElement)
	} else {
		for name := range p.indexedAggregates {
			delete(p.indexedAggregates, name)
		}
	}
	if p.producerCache == nil {
		p.producerCache = make(map[*ir.Signal]cachedProducer)
	} else {
		for sig := range p.producerCache {
			delete(p.producerCache, sig)
		}
	}
	p.producerCacheSeeded = false
	if p.blockCondTermsCache == nil {
		p.blockCondTermsCache = make(map[*ir.BasicBlock][]condTerm)
	} else {
		for block := range p.blockCondTermsCache {
			delete(p.blockCondTermsCache, block)
		}
	}
	if p.phiCondTermsCache == nil {
		p.phiCondTermsCache = make(map[phiCondCacheKey][]condTerm)
	} else {
		for key := range p.phiCondTermsCache {
			delete(p.phiCondTermsCache, key)
		}
	}
	if p.persistentValues == nil {
		p.persistentValues = make(map[*ir.Signal]string)
	} else {
		for sig := range p.persistentValues {
			delete(p.persistentValues, sig)
		}
	}
	if p.convertCache == nil {
		p.convertCache = make(map[convertCacheKey]string)
	} else {
		for key := range p.convertCache {
			delete(p.convertCache, key)
		}
	}
	if p.exprCache == nil {
		p.exprCache = make(map[exprCacheKey]string)
	} else {
		for key := range p.exprCache {
			delete(p.exprCache, key)
		}
	}
	p.persistPureOpCaches = false
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
	p.seedProducerCache()
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
		p.emitPrecomputedPureOps(proc)
		if hasPrintOps {
			p.stdoutConstant()
		}
		p.clearPureOpCaches()
		p.fsm.emitChannelPortLogic()
		p.fsm.emitControlLogic()
	} else if p.directClocked {
		p.emitPrecomputedPureOps(proc)
		p.clearPureOpCaches()
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

func (p *processPrinter) emitPrecomputedPureOps(proc *ir.Process) {
	if p == nil || proc == nil {
		return
	}
	prev := p.persistPureOpCaches
	p.persistPureOpCaches = true
	defer func() {
		p.persistPureOpCaches = prev
	}()
	for _, block := range proc.Blocks {
		p.beginBlockValueScope()
		for _, op := range block.Ops {
			p.emitOperation(block, op, proc)
		}
		p.endBlockValueScope()
	}
}

func (p *processPrinter) clearPureOpCaches() {
	if p == nil {
		return
	}
	for name := range p.internalSignalReads {
		delete(p.internalSignalReads, name)
	}
	for key := range p.convertCache {
		delete(p.convertCache, key)
	}
	for key := range p.exprCache {
		delete(p.exprCache, key)
	}
}

func (p *processPrinter) shouldCachePureOpResults() bool {
	return p != nil && (p.directClocked || p.fsm != nil)
}

func (p *processPrinter) cachePureOpResult(dest *ir.Signal, key exprCacheKey, emit func(name string)) string {
	if p == nil || dest == nil {
		return ""
	}
	if cached, ok := p.exprCache[key]; ok && cached != "" {
		p.valueNames[dest] = cached
		return cached
	}
	name := p.bindSSA(dest)
	p.recordBlockExprOverride(key)
	p.exprCache[key] = name
	emit(name)
	return name
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
	if !processHasExplicitEmitterClockGuard(proc) {
		for _, block := range proc.Blocks {
			if block != nil {
				blocks[block] = true
			}
		}
		return blocks
	}
	if p.hasImplicitClockedOutputState(proc) {
		for _, block := range proc.Blocks {
			if block != nil {
				blocks[block] = true
			}
		}
		return blocks
	}
	entry := p.directClockControlEntry(proc)
	if entry == nil {
		entry = proc.Blocks[0]
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
				trueClocked, falseClocked := p.clockedBranchPolarity(term)
				visit(term.True, trueClocked)
				visit(term.False, falseClocked)
				return
			}
			if term.Cond != nil && isResetPortName(term.Cond.Name) && block == entry {
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
	visit(entry, false)
	for block := range clockedReach {
		if !nonClockedReach[block] {
			blocks[block] = true
		}
	}
	if p.processHasDualClockEdges(proc) {
		for block := range blocks {
			if emitterBlockHasOnlyOutputAssignments(block) && !emitterShouldRetainDualEdgeOutputBlock(proc, block, blocks) {
				delete(blocks, block)
			}
		}
	}
	return blocks
}

func processHasExplicitEmitterClockGuard(proc *ir.Process) bool {
	if proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		term, ok := block.Terminator.(*ir.BranchTerminator)
		if !ok || term == nil || term.Cond == nil {
			continue
		}
		if isClockLikeName(term.Cond.Name) {
			return true
		}
	}
	return false
}

func emitterSignalAssignmentKinds(proc *ir.Process, sig *ir.Signal, clockedBlocks map[*ir.BasicBlock]bool) (bool, bool) {
	if proc == nil || sig == nil {
		return false, false
	}
	hasClocked := false
	hasNonClocked := false
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil {
				continue
			}
			if !emitterSameSignal(assign.Dest, sig) {
				continue
			}
			if clockedBlocks[block] {
				hasClocked = true
			} else {
				hasNonClocked = true
			}
		}
	}
	return hasClocked, hasNonClocked
}

func emitterShouldRetainDualEdgeOutputBlock(proc *ir.Process, block *ir.BasicBlock, clockedBlocks map[*ir.BasicBlock]bool) bool {
	if proc == nil || block == nil {
		return false
	}
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil || !isOutputGlobalName(assign.Dest.Name) {
			continue
		}
		clockedAssign, nonClockedAssign := emitterSignalAssignmentKinds(proc, assign.Dest, clockedBlocks)
		if clockedAssign && !nonClockedAssign {
			return true
		}
	}
	return false
}

func (p *processPrinter) processHasDualClockEdges(proc *ir.Process) bool {
	if p == nil || proc == nil {
		return false
	}
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		term, ok := block.Terminator.(*ir.BranchTerminator)
		if !ok || term == nil || term.Cond == nil || !isClockLikeName(term.Cond.Name) {
			continue
		}
		trueClocked, falseClocked := p.clockedBranchPolarity(term)
		if trueClocked && falseClocked {
			return true
		}
	}
	return false
}

func emitterBlockHasOnlyOutputAssignments(block *ir.BasicBlock) bool {
	if block == nil {
		return false
	}
	hasAssign := false
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil {
			return false
		}
		if !isOutputGlobalName(assign.Dest.Name) {
			return false
		}
		hasAssign = true
	}
	return hasAssign
}

func (p *processPrinter) hasImplicitClockedOutputState(proc *ir.Process) bool {
	if p == nil || proc == nil {
		return false
	}
	hasClock := false
	outputNames := make(map[string]struct{})
	for name := range p.portNames {
		if isClockLikeName(name) {
			hasClock = true
		}
	}
	for name := range p.moduleSignals {
		if !isOutputGlobalName(name) {
			continue
		}
		outputNames[strings.TrimPrefix(name, "out_")] = struct{}{}
	}
	if !hasClock || len(outputNames) == 0 {
		return false
	}
	stateParamNames := make(map[string]struct{})
	for _, param := range proc.Params {
		if param == nil {
			continue
		}
		if _, ok := outputNames[param.Name]; ok {
			stateParamNames[param.Name] = struct{}{}
		}
	}
	if len(stateParamNames) == 0 {
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
			if _, ok := stateParamNames[strings.TrimPrefix(assign.Dest.Name, "out_")]; ok {
				return true
			}
		}
	}
	return false
}

func (p *processPrinter) clockedBranchPolarity(term *ir.BranchTerminator) (bool, bool) {
	if p == nil || term == nil {
		return false, false
	}
	includeOutputs := !p.processHasInternalStateAssignments()
	trueClocked := p.blockPathHasPersistentAssignment(term.True, includeOutputs, make(map[*ir.BasicBlock]bool))
	falseClocked := p.blockPathHasPersistentAssignment(term.False, includeOutputs, make(map[*ir.BasicBlock]bool))
	switch {
	case trueClocked && falseClocked:
		return true, true
	case trueClocked:
		return true, false
	case falseClocked:
		return false, true
	default:
		return true, false
	}
}

func (p *processPrinter) blockPathHasPersistentAssignment(block *ir.BasicBlock, includeOutputs bool, seen map[*ir.BasicBlock]bool) bool {
	if p == nil || block == nil {
		return false
	}
	if seen[block] {
		return false
	}
	seen[block] = true
	defer delete(seen, block)
	for _, op := range block.Ops {
		assign, ok := op.(*ir.AssignOperation)
		if !ok || assign == nil || assign.Dest == nil {
			continue
		}
		if p.isClockBodyAssignDest(assign.Dest, includeOutputs) {
			return true
		}
	}
	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		return p.blockPathHasPersistentAssignment(term.True, includeOutputs, seen) || p.blockPathHasPersistentAssignment(term.False, includeOutputs, seen)
	case *ir.JumpTerminator:
		return p.blockPathHasPersistentAssignment(term.Target, includeOutputs, seen)
	default:
		return false
	}
}

func (p *processPrinter) isClockBodyAssignDest(sig *ir.Signal, includeOutputs bool) bool {
	if p == nil || sig == nil {
		return false
	}
	if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg && !isOutputGlobalName(sig.Name) {
		return true
	}
	return includeOutputs && isOutputGlobalName(sig.Name)
}

func (p *processPrinter) processHasInternalStateAssignments() bool {
	if p == nil || p.proc == nil {
		return false
	}
	for _, block := range p.proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil || assign.Dest == nil {
				continue
			}
			if moduleSig, ok := p.moduleSignals[assign.Dest.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg && !isOutputGlobalName(assign.Dest.Name) {
				return true
			}
		}
	}
	return false
}

func (p *processPrinter) emitSignals() {
	// Signal declarations are emitted on demand by operation emission.
}

func (p *processPrinter) beginBlockValueScope() {
	if p == nil {
		return
	}
	p.blockValueRestores = append(p.blockValueRestores, make(map[*ir.Signal]valueRestore))
	p.blockReadRestores = append(p.blockReadRestores, make(map[string]valueRestore))
	p.blockZeroRestores = append(p.blockZeroRestores, make(map[string]valueRestore))
	p.blockOnesRestores = append(p.blockOnesRestores, make(map[string]valueRestore))
	p.blockConvertRestores = append(p.blockConvertRestores, make(map[convertCacheKey]valueRestore))
	p.blockExprRestores = append(p.blockExprRestores, make(map[exprCacheKey]valueRestore))
}

func (p *processPrinter) endBlockValueScope() {
	if p == nil || len(p.blockValueRestores) == 0 {
		return
	}
	restores := p.blockValueRestores[len(p.blockValueRestores)-1]
	for sig, restore := range restores {
		if !restore.existed {
			delete(p.valueNames, sig)
			continue
		}
		p.valueNames[sig] = restore.value
	}
	p.blockValueRestores = p.blockValueRestores[:len(p.blockValueRestores)-1]
	if len(p.blockReadRestores) > 0 {
		restores := p.blockReadRestores[len(p.blockReadRestores)-1]
		for name, restore := range restores {
			if !restore.existed {
				delete(p.internalSignalReads, name)
				continue
			}
			p.internalSignalReads[name] = restore.value
		}
		p.blockReadRestores = p.blockReadRestores[:len(p.blockReadRestores)-1]
	}
	if len(p.blockZeroRestores) > 0 {
		restores := p.blockZeroRestores[len(p.blockZeroRestores)-1]
		for key, restore := range restores {
			if !restore.existed {
				delete(p.zeroConsts, key)
				continue
			}
			p.zeroConsts[key] = restore.value
		}
		p.blockZeroRestores = p.blockZeroRestores[:len(p.blockZeroRestores)-1]
	}
	if len(p.blockOnesRestores) > 0 {
		restores := p.blockOnesRestores[len(p.blockOnesRestores)-1]
		for key, restore := range restores {
			if !restore.existed {
				delete(p.onesConsts, key)
				continue
			}
			p.onesConsts[key] = restore.value
		}
		p.blockOnesRestores = p.blockOnesRestores[:len(p.blockOnesRestores)-1]
	}
	if len(p.blockConvertRestores) > 0 {
		restores := p.blockConvertRestores[len(p.blockConvertRestores)-1]
		for key, restore := range restores {
			if !restore.existed {
				delete(p.convertCache, key)
				continue
			}
			p.convertCache[key] = restore.value
		}
		p.blockConvertRestores = p.blockConvertRestores[:len(p.blockConvertRestores)-1]
	}
	if len(p.blockExprRestores) > 0 {
		restores := p.blockExprRestores[len(p.blockExprRestores)-1]
		for key, restore := range restores {
			if !restore.existed {
				delete(p.exprCache, key)
				continue
			}
			p.exprCache[key] = restore.value
		}
		p.blockExprRestores = p.blockExprRestores[:len(p.blockExprRestores)-1]
	}
}

func (p *processPrinter) recordBlockValueOverride(sig *ir.Signal) {
	if p == nil || sig == nil || len(p.blockValueRestores) == 0 {
		return
	}
	restores := p.blockValueRestores[len(p.blockValueRestores)-1]
	if _, exists := restores[sig]; exists {
		return
	}
	value, existed := p.valueNames[sig]
	restores[sig] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) recordBlockReadOverride(name string) {
	if p == nil || name == "" || p.persistPureOpCaches || len(p.blockReadRestores) == 0 {
		return
	}
	restores := p.blockReadRestores[len(p.blockReadRestores)-1]
	if _, exists := restores[name]; exists {
		return
	}
	value, existed := p.internalSignalReads[name]
	restores[name] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) hasScopedValueOverride(sig *ir.Signal) bool {
	if p == nil || sig == nil {
		return false
	}
	for i := len(p.blockValueRestores) - 1; i >= 0; i-- {
		if _, ok := p.blockValueRestores[i][sig]; ok {
			return true
		}
	}
	return false
}

func (p *processPrinter) recordBlockZeroOverride(key string) {
	if p == nil || key == "" || len(p.blockZeroRestores) == 0 {
		return
	}
	restores := p.blockZeroRestores[len(p.blockZeroRestores)-1]
	if _, exists := restores[key]; exists {
		return
	}
	value, existed := p.zeroConsts[key]
	restores[key] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) recordBlockOnesOverride(key string) {
	if p == nil || key == "" || len(p.blockOnesRestores) == 0 {
		return
	}
	restores := p.blockOnesRestores[len(p.blockOnesRestores)-1]
	if _, exists := restores[key]; exists {
		return
	}
	value, existed := p.onesConsts[key]
	restores[key] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) recordBlockConvertOverride(key convertCacheKey) {
	if p == nil || (key == convertCacheKey{}) || p.persistPureOpCaches || len(p.blockConvertRestores) == 0 {
		return
	}
	restores := p.blockConvertRestores[len(p.blockConvertRestores)-1]
	if _, exists := restores[key]; exists {
		return
	}
	value, existed := p.convertCache[key]
	restores[key] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) recordBlockExprOverride(key exprCacheKey) {
	if p == nil || (key == exprCacheKey{}) || p.persistPureOpCaches || len(p.blockExprRestores) == 0 {
		return
	}
	restores := p.blockExprRestores[len(p.blockExprRestores)-1]
	if _, exists := restores[key]; exists {
		return
	}
	value, existed := p.exprCache[key]
	restores[key] = valueRestore{existed: existed, value: value}
}

func (p *processPrinter) setScopedValueName(sig *ir.Signal, value string) {
	if p == nil || sig == nil {
		return
	}
	p.recordBlockValueOverride(sig)
	p.valueNames[sig] = value
}

func (p *processPrinter) setScopedInternalRead(name, value string) {
	if p == nil || name == "" {
		return
	}
	p.recordBlockReadOverride(name)
	p.internalSignalReads[name] = value
}

func (p *processPrinter) invalidateScopedValue(sig *ir.Signal) {
	if p == nil || sig == nil {
		return
	}
	p.recordBlockValueOverride(sig)
	delete(p.valueNames, sig)
}

func (p *processPrinter) invalidateScopedRead(name string) {
	if p == nil || name == "" {
		return
	}
	p.recordBlockReadOverride(name)
	delete(p.internalSignalReads, name)
}

func (p *processPrinter) invalidatePackedAggregateCache(elementName string) {
	if p == nil || elementName == "" || p.moduleSignals == nil {
		return
	}
	base, _, ok := indexedSignalName(elementName)
	if !ok || base == "" {
		return
	}
	baseSig, ok := p.moduleSignals[base]
	if !ok || baseSig == nil {
		return
	}
	p.invalidateScopedValue(baseSig)
	p.invalidateScopedRead(base)
	if p.persistentValues != nil {
		delete(p.persistentValues, baseSig)
	}
}

func (p *processPrinter) signalProducer(sig *ir.Signal) (ir.Operation, *ir.BasicBlock) {
	if p == nil || p.proc == nil || sig == nil {
		return nil, nil
	}
	p.seedProducerCache()
	if cached, ok := p.producerCache[sig]; ok {
		return cached.op, cached.block
	}
	op, block := findSignalProducer(p.proc, sig)
	p.producerCache[sig] = cachedProducer{op: op, block: block}
	return op, block
}

func (p *processPrinter) signalAssignedAnywhere(name string) bool {
	if p == nil || name == "" || p.emitter == nil || p.emitter.currentAssigned == nil {
		return false
	}
	_, ok := p.emitter.currentAssigned[name]
	return ok
}

func (p *processPrinter) moduleSignalNeedsStorage(name string) bool {
	if p == nil || name == "" || p.moduleSignals == nil {
		return false
	}
	sig, ok := p.moduleSignals[name]
	if !ok || sig == nil {
		return false
	}
	if sig.Kind == ir.Reg {
		return true
	}
	if sig.Kind != ir.Wire || p.proc == nil {
		return false
	}
	if !p.signalAssignedAnywhere(name) {
		return false
	}
	return signalReadOnRHS(p.proc, sig)
}

func (p *processPrinter) isImmutableRegSignal(sig *ir.Signal) bool {
	if p == nil || sig == nil || sig.Name == "" || p.moduleSignals == nil {
		return false
	}
	moduleSig, ok := p.moduleSignals[sig.Name]
	if !ok || moduleSig == nil || moduleSig.Kind != ir.Reg || moduleSig.Value == nil || sig.Type == nil {
		return false
	}
	if p.signalAssignedAnywhere(sig.Name) {
		return false
	}
	if producer, _ := p.signalProducer(sig); producer != nil {
		return false
	}
	return true
}

func (p *processPrinter) seedProducerCache() {
	if p == nil || p.proc == nil || p.producerCacheSeeded {
		return
	}
	for _, block := range p.proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			dest := producerDestSignal(op)
			if dest == nil {
				continue
			}
			p.producerCache[dest] = cachedProducer{op: op, block: block}
		}
	}
	p.producerCacheSeeded = true
}

func (p *processPrinter) clearTransientStateCaseValues() {
	if p == nil {
		return
	}
	for sig, name := range p.valueNames {
		if p.shouldKeepStateCaseValue(sig, name) {
			continue
		}
		delete(p.valueNames, sig)
	}
	for name := range p.internalSignalReads {
		delete(p.internalSignalReads, name)
	}
}

func (p *processPrinter) shouldKeepStateCaseValue(sig *ir.Signal, name string) bool {
	if p == nil || sig == nil || name == "" {
		return false
	}
	if sig.Kind == ir.Reg {
		return true
	}
	if sig.Name != "" {
		if sig.Name == "clk" || sig.Name == "rst" {
			return true
		}
		if _, ok := p.portNames[sig.Name]; ok {
			return true
		}
	}
	if p.persistentValues != nil {
		if value, ok := p.persistentValues[sig]; ok && value == name {
			return true
		}
	}
	producer, _ := p.signalProducer(sig)
	switch producer.(type) {
	case *ir.PhiOperation, *ir.RecvOperation:
		return true
	default:
		return false
	}
}

func producerDestSignal(op ir.Operation) *ir.Signal {
	switch typed := op.(type) {
	case *ir.AssignOperation:
		if typed != nil {
			return typed.Dest
		}
	case *ir.PhiOperation:
		if typed != nil {
			return typed.Dest
		}
	case *ir.NotOperation:
		if typed != nil {
			return typed.Dest
		}
	case *ir.BinOperation:
		if typed != nil {
			return typed.Dest
		}
	case *ir.CompareOperation:
		if typed != nil {
			return typed.Dest
		}
	case *ir.MuxOperation:
		if typed != nil {
			return typed.Dest
		}
	case *ir.ConvertOperation:
		if typed != nil {
			return typed.Dest
		}
	}
	return nil
}

func (p *processPrinter) immutableRegConstRef(sig *ir.Signal) string {
	if !p.isImmutableRegSignal(sig) {
		return ""
	}
	moduleSig := p.moduleSignals[sig.Name]
	if p.persistentValues != nil {
		if cached, ok := p.persistentValues[sig]; ok && cached != "" {
			p.setScopedValueName(sig, cached)
			return cached
		}
	}
	constName := p.freshValueName("init_const")
	p.printIndent()
	fmt.Fprintf(p.w, "%s = hw.constant %s : %s\n", constName, formatHWConstant(moduleSig.Value, sig.Type), typeString(sig.Type))
	p.setScopedValueName(sig, constName)
	if p.persistentValues != nil {
		p.persistentValues[sig] = constName
	}
	return constName
}

func (p *processPrinter) emitDirectClockedControl(proc *ir.Process) {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return
	}
	entry := p.directClockControlEntry(proc)
	if entry == nil {
		entry = proc.Blocks[0]
	}
	posedge, negedge := p.directClockEdges(proc)
	if posedge && negedge && !p.directClockedHasAsyncReset(proc) {
		for _, mode := range []clockEdgeMode{clockEdgePos, clockEdgeNeg} {
			p.printIndent()
			if mode == clockEdgePos {
				fmt.Fprintf(p.w, "sv.always posedge %s {\n", p.clockPortRef())
			} else {
				fmt.Fprintf(p.w, "sv.always negedge %s {\n", p.clockPortRef())
			}
			p.indent++
			p.emitDirectClockedBlockForEdge(entry, mode, make(map[*ir.BasicBlock]bool))
			p.indent--
			p.printIndent()
			fmt.Fprintln(p.w, "}")
		}
		return
	}
	sensitivity := p.directClockedSensitivity(proc)
	p.printIndent()
	fmt.Fprintf(p.w, "sv.always %s {\n", sensitivity)
	p.indent++
	p.emitDirectClockedBlock(entry, make(map[*ir.BasicBlock]bool))
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
	if !ok || moduleSig == nil || !p.moduleSignalNeedsStorage(op.Dest.Name) {
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
	clk := p.clockPortRef()
	posedge, negedge := p.directClockEdges(proc)
	switch {
	case posedge && negedge:
		if p != nil && proc != nil && p.hasResetPort && p.directClockedHasAsyncReset(proc) {
			if p.resetActiveLow {
				return fmt.Sprintf("posedge %s, negedge %s, negedge %s", clk, clk, p.resetPortRef)
			}
			return fmt.Sprintf("posedge %s, negedge %s, posedge %s", clk, clk, p.resetPortRef)
		}
		return fmt.Sprintf("posedge %s, negedge %s", clk, clk)
	case negedge:
		if p == nil || proc == nil || !p.hasResetPort || !p.directClockedHasAsyncReset(proc) {
			return fmt.Sprintf("negedge %s", clk)
		}
		if p.resetActiveLow {
			return fmt.Sprintf("negedge %s, negedge %s", clk, p.resetPortRef)
		}
		return fmt.Sprintf("negedge %s, posedge %s", clk, p.resetPortRef)
	default:
		if p == nil || proc == nil || !p.hasResetPort || !p.directClockedHasAsyncReset(proc) {
			return fmt.Sprintf("posedge %s", clk)
		}
		if p.resetActiveLow {
			return fmt.Sprintf("posedge %s, negedge %s", clk, p.resetPortRef)
		}
		return fmt.Sprintf("posedge %s, posedge %s", clk, p.resetPortRef)
	}
}

func (p *processPrinter) directClockEdges(proc *ir.Process) (bool, bool) {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return true, false
	}
	if p.hasImplicitClockedOutputState(proc) {
		return true, false
	}
	entry := p.directClockControlEntry(proc)
	if entry == nil {
		entry = proc.Blocks[0]
	}
	return p.directClockEdgesFromBlock(entry, make(map[*ir.BasicBlock]bool))
}

func (p *processPrinter) directClockEdgesFromBlock(block *ir.BasicBlock, seen map[*ir.BasicBlock]bool) (bool, bool) {
	if p == nil || block == nil {
		return false, false
	}
	if seen[block] {
		return false, false
	}
	seen[block] = true
	defer delete(seen, block)
	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		if term.Cond != nil && isClockLikeName(term.Cond.Name) {
			return p.clockedBranchPolarity(term)
		}
		if term.Cond != nil && p.directClockedHasAsyncResetInEntry(block, term) {
			posA, negA := p.directClockEdgesFromBlock(term.True, seen)
			posB, negB := p.directClockEdgesFromBlock(term.False, seen)
			return posA || posB, negA || negB
		}
		posA, negA := p.directClockEdgesFromBlock(term.True, seen)
		posB, negB := p.directClockEdgesFromBlock(term.False, seen)
		return posA || posB, negA || negB
	case *ir.JumpTerminator:
		return p.directClockEdgesFromBlock(term.Target, seen)
	default:
		return false, false
	}
}

func (p *processPrinter) directClockedHasAsyncReset(proc *ir.Process) bool {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return false
	}
	entry := p.directClockControlEntry(proc)
	if entry == nil {
		entry = proc.Blocks[0]
	}
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
	p.beginBlockValueScope()
	defer p.endBlockValueScope()

	if p.clockedBlocks != nil {
		for _, op := range block.Ops {
			assign, ok := op.(*ir.AssignOperation)
			if !ok || assign == nil {
				continue
			}
			if !p.clockedBlocks[block] {
				moduleSig, ok := p.moduleSignals[assign.Dest.Name]
				if !ok || moduleSig == nil || !p.moduleSignalNeedsStorage(assign.Dest.Name) || !isOutputGlobalName(assign.Dest.Name) {
					continue
				}
			}
			p.emitDirectAssignUpdate(assign)
		}
	}

	switch term := block.Terminator.(type) {
	case *ir.BranchTerminator:
		if term.Cond != nil && isClockLikeName(term.Cond.Name) {
			trueClocked, falseClocked := p.clockedBranchPolarity(term)
			switch {
			case trueClocked && falseClocked:
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
			case trueClocked:
				p.emitDirectClockedBlock(term.True, active)
			case falseClocked:
				p.emitDirectClockedBlock(term.False, active)
			}
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

func (p *processPrinter) emitDirectClockedBlockForEdge(block *ir.BasicBlock, mode clockEdgeMode, active map[*ir.BasicBlock]bool) {
	if p == nil || block == nil {
		return
	}
	if active[block] {
		return
	}
	active[block] = true
	defer delete(active, block)
	p.beginBlockValueScope()
	defer p.endBlockValueScope()

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
			if mode == clockEdgePos {
				p.emitDirectClockedBlockForEdge(term.True, mode, active)
			} else {
				p.emitDirectClockedBlockForEdge(term.False, mode, active)
			}
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
			p.emitDirectClockedBlockForEdge(assertedBlock, mode, active)
			p.indent--
			p.printIndent()
			fmt.Fprintln(p.w, "} else {")
			p.indent++
			p.emitDirectClockedBlockForEdge(deassertedBlock, mode, active)
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
		p.emitDirectClockedBlockForEdge(term.True, mode, active)
		p.indent--
		p.printIndent()
		fmt.Fprintln(p.w, "} else {")
		p.indent++
		p.emitDirectClockedBlockForEdge(term.False, mode, active)
		p.indent--
		p.printIndent()
		fmt.Fprintln(p.w, "}")
	case *ir.JumpTerminator:
		p.emitDirectClockedBlockForEdge(term.Target, mode, active)
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
	entry := p.directClockControlEntry(p.proc)
	if entry == nil {
		if p.proc == nil || len(p.proc.Blocks) == 0 {
			return false
		}
		entry = p.proc.Blocks[0]
	}
	if entry != block {
		return false
	}
	return true
}

func (p *processPrinter) directClockControlEntry(proc *ir.Process) *ir.BasicBlock {
	if p == nil || proc == nil || len(proc.Blocks) == 0 {
		return nil
	}
	start := proc.Blocks[0]
	queue := []*ir.BasicBlock{start}
	seen := map[*ir.BasicBlock]bool{start: true}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil {
			continue
		}
		if term, ok := block.Terminator.(*ir.BranchTerminator); ok && term != nil && term.Cond != nil {
			if isResetPortName(term.Cond.Name) || isClockLikeName(term.Cond.Name) {
				return block
			}
		}
		for _, succ := range block.Successors {
			if succ == nil || seen[succ] {
				continue
			}
			seen[succ] = true
			queue = append(queue, succ)
		}
	}
	return start
}

func (p *processPrinter) emitDirectAssignUpdate(op *ir.AssignOperation) {
	if p == nil || op == nil || op.Dest == nil || op.Value == nil {
		return
	}
	moduleSig, ok := p.moduleSignals[op.Dest.Name]
	if !ok || moduleSig == nil || !p.moduleSignalNeedsStorage(op.Dest.Name) {
		return
	}
	if p.portNames != nil {
		if _, isPort := p.portNames[op.Dest.Name]; isPort {
			return
		}
	}
	value := p.edgeValueRef(op.Value)
	if value == "" || value == "%unknown" {
		return
	}
	dest := "%" + sanitize(op.Dest.Name)
	p.printIndent()
	fmt.Fprintf(p.w, "sv.passign %s, %s : %s\n", dest, value, typeString(op.Dest.Type))
	p.emitIndexedAggregateAssignMirrors(op.Dest, value, false)
	p.recordBlockValueOverride(op.Dest)
	p.valueNames[op.Dest] = value
	p.invalidatePackedAggregateCache(op.Dest.Name)
}

func (p *processPrinter) emitIndexedAggregateAssignMirrors(dest *ir.Signal, value string, recordOverride bool) {
	if p == nil || dest == nil || value == "" || value == "%unknown" {
		return
	}
	elements := p.indexedAggregateElements(dest)
	if len(elements) == 0 {
		return
	}
	containerType := typeString(dest.Type)
	for _, elem := range elements {
		if elem.sig == nil {
			continue
		}
		extract := p.freshValueName("agg_elem")
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.extract %s from %d : (%s) -> %s\n",
			extract,
			value,
			elem.offset,
			containerType,
			typeString(elem.sig.Type),
		)
		elemDest := "%" + sanitize(elem.sig.Name)
		p.printIndent()
		fmt.Fprintf(p.w, "sv.passign %s, %s : %s\n", elemDest, extract, typeString(elem.sig.Type))
		if recordOverride {
			p.recordBlockValueOverride(elem.sig)
			p.valueNames[elem.sig] = extract
		}
	}
}

type indexedAggregateElement struct {
	sig    *ir.Signal
	offset int
}

func (p *processPrinter) indexedAggregateElements(sig *ir.Signal) []indexedAggregateElement {
	if p == nil || sig == nil || sig.Name == "" || sig.Type == nil || p.moduleSignals == nil {
		return nil
	}
	if p.indexedAggregates != nil {
		if cached, ok := p.indexedAggregates[sig.Name]; ok {
			return cached
		}
	}
	elementsByIndex := make(map[int]*ir.Signal)
	indices := make([]int, 0)
	for name, candidate := range p.moduleSignals {
		if candidate == nil {
			continue
		}
		base, index, ok := indexedSignalName(name)
		if !ok || base != sig.Name {
			continue
		}
		elementsByIndex[index] = candidate
		indices = append(indices, index)
	}
	if len(indices) == 0 {
		if p.indexedAggregates != nil {
			p.indexedAggregates[sig.Name] = nil
		}
		return nil
	}
	sort.Ints(indices)
	elements := make([]indexedAggregateElement, 0, len(indices))
	offset := 0
	for _, index := range indices {
		elemSig := elementsByIndex[index]
		if elemSig == nil || elemSig.Type == nil {
			if p.indexedAggregates != nil {
				p.indexedAggregates[sig.Name] = nil
			}
			return nil
		}
		elements = append(elements, indexedAggregateElement{sig: elemSig, offset: offset})
		offset += signalWidth(elemSig.Type)
	}
	if offset != signalWidth(sig.Type) {
		if p.indexedAggregates != nil {
			p.indexedAggregates[sig.Name] = nil
		}
		return nil
	}
	if p.indexedAggregates != nil {
		p.indexedAggregates[sig.Name] = elements
	}
	return elements
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
	emitted := make(map[string]struct{})
	for _, name := range names {
		sig := p.moduleSignals[name]
		if sig == nil {
			continue
		}
		switch sig.Kind {
		case ir.Const:
			if _, ok := p.usedSignals[sig]; !ok {
				continue
			}
			ssaName := p.assignConst(sig)
			if _, ok := emitted[ssaName]; ok {
				continue
			}
			emitted[ssaName] = struct{}{}
			p.printIndent()
			fmt.Fprintf(p.w, "%s = hw.constant %s : %s\n", ssaName, formatHWConstant(sig.Value, sig.Type), typeString(sig.Type))
		case ir.Wire:
			if sig.Value == nil || sig.Type == nil {
				continue
			}
			if producer, _ := p.signalProducer(sig); producer != nil {
				continue
			}
			ssaName := p.freshValueName("init_const")
			if _, ok := emitted[ssaName]; ok {
				continue
			}
			emitted[ssaName] = struct{}{}
			if p.persistentValues != nil {
				p.persistentValues[sig] = ssaName
			}
			p.printIndent()
			fmt.Fprintf(p.w, "%s = hw.constant %s : %s\n", ssaName, formatHWConstant(sig.Value, sig.Type), typeString(sig.Type))
		case ir.Reg:
			if !p.isImmutableRegSignal(sig) {
				continue
			}
			ssaName := p.freshValueName("init_const")
			if _, ok := emitted[ssaName]; ok {
				continue
			}
			emitted[ssaName] = struct{}{}
			if p.persistentValues != nil {
				p.persistentValues[sig] = ssaName
			}
			p.printIndent()
			fmt.Fprintf(p.w, "%s = hw.constant %s : %s\n", ssaName, formatHWConstant(sig.Value, sig.Type), typeString(sig.Type))
		}
	}
}

func (p *processPrinter) emitOperation(block *ir.BasicBlock, op ir.Operation, proc *ir.Process) {
	switch o := op.(type) {
	case *ir.BinOperation:
		left := p.valueRef(o.Left)
		right := p.valueRef(o.Right)
		if p.shouldCachePureOpResults() && left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			opName := binOpName(o.Op)
			keyLeft, keyRight := normalizeExprOperands(left, right, isCommutativeBinOp(o.Op))
			key := exprCacheKey{kind: "bin:" + opName, a: keyLeft, b: keyRight, typ: typeString(o.Dest.Type)}
			p.cachePureOpResult(o.Dest, key, func(dest string) {
				p.printIndent()
				fmt.Fprintf(p.w, "%s = comb.%s %s, %s : %s\n",
					dest,
					opName,
					left,
					right,
					typeString(o.Dest.Type),
				)
			})
			return
		}
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
				isModuleReg := ok && moduleSig != nil && p.moduleSignalNeedsStorage(o.Dest.Name)
				if p.clockedBlocks == nil || !p.clockedBlocks[block] {
					if isModuleReg && !isOutputGlobalName(o.Dest.Name) {
						return
					}
					src := p.valueRef(o.Value)
					if src != "" && src != "%unknown" {
						p.recordBlockValueOverride(o.Dest)
						p.valueNames[o.Dest] = src
					}
					return
				}
				// Inside clocked blocks, keep any assigned value visible to later RHS reads
				// in the same cycle. This matches the blocking-style source semantics,
				// including local arrays materialized as regs.
				src := p.valueRef(o.Value)
				if src != "" && src != "%unknown" && !isModuleReg {
					p.recordBlockValueOverride(o.Dest)
					p.valueNames[o.Dest] = src
				}
				return
			}
			src := p.valueRef(o.Value)
			if src != "" && src != "%unknown" {
				p.recordBlockValueOverride(o.Dest)
				p.valueNames[o.Dest] = src
			}
			moduleSig, ok := p.moduleSignals[o.Dest.Name]
			if ok && moduleSig != nil && p.moduleSignalNeedsStorage(o.Dest.Name) {
				return
			}
			return
		}
		src := p.valueRef(o.Value)

		// For combinational processes, just map the value directly without creating a register
		if p.proc != nil && p.proc.Sensitivity == ir.Combinational {
			p.recordBlockValueOverride(o.Dest)
			p.valueNames[o.Dest] = src
			if _, ok := p.moduleSignals[o.Dest.Name]; ok {
				p.persistentValues[o.Dest] = src
			}
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
		if p.shouldCachePureOpResults() && left != "" && right != "" && left != "%unknown" && right != "%unknown" {
			predicate := comparePredicateName(o.Predicate)
			if left == right {
				switch predicate {
				case "eq", "sle", "sge", "ule", "uge":
					p.valueNames[o.Dest] = p.boolConst(true)
					return
				case "ne", "slt", "sgt", "ult", "ugt":
					p.valueNames[o.Dest] = p.boolConst(false)
					return
				}
			}
			keyLeft, keyRight := normalizeExprOperands(left, right, predicate == "eq" || predicate == "ne")
			key := exprCacheKey{kind: "icmp:" + predicate, a: keyLeft, b: keyRight, typ: typeString(o.Left.Type)}
			p.cachePureOpResult(o.Dest, key, func(dest string) {
				p.printIndent()
				fmt.Fprintf(p.w, "%s = comb.icmp %s %s, %s : %s\n",
					dest,
					predicate,
					left,
					right,
					typeString(o.Left.Type),
				)
			})
			return
		}
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
		ones := p.typedAllOnesConst(o.Value.Type)
		if p.shouldCachePureOpResults() && value != "" && value != "%unknown" {
			key := exprCacheKey{kind: "not", a: value, b: ones, typ: typeString(o.Value.Type)}
			p.cachePureOpResult(o.Dest, key, func(dest string) {
				p.printIndent()
				fmt.Fprintf(p.w, "%s = comb.xor %s, %s : %s\n", dest, value, ones, typeString(o.Value.Type))
			})
			return
		}
		dest := p.bindSSA(o.Dest)
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.xor %s, %s : %s\n", dest, value, ones, typeString(o.Value.Type))
	case *ir.MuxOperation:
		cond := p.valueRef(o.Cond)
		tVal := p.valueRef(o.TrueValue)
		fVal := p.valueRef(o.FalseValue)
		if folded, ok := p.foldMuxRefs(cond, tVal, fVal); ok {
			p.valueNames[o.Dest] = folded
			return
		}
		if p.shouldCachePureOpResults() && cond != "" && tVal != "" && fVal != "" && cond != "%unknown" && tVal != "%unknown" && fVal != "%unknown" {
			key := exprCacheKey{kind: "mux", a: cond, b: tVal, c: fVal, typ: typeString(o.Dest.Type)}
			p.cachePureOpResult(o.Dest, key, func(dest string) {
				p.printIndent()
				fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n",
					dest,
					cond,
					tVal,
					fVal,
					typeString(o.Dest.Type),
				)
			})
			return
		}
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
	clk := p.clockPortRef()
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
		incomingVal := p.valueRef(incoming.Value)
		condRef := ""
		phiBlock := phiBlockForIncoming(phi, incoming.Block)
		if incoming.Block != nil && phiBlock != nil {
			condCache := make(map[*ir.BasicBlock]string)
			condRef = p.emitPhiIncomingConditionRef(incoming.Block, phiBlock, condCache, make(map[*ir.BasicBlock]bool))
		}
		if condRef == "" || condRef == "%unknown" {
			incomingBlock := incoming.Block
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
			if cond != nil {
				condRef = p.valueRef(cond)
			}
		}
		if condRef == "" || condRef == "%unknown" {
			currentVal = incomingVal
			continue
		}

		name := dest
		if i > 0 {
			name = p.freshValueName("mux")
		}
		if folded, ok := p.foldMuxRefs(condRef, incomingVal, currentVal); ok {
			currentVal = folded
			continue
		}
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n",
			name, condRef, incomingVal, currentVal, typeString(phi.Dest.Type))
		currentVal = name
	}
	p.valueNames[phi.Dest] = currentVal
}

func (p *processPrinter) emitPhiIncomingConditionRef(pred, target *ir.BasicBlock, cache map[*ir.BasicBlock]string, active map[*ir.BasicBlock]bool) string {
	if p == nil || pred == nil {
		return ""
	}
	_ = cache
	_ = active
	termsCache := make(map[*ir.BasicBlock][]condTerm)
	terms := p.phiIncomingConditionTerms(pred, target, termsCache, make(map[*ir.BasicBlock]bool))
	return p.emitCondTermsRef(terms)
}

func (p *processPrinter) phiIncomingConditionTerms(pred, target *ir.BasicBlock, cache map[*ir.BasicBlock][]condTerm, active map[*ir.BasicBlock]bool) []condTerm {
	if p == nil || pred == nil {
		return nil
	}
	var cacheKey phiCondCacheKey
	useGlobalCache := target != nil && p.phiCondTermsCache != nil
	if target != nil && p.phiCondTermsCache != nil {
		cacheKey = phiCondCacheKey{pred: pred, target: target}
		if cached, ok := p.phiCondTermsCache[cacheKey]; ok {
			return cached
		}
	}
	terms := p.blockReachabilityTerms(pred, cache, active)
	if len(terms) == 0 {
		if useGlobalCache {
			p.phiCondTermsCache[cacheKey] = nil
		}
		return nil
	}
	if pred == target || target == nil {
		if useGlobalCache {
			p.phiCondTermsCache[cacheKey] = terms
		}
		return terms
	}
	switch term := pred.Terminator.(type) {
	case *ir.BranchTerminator:
		switch {
		case term.True == target:
			terms = appendLiteralToTerms(terms, condLiteral{sig: term.Cond, positive: true})
			if useGlobalCache {
				p.phiCondTermsCache[cacheKey] = terms
			}
			return terms
		case term.False == target:
			terms = appendLiteralToTerms(terms, condLiteral{sig: term.Cond, positive: false})
			if useGlobalCache {
				p.phiCondTermsCache[cacheKey] = terms
			}
			return terms
		default:
			if useGlobalCache {
				p.phiCondTermsCache[cacheKey] = nil
			}
			return nil
		}
	case *ir.JumpTerminator:
		if term.Target == target {
			if useGlobalCache {
				p.phiCondTermsCache[cacheKey] = terms
			}
			return terms
		}
	}
	if useGlobalCache {
		p.phiCondTermsCache[cacheKey] = nil
	}
	return nil
}

func (p *processPrinter) blockReachabilityTerms(block *ir.BasicBlock, cache map[*ir.BasicBlock][]condTerm, active map[*ir.BasicBlock]bool) []condTerm {
	if p == nil || block == nil {
		return nil
	}
	if p.blockCondTermsCache != nil {
		if cached, ok := p.blockCondTermsCache[block]; ok {
			return cached
		}
	}
	if cache != nil {
		if cached, ok := cache[block]; ok {
			return cached
		}
	}
	if active[block] {
		return nil
	}
	if p.proc != nil && len(p.proc.Blocks) > 0 && p.proc.Blocks[0] == block {
		terms := []condTerm{{}}
		if cache != nil {
			cache[block] = terms
		}
		return terms
	}
	active[block] = true
	defer delete(active, block)

	var terms []condTerm
	for _, pred := range block.Predecessors {
		terms = append(terms, p.phiIncomingConditionTerms(pred, block, cache, active)...)
	}
	terms = simplifyCondTerms(terms)
	if p.blockCondTermsCache != nil {
		p.blockCondTermsCache[block] = terms
	}
	if cache != nil {
		cache[block] = terms
	}
	return terms
}

func phiBlockForIncoming(phi *ir.PhiOperation, pred *ir.BasicBlock) *ir.BasicBlock {
	if phi == nil || pred == nil {
		return nil
	}
	// The phi itself lives in the block that contains the incoming predecessor.
	// All incoming predecessors flow into that containing block.
	return predSuccessorWithPhi(pred, phi)
}

func predSuccessorWithPhi(pred *ir.BasicBlock, phi *ir.PhiOperation) *ir.BasicBlock {
	if pred == nil || phi == nil {
		return nil
	}
	for _, succ := range pred.Successors {
		if succ == nil {
			continue
		}
		for _, op := range succ.Ops {
			if op == phi {
				return succ
			}
		}
	}
	return nil
}

type pathConditionStep struct {
	cond     *ir.Signal
	takeTrue bool
}

type condLiteral struct {
	sig      *ir.Signal
	positive bool
}

type condTerm []condLiteral

func (p *processPrinter) findPathConditionsToBlock(target *ir.BasicBlock) ([]pathConditionStep, bool) {
	if p == nil || p.proc == nil || len(p.proc.Blocks) == 0 || target == nil {
		return nil, false
	}
	visited := make(map[*ir.BasicBlock]bool)
	return p.findPathConditions(p.proc.Blocks[0], target, visited)
}

func (p *processPrinter) findPathConditionsToEdge(pred, target *ir.BasicBlock) ([]pathConditionStep, bool) {
	if p == nil || pred == nil {
		return nil, false
	}
	steps, ok := p.findPathConditionsToBlock(pred)
	if !ok {
		return nil, false
	}
	if pred == target || target == nil {
		return steps, true
	}
	switch term := pred.Terminator.(type) {
	case *ir.BranchTerminator:
		switch {
		case term.True == target:
			return append(steps, pathConditionStep{cond: term.Cond, takeTrue: true}), true
		case term.False == target:
			return append(steps, pathConditionStep{cond: term.Cond, takeTrue: false}), true
		default:
			return nil, false
		}
	case *ir.JumpTerminator:
		if term.Target == target {
			return steps, true
		}
	}
	return nil, false
}

func (p *processPrinter) findPathConditions(current, target *ir.BasicBlock, visited map[*ir.BasicBlock]bool) ([]pathConditionStep, bool) {
	if current == nil || target == nil {
		return nil, false
	}
	if current == target {
		return []pathConditionStep{}, true
	}
	if visited[current] {
		return nil, false
	}
	visited[current] = true
	defer delete(visited, current)

	switch term := current.Terminator.(type) {
	case *ir.BranchTerminator:
		if steps, ok := p.findPathConditions(term.True, target, visited); ok {
			return append([]pathConditionStep{{cond: term.Cond, takeTrue: true}}, steps...), true
		}
		if steps, ok := p.findPathConditions(term.False, target, visited); ok {
			return append([]pathConditionStep{{cond: term.Cond, takeTrue: false}}, steps...), true
		}
	case *ir.JumpTerminator:
		return p.findPathConditions(term.Target, target, visited)
	}
	return nil, false
}

func (p *processPrinter) emitPathConditionRef(steps []pathConditionStep) string {
	if p == nil {
		return ""
	}
	if len(steps) == 0 {
		return p.boolConst(true)
	}
	var current string
	for _, step := range steps {
		if step.cond == nil {
			return ""
		}
		lit := p.valueRef(step.cond)
		if lit == "" || lit == "%unknown" {
			return ""
		}
		if !step.takeTrue {
			one := p.boolConst(true)
			name := p.freshValueName("path_not")
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.xor %s, %s : i1\n", name, lit, one)
			lit = name
		}
		if current == "" {
			current = lit
			continue
		}
		name := p.freshValueName("path_and")
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.and %s, %s : i1\n", name, current, lit)
		current = name
	}
	return current
}

func appendLiteralToTerms(terms []condTerm, lit condLiteral) []condTerm {
	if lit.sig == nil || len(terms) == 0 {
		return simplifyCondTerms(terms)
	}
	out := make([]condTerm, 0, len(terms))
	for _, term := range terms {
		next := append(condTerm{}, term...)
		next = append(next, lit)
		out = append(out, next)
	}
	return simplifyCondTerms(out)
}

func simplifyCondTerms(terms []condTerm) []condTerm {
	normalized := make([]condTerm, 0, len(terms))
	for _, term := range terms {
		if next, ok := normalizeCondTerm(term); ok {
			normalized = append(normalized, next)
		}
	}
	terms = dedupeCondTerms(normalized)
	changed := true
	for changed {
		changed = false
		filtered := make([]condTerm, 0, len(terms))
		for i, term := range terms {
			subsumed := false
			for j, other := range terms {
				if i == j {
					continue
				}
				if condTermSubsumes(other, term) {
					subsumed = true
					changed = true
					break
				}
			}
			if !subsumed {
				filtered = append(filtered, term)
			}
		}
		terms = dedupeCondTerms(filtered)
	combinedLoop:
		for i := 0; i < len(terms); i++ {
			for j := i + 1; j < len(terms); j++ {
				if combined, ok := combineCondTerms(terms[i], terms[j]); ok {
					next := make([]condTerm, 0, len(terms)-1)
					for k, term := range terms {
						if k == i || k == j {
							continue
						}
						next = append(next, term)
					}
					next = append(next, combined)
					terms = dedupeCondTerms(next)
					changed = true
					break combinedLoop
				}
			}
		}
	}
	return terms
}

func normalizeCondTerm(term condTerm) (condTerm, bool) {
	if len(term) == 0 {
		return condTerm{}, true
	}
	bySignal := make(map[*ir.Signal]bool)
	for _, lit := range term {
		if lit.sig == nil {
			return nil, false
		}
		if existing, ok := bySignal[lit.sig]; ok {
			if existing != lit.positive {
				return nil, false
			}
			continue
		}
		bySignal[lit.sig] = lit.positive
	}
	keys := make([]*ir.Signal, 0, len(bySignal))
	for sig := range bySignal {
		keys = append(keys, sig)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name < keys[j].Name
	})
	out := make(condTerm, 0, len(keys))
	for _, sig := range keys {
		out = append(out, condLiteral{sig: sig, positive: bySignal[sig]})
	}
	return out, true
}

func dedupeCondTerms(terms []condTerm) []condTerm {
	seen := make(map[string]struct{})
	out := make([]condTerm, 0, len(terms))
	for _, term := range terms {
		key := condTermKey(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, term)
	}
	return out
}

func condTermKey(term condTerm) string {
	if len(term) == 0 {
		return "true"
	}
	parts := make([]string, 0, len(term))
	for _, lit := range term {
		prefix := "+"
		if !lit.positive {
			prefix = "-"
		}
		parts = append(parts, prefix+lit.sig.Name)
	}
	return strings.Join(parts, "|")
}

func condTermSubsumes(a, b condTerm) bool {
	if len(a) > len(b) {
		return false
	}
	i := 0
	j := 0
	for i < len(a) && j < len(b) {
		nameA := a[i].sig.Name
		nameB := b[j].sig.Name
		switch {
		case nameA == nameB:
			if a[i].positive != b[j].positive {
				return false
			}
			i++
			j++
		case nameA > nameB:
			j++
		default:
			return false
		}
	}
	return i == len(a)
}

func combineCondTerms(a, b condTerm) (condTerm, bool) {
	if len(a) != len(b) {
		return nil, false
	}
	diffIdx := -1
	for i := range a {
		if a[i].sig != b[i].sig {
			return nil, false
		}
		if a[i].positive != b[i].positive {
			if diffIdx >= 0 {
				return nil, false
			}
			diffIdx = i
		}
	}
	if diffIdx < 0 {
		return nil, false
	}
	out := make(condTerm, 0, len(a)-1)
	for i := range a {
		if i == diffIdx {
			continue
		}
		out = append(out, a[i])
	}
	return out, true
}

func (p *processPrinter) emitCondTermsRef(terms []condTerm) string {
	if p == nil || len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 && len(terms[0]) == 0 {
		return p.boolConst(true)
	}
	termRefs := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) == 0 {
			return p.boolConst(true)
		}
		ref := ""
		for _, lit := range term {
			litRef := p.valueRef(lit.sig)
			if litRef == "" || litRef == "%unknown" {
				return ""
			}
			if !lit.positive {
				one := p.boolConst(true)
				name := p.freshValueName("phi_not")
				p.printIndent()
				fmt.Fprintf(p.w, "%s = comb.xor %s, %s : i1\n", name, litRef, one)
				litRef = name
			}
			if ref == "" {
				ref = litRef
				continue
			}
			name := p.freshValueName("phi_and")
			p.printIndent()
			fmt.Fprintf(p.w, "%s = comb.and %s, %s : i1\n", name, ref, litRef)
			ref = name
		}
		if ref != "" {
			termRefs = append(termRefs, ref)
		}
	}
	if len(termRefs) == 0 {
		return ""
	}
	current := termRefs[0]
	for i := 1; i < len(termRefs); i++ {
		name := p.freshValueName("phi_or")
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.or %s, %s : i1\n", name, current, termRefs[i])
		current = name
	}
	return current
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
	if key := p.literalConstKey(sig); key != "" {
		if name, ok := p.literalConstNames[key]; ok && name != "" {
			p.constNames[sig] = name
			return name
		}
	}
	name := fmt.Sprintf("%%c%d", p.emitter.globalTempID)
	p.emitter.globalTempID++
	p.constNames[sig] = name
	if key := p.literalConstKey(sig); key != "" {
		p.literalConstNames[key] = name
	}
	return name
}

func (p *processPrinter) literalConstKey(sig *ir.Signal) string {
	if p == nil || sig == nil || sig.Kind != ir.Const || sig.Type == nil {
		return ""
	}
	return typeString(sig.Type) + "=" + formatHWConstant(sig.Value, sig.Type)
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

func (p *processPrinter) shouldReuseCachedValueName(sig *ir.Signal, name string) bool {
	if p == nil || sig == nil || name == "" {
		return false
	}
	if !strings.HasPrefix(name, "%init_const") {
		return true
	}
	if sig.Name == "" || p.moduleSignals == nil {
		return true
	}
	moduleSig, ok := p.moduleSignals[sig.Name]
	if !ok || moduleSig == nil || moduleSig.Kind != ir.Wire || moduleSig.Value == nil {
		return true
	}
	if producer, _ := p.signalProducer(sig); producer != nil {
		return true
	}
	return false
}

func (p *processPrinter) valueRef(sig *ir.Signal) string {
	if sig == nil {
		return "%unknown"
	}
	if sig.Kind == ir.Const {
		return p.assignConst(sig)
	}
	if p.persistentValues != nil {
		if value, ok := p.persistentValues[sig]; ok && value != "" {
			p.setScopedValueName(sig, value)
			return value
		}
	}
	if unpacked := p.inputArrayElementRef(sig); unpacked != "" {
		p.setScopedValueName(sig, unpacked)
		return unpacked
	}
	if fallback := p.combinationalOutputArrayElementRef(sig); fallback != "" {
		p.setScopedValueName(sig, fallback)
		return fallback
	}
	if sig.Name != "" {
		if portName, ok := p.portNames[sig.Name]; ok {
			p.setScopedValueName(sig, portName)
			return portName
		}
	}
	if (p.fsm != nil || p.directClocked) && sig.Name != "" && sig.Name != "clk" && sig.Name != "rst" {
		if sig.Name == "varargs" {
			if producer, _ := p.signalProducer(sig); producer == nil {
				zero := p.typedZeroConst(sig.Type)
				p.setScopedValueName(sig, zero)
				return zero
			}
		}
		if name, ok := p.valueNames[sig]; ok && name != "" {
			if !p.shouldReuseCachedValueName(sig, name) {
				delete(p.valueNames, sig)
			} else {
				rawName := "%" + sanitize(sig.Name)
				if name != rawName {
					return name
				}
			}
		}
		if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
			if packed := p.packArraySignalValue(sig); packed != "" {
				p.setScopedValueName(sig, packed)
				return packed
			}
			if constRef := p.immutableRegConstRef(sig); constRef != "" {
				return constRef
			}
			wireName := "%" + sanitize(sig.Name)
			readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
			p.emitter.globalTempID++
			p.printIndent()
			fmt.Fprintf(p.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", readName, wireName, typeString(sig.Type))
			return readName
		}
	}
	if name, ok := p.valueNames[sig]; ok {
		if p.shouldReuseCachedValueName(sig, name) {
			return name
		}
		delete(p.valueNames, sig)
	}

	// Check if this is an internal signal (not a readable port).
	isPort := sig.Name == "clk" || sig.Name == "rst"
	if !isPort && p.portNames != nil {
		_, isPort = p.portNames[sig.Name]
	}

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
			p.setScopedValueName(sig, packed)
			return packed
		}
		// Check if this is a scalar global register (pre-declared at module level)
		// All register-kind signals that are not array elements are pre-declared.
		// In combinational logic they still need a read_inout to produce a plain value.
		if sig.Kind == ir.Reg {
			if constRef := p.immutableRegConstRef(sig); constRef != "" {
				return constRef
			}
			if readName, ok := p.internalSignalReads[sig.Name]; ok {
				return readName
			}
			wireName := "%" + sanitize(sig.Name)
			readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
			p.emitter.globalTempID++
			p.printIndent()
			fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n", readName, wireName, inoutTypeString(sig.Type))
			p.setScopedInternalRead(sig.Name, readName)
			p.setScopedValueName(sig, readName)
			return readName
		}

		// This is an internal signal, need to read it with sv.read_inout
		if readName, ok := p.internalSignalReads[sig.Name]; ok {
			return readName
		}
		if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Wire {
			if producer, _ := p.signalProducer(sig); producer == nil {
				if moduleSig.Value != nil {
					constName := p.freshValueName("init_const")
					p.printIndent()
					fmt.Fprintf(p.w, "%s = hw.constant %s : %s\n", constName, formatHWConstant(moduleSig.Value, sig.Type), typeString(sig.Type))
					return constName
				}
				// Module-scope wires without a producer model zero-initialized globals
				// such as packed arrays or print varargs scratch that survive DCE.
				zero := p.typedZeroConst(sig.Type)
				p.setScopedValueName(sig, zero)
				return zero
			}
		}

		// Emit sv.read_inout to get the regular type value
		wireName := "%" + sanitize(sig.Name)
		readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
		p.emitter.globalTempID++

		p.printIndent()
		fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n", readName, wireName, inoutTypeString(sig.Type))

		p.setScopedInternalRead(sig.Name, readName)
		p.setScopedValueName(sig, readName)
		return readName
	}

	if isArrayElement {
		if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil {
			if moduleSig.Kind == ir.Reg {
				if constRef := p.immutableRegConstRef(sig); constRef != "" {
					return constRef
				}
				if readName, ok := p.internalSignalReads[sig.Name]; ok {
					return readName
				}
				wireName := "%" + sanitize(sig.Name)
				readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
				p.emitter.globalTempID++
				p.printIndent()
				fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n", readName, wireName, inoutTypeString(sig.Type))
				p.setScopedInternalRead(sig.Name, readName)
				p.setScopedValueName(sig, readName)
				return readName
			}
			if moduleSig.Kind == ir.Wire {
				// Local indexed arrays in combinational processes do not have a
				// backing packed SSA value. When an element has not been assigned
				// yet, treat it as the zero-initialized default instead of
				// emitting an undeclared raw %name reference.
				return p.typedZeroConst(sig.Type)
			}
		}
	}

	// Ports can be referenced directly by their block-argument SSA names.
	name := "%" + sanitize(sig.Name)
	p.setScopedValueName(sig, name)
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

func (p *processPrinter) combinationalOutputArrayElementRef(sig *ir.Signal) string {
	if p == nil || sig == nil || sig.Name == "" || sig.Kind != ir.Wire || p.portTypes == nil {
		return ""
	}
	base, _, ok := indexedSignalName(sig.Name)
	if !ok || base == "" {
		return ""
	}
	if _, isReadablePort := p.portNames[base]; isReadablePort {
		return ""
	}
	portType, ok := p.portTypes[base]
	if !ok || portType == nil || signalWidth(portType) <= signalWidth(sig.Type) {
		return ""
	}
	return p.typedZeroConst(sig.Type)
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

func resolveIndexedElementSignal(signals map[string]*ir.Signal, base string, index int, elemWidth int) *ir.Signal {
	if signals == nil || strings.TrimSpace(base) == "" || index < 0 {
		return nil
	}
	name := fmt.Sprintf("%s_%d", base, index)
	for depth := 0; depth < 4; depth++ {
		sig, ok := signals[name]
		if ok && sig != nil && signalWidth(sig.Type) == elemWidth {
			return sig
		}
		name += "_0"
	}
	return nil
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

func collectReadablePortTypesFromIRPorts(ports []ir.Port) map[string]*ir.SignalType {
	portTypes := make(map[string]*ir.SignalType, len(ports))
	for _, port := range ports {
		if port.Direction == ir.Output {
			continue
		}
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
	indexed := p.indexedAggregateElements(sig)
	if len(indexed) > 0 {
		if name, ok := p.valueNames[sig]; ok && name != "" && name != "%unknown" {
			raw := "%" + sanitize(sig.Name)
			if name != raw {
				return name
			}
		}
		elements := make([]string, 0, len(indexed))
		elementTypes := make([]string, 0, len(indexed))
		canPersist := !p.signalAssignedAnywhere(sig.Name)
		for _, entry := range indexed {
			if entry.sig == nil || entry.sig.Type == nil {
				return ""
			}
			if canPersist && p.signalAssignedAnywhere(entry.sig.Name) {
				canPersist = false
			}
			elemSig := entry.sig
			elements = append(elements, p.valueRef(elemSig))
			elementTypes = append(elementTypes, typeString(elemSig.Type))
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
		for i := len(elementTypes) - 1; i >= 0; i-- {
			if i < len(elementTypes)-1 {
				fmt.Fprint(p.w, ", ")
			}
			fmt.Fprint(p.w, elementTypes[i])
		}
		fmt.Fprintln(p.w)
		p.setScopedValueName(sig, name)
		if canPersist && p.persistentValues != nil {
			p.persistentValues[sig] = name
		}
		return name
	}
	if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
		if constRef := p.immutableRegConstRef(sig); constRef != "" {
			return constRef
		}
		if readName, ok := p.internalSignalReads[sig.Name]; ok {
			return readName
		}
		wireName := "%" + sanitize(sig.Name)
		readName := fmt.Sprintf("%%v%d", p.emitter.globalTempID)
		p.emitter.globalTempID++
		p.printIndent()
		fmt.Fprintf(p.w, "%s = sv.read_inout %s : %s\n", readName, wireName, inoutTypeString(sig.Type))
		p.setScopedInternalRead(sig.Name, readName)
		return readName
	}
	return ""
}

func (p *processPrinter) portRef(name string) string {
	if val, ok := p.portNames[name]; ok {
		return val
	}
	return fmt.Sprintf("%%%s", sanitize(name))
}

func (p *processPrinter) clockPortName() string {
	if p == nil {
		return "clk"
	}
	if p.proc != nil {
		for _, block := range p.proc.Blocks {
			term, ok := block.Terminator.(*ir.BranchTerminator)
			if !ok || term == nil || term.Cond == nil {
				continue
			}
			if isClockLikeName(term.Cond.Name) {
				return term.Cond.Name
			}
		}
		for _, param := range p.proc.Params {
			if param != nil && isClockLikeName(param.Name) {
				return param.Name
			}
		}
	}
	if p.portNames != nil {
		if p.portNames["clock"] != "" {
			return "clock"
		}
		if p.portNames["clk"] != "" {
			return "clk"
		}
	}
	return "clk"
}

func (p *processPrinter) clockPortRef() string {
	return p.portRef(p.clockPortName())
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
	return p.edgeValueRefWithActive(sig, make(map[*ir.Signal]bool))
}

func (p *processPrinter) edgeValueRefForPred(sig *ir.Signal, pred *ir.BasicBlock) string {
	if p == nil {
		return "%unknown"
	}
	prev := p.currentPhiPred
	p.currentPhiPred = pred
	defer func() {
		p.currentPhiPred = prev
	}()
	return p.edgeValueRefWithActive(sig, make(map[*ir.Signal]bool))
}

func (p *processPrinter) edgeValueRefWithActive(sig *ir.Signal, active map[*ir.Signal]bool) string {
	if p == nil || sig == nil {
		return "%unknown"
	}
	if active[sig] {
		return p.valueRef(sig)
	}
	active[sig] = true
	defer delete(active, sig)
	if sig.Kind == ir.Const {
		return p.assignConst(sig)
	}
	if sig.Kind == ir.Reg && sig.Name != "" && p.moduleSignals != nil {
		if p.portNames != nil {
			if _, isPort := p.portNames[sig.Name]; isPort {
				goto skipDirectClockedRegRead
			}
		}
		if moduleSig, ok := p.moduleSignals[sig.Name]; ok && moduleSig != nil && moduleSig.Kind == ir.Reg {
			if packed := p.packArraySignalValue(sig); packed != "" {
				return packed
			}
			if constRef := p.immutableRegConstRef(sig); constRef != "" {
				return constRef
			}
			wireName := "%" + sanitize(sig.Name)
			readName := p.freshValueName("edge_reg")
			p.printIndent()
			fmt.Fprintf(p.w, "%s = sv.read_inout %s : !hw.inout<%s>\n", readName, wireName, typeString(sig.Type))
			return readName
		}
	}
skipDirectClockedRegRead:
	if sig.Name != "" {
		if p.proc != nil && isClockLikeName(sig.Name) && !p.processHasDualClockEdges(p.proc) {
			return p.boolConst(true)
		}
		if portName, ok := p.portNames[sig.Name]; ok {
			return portName
		}
		if name, ok := p.valueNames[sig]; ok && name != "" && name != "%unknown" {
			raw := "%" + sanitize(sig.Name)
			if name != raw || sig.Kind == ir.Reg {
				// Direct-clocked lowering pre-emits the process once to resolve root
				// outputs. Reusing those temporary names here can accidentally bake
				// %clk into combinational helpers that are later sampled inside
				// sv.always posedge/negedge blocks. Recompute non-register values
				// from their producers instead; only keep cached names for signals
				// without a producer (ports/roots) and true storage values.
				if sig.Kind == ir.Reg || p.hasScopedValueOverride(sig) {
					return name
				}
				if p.persistentValues != nil {
					if cached, ok := p.persistentValues[sig]; ok && cached == name {
						return name
					}
				}
				if p.proc == nil {
					return name
				}
				if producer, _ := p.signalProducer(sig); producer == nil {
					return name
				}
			}
		}
	}
	if p.proc != nil {
		producer, _ := p.signalProducer(sig)
		switch op := producer.(type) {
		case *ir.AssignOperation:
			ref := p.edgeValueRefWithActive(op.Value, active)
			if ref != "" && ref != "%unknown" {
				p.recordBlockValueOverride(sig)
				p.valueNames[sig] = ref
				return ref
			}
		case *ir.NotOperation:
			if val, ok := signalBoolConst(op.Value); ok {
				return p.boolConst(!val)
			}
			value := p.edgeValueRefWithActive(op.Value, active)
			if value != "" && value != "%unknown" {
				ones := p.boolConst(true)
				if signalWidth(op.Value.Type) != 1 {
					ones = p.typedAllOnesConst(op.Value.Type)
				}
				name := p.cachedEdgeNot(value, ones, typeString(op.Value.Type))
				p.recordBlockValueOverride(sig)
				p.valueNames[sig] = name
				return name
			}
		case *ir.BinOperation:
			if folded, ok := p.tryFoldEdgeBinOp(op, active); ok {
				return folded
			}
			left := p.edgeValueRefWithActive(op.Left, active)
			right := p.edgeValueRefWithActive(op.Right, active)
			if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
				name := p.cachedEdgeBinary(
					binOpName(op.Op),
					left,
					right,
					typeString(op.Dest.Type),
					"edge_bin",
					isCommutativeBinOp(op.Op),
				)
				p.recordBlockValueOverride(sig)
				p.valueNames[sig] = name
				return name
			}
		case *ir.CompareOperation:
			left := p.edgeValueRefWithActive(op.Left, active)
			right := p.edgeValueRefWithActive(op.Right, active)
			if left != "" && right != "" && left != "%unknown" && right != "%unknown" {
				name := p.cachedEdgeCompare(comparePredicateName(op.Predicate), left, right, typeString(op.Left.Type))
				p.recordBlockValueOverride(sig)
				p.valueNames[sig] = name
				return name
			}
		case *ir.PhiOperation:
			if p.currentPhiPred != nil {
				for _, incoming := range op.Incomings {
					if incoming.Block != p.currentPhiPred || incoming.Value == nil {
						continue
					}
					ref := p.edgeValueRefWithActive(incoming.Value, active)
					if ref != "" && ref != "%unknown" {
						p.recordBlockValueOverride(sig)
						p.valueNames[sig] = ref
						return ref
					}
				}
			}
			if len(op.Incomings) > 0 {
				currentVal := ""
				for i := len(op.Incomings) - 1; i >= 0; i-- {
					incoming := op.Incomings[i]
					if incoming.Value == nil {
						continue
					}
					incomingVal := p.edgeValueRefWithActive(incoming.Value, active)
					if incomingVal == "" || incomingVal == "%unknown" {
						continue
					}
					if currentVal == "" || incoming.Block == nil {
						currentVal = incomingVal
						continue
					}
					phiBlock := phiBlockForIncoming(op, incoming.Block)
					if incoming.Block == nil || phiBlock == nil {
						currentVal = incomingVal
						continue
					}
					termsCache := make(map[*ir.BasicBlock][]condTerm)
					terms := p.phiIncomingConditionTerms(incoming.Block, phiBlock, termsCache, make(map[*ir.BasicBlock]bool))
					condRef := p.emitCondTermsEdgeRef(terms, active)
					if condRef == "" || condRef == "%unknown" {
						currentVal = incomingVal
						continue
					}
					currentVal = p.cachedEdgeMux(condRef, incomingVal, currentVal, typeString(op.Dest.Type))
				}
				if currentVal != "" && currentVal != "%unknown" {
					p.recordBlockValueOverride(sig)
					p.valueNames[sig] = currentVal
					return currentVal
				}
			}
		case *ir.MuxOperation:
			if val, ok := signalBoolConst(op.Cond); ok {
				if val {
					return p.edgeValueRefWithActive(op.TrueValue, active)
				}
				return p.edgeValueRefWithActive(op.FalseValue, active)
			}
			cond := p.edgeValueRefWithActive(op.Cond, active)
			tVal := p.edgeValueRefWithActive(op.TrueValue, active)
			fVal := p.edgeValueRefWithActive(op.FalseValue, active)
			if cond != "" && tVal != "" && fVal != "" && cond != "%unknown" && tVal != "%unknown" && fVal != "%unknown" {
				name := p.cachedEdgeMux(cond, tVal, fVal, typeString(op.Dest.Type))
				p.recordBlockValueOverride(sig)
				p.valueNames[sig] = name
				return name
			}
		case *ir.ConvertOperation:
			value := p.edgeValueRefWithActive(op.Value, active)
			if value != "" && value != "%unknown" {
				converted := p.emitEdgeResolvedConvert(value, op.Value.Type, op.Dest.Type)
				p.recordBlockValueOverride(sig)
				p.valueNames[sig] = converted
				return converted
			}
		}
	}
	return p.valueRef(sig)
}

func (p *processPrinter) emitEdgeResolvedConvert(value string, srcType, destType *ir.SignalType) string {
	if p == nil || value == "" || value == "%unknown" {
		return value
	}
	srcWidth := signalWidth(srcType)
	destWidth := signalWidth(destType)
	if srcWidth <= 0 || destWidth <= 0 || srcWidth == destWidth {
		return value
	}
	from := typeString(srcType)
	to := typeString(destType)
	key := convertCacheKey{value: value, from: from, to: to}
	if cached, ok := p.convertCache[key]; ok && cached != "" {
		return cached
	}
	if destWidth > srcWidth {
		dest := p.freshValueName("edge_conv")
		p.recordBlockConvertOverride(key)
		p.convertCache[key] = dest
		p.printIndent()
		if srcType != nil && srcType.Signed {
			fmt.Fprintf(p.w, "%s = arith.extsi %s : %s to %s\n", dest, value, from, to)
			return dest
		}
		fmt.Fprintf(p.w, "%s = arith.extui %s : %s to %s\n", dest, value, from, to)
		return dest
	}
	dest := p.freshValueName("edge_conv")
	p.recordBlockConvertOverride(key)
	p.convertCache[key] = dest
	p.printIndent()
	fmt.Fprintf(p.w, "%s = arith.trunci %s : %s to %s\n", dest, value, from, to)
	return dest
}

func (p *processPrinter) tryFoldEdgeBinOp(op *ir.BinOperation, active map[*ir.Signal]bool) (string, bool) {
	if p == nil || op == nil {
		return "", false
	}
	switch op.Op {
	case ir.And:
		if val, ok := signalBoolConst(op.Left); ok {
			if val {
				return p.edgeValueRefWithActive(op.Right, active), true
			}
			return p.boolConst(false), true
		}
		if val, ok := signalBoolConst(op.Right); ok {
			if val {
				return p.edgeValueRefWithActive(op.Left, active), true
			}
			return p.boolConst(false), true
		}
	case ir.Or:
		if val, ok := signalBoolConst(op.Left); ok {
			if val {
				return p.boolConst(true), true
			}
			return p.edgeValueRefWithActive(op.Right, active), true
		}
		if val, ok := signalBoolConst(op.Right); ok {
			if val {
				return p.boolConst(true), true
			}
			return p.edgeValueRefWithActive(op.Left, active), true
		}
	case ir.Xor:
		if val, ok := signalBoolConst(op.Left); ok {
			if val {
				return "", false
			}
			return p.edgeValueRefWithActive(op.Right, active), true
		}
		if val, ok := signalBoolConst(op.Right); ok {
			if val {
				return "", false
			}
			return p.edgeValueRefWithActive(op.Left, active), true
		}
	}
	return "", false
}

func signalBoolConst(sig *ir.Signal) (bool, bool) {
	if sig == nil || sig.Kind != ir.Const || signalWidth(sig.Type) != 1 {
		return false, false
	}
	switch v := sig.Value.(type) {
	case bool:
		return v, true
	case int:
		return v != 0, true
	case int8:
		return v != 0, true
	case int16:
		return v != 0, true
	case int32:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint:
		return v != 0, true
	case uint8:
		return v != 0, true
	case uint16:
		return v != 0, true
	case uint32:
		return v != 0, true
	case uint64:
		return v != 0, true
	default:
		return false, false
	}
}

func (p *processPrinter) cachedExprValue(key exprCacheKey, prefix string, emit func(name string)) string {
	if p == nil {
		return ""
	}
	if cached, ok := p.exprCache[key]; ok && cached != "" {
		return cached
	}
	name := p.freshValueName(prefix)
	p.recordBlockExprOverride(key)
	p.exprCache[key] = name
	emit(name)
	return name
}

func normalizeExprOperands(a, b string, commutative bool) (string, string) {
	if !commutative || a <= b {
		return a, b
	}
	return b, a
}

func isCommutativeBinOp(op ir.BinOp) bool {
	switch op {
	case ir.Add, ir.Mul, ir.And, ir.Or, ir.Xor:
		return true
	default:
		return false
	}
}

func (p *processPrinter) cachedEdgeBinary(opName, left, right, typ, prefix string, commutative bool) string {
	left, right = normalizeExprOperands(left, right, commutative)
	key := exprCacheKey{kind: "bin:" + opName, a: left, b: right, typ: typ}
	return p.cachedExprValue(key, prefix, func(name string) {
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.%s %s, %s : %s\n", name, opName, left, right, typ)
	})
}

func (p *processPrinter) cachedEdgeCompare(predicate, left, right, typ string) string {
	if left == right {
		switch predicate {
		case "eq", "sle", "sge", "ule", "uge":
			return p.boolConst(true)
		case "ne", "slt", "sgt", "ult", "ugt":
			return p.boolConst(false)
		}
	}
	left, right = normalizeExprOperands(left, right, predicate == "eq" || predicate == "ne")
	key := exprCacheKey{kind: "icmp:" + predicate, a: left, b: right, typ: typ}
	return p.cachedExprValue(key, "edge_cmp", func(name string) {
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.icmp %s %s, %s : %s\n", name, predicate, left, right, typ)
	})
}

func (p *processPrinter) cachedEdgeMux(cond, tVal, fVal, typ string) string {
	if folded, ok := p.foldMuxRefs(cond, tVal, fVal); ok {
		return folded
	}
	key := exprCacheKey{kind: "mux", a: cond, b: tVal, c: fVal, typ: typ}
	return p.cachedExprValue(key, "edge_mux", func(name string) {
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.mux %s, %s, %s : %s\n", name, cond, tVal, fVal, typ)
	})
}

func (p *processPrinter) cachedEdgeNot(value, ones, typ string) string {
	key := exprCacheKey{kind: "not", a: value, b: ones, typ: typ}
	return p.cachedExprValue(key, "edge_not", func(name string) {
		p.printIndent()
		fmt.Fprintf(p.w, "%s = comb.xor %s, %s : %s\n", name, value, ones, typ)
	})
}

func (p *processPrinter) invertBoolRef(ref string) (string, bool) {
	if p == nil || ref == "" || ref == "%unknown" {
		return "", false
	}
	if ref == p.boolConst(true) {
		return p.boolConst(false), true
	}
	if ref == p.boolConst(false) {
		return p.boolConst(true), true
	}
	return "", false
}

func (p *processPrinter) foldBoolBinaryRefs(op, left, right string) (string, bool) {
	if p == nil || left == "" || right == "" || left == "%unknown" || right == "%unknown" {
		return "", false
	}
	trueRef := p.boolConst(true)
	falseRef := p.boolConst(false)
	switch op {
	case "and":
		switch {
		case left == falseRef || right == falseRef:
			return falseRef, true
		case left == trueRef:
			return right, true
		case right == trueRef:
			return left, true
		case left == right:
			return left, true
		}
	case "or":
		switch {
		case left == trueRef || right == trueRef:
			return trueRef, true
		case left == falseRef:
			return right, true
		case right == falseRef:
			return left, true
		case left == right:
			return left, true
		}
	}
	return "", false
}

func (p *processPrinter) emitCondTermsEdgeRef(terms []condTerm, active map[*ir.Signal]bool) string {
	if p == nil || len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 && len(terms[0]) == 0 {
		return p.boolConst(true)
	}
	termRefs := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) == 0 {
			return p.boolConst(true)
		}
		ref := ""
		for _, lit := range term {
			litRef := p.edgeValueRefWithActive(lit.sig, active)
			if litRef == "" || litRef == "%unknown" {
				return ""
			}
			if !lit.positive {
				if folded, ok := p.invertBoolRef(litRef); ok {
					litRef = folded
				} else {
					litRef = p.cachedEdgeNot(litRef, p.boolConst(true), "i1")
				}
			}
			if ref == "" {
				ref = litRef
				continue
			}
			if folded, ok := p.foldBoolBinaryRefs("and", ref, litRef); ok {
				ref = folded
			} else {
				ref = p.cachedEdgeBinary("and", ref, litRef, "i1", "edge_phi_and", true)
			}
		}
		if ref != "" {
			termRefs = append(termRefs, ref)
		}
	}
	if len(termRefs) == 0 {
		return ""
	}
	current := termRefs[0]
	for _, ref := range termRefs[1:] {
		if folded, ok := p.foldBoolBinaryRefs("or", current, ref); ok {
			current = folded
		} else {
			current = p.cachedEdgeBinary("or", current, ref, "i1", "edge_phi_or", true)
		}
	}
	return current
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

func (p *processPrinter) foldMuxRefs(cond, tVal, fVal string) (string, bool) {
	if tVal == "" || fVal == "" || tVal == "%unknown" || fVal == "%unknown" {
		return "", false
	}
	if tVal == fVal {
		return tVal, true
	}
	if cond == p.boolConst(true) {
		return tVal, true
	}
	if cond == p.boolConst(false) {
		return fVal, true
	}
	return "", false
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
	key := typeString(t)
	if name, ok := p.zeroConsts[key]; ok && name != "" {
		return name
	}
	name := p.freshValueName("c_zero")
	p.recordBlockZeroOverride(key)
	p.zeroConsts[key] = name
	p.printIndent()
	fmt.Fprintf(p.w, "%s = hw.constant 0 : %s\n", name, key)
	return name
}

func (p *processPrinter) typedAllOnesConst(t *ir.SignalType) string {
	if signalWidth(t) == 1 {
		return p.boolConst(true)
	}
	key := typeString(t)
	if name, ok := p.onesConsts[key]; ok && name != "" {
		return name
	}
	name := p.freshValueName("c_ones")
	p.recordBlockOnesOverride(key)
	p.onesConsts[key] = name
	p.printIndent()
	fmt.Fprintf(p.w, "%s = hw.constant -1 : %s\n", name, key)
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
	cacheKey := convertCacheKey{value: src, from: from, to: to}

	switch {
	case destWidth == srcWidth:
		// Sign-only reinterpretation: MLIR integer types are signless, so we can
		// alias the source value directly instead of emitting an explicit bitcast.
		p.valueNames[o.Dest] = src
		return
	case destWidth > srcWidth:
		if cached, ok := p.convertCache[cacheKey]; ok && cached != "" {
			p.valueNames[o.Dest] = cached
			return
		}
		p.recordBlockConvertOverride(cacheKey)
		p.convertCache[cacheKey] = dest
		if o.Value.Type != nil && o.Value.Type.Signed {
			p.printIndent()
			fmt.Fprintf(p.w, "%s = arith.extsi %s : %s to %s\n", dest, src, from, to)
		} else {
			p.printIndent()
			fmt.Fprintf(p.w, "%s = arith.extui %s : %s to %s\n", dest, src, from, to)
		}
	default:
		if cached, ok := p.convertCache[cacheKey]; ok && cached != "" {
			p.valueNames[o.Dest] = cached
			return
		}
		p.recordBlockConvertOverride(cacheKey)
		p.convertCache[cacheKey] = dest
		p.printIndent()
		fmt.Fprintf(p.w, "%s = arith.trunci %s : %s to %s\n", dest, src, from, to)
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
	clk := p.clockPortRef()
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
