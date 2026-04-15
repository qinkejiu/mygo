package ir

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// inferProcessSensitivity determines whether a process needs sequential or combinational logic.
// Loops alone are not enough to force sequential lowering; bounded combinational loops
// over local temporaries are common in the golden corpus. Sequential lowering should
// only be selected for truly stateful/control-driven behavior.
func (b *builder) inferProcessSensitivity(proc *Process) {
	if proc == nil {
		return
	}

	// Check if process has channel operations
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		for _, op := range block.Ops {
			switch op.(type) {
			case *SendOperation, *RecvOperation:
				proc.Sensitivity = Sequential
				b.markGlobalsAsRegisters()
				return
			}
		}
	}

	// Check if process is spawned (concurrent)
	if proc.Spawned {
		proc.Sensitivity = Sequential
		b.markGlobalsAsRegisters()
		return
	}

	// TopModule-style clock/reset control flow such as:
	//   if clk { q_reg = d }
	//   if reset { ... } else if clk { ... }
	// should lower as sequential logic even without loops or channels.
	if b.hasClockedGlobalAssignments(proc) {
		proc.Sensitivity = Sequential
		b.classifyGlobalSignals(proc)
		return
	}

	// Otherwise, it's combinational - mark globals as wires
	proc.Sensitivity = Combinational
	b.classifyGlobalSignals(proc)
	b.markIndexedLocalsAsWires()
	b.markIndexedOutputsAsCombinational()
}

func (b *builder) classifyGlobalSignals(proc *Process) {
	if b == nil || b.module == nil || proc == nil {
		return
	}
	if proc.Sensitivity == Sequential {
		clockedBlocks := b.computeDirectClockedBlocks(proc)
		for _, sig := range b.module.Signals {
			if sig == nil || sig.Kind == Const || !b.isGlobalPersistentSignal(sig) {
				continue
			}
			clockedAssign, nonClockedAssign := b.signalAssignmentKinds(proc, sig, clockedBlocks)
			if isOutputLikeSignal(sig.Name) && nonClockedAssign && !clockedAssign {
				sig.Kind = Wire
			} else {
				sig.Kind = Reg
			}
		}
		return
	}

	for _, sig := range b.module.Signals {
		if sig == nil || sig.Kind == Const || !b.isGlobalPersistentSignal(sig) {
			continue
		}
		base := b.globalForSignal(sig)
		if base != nil {
			if _, ok := b.latchGlobals[base]; ok {
				sig.Kind = Reg
			} else {
				sig.Kind = Wire
			}
		}
	}
}

func (b *builder) computeDirectClockedBlocks(proc *Process) map[*BasicBlock]bool {
	blocks := make(map[*BasicBlock]bool)
	if b == nil || proc == nil || len(proc.Blocks) == 0 {
		return blocks
	}
	type visitKey struct {
		block     *BasicBlock
		inClocked bool
	}
	visited := make(map[visitKey]bool)
	clockedReach := make(map[*BasicBlock]bool)
	nonClockedReach := make(map[*BasicBlock]bool)
	var visit func(block *BasicBlock, inClocked bool)
	visit = func(block *BasicBlock, inClocked bool) {
		if block == nil {
			return
		}
		key := visitKey{block: block, inClocked: inClocked}
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
		case *BranchTerminator:
			if term.Cond != nil && isClockLikeName(term.Cond.Name) {
				visit(term.True, true)
				visit(term.False, false)
				return
			}
			if term.Cond != nil && isResetLikeName(term.Cond.Name) && block == proc.Blocks[0] {
				visit(term.True, true)
				visit(term.False, false)
				return
			}
			visit(term.True, inClocked)
			visit(term.False, inClocked)
		case *JumpTerminator:
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

func (b *builder) signalAssignmentKinds(proc *Process, sig *Signal, clockedBlocks map[*BasicBlock]bool) (bool, bool) {
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
			assign, ok := op.(*AssignOperation)
			if !ok || assign == nil || assign.Dest == nil {
				continue
			}
			if !sameSignal(assign.Dest, sig) {
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

func (b *builder) isGlobalPersistentSignal(sig *Signal) bool {
	if b == nil || sig == nil {
		return false
	}
	return b.isGlobalStorageSignal(sig) || b.globalIndexedBaseForSignal(sig) != nil
}

func (b *builder) globalForSignal(sig *Signal) *ssa.Global {
	if b == nil || sig == nil {
		return nil
	}
	for g, storage := range b.globalStorage {
		if g == nil || storage == nil {
			continue
		}
		if signalMatchesStorage(sig, storage) {
			return g
		}
	}
	return b.globalIndexedBaseForSignal(sig)
}

func (b *builder) globalIndexedBaseForSignal(sig *Signal) *ssa.Global {
	if b == nil || sig == nil {
		return nil
	}
	for _, state := range b.indexedBases {
		if state == nil {
			continue
		}
		g, ok := state.base.(*ssa.Global)
		if !ok || g == nil {
			continue
		}
		for _, storage := range state.storage {
			if sameSignal(storage, sig) {
				return g
			}
		}
	}
	return nil
}

func isOutputLikeSignal(name string) bool {
	if strings.HasPrefix(name, "out_") {
		return true
	}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] != '_' {
			continue
		}
		if i == len(name)-1 {
			return false
		}
		for _, ch := range name[i+1:] {
			if ch < '0' || ch > '9' {
				return false
			}
		}
		return strings.HasPrefix(name[:i], "out_")
	}
	return false
}

func (b *builder) hasClockedGlobalAssignments(proc *Process) bool {
	if b == nil || proc == nil {
		return false
	}

	controlSignals := make(map[*Signal]struct{})
	for _, param := range proc.Params {
		if param == nil {
			continue
		}
		if isClockLikeName(param.Name) || isResetLikeName(param.Name) {
			controlSignals[param] = struct{}{}
		}
	}
	if len(controlSignals) == 0 {
		return false
	}

	changed := true
	for changed {
		changed = false
		for _, block := range proc.Blocks {
			if block == nil {
				continue
			}
			for _, op := range block.Ops {
				switch typed := op.(type) {
				case *NotOperation:
					if typed == nil || typed.Value == nil || typed.Dest == nil {
						continue
					}
					if _, ok := controlSignals[typed.Value]; ok {
						if _, exists := controlSignals[typed.Dest]; !exists {
							controlSignals[typed.Dest] = struct{}{}
							changed = true
						}
					}
				case *PhiOperation:
					if typed == nil || typed.Dest == nil {
						continue
					}
					for _, incoming := range typed.Incomings {
						if incoming.Value == nil {
							continue
						}
						if _, ok := controlSignals[incoming.Value]; ok {
							if _, exists := controlSignals[typed.Dest]; !exists {
								controlSignals[typed.Dest] = struct{}{}
								changed = true
							}
							break
						}
					}
				}
			}
		}
	}

	hasControlBranch := false
	hasGlobalAssign := false
	for _, block := range proc.Blocks {
		if block == nil {
			continue
		}
		if branch, ok := block.Terminator.(*BranchTerminator); ok && branch != nil && branch.Cond != nil {
			if _, isControl := controlSignals[branch.Cond]; isControl {
				hasControlBranch = true
			}
		}
		for _, op := range block.Ops {
			assign, ok := op.(*AssignOperation)
			if !ok || assign == nil || assign.Dest == nil {
				continue
			}
			if b.isGlobalStorageSignal(assign.Dest) || strings.HasPrefix(assign.Dest.Name, "out_") {
				hasGlobalAssign = true
			}
		}
	}

	return hasControlBranch && hasGlobalAssign
}

func (b *builder) isGlobalStorageSignal(sig *Signal) bool {
	if b == nil || sig == nil {
		return false
	}
	for _, storageSig := range b.globalStorage {
		if storageSig == sig || signalMatchesStorage(sig, storageSig) {
			return true
		}
	}
	return false
}

func signalMatchesStorage(sig *Signal, storageSig *Signal) bool {
	if sig == nil || storageSig == nil {
		return false
	}
	if sig == storageSig {
		return true
	}
	if sig.Name == "" || storageSig.Name == "" {
		return false
	}
	prefix := storageSig.Name + "_"
	return strings.HasPrefix(sig.Name, prefix)
}

func isClockLikeName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "clk", "clock":
		return true
	default:
		return false
	}
}

func isResetLikeName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rst", "reset", "areset", "resetn", "aresetn":
		return true
	default:
		return false
	}
}

// markGlobalsAsRegisters marks global storage signals as registers
// Only marks signals that correspond to actual global variables, not temporary signals
func (b *builder) markGlobalsAsRegisters() {
	if b == nil || b.module == nil {
		return
	}
	for _, sig := range b.module.Signals {
		if sig == nil || sig.Kind == Const {
			continue
		}
		// Only mark signals that are actual global storage (from globalStorage map)
		// Skip temporary signals like loadsnap, idxload, etc.
		isGlobalStorage := false
		for _, storageSig := range b.globalStorage {
			if signalMatchesStorage(sig, storageSig) {
				isGlobalStorage = true
				break
			}
		}
		if isGlobalStorage {
			sig.Kind = Reg
		}
	}
}

// markGlobalsAsWires marks global storage signals as wires
// Only marks signals that correspond to actual global variables, not temporary signals
func (b *builder) markGlobalsAsWires() {
	if b == nil || b.module == nil {
		return
	}
	for _, sig := range b.module.Signals {
		if sig == nil || sig.Kind == Const {
			continue
		}
		// Only mark signals that are actual global storage (from globalStorage map)
		// Skip temporary signals like loadsnap, idxload, etc.
		isGlobalStorage := false
		for _, storageSig := range b.globalStorage {
			if signalMatchesStorage(sig, storageSig) {
				isGlobalStorage = true
				break
			}
		}
		if isGlobalStorage {
			sig.Kind = Wire
		}
	}
}

func (b *builder) markIndexedLocalsAsWires() {
	if b == nil {
		return
	}
	for _, state := range b.indexedBases {
		if state == nil || state.base == nil {
			continue
		}
		if _, isGlobal := state.base.(*ssa.Global); isGlobal {
			continue
		}
		for _, sig := range state.storage {
			if sig == nil || sig.Kind == Const {
				continue
			}
			sig.Kind = Wire
		}
	}
}

func (b *builder) markIndexedOutputsAsCombinational() {
	if b == nil {
		return
	}
	for _, state := range b.indexedBases {
		if state == nil {
			continue
		}
		g, ok := state.base.(*ssa.Global)
		if !ok || g == nil || !strings.HasPrefix(g.Name(), "out_") {
			continue
		}
		for idx, sig := range state.storage {
			if sig == nil {
				continue
			}
			sig.Kind = Wire
			if current, ok := state.elements[idx]; !ok || current == nil || current == sig {
				state.elements[idx] = b.newConstSignal(0, sig.Type, g.Pos())
			}
		}
	}
}
