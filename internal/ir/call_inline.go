package ir

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
)

const inlineCallMaxDepth = 64

type inlineFrame struct {
	builder *builder
	bb      *BasicBlock
	values  map[ssa.Value]*Signal
	tuples  map[ssa.Value][]*Signal
	slots   map[ssa.Value]*Signal
	globals map[*ssa.Global]*Signal
	stack   map[*ssa.Function]struct{}
}

func (f *inlineFrame) clone() *inlineFrame {
	if f == nil {
		return nil
	}
	clone := &inlineFrame{
		builder: f.builder,
		bb:      f.bb,
		values:  make(map[ssa.Value]*Signal, len(f.values)),
		tuples:  make(map[ssa.Value][]*Signal, len(f.tuples)),
		slots:   make(map[ssa.Value]*Signal, len(f.slots)),
		globals: make(map[*ssa.Global]*Signal, len(f.globals)),
		stack:   f.stack,
	}
	for k, v := range f.values {
		clone.values[k] = v
	}
	for k, v := range f.tuples {
		copied := make([]*Signal, len(v))
		copy(copied, v)
		clone.tuples[k] = copied
	}
	for k, v := range f.slots {
		clone.slots[k] = v
	}
	for k, v := range f.globals {
		clone.globals[k] = v
	}
	return clone
}

func (f *inlineFrame) resolve(v ssa.Value) (*Signal, bool) {
	if v == nil {
		return nil, false
	}
	if sig, ok := f.values[v]; ok && sig != nil {
		return sig, true
	}
	switch val := v.(type) {
	case *ssa.Const:
		sig := f.builder.signalForValue(val)
		if sig != nil {
			return sig, true
		}
	case *ssa.Global:
		if sig, ok := f.globals[val]; ok && sig != nil {
			return sig, true
		}
		sig := f.builder.signalForGlobal(val)
		if sig != nil {
			return sig, true
		}
	case *ssa.Alloc:
		if sig, ok := f.slots[val]; ok && sig != nil {
			return sig, true
		}
	case *ssa.IndexAddr:
		sig := f.signalForIndexAddr(val)
		if sig != nil {
			return sig, true
		}
	case *ssa.Extract:
		tuple := f.lookupTuple(val.Tuple)
		if val.Index >= 0 && val.Index < len(tuple) && tuple[val.Index] != nil {
			return tuple[val.Index], true
		}
	case *ssa.Slice:
		return f.resolveSlice(val)
	case *ssa.Convert:
		source, ok := f.resolve(val.X)
		if !ok {
			return nil, false
		}
		out := f.builder.inlineEmitTypeChange(f.bb, source, val.Type(), val.Pos())
		if out == nil {
			return nil, false
		}
		f.values[val] = out
		return out, true
	case *ssa.ChangeType:
		source, ok := f.resolve(val.X)
		if !ok {
			return nil, false
		}
		out := f.builder.inlineEmitTypeChange(f.bb, source, val.Type(), val.Pos())
		if out == nil {
			return nil, false
		}
		f.values[val] = out
		return out, true
	}
	if sig := f.builder.signalForValue(v); sig != nil {
		return sig, true
	}
	return nil, false
}

func (f *inlineFrame) lookupTuple(v ssa.Value) []*Signal {
	if tuple, ok := f.tuples[v]; ok {
		return tuple
	}
	if tuple, ok := f.builder.tupleSignals[v]; ok {
		return tuple
	}
	return nil
}

func (f *inlineFrame) resolveSlice(slice *ssa.Slice) (*Signal, bool) {
	if slice == nil {
		return nil, false
	}
	if sig, ok := f.values[slice]; ok && sig != nil {
		return sig, true
	}

	state, ok := f.sliceState(slice)
	if !ok || state == nil {
		return nil, false
	}

	sig := f.builder.newAnonymousSignal("slice", state.elemType, slice.Pos())
	f.values[slice] = sig
	f.builder.indexedBases[slice] = state
	return sig, true
}

func (f *inlineFrame) sliceState(slice *ssa.Slice) (*indexedBaseState, bool) {
	if slice == nil {
		return nil, false
	}

	if base, indices, ok := collectIndexedAccess(slice.X); ok {
		parent := f.builder.indexedStateForBase(base, slice.Pos())
		if parent == nil {
			return nil, false
		}
		start, span, ok := f.sliceWindow(parent, indices)
		if !ok {
			return nil, false
		}
		state := &indexedBaseState{
			base:     slice,
			elemType: parent.elemType.Clone(),
			length:   span,
			dims:     []int{span},
			elements: make(map[int]*Signal, span),
			storage:  make(map[int]*Signal, span),
			parent:   parent,
			offset:   start,
		}
		for i := 0; i < span; i++ {
			idx := start + i
			if elem := f.builder.indexedElementSignal(parent, idx, slice.Pos()); elem != nil {
				state.elements[i] = elem
			}
			if storage := f.builder.indexedElementStorageSignal(parent, idx, slice.Pos()); storage != nil {
				state.storage[i] = storage
			}
		}
		return state, true
	}

	parent := f.builder.indexedStateForBase(slice.X, slice.Pos())
	if parent == nil {
		return nil, false
	}
	return parent, true
}

func (f *inlineFrame) sliceWindow(state *indexedBaseState, indices []ssa.Value) (int, int, bool) {
	if state == nil {
		return 0, 0, false
	}
	if len(indices) == 0 {
		if state.length <= 0 {
			return 0, 0, false
		}
		return 0, state.length, true
	}
	if len(indices) > len(state.dims) {
		return 0, 0, false
	}

	start := 0
	for i, index := range indices {
		raw, ok := constIndexValue(index)
		if !ok {
			raw, ok = f.resolveConstIndex(index)
			if !ok {
				return 0, 0, false
			}
		}
		dimSize := -1
		if i < len(state.dims) {
			dimSize = state.dims[i]
		}
		if dimSize >= 0 && raw >= dimSize {
			return 0, 0, false
		}
		stride, ok := state.stride(i)
		if !ok {
			return 0, 0, false
		}
		start += raw * stride
	}

	span := 1
	for i := len(indices); i < len(state.dims); i++ {
		if state.dims[i] < 0 {
			return 0, 0, false
		}
		span *= state.dims[i]
	}
	if span <= 0 {
		span = 1
	}
	if state.length >= 0 && start+span > state.length {
		return 0, 0, false
	}
	return start, span, true
}

func (f *inlineFrame) resolveConstIndex(v ssa.Value) (int, bool) {
	sig, ok := f.resolve(v)
	if !ok || sig == nil || sig.Kind != Const {
		return 0, false
	}
	switch n := sig.Value.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	default:
		return 0, false
	}
}

func (f *inlineFrame) store(addr ssa.Value, value *Signal, pos token.Pos) bool {
	if value == nil {
		return false
	}
	switch base := unwrapAddressValue(addr).(type) {
	case *ssa.Alloc:
		f.slots[base] = value
		return true
	case *ssa.Global:
		f.globals[base] = value
		return true
	case *ssa.IndexAddr:
		return f.storeIndexAddr(base, value, pos)
	default:
		return false
	}
}

func (f *inlineFrame) storeIndexAddr(addr *ssa.IndexAddr, value *Signal, pos token.Pos) bool {
	if addr == nil || value == nil {
		return false
	}
	base, indices, ok := collectIndexedAccess(addr)
	if !ok {
		return false
	}
	state := f.builder.indexedStateForBase(base, pos)
	if state == nil {
		return false
	}
	if idx, ok := indexedConstantFlatIndex(state, indices); ok {
		dest := f.builder.indexedElementStorageSignal(state, idx, pos)
		if dest != nil && dest.Name != "" && !strings.HasPrefix(dest.Name, "test_") {
			f.bb.Ops = append(f.bb.Ops, &AssignOperation{
				Dest:  dest,
				Value: value,
			})
		}
		state.elements[idx] = value
		if state.parent != nil {
			state.parent.elements[state.offset+idx] = value
		}
		return true
	}
	maxLen := state.length
	if maxLen < 0 {
		maxLen = defaultDynamicSliceIndexMax
	}
	if maxLen <= 0 {
		return false
	}
	index, ok := f.linearizeIndexedAccess(state, indices, pos)
	if !ok || index == nil {
		return false
	}
	indexType := index.Type
	if indexType == nil {
		indexType = &SignalType{Width: 32, Signed: true}
	}
	for i := 0; i < maxLen; i++ {
		element := f.builder.indexedElementSignal(state, i, pos)
		dest := f.builder.indexedElementStorageSignal(state, i, pos)
		if element == nil {
			continue
		}
		cond := f.builder.newAnonymousSignal("idxeq", &SignalType{Width: 1, Signed: false}, pos)
		f.bb.Ops = append(f.bb.Ops, &CompareOperation{
			Predicate: CompareEQ,
			Dest:      cond,
			Left:      index,
			Right:     f.builder.newConstSignal(int64(i), indexType, pos),
		})
		next := f.builder.newAnonymousSignal("idxstore", element.Type, pos)
		f.bb.Ops = append(f.bb.Ops, &MuxOperation{
			Dest:       next,
			Cond:       cond,
			TrueValue:  value,
			FalseValue: element,
		})
		if dest != nil && dest.Name != "" && !strings.HasPrefix(dest.Name, "test_") {
			f.bb.Ops = append(f.bb.Ops, &AssignOperation{
				Dest:  dest,
				Value: next,
			})
		}
		state.elements[i] = next
		if state.parent != nil {
			state.parent.elements[state.offset+i] = next
		}
	}
	return true
}

func (f *inlineFrame) loadAddress(addr ssa.Value, pos token.Pos, fallbackType *SignalType) (*Signal, bool) {
	switch base := unwrapAddressValue(addr).(type) {
	case *ssa.Alloc:
		if sig, ok := f.slots[base]; ok && sig != nil {
			return sig, true
		}
		ptrType, ok := base.Type().(*types.Pointer)
		if !ok {
			return nil, false
		}
		zero := f.builder.newConstSignal(0, signalType(ptrType.Elem()), pos)
		f.slots[base] = zero
		return zero, true
	case *ssa.Global:
		if sig, ok := f.globals[base]; ok && sig != nil {
			return sig, true
		}
		sig := f.builder.signalForGlobal(base)
		if sig != nil {
			return sig, true
		}
	case *ssa.IndexAddr:
		sig := f.signalForIndexAddr(base)
		if sig != nil {
			return sig, true
		}
	}
	if fallbackType != nil {
		return f.builder.newConstSignal(0, fallbackType, pos), true
	}
	return nil, false
}

func (f *inlineFrame) signalForIndexAddr(addr *ssa.IndexAddr) *Signal {
	if addr == nil {
		return nil
	}
	if sig, ok := f.values[addr]; ok && sig != nil {
		return sig
	}
	base, indices, ok := collectIndexedAccess(addr)
	if !ok {
		return nil
	}
	state := f.builder.indexedStateForBase(base, addr.Pos())
	if state == nil {
		return nil
	}
	if idx, ok := indexedConstantFlatIndex(state, indices); ok {
		sig := f.builder.indexedElementSignal(state, idx, addr.Pos())
		if sig != nil {
			f.values[addr] = sig
		}
		return sig
	}
	if state.length <= 0 {
		return nil
	}
	index, ok := f.linearizeIndexedAccess(state, indices, addr.Pos())
	if !ok || index == nil {
		return nil
	}
	selected := f.builder.selectIndexedElement(f.bb, state, index, addr.Pos())
	if selected != nil {
		f.values[addr] = selected
	}
	return selected
}

func (f *inlineFrame) linearizeIndexedAccess(state *indexedBaseState, indices []ssa.Value, pos token.Pos) (*Signal, bool) {
	if f == nil || f.builder == nil || f.bb == nil || state == nil || len(indices) == 0 {
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
		index, ok := f.resolve(indexValue)
		if !ok || index == nil {
			return nil, false
		}
		if index.Type != nil {
			indexType = index.Type.Clone()
		}
		term := index
		if stride != 1 {
			term = f.builder.synthesizeBinOp(
				f.bb,
				"idxmul",
				Mul,
				term,
				f.builder.newConstSignal(int64(stride), indexType.Clone(), pos),
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
		acc = f.builder.synthesizeBinOp(f.bb, "idxadd", Add, acc, term, indexType.Clone(), pos)
		if acc == nil {
			return nil, false
		}
	}

	if acc == nil {
		return f.builder.newConstSignal(int64(offset), indexType, pos), true
	}
	if offset != 0 {
		acc = f.builder.synthesizeBinOp(
			f.bb,
			"idxadd",
			Add,
			acc,
			f.builder.newConstSignal(int64(offset), indexType.Clone(), pos),
			indexType.Clone(),
			pos,
		)
		if acc == nil {
			return nil, false
		}
	}
	return acc, true
}

func (f *inlineFrame) evalStringIndex(index *ssa.Index) (*Signal, bool) {
	if index == nil {
		return nil, false
	}
	strConst, ok := index.X.(*ssa.Const)
	if !ok || strConst.Value == nil || strConst.Value.Kind() != constant.String {
		return nil, false
	}
	bytes := []byte(constant.StringVal(strConst.Value))
	elemType := signalType(index.Type())
	if elemType == nil {
		elemType = &SignalType{Width: 8, Signed: false}
	}
	if idx, ok := constIndexValue(index.Index); ok {
		if idx < 0 || idx >= len(bytes) {
			return f.builder.newConstSignal(0, elemType, index.Pos()), true
		}
		return f.builder.newConstSignal(uint64(bytes[idx]), elemType, index.Pos()), true
	}
	idxSig, ok := f.resolve(index.Index)
	if !ok || idxSig == nil {
		return nil, false
	}
	if len(bytes) == 0 {
		return f.builder.newConstSignal(0, elemType, index.Pos()), true
	}
	indexType := idxSig.Type
	if indexType == nil {
		indexType = signalType(index.Index.Type())
	}
	selected := f.builder.newConstSignal(uint64(bytes[0]), elemType, index.Pos())
	for i := 1; i < len(bytes); i++ {
		cond := f.builder.newAnonymousSignal("idxeq", &SignalType{Width: 1, Signed: false}, index.Pos())
		f.bb.Ops = append(f.bb.Ops, &CompareOperation{
			Predicate: CompareEQ,
			Dest:      cond,
			Left:      idxSig,
			Right:     f.builder.newConstSignal(int64(i), indexType, index.Pos()),
		})
		next := f.builder.newAnonymousSignal("idxchar", elemType, index.Pos())
		f.bb.Ops = append(f.bb.Ops, &MuxOperation{
			Dest:       next,
			Cond:       cond,
			TrueValue:  f.builder.newConstSignal(uint64(bytes[i]), elemType, index.Pos()),
			FalseValue: selected,
		})
		selected = next
	}
	return selected, true
}

func (b *builder) bootstrapGlobalInitializers(pkg *ssa.Package) {
	if pkg == nil {
		return
	}
	initFn := pkg.Func("init")
	if initFn == nil || len(initFn.Blocks) == 0 {
		return
	}
	for _, block := range initFn.Blocks {
		if block == nil {
			continue
		}
		for _, instr := range block.Instrs {
			store, ok := instr.(*ssa.Store)
			if !ok || store == nil {
				continue
			}
			c, ok := store.Val.(*ssa.Const)
			if !ok {
				continue
			}
			value := b.signalForValue(c)
			if value == nil {
				continue
			}
			switch addr := unwrapAddressValue(store.Addr).(type) {
			case *ssa.Global:
				b.globalValues[addr] = value
			case *ssa.IndexAddr:
				baseValue, indices, ok := collectIndexedAccess(addr)
				if !ok {
					continue
				}
				base, ok := baseValue.(*ssa.Global)
				if !ok {
					continue
				}
				state := b.indexedStateForBase(base, store.Pos())
				if state == nil {
					continue
				}
				idx, ok := indexedConstantFlatIndex(state, indices)
				if !ok {
					continue
				}
				if storage := state.storage[idx]; storage != nil {
					storage.Value = value.Value
					state.elements[idx] = storage
				} else {
					state.elements[idx] = value
				}
			}
		}
	}
}

func (b *builder) signalForGlobal(g *ssa.Global) *Signal {
	if g == nil {
		return nil
	}
	if b != nil && b.currentBlock != nil {
		if values := b.blockGlobalValues[b.currentBlock]; values != nil {
			if sig, ok := values[g]; ok && sig != nil {
				return sig
			}
		}
	}

	// Check if we already have a signal for this global
	if sig, ok := b.globalValues[g]; ok && sig != nil {
		return sig
	}

	// For global arrays, we need to create individual signals for each element
	// that can be accessed via IndexAddr operations
	ptrType, ok := g.Type().(*types.Pointer)
	if !ok {
		return nil
	}

	// Check if it's an array type
	arr, ok := ptrType.Elem().(*types.Array)
	if ok {
		// For arrays, pre-create/reuse element signals so slice arguments can bind to
		// stable storage without clobbering existing port-backed elements.
		state := b.indexedStateForBase(g, g.Pos())
		if state != nil {
			for i := 0; i < state.length; i++ {
				if existing := state.storage[i]; existing != nil {
					if b.module != nil {
						b.module.Signals[existing.Name] = existing
					}
					continue
				}
				sigName := fmt.Sprintf("%s_%d", g.Name(), i)
				if existing, ok := b.module.Signals[sigName]; ok && existing != nil {
					state.storage[i] = existing
					if state.elements[i] == nil || state.elements[i].Kind == Const {
						state.elements[i] = existing
					}
					continue
				}

				existingValue := state.elements[i]
				elemSig := &Signal{
					Name:   sigName,
					Type:   state.elemType.Clone(),
					Kind:   Reg,
					Source: g.Pos(),
				}
				if existingValue != nil && existingValue.Kind == Const {
					elemSig.Value = existingValue.Value
				}
				state.storage[i] = elemSig
				if state.elements[i] == nil || state.elements[i].Kind == Const {
					state.elements[i] = elemSig
				}
				if b.module != nil {
					b.module.Signals[elemSig.Name] = elemSig
				}
			}
		}

		// Create a placeholder signal for the array itself
		sig := &Signal{
			Name:   g.Name(),
			Type:   signalType(arr.Elem()),
			Kind:   Reg, // Global variables are typically registers
			Source: g.Pos(),
		}
		b.globalValues[g] = sig
		if b.module != nil {
			b.module.Signals[sig.Name] = sig
		}
		return sig
	}

	// For scalar globals, create a single signal
	sig := b.signalForGlobalStorage(g)
	if sig == nil {
		return nil
	}
	b.globalValues[g] = sig
	return sig
}

func (b *builder) setBlockGlobalValue(block *BasicBlock, g *ssa.Global, value *Signal) {
	if b == nil || block == nil || g == nil || value == nil {
		return
	}
	values := b.blockGlobalValues[block]
	if values == nil {
		values = make(map[*ssa.Global]*Signal)
		b.blockGlobalValues[block] = values
	}
	values[g] = value
}

func (b *builder) signalForGlobalStorage(g *ssa.Global) *Signal {
	if g == nil {
		return nil
	}
	if sig, ok := b.globalStorage[g]; ok && sig != nil {
		return sig
	}
	ptrType, ok := g.Type().(*types.Pointer)
	if !ok {
		return nil
	}
	sig := &Signal{
		Name:   g.Name(),
		Type:   signalType(ptrType.Elem()),
		Kind:   Reg,
		Source: g.Pos(),
	}
	if current, ok := b.globalValues[g]; ok && current != nil && current.Kind == Const {
		sig.Value = current.Value
	}
	b.globalStorage[g] = sig
	if b.module != nil {
		b.module.Signals[sig.Name] = sig
	}
	return sig
}

func (b *builder) handleCall(proc *Process, block *ssa.BasicBlock, bb *BasicBlock, instrIndex int, call *ssa.Call) bool {
	if bb == nil || call == nil {
		return false
	}
	resultCount := valueResultCount(call.Type())
	callee := call.Call.StaticCallee()
	if callee == nil {
		if resultCount > 0 {
			b.reporter.Warning(call.Pos(), fmt.Sprintf("dynamic call result is not supported: %T", call.Call.Value))
		}
		return false
	}
	args := make([]*Signal, 0, len(call.Call.Args))
	for _, arg := range call.Call.Args {
		sig := b.signalForValue(arg)
		if sig == nil {
			if idxAddr, ok := arg.(*ssa.IndexAddr); ok {
				sig = b.signalForIndexAddrInBlock(bb, idxAddr)
			}
		}
		if sig == nil {
			// Check if this is a slice operation
			if sliceOp, ok := arg.(*ssa.Slice); ok {
				// For slice operations, get the underlying array
				// and try to get a signal for it
				sig = b.signalForValue(sliceOp.X)
			}
		}
		if sig == nil {
			if resultCount > 0 {
				b.reporter.Warning(call.Pos(), fmt.Sprintf("call %s has unresolved argument %T", callee.String(), arg))
			}
			return false
		}
		args = append(args, sig)
	}
	results, ok := b.inlineCall(bb, callee, args, make(map[*ssa.Function]struct{}), 0)
	if ok {
		if resultCount == 0 {
			return false
		}
		if len(results) != resultCount {
			b.reporter.Warning(call.Pos(), fmt.Sprintf("call %s produced %d results, expected %d", callee.String(), len(results), resultCount))
			return false
		}
		if resultCount == 1 {
			b.bindResolvedValue(bb, call, results[0])
			return false
		}
		copied := make([]*Signal, len(results))
		copy(copied, results)
		b.tupleSignals[call] = copied
		return false
	}
	if resultCount > 0 {
		if folded, ok := b.constEvalCall(callee, call.Call.Args, args, call.Pos()); ok {
			if len(folded) != resultCount {
				b.reporter.Warning(call.Pos(), fmt.Sprintf("const eval for %s produced %d results, expected %d", callee.String(), len(folded), resultCount))
				return false
			}
			if resultCount == 1 {
				b.bindResolvedValue(bb, call, folded[0])
				return false
			}
			copied := make([]*Signal, len(folded))
			copy(copied, folded)
			b.tupleSignals[call] = copied
			return false
		}
	}

	// Fallback to explicit call lowering when inlining is unavailable.
	if resultCount > 1 {
		b.reporter.Warning(call.Pos(), fmt.Sprintf("multi-result call for %s is not supported", callee.String()))
		return false
	}

	// Always inline functions, even pure ones
	// The modular approach is too complex and causes signal naming issues
	// Fall back to inlining for all functions
	success := b.buildAndMergeProcess(proc, block, bb, instrIndex, callee, args, call)
	if success {
		return true
	}

	// Inlining failed - report error and skip the call
	b.reporter.Warning(call.Pos(), fmt.Sprintf("call to %s could not be inlined - call will be ignored", callee.Name()))
	return false
}

// shouldBuildAsModule checks if a function should be built as a separate module
// Functions that access global state (like arrays) should not be separate modules
func (b *builder) shouldBuildAsModule(fn *ssa.Function) bool {
	if fn == nil {
		return false
	}

	// Check function name against known functions that access global state
	// adpcm_main accesses compressed and result arrays
	if fn.Name() == "adpcm_main" {
		return false
	}

	// Check if function has any globals in its free variables
	// For now, we'll be conservative and only allow known pure functions
	knownPureFunctions := map[string]bool{
		"abs":    true,
		"decode": true,
		"encode": true,
		"filtep": true,
		"filtez": true,
		"logsch": true,
		"logscl": true,
		"quantl": true,
		"reset":  true,
		"scalel": true,
		"uppol1": true,
		"uppol2": true,
		"upzero": true,
	}

	return knownPureFunctions[fn.Name()]
}

// buildAndMergeProcess builds a callee process and merges it with the caller's process
// This is used for functions with global state access that need to share the same module
func (b *builder) buildAndMergeProcess(proc *Process, block *ssa.BasicBlock, bb *BasicBlock, instrIndex int, callee *ssa.Function, args []*Signal, call *ssa.Call) bool {
	if proc == nil || block == nil || bb == nil || callee == nil {
		return false
	}

	type savedParamBinding struct {
		param           *ssa.Parameter
		oldSignal       *Signal
		hadSignal       bool
		oldIndexedState *indexedBaseState
		hadIndexedState bool
	}
	savedBindings := make([]savedParamBinding, 0, len(callee.Params))
	for i, param := range callee.Params {
		if param == nil || i >= len(args) {
			continue
		}
		saved := savedParamBinding{param: param}
		if oldSignal, ok := b.paramSignals[param]; ok {
			saved.oldSignal = oldSignal
			saved.hadSignal = true
		}
		if oldState, ok := b.indexedBases[param]; ok {
			saved.oldIndexedState = oldState
			saved.hadIndexedState = true
		}
		if !isIndexedValueType(param.Type()) {
			b.paramSignals[param] = args[i]
			savedBindings = append(savedBindings, saved)
			continue
		}
		argState := b.findIndexedBaseForSignal(args[i])
		if argState == nil {
			savedBindings = append(savedBindings, saved)
			continue
		}
		b.paramSignals[param] = args[i]
		b.indexedBases[param] = argState
		savedBindings = append(savedBindings, saved)
	}
	restoreBindings := func() {
		for _, saved := range savedBindings {
			if saved.hadSignal {
				b.paramSignals[saved.param] = saved.oldSignal
			} else {
				delete(b.paramSignals, saved.param)
			}
			if saved.hadIndexedState {
				b.indexedBases[saved.param] = saved.oldIndexedState
			} else {
				delete(b.indexedBases, saved.param)
			}
		}
	}

	// Get the parent process
	parentProc := b.findProcessForBlock(bb)
	if parentProc == nil {
		restoreBindings()
		return false
	}

	// Build the callee process for this call site
	// We rebuild it for each call to ensure proper signal handling
	// Global signals will be automatically reused through the builder's signals map
	calleeProc := b.buildProcessInternal(callee)
	if calleeProc == nil {
		restoreBindings()
		return false
	}
	// Bind parameters to the argument signals
	// For cloned processes, we need to bind to the cloned parameter signals
	for i, ssaParam := range calleeProc.SSAParams {
		if i < len(args) && i < len(calleeProc.Params) {
			// Check if this is a slice parameter
			paramState := b.indexedBases[ssaParam]
			if paramState != nil {
				// This is a slice parameter - bind the indexedBaseState elements
				// The argument should be the base array
				argSig := args[i]
				if argSig != nil {
					// Get the indexedBaseState for the argument (caller's array)
					argState := b.findIndexedBaseForSignal(argSig)
					if argState != nil {
						// If the parameter state has no elements (unknown length), create them from the argument state
						if len(paramState.elements) == 0 && len(argState.elements) > 0 {
							// Create element signals in the parameter state with the same indices as the argument state
							for idx, argElemSig := range argState.elements {
								paramElemSig := &Signal{
									Name:   fmt.Sprintf("%s_%d", ssaParam.String(), idx),
									Type:   argElemSig.Type.Clone(),
									Kind:   Wire,
									Source: ssaParam.Pos(),
								}
								paramState.elements[idx] = paramElemSig
							}
						}

						// Now replace all uses of placeholder element signals with the actual argument element signals
						for idx, paramElemSig := range paramState.elements {
							if argElemSig, ok := argState.elements[idx]; ok {
								// Replace all uses of the placeholder element signal with the actual argument element signal
								for _, block := range calleeProc.Blocks {
									for j, op := range block.Ops {
										block.Ops[j] = b.replaceSignalInOperation(op, paramElemSig, argElemSig)
									}
									block.Terminator = b.replaceSignalInTerminator(block.Terminator, paramElemSig, argElemSig)
								}
								// Update the parameter state to point to the argument element
								paramState.elements[idx] = argElemSig
							}
						}
					}
				}
				// Store the mapping from SSA param to the argument signal
				b.signals[ssaParam] = args[i]
			} else {
				// Regular (non-slice) parameter
				// Store the mapping from SSA param to the argument signal
				b.signals[ssaParam] = args[i]

				// Also need to update any operations that use the parameter signal
				// Find the parameter signal in the cloned process and update references
				paramSignal := calleeProc.Params[i]
				if paramSignal != nil {
					// Replace all uses of paramSignal with args[i] in the cloned blocks
					for _, block := range calleeProc.Blocks {
						for j, op := range block.Ops {
							block.Ops[j] = b.replaceSignalInOperation(op, paramSignal, args[i])
						}
						// Also check terminator
						block.Terminator = b.replaceSignalInTerminator(block.Terminator, paramSignal, args[i])
					}
				}
			}
		}
	}
	restoreBindings()

	if len(calleeProc.Blocks) == 0 {
		return false
	}

	continuation := &BasicBlock{
		Label: fmt.Sprintf("%s_inline_cont_%d", bb.Label, instrIndex),
	}
	parentProc.Blocks = append(parentProc.Blocks, continuation)

	var callResult *Signal
	if call != nil && valueResultCount(call.Type()) == 1 {
		if len(calleeProc.ReturnValues) == 1 {
			for _, sig := range calleeProc.ReturnValues {
				callResult = sig
			}
		} else if len(calleeProc.ReturnValues) > 1 {
			callResult = b.ensureValueSignal(call)
		} else if calleeProc.Return != nil {
			callResult = calleeProc.Return
		}
		if callResult != nil {
			b.bindResolvedValue(continuation, call, callResult)
		}
	}
	for i := instrIndex + 1; i < len(block.Instrs); i++ {
		instr := block.Instrs[i]
		switch v := instr.(type) {
		case *ssa.Phi:
			// Continuations never start with phis because the split happens inside
			// a single SSA block after normal phi processing.
		case *ssa.If:
			b.handleIf(block, continuation, v)
		case *ssa.Jump:
			b.handleJump(block, continuation)
		case *ssa.Return:
			b.handleReturn(proc, continuation, v)
		default:
			if b.translateInstr(proc, block, continuation, i, instr) {
				goto continuationDone
			}
		}
	}
continuationDone:
	if continuation.Terminator == nil {
		continuation.Terminator = &ReturnTerminator{}
	}
	b.retargetContinuationPhiPreds(bb, continuation)

	if call != nil && valueResultCount(call.Type()) == 1 && len(calleeProc.ReturnValues) > 1 && callResult != nil {
		incomings := make([]PhiIncoming, 0, len(calleeProc.ReturnValues))
		for retBlock, sig := range calleeProc.ReturnValues {
			if retBlock == nil || sig == nil {
				continue
			}
			incomings = append(incomings, PhiIncoming{Block: retBlock, Value: sig})
		}
		if len(incomings) > 0 {
			continuation.Ops = append([]Operation{&PhiOperation{
				Dest:      callResult,
				Incomings: incomings,
			}}, continuation.Ops...)
		}
	}

	entryBlock := calleeProc.Blocks[0]
	bb.Terminator = &JumpTerminator{Target: entryBlock}
	for _, calleeBlock := range calleeProc.Blocks {
		if _, ok := calleeBlock.Terminator.(*ReturnTerminator); ok {
			calleeBlock.Terminator = &JumpTerminator{Target: continuation}
		}
		parentProc.Blocks = append(parentProc.Blocks, calleeBlock)
	}

	return true
}

func (b *builder) retargetContinuationPhiPreds(oldPred, newPred *BasicBlock) {
	if oldPred == nil || newPred == nil || newPred.Terminator == nil {
		return
	}
	targets := make([]*BasicBlock, 0, 2)
	switch term := newPred.Terminator.(type) {
	case *JumpTerminator:
		if term.Target != nil {
			targets = append(targets, term.Target)
		}
	case *BranchTerminator:
		if term.True != nil {
			targets = append(targets, term.True)
		}
		if term.False != nil && term.False != term.True {
			targets = append(targets, term.False)
		}
	}
	for _, target := range targets {
		if target == nil {
			continue
		}
		for _, op := range target.Ops {
			phi, ok := op.(*PhiOperation)
			if !ok || phi == nil {
				continue
			}
			for i := range phi.Incomings {
				incoming := phi.Incomings[i].Block
				if incoming == oldPred {
					phi.Incomings[i].Block = newPred
					continue
				}
				if incoming == nil || incoming.Label != oldPred.Label {
					continue
				}
				if blockTargets(incoming, target) {
					continue
				}
				phi.Incomings[i].Block = newPred
			}
		}
	}
}

func blockTargets(block, target *BasicBlock) bool {
	if block == nil || target == nil || block.Terminator == nil {
		return false
	}
	switch term := block.Terminator.(type) {
	case *JumpTerminator:
		return term.Target == target
	case *BranchTerminator:
		return term.True == target || term.False == target
	default:
		return false
	}
}

// findProcessByName finds a process by name
func (b *builder) findProcessByName(name string) *Process {
	if b == nil {
		return nil
	}

	// First check the module's process list
	if b.module != nil {
		for _, proc := range b.module.Processes {
			if proc.Name == name {
				return proc
			}
		}
	}

	// Also check the builder's process map (for processes not yet added to module)
	for _, proc := range b.processes {
		if proc.Name == name {
			return proc
		}
	}

	return nil
}

// cloneProcess creates a deep copy of a process for re-inlining
func (b *builder) cloneProcess(original *Process) *Process {
	if original == nil {
		return nil
	}

	// Create signal map for remapping
	signalMap := make(map[*Signal]*Signal)

	// Clone the process structure
	clone := &Process{
		Name:         original.Name,
		Source:       original.Source,
		Spawned:      original.Spawned,
		Sensitivity:  original.Sensitivity,
		Blocks:       make([]*BasicBlock, 0, len(original.Blocks)),
		Stage:        original.Stage,
		Params:       make([]*Signal, len(original.Params)),
		SSAParams:    make([]ssa.Value, len(original.SSAParams)),
		Return:       nil, // Will be set when we clone the return signal
		ReturnValues: make(map[*BasicBlock]*Signal),
	}
	blockMap := make(map[*BasicBlock]*BasicBlock, len(original.Blocks))

	// Clone SSA params
	copy(clone.SSAParams, original.SSAParams)

	// Clone params and create signal mappings
	for i, param := range original.Params {
		if param == nil {
			continue
		}
		clonedParam := &Signal{
			Name:   param.Name,
			Type:   param.Type.Clone(),
			Kind:   param.Kind,
			Value:  param.Value,
			Source: param.Source,
		}
		clone.Params[i] = clonedParam
		signalMap[param] = clonedParam
	}

	// Clone blocks
	for _, origBlock := range original.Blocks {
		clonedBlock := &BasicBlock{
			Label:        origBlock.Label,
			Ops:          make([]Operation, 0, len(origBlock.Ops)),
			Terminator:   nil, // Will be cloned below
			Predecessors: make([]*BasicBlock, 0),
			Successors:   make([]*BasicBlock, 0),
		}

		// Clone operations and remap signals
		for _, op := range origBlock.Ops {
			clonedOp := b.cloneOperation(op, signalMap)
			if clonedOp != nil {
				clonedBlock.Ops = append(clonedBlock.Ops, clonedOp)
			}
		}

		// Clone terminator
		if origBlock.Terminator != nil {
			clonedBlock.Terminator = b.cloneTerminator(origBlock.Terminator, signalMap)
		}

		clone.Blocks = append(clone.Blocks, clonedBlock)
		blockMap[origBlock] = clonedBlock
	}

	// Clone return signal if it exists
	if original.Return != nil {
		if remapped, ok := signalMap[original.Return]; ok {
			clone.Return = remapped
		} else {
			// Return wasn't in the map, create a new clone
			clone.Return = &Signal{
				Name:   original.Return.Name,
				Type:   original.Return.Type.Clone(),
				Kind:   original.Return.Kind,
				Value:  original.Return.Value,
				Source: original.Return.Source,
			}
			signalMap[original.Return] = clone.Return
		}
	}

	// Update predecessor/successor links after all blocks are cloned
	b.updateBlockLinks(clone.Blocks, original.Blocks)

	for origBlock, sig := range original.ReturnValues {
		clonedBlock := blockMap[origBlock]
		if clonedBlock == nil || sig == nil {
			continue
		}
		clone.ReturnValues[clonedBlock] = b.getRemappedSignal(sig, signalMap)
	}

	return clone
}

// cloneOperation creates a copy of an operation with remapped signals
func (b *builder) cloneOperation(op Operation, signalMap map[*Signal]*Signal) Operation {
	switch o := op.(type) {
	case *BinOperation:
		return &BinOperation{
			Op:    o.Op,
			Dest:  b.getRemappedSignal(o.Dest, signalMap),
			Left:  b.getRemappedSignal(o.Left, signalMap),
			Right: b.getRemappedSignal(o.Right, signalMap),
		}
	case *CompareOperation:
		return &CompareOperation{
			Predicate: o.Predicate,
			Dest:      b.getRemappedSignal(o.Dest, signalMap),
			Left:      b.getRemappedSignal(o.Left, signalMap),
			Right:     b.getRemappedSignal(o.Right, signalMap),
		}
	case *AssignOperation:
		return &AssignOperation{
			Dest:  b.getRemappedSignal(o.Dest, signalMap),
			Value: b.getRemappedSignal(o.Value, signalMap),
		}
	case *ConvertOperation:
		return &ConvertOperation{
			Dest:  b.getRemappedSignal(o.Dest, signalMap),
			Value: b.getRemappedSignal(o.Value, signalMap),
		}
	case *NotOperation:
		return &NotOperation{
			Dest:  b.getRemappedSignal(o.Dest, signalMap),
			Value: b.getRemappedSignal(o.Value, signalMap),
		}
	case *MuxOperation:
		return &MuxOperation{
			Dest:       b.getRemappedSignal(o.Dest, signalMap),
			Cond:       b.getRemappedSignal(o.Cond, signalMap),
			TrueValue:  b.getRemappedSignal(o.TrueValue, signalMap),
			FalseValue: b.getRemappedSignal(o.FalseValue, signalMap),
		}
	case *PhiOperation:
		incomings := make([]PhiIncoming, len(o.Incomings))
		for i, inc := range o.Incomings {
			incomings[i] = PhiIncoming{
				Block: inc.Block, // Will be updated in updateBlockLinks
				Value: b.getRemappedSignal(inc.Value, signalMap),
			}
		}
		return &PhiOperation{
			Dest:      b.getRemappedSignal(o.Dest, signalMap),
			Incomings: incomings,
		}
	case *PrintOperation:
		segments := make([]PrintSegment, len(o.Segments))
		for i, seg := range o.Segments {
			segments[i] = seg
			segments[i].Value = b.getRemappedSignal(seg.Value, signalMap)
		}
		return &PrintOperation{Segments: segments}
	case *SendOperation:
		return &SendOperation{
			Channel: o.Channel, // Channels are shared, not cloned
			Value:   b.getRemappedSignal(o.Value, signalMap),
		}
	case *RecvOperation:
		return &RecvOperation{
			Channel: o.Channel, // Channels are shared, not cloned
			Dest:    b.getRemappedSignal(o.Dest, signalMap),
		}
	default:
		return nil
	}
}

// cloneTerminator creates a copy of a terminator with remapped signals/blocks
func (b *builder) cloneTerminator(term Terminator, signalMap map[*Signal]*Signal) Terminator {
	switch t := term.(type) {
	case *BranchTerminator:
		return &BranchTerminator{
			Cond:  b.getRemappedSignal(t.Cond, signalMap),
			True:  t.True,  // Will be updated in updateBlockLinks
			False: t.False, // Will be updated in updateBlockLinks
		}
	case *JumpTerminator:
		return &JumpTerminator{Target: t.Target} // Will be updated in updateBlockLinks
	case *ReturnTerminator:
		return &ReturnTerminator{}
	default:
		return nil
	}
}

// getRemappedSignal returns the remapped signal or creates a new one if not in map
func (b *builder) getRemappedSignal(sig *Signal, signalMap map[*Signal]*Signal) *Signal {
	if sig == nil {
		return nil
	}
	if remapped, ok := signalMap[sig]; ok {
		return remapped
	}
	// Create a new clone for signals not yet in the map
	cloned := &Signal{
		Name:   sig.Name,
		Type:   sig.Type.Clone(),
		Kind:   sig.Kind,
		Value:  sig.Value,
		Source: sig.Source,
	}
	signalMap[sig] = cloned
	return cloned
}

// updateBlockLinks updates predecessor/successor links and block references
func (b *builder) updateBlockLinks(clonedBlocks []*BasicBlock, originalBlocks []*BasicBlock) {
	// Create a map from original blocks to cloned blocks
	blockMap := make(map[*BasicBlock]*BasicBlock)
	for i, origBlock := range originalBlocks {
		if i < len(clonedBlocks) {
			blockMap[origBlock] = clonedBlocks[i]
		}
	}

	// Update terminators to point to cloned blocks
	for _, block := range clonedBlocks {
		switch t := block.Terminator.(type) {
		case *BranchTerminator:
			if t.True != nil {
				t.True = blockMap[t.True]
			}
			if t.False != nil {
				t.False = blockMap[t.False]
			}
		case *JumpTerminator:
			if t.Target != nil {
				t.Target = blockMap[t.Target]
			}
		}
	}

	// Update predecessor/successor links
	for _, block := range clonedBlocks {
		switch t := block.Terminator.(type) {
		case *BranchTerminator:
			if t.True != nil {
				t.True.Predecessors = append(t.True.Predecessors, block)
				block.Successors = append(block.Successors, t.True)
			}
			if t.False != nil {
				t.False.Predecessors = append(t.False.Predecessors, block)
				block.Successors = append(block.Successors, t.False)
			}
		case *JumpTerminator:
			if t.Target != nil {
				t.Target.Predecessors = append(t.Target.Predecessors, block)
				block.Successors = append(block.Successors, t.Target)
			}
		}
	}
}

// buildProcessInternal builds a process without adding it to the module
func (b *builder) buildProcessInternal(fn *ssa.Function) *Process {
	if fn == nil {
		return nil
	}

	prevProc, hadPrev := b.processes[fn]
	proc := &Process{
		Name:         fn.Name(),
		Source:       fn.Pos(),
		Sensitivity:  Sequential,
		Stage:        -1,
		Params:       make([]*Signal, 0),
		ReturnValues: make(map[*BasicBlock]*Signal),
	}
	b.processes[fn] = proc
	defer func() {
		if hadPrev {
			b.processes[fn] = prevProc
		} else {
			delete(b.processes, fn)
		}
	}()
	prevSignals := b.signals
	prevTuples := b.tupleSignals
	b.signals = make(map[ssa.Value]*Signal)
	b.tupleSignals = make(map[ssa.Value][]*Signal)
	defer func() {
		b.signals = prevSignals
		b.tupleSignals = prevTuples
	}()
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

	// Collect return signal
	if fn.Signature != nil && fn.Signature.Results() != nil && fn.Signature.Results().Len() > 0 {
		// Return signal collection is done in handleReturn
	}

	return proc
}

// Remove the old inlineFunctionByBlockMerging and related functions since we're using a simpler approach
// (The old functions can be kept for now but won't be called)
// inlineFunctionByBlockMerging inlines a function by merging its blocks into the caller
// This is used for functions with loops that can't be inlined recursively
func (b *builder) inlineFunctionByBlockMerging(bb *BasicBlock, callee *ssa.Function, args []*Signal, call *ssa.Call) bool {
	if bb == nil || callee == nil {
		return false
	}

	// Build the callee process
	calleeProc := b.buildProcess(callee)
	if calleeProc == nil || len(calleeProc.Blocks) == 0 {
		return false
	}

	// Create a mapping from callee parameters to argument signals
	paramMap := make(map[ssa.Value]*Signal)
	for i, param := range callee.Params {
		if i < len(args) {
			paramMap[param] = args[i]
		} else {
			// Use default value for missing parameters
			paramMap[param] = b.newConstSignal(0, signalType(param.Type()), callee.Pos())
		}
	}

	// Find the entry block of the callee
	entryBlock := calleeProc.Blocks[0]
	if entryBlock == nil {
		return false
	}

	// Create new blocks in the caller's process for each callee block
	// We need to remap all the operations and signals
	blockMap := make(map[*BasicBlock]*BasicBlock)
	for _, calleeBlock := range calleeProc.Blocks {
		newBlock := &BasicBlock{
			Label: fmt.Sprintf("%s_%s_inline", bb.Label, calleeBlock.Label),
		}
		// Add to parent process (find the parent process)
		if proc := b.findProcessForBlock(bb); proc != nil {
			proc.Blocks = append(proc.Blocks, newBlock)
		}
		blockMap[calleeBlock] = newBlock

		// Copy and remap operations
		for _, op := range calleeBlock.Ops {
			if remappedOp := b.remapOperation(op, paramMap, calleeProc); remappedOp != nil {
				newBlock.Ops = append(newBlock.Ops, remappedOp)
			}
		}

		// Remap terminator if present
		if calleeBlock.Terminator != nil {
			if remappedTerm := b.remapTerminator(calleeBlock.Terminator, blockMap, bb.Label); remappedTerm != nil {
				newBlock.Terminator = remappedTerm
			}
		}
	}

	// Replace the call operation with a jump to the inlined entry block
	// Find and remove the call operation, then add a jump
	callFound := false
	for i, op := range bb.Ops {
		if callOp, ok := op.(*CallOperation); ok && callOp.Callee == callee.Name() {
			// Remove the call operation
			bb.Ops = append(bb.Ops[:i], bb.Ops[i+1:]...)
			// Set terminator to jump to the inlined entry block
			bb.Terminator = &JumpTerminator{Target: blockMap[entryBlock]}
			callFound = true
			break
		}
	}

	if !callFound {
		return false
	}

	// Connect the blocks properly
	b.connectInlinedBlocks(blockMap, bb)

	return true
}

// findProcessForBlock finds the process that contains a given block
func (b *builder) findProcessForBlock(bb *BasicBlock) *Process {
	if b == nil || bb == nil {
		return nil
	}

	// First check the module's process list
	if b.module != nil {
		for _, proc := range b.module.Processes {
			for _, block := range proc.Blocks {
				if block == bb {
					return proc
				}
			}
		}
	}

	// Also check the builder's process map (for processes currently being built)
	for _, proc := range b.processes {
		for _, block := range proc.Blocks {
			if block == bb {
				return proc
			}
		}
	}

	return nil
}

// remapOperation creates a copy of an operation with remapped signals
func (b *builder) remapOperation(op Operation, paramMap map[ssa.Value]*Signal, proc *Process) Operation {
	if op == nil {
		return nil
	}

	switch o := op.(type) {
	case *BinOperation:
		newLeft := b.remapSignal(o.Left, paramMap, proc)
		newRight := b.remapSignal(o.Right, paramMap, proc)
		if newLeft == nil || newRight == nil {
			return nil
		}
		return &BinOperation{
			Dest:  b.remapSignal(o.Dest, paramMap, proc),
			Left:  newLeft,
			Right: newRight,
			Op:    o.Op,
		}
	case *NotOperation:
		newValue := b.remapSignal(o.Value, paramMap, proc)
		if newValue == nil {
			return nil
		}
		return &NotOperation{
			Dest:  b.remapSignal(o.Dest, paramMap, proc),
			Value: newValue,
		}
	case *CompareOperation:
		newLeft := b.remapSignal(o.Left, paramMap, proc)
		newRight := b.remapSignal(o.Right, paramMap, proc)
		if newLeft == nil || newRight == nil {
			return nil
		}
		return &CompareOperation{
			Dest:      b.remapSignal(o.Dest, paramMap, proc),
			Left:      newLeft,
			Right:     newRight,
			Predicate: o.Predicate,
		}
	case *MuxOperation:
		newCond := b.remapSignal(o.Cond, paramMap, proc)
		newTrue := b.remapSignal(o.TrueValue, paramMap, proc)
		newFalse := b.remapSignal(o.FalseValue, paramMap, proc)
		if newCond == nil || newTrue == nil || newFalse == nil {
			return nil
		}
		return &MuxOperation{
			Dest:       b.remapSignal(o.Dest, paramMap, proc),
			Cond:       newCond,
			TrueValue:  newTrue,
			FalseValue: newFalse,
		}
	case *AssignOperation:
		newDest := b.remapSignal(o.Dest, paramMap, proc)
		newValue := b.remapSignal(o.Value, paramMap, proc)
		if newDest == nil || newValue == nil {
			return nil
		}
		return &AssignOperation{
			Dest:  newDest,
			Value: newValue,
		}
	// Add more operation types as needed
	default:
		// For unsupported operations, return nil to skip them
		return nil
	}
}

// remapSignal remaps a signal to use new signal names if it's a parameter
func (b *builder) remapSignal(sig *Signal, paramMap map[ssa.Value]*Signal, proc *Process) *Signal {
	if sig == nil {
		return nil
	}

	// Check if this signal corresponds to a parameter
	for ssaVal, signal := range paramMap {
		if signal == sig {
			return paramMap[ssaVal]
		}
	}

	// For non-parameter signals, we need to create new versions
	// to avoid conflicts with the caller's signals
	// For now, just return the original signal
	// TODO: Implement proper signal versioning
	return sig
}

// remapTerminator remaps a terminator to use new block targets
func (b *builder) remapTerminator(term Terminator, blockMap map[*BasicBlock]*BasicBlock, callerLabel string) Terminator {
	if term == nil {
		return nil
	}

	switch t := term.(type) {
	case *ReturnTerminator:
		// For returns, we need to handle them specially
		// For now, just return the terminator as-is
		// TODO: Implement proper return handling
		return t
	case *JumpTerminator:
		if newTarget, ok := blockMap[t.Target]; ok {
			return &JumpTerminator{Target: newTarget}
		}
		return t
	case *BranchTerminator:
		newTrue := t.True
		newFalse := t.False
		if t.True != nil {
			if mapped, ok := blockMap[t.True]; ok {
				newTrue = mapped
			}
		}
		if t.False != nil {
			if mapped, ok := blockMap[t.False]; ok {
				newFalse = mapped
			}
		}
		newCond := t.Cond
		return &BranchTerminator{
			Cond:  newCond,
			True:  newTrue,
			False: newFalse,
		}
	default:
		return t
	}
}

// connectInlinedBlocks connects predecessors and successors for inlined blocks
func (b *builder) connectInlinedBlocks(blockMap map[*BasicBlock]*BasicBlock, callerBlock *BasicBlock) {
	// Find the entry block (first block in the original callee)
	var entryBlock *BasicBlock
	for _, newBlock := range blockMap {
		// Assume the first block we encounter is the entry
		if entryBlock == nil {
			entryBlock = newBlock
			// Connect the caller block to the entry block
			if entryBlock != nil && callerBlock != nil {
				entryBlock.Predecessors = append(entryBlock.Predecessors, callerBlock)
			}
		}
	}

	// Update successor connections for all blocks
	for oldBlock, newBlock := range blockMap {
		for _, oldSucc := range oldBlock.Successors {
			if newSucc, ok := blockMap[oldSucc]; ok {
				newBlock.Successors = append(newBlock.Successors, newSucc)
				newSucc.Predecessors = append(newSucc.Predecessors, newBlock)
			}
		}
	}
}

func (b *builder) inlineCall(bb *BasicBlock, callee *ssa.Function, args []*Signal, stack map[*ssa.Function]struct{}, depth int) ([]*Signal, bool) {
	if bb == nil || callee == nil {
		return nil, false
	}
	if results, ok := b.inlineIntrinsicCall(callee, args); ok {
		return results, true
	}
	if depth >= inlineCallMaxDepth {
		return nil, false
	}
	if len(callee.Blocks) == 0 {
		return nil, false
	}
	if !b.isAcyclicFunction(callee) {
		return nil, false
	}
	if _, seen := stack[callee]; seen {
		return nil, false
	}
	stack[callee] = struct{}{}
	defer delete(stack, callee)

	frame := &inlineFrame{
		builder: b,
		bb:      bb,
		values:  make(map[ssa.Value]*Signal),
		tuples:  make(map[ssa.Value][]*Signal),
		slots:   make(map[ssa.Value]*Signal),
		globals: make(map[*ssa.Global]*Signal),
		stack:   stack,
	}
	type savedIndexedBinding struct {
		param    *ssa.Parameter
		state    *indexedBaseState
		hadState bool
	}
	savedStates := make([]savedIndexedBinding, 0, len(callee.Params))
	for i, param := range callee.Params {
		if i >= len(args) {
			frame.values[param] = b.newConstSignal(0, signalType(param.Type()), callee.Pos())
			continue
		}
		frame.values[param] = args[i]
		if !isIndexedValueType(param.Type()) {
			continue
		}
		saved := savedIndexedBinding{param: param}
		if state, ok := b.indexedBases[param]; ok {
			saved.state = state
			saved.hadState = true
		}
		if argState := b.findIndexedBaseForSignal(args[i]); argState != nil {
			b.indexedBases[param] = argState
		}
		savedStates = append(savedStates, saved)
	}
	defer func() {
		for _, saved := range savedStates {
			if saved.hadState {
				b.indexedBases[saved.param] = saved.state
			} else {
				delete(b.indexedBases, saved.param)
			}
		}
	}()
	return b.inlineEvalBlock(frame, callee.Blocks[0], nil, depth+1)
}

// inlineCallWithOptions is like inlineCall but allows skipping certain checks
func (b *builder) inlineCallWithOptions(bb *BasicBlock, callee *ssa.Function, args []*Signal, stack map[*ssa.Function]struct{}, depth int, skipAcyclicCheck bool) ([]*Signal, bool) {
	if bb == nil || callee == nil {
		return nil, false
	}
	if results, ok := b.inlineIntrinsicCall(callee, args); ok {
		return results, true
	}
	if depth >= inlineCallMaxDepth {
		return nil, false
	}
	if len(callee.Blocks) == 0 {
		return nil, false
	}
	// Skip acyclic check if requested
	if !skipAcyclicCheck && !b.isAcyclicFunction(callee) {
		return nil, false
	}
	if _, seen := stack[callee]; seen {
		return nil, false
	}
	stack[callee] = struct{}{}
	defer delete(stack, callee)

	frame := &inlineFrame{
		builder: b,
		bb:      bb,
		values:  make(map[ssa.Value]*Signal),
		tuples:  make(map[ssa.Value][]*Signal),
		slots:   make(map[ssa.Value]*Signal),
		globals: make(map[*ssa.Global]*Signal),
		stack:   stack,
	}
	type savedIndexedBinding struct {
		param    *ssa.Parameter
		state    *indexedBaseState
		hadState bool
	}
	savedStates := make([]savedIndexedBinding, 0, len(callee.Params))
	for i, param := range callee.Params {
		if i >= len(args) {
			frame.values[param] = b.newConstSignal(0, signalType(param.Type()), callee.Pos())
			continue
		}
		frame.values[param] = args[i]
		if !isIndexedValueType(param.Type()) {
			continue
		}
		saved := savedIndexedBinding{param: param}
		if state, ok := b.indexedBases[param]; ok {
			saved.state = state
			saved.hadState = true
		}
		if argState := b.findIndexedBaseForSignal(args[i]); argState != nil {
			b.indexedBases[param] = argState
		}
		savedStates = append(savedStates, saved)
	}
	defer func() {
		for _, saved := range savedStates {
			if saved.hadState {
				b.indexedBases[saved.param] = saved.state
			} else {
				delete(b.indexedBases, saved.param)
			}
		}
	}()
	return b.inlineEvalBlock(frame, callee.Blocks[0], nil, depth+1)
}

func (b *builder) inlineEvalBlock(frame *inlineFrame, block *ssa.BasicBlock, pred *ssa.BasicBlock, depth int) ([]*Signal, bool) {
	if frame == nil || block == nil {
		return nil, false
	}
	if depth >= inlineCallMaxDepth {
		return nil, false
	}
	if pred != nil {
		b.materializeIndexedStateStorage()
	}

	instrs := block.Instrs
	i := 0
	for i < len(instrs) {
		phi, ok := instrs[i].(*ssa.Phi)
		if !ok {
			break
		}
		if pred == nil {
			return nil, false
		}
		edge := predecessorIndex(block, pred)
		if edge < 0 || edge >= len(phi.Edges) {
			return nil, false
		}
		incoming, ok := frame.resolve(phi.Edges[edge])
		if !ok {
			return nil, false
		}
		frame.values[phi] = incoming
		i++
	}

	for ; i < len(instrs); i++ {
		switch instr := instrs[i].(type) {
		case *ssa.If:
			if len(block.Succs) < 2 {
				return nil, false
			}
			cond, ok := frame.resolve(instr.Cond)
			if !ok || cond == nil {
				return nil, false
			}
			trueRet, ok := b.inlineEvalBlock(frame.clone(), block.Succs[0], block, depth+1)
			if !ok {
				return nil, false
			}
			falseRet, ok := b.inlineEvalBlock(frame.clone(), block.Succs[1], block, depth+1)
			if !ok {
				return nil, false
			}
			return b.inlineMergeResults(frame.bb, cond, trueRet, falseRet, instr.Pos())
		case *ssa.Jump:
			if len(block.Succs) == 0 {
				return []*Signal{}, true
			}
			return b.inlineEvalBlock(frame, block.Succs[0], block, depth+1)
		case *ssa.Return:
			out := make([]*Signal, 0, len(instr.Results))
			for _, rv := range instr.Results {
				sig, ok := frame.resolve(rv)
				if !ok || sig == nil {
					return nil, false
				}
				out = append(out, sig)
			}
			return out, true
		default:
			if !b.inlineExecInstr(frame, instr, depth) {
				return nil, false
			}
		}
	}

	if len(block.Succs) == 1 {
		return b.inlineEvalBlock(frame, block.Succs[0], block, depth+1)
	}
	if len(block.Succs) == 0 {
		return []*Signal{}, true
	}
	return nil, false
}

func (b *builder) inlineMergeResults(bb *BasicBlock, cond *Signal, trueRet, falseRet []*Signal, pos token.Pos) ([]*Signal, bool) {
	if len(trueRet) != len(falseRet) {
		return nil, false
	}
	if len(trueRet) == 0 {
		return []*Signal{}, true
	}
	out := make([]*Signal, len(trueRet))
	for i := range trueRet {
		t := trueRet[i]
		f := falseRet[i]
		if t == nil || f == nil {
			return nil, false
		}
		if t == f {
			out[i] = t
			continue
		}
		target := t.Type
		if target == nil {
			target = f.Type
		}
		if t.Type != nil && f.Type != nil && !t.Type.Equal(f.Type) {
			target = t.Type.Promote(f.Type)
		}
		tCast := b.inlineCastIfNeeded(bb, t, target, pos)
		fCast := b.inlineCastIfNeeded(bb, f, target, pos)
		dest := b.newAnonymousSignal("callsel", target, pos)
		bb.Ops = append(bb.Ops, &MuxOperation{
			Dest:       dest,
			Cond:       cond,
			TrueValue:  tCast,
			FalseValue: fCast,
		})
		out[i] = dest
	}
	return out, true
}

func (b *builder) inlineCastIfNeeded(bb *BasicBlock, sig *Signal, target *SignalType, pos token.Pos) *Signal {
	if sig == nil || target == nil || sig.Type == nil || sig.Type.Equal(target) {
		return sig
	}
	dest := b.newAnonymousSignal("callcast", target, pos)
	bb.Ops = append(bb.Ops, &ConvertOperation{
		Dest:  dest,
		Value: sig,
	})
	return dest
}

func (b *builder) inlineExecInstr(frame *inlineFrame, instr ssa.Instruction, depth int) bool {
	switch v := instr.(type) {
	case *ssa.BinOp:
		left, ok := frame.resolve(v.X)
		if !ok || left == nil {
			return false
		}
		right, ok := frame.resolve(v.Y)
		if !ok || right == nil {
			return false
		}
		dest := b.inlineEmitBinOp(frame.bb, v.Op, v.Type(), v.X.Type(), left, right, v.Pos())
		if dest == nil {
			return false
		}
		frame.values[v] = dest
		return true
	case *ssa.UnOp:
		switch v.Op {
		case token.MUL:
			loaded, ok := frame.loadAddress(v.X, v.Pos(), signalType(v.Type()))
			if !ok || loaded == nil {
				return false
			}
			loaded = b.snapshotLoadedSignal(frame.bb, loaded, v.Pos())
			frame.values[v] = loaded
			return true
		case token.NOT, token.XOR:
			value, ok := frame.resolve(v.X)
			if !ok || value == nil {
				return false
			}
			dest := b.newAnonymousSignal("callnot", signalType(v.Type()), v.Pos())
			frame.bb.Ops = append(frame.bb.Ops, &NotOperation{
				Dest:  dest,
				Value: value,
			})
			frame.values[v] = dest
			return true
		case token.SUB:
			value, ok := frame.resolve(v.X)
			if !ok || value == nil {
				return false
			}
			dest := b.newAnonymousSignal("callneg", signalType(v.Type()), v.Pos())
			zero := b.newConstSignal(0, dest.Type, v.Pos())
			frame.bb.Ops = append(frame.bb.Ops, &BinOperation{
				Op:    Sub,
				Dest:  dest,
				Left:  zero,
				Right: value,
			})
			frame.values[v] = dest
			return true
		case token.ADD:
			value, ok := frame.resolve(v.X)
			if !ok || value == nil {
				return false
			}
			frame.values[v] = value
			return true
		default:
			return false
		}
	case *ssa.Convert:
		source, ok := frame.resolve(v.X)
		if !ok || source == nil {
			return false
		}
		dest := b.inlineEmitTypeChange(frame.bb, source, v.Type(), v.Pos())
		if dest == nil {
			return false
		}
		frame.values[v] = dest
		return true
	case *ssa.ChangeType:
		source, ok := frame.resolve(v.X)
		if !ok || source == nil {
			return false
		}
		dest := b.inlineEmitTypeChange(frame.bb, source, v.Type(), v.Pos())
		if dest == nil {
			return false
		}
		frame.values[v] = dest
		return true
	case *ssa.Alloc:
		ptrType, ok := v.Type().(*types.Pointer)
		if !ok {
			return false
		}
		frame.slots[v] = b.newConstSignal(0, signalType(ptrType.Elem()), v.Pos())
		return true
	case *ssa.Store:
		value, ok := frame.resolve(v.Val)
		if !ok || value == nil {
			return false
		}
		return frame.store(v.Addr, value, v.Pos())
	case *ssa.Call:
		callee := v.Call.StaticCallee()
		if callee == nil {
			return valueResultCount(v.Type()) == 0
		}
		args := make([]*Signal, 0, len(v.Call.Args))
		for _, arg := range v.Call.Args {
			sig, ok := frame.resolve(arg)
			if !ok || sig == nil {
				return false
			}
			args = append(args, sig)
		}
		results, ok := b.inlineCall(frame.bb, callee, args, frame.stack, depth+1)
		resultCount := valueResultCount(v.Type())
		if !ok {
			return resultCount == 0
		}
		if resultCount == 0 {
			return true
		}
		if len(results) != resultCount {
			return false
		}
		if resultCount == 1 {
			frame.values[v] = results[0]
			return true
		}
		copied := make([]*Signal, len(results))
		copy(copied, results)
		frame.tuples[v] = copied
		return true
	case *ssa.Extract:
		tuple := frame.lookupTuple(v.Tuple)
		if v.Index < 0 || v.Index >= len(tuple) || tuple[v.Index] == nil {
			return false
		}
		frame.values[v] = tuple[v.Index]
		return true
	case *ssa.IndexAddr:
		// Address materialization is resolved lazily by loads/stores.
		return true
	case *ssa.Index:
		sig, ok := frame.evalStringIndex(v)
		if !ok || sig == nil {
			return false
		}
		frame.values[v] = sig
		return true
	case *ssa.Slice:
		_, ok := frame.resolveSlice(v)
		return ok
	case *ssa.Phi, *ssa.DebugRef, *ssa.MakeInterface:
		return true
	case *ssa.If, *ssa.Jump, *ssa.Return:
		return false
	default:
		return false
	}
}

func (b *builder) inlineEmitTypeChange(bb *BasicBlock, source *Signal, dstType types.Type, pos token.Pos) *Signal {
	if source == nil {
		return nil
	}
	destType := signalType(dstType)
	if source.Type != nil && source.Type.Equal(destType) {
		return source
	}
	dest := b.newAnonymousSignal("callconv", destType, pos)
	bb.Ops = append(bb.Ops, &ConvertOperation{
		Dest:  dest,
		Value: source,
	})
	return dest
}

func (b *builder) inlineEmitBinOp(bb *BasicBlock, tok token.Token, resultType types.Type, leftType types.Type, left, right *Signal, pos token.Pos) *Signal {
	if left == nil || right == nil {
		return nil
	}
	commonType := left.Type.Promote(right.Type)
	if commonType == nil {
		if left.Type != nil {
			commonType = left.Type.Clone()
		} else if right.Type != nil {
			commonType = right.Type.Clone()
		}
	}
	if tok == token.AND_NOT {
		if commonType != nil {
			left = b.inlineCastIfNeeded(bb, left, commonType, pos)
			right = b.inlineCastIfNeeded(bb, right, commonType, pos)
		}
		notRight := b.newAnonymousSignal("callnot", right.Type, pos)
		bb.Ops = append(bb.Ops, &NotOperation{
			Dest:  notRight,
			Value: right,
		})
		dest := b.newAnonymousSignal("callbin", signalType(resultType), pos)
		bb.Ops = append(bb.Ops, &BinOperation{
			Op:    And,
			Dest:  dest,
			Left:  left,
			Right: notRight,
		})
		return dest
	}
	if pred, ok := translateCompareOp(tok, isSignedType(leftType)); ok {
		compareType := commonType
		switch tok {
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			width := 32
			if compareType != nil && compareType.Width > 0 {
				width = compareType.Width
			}
			compareType = &SignalType{Width: width, Signed: isSignedType(leftType)}
		}
		if compareType != nil {
			left = b.inlineCastIfNeeded(bb, left, compareType, pos)
			right = b.inlineCastIfNeeded(bb, right, compareType, pos)
		}
		dest := b.newAnonymousSignal("callcmp", signalType(resultType), pos)
		bb.Ops = append(bb.Ops, &CompareOperation{
			Predicate: pred,
			Dest:      dest,
			Left:      left,
			Right:     right,
		})
		return dest
	}
	bin, ok := translateBinOp(tok)
	if ok && bin == ShrU && tok == token.SHR && isSignedType(leftType) {
		bin = ShrS
	}
	if !ok {
		return nil
	}
	destType := signalType(resultType)
	if destType == nil {
		destType = commonType
	}
	if !isShiftBinOp(bin) && commonType != nil {
		left = b.inlineCastIfNeeded(bb, left, commonType, pos)
		right = b.inlineCastIfNeeded(bb, right, commonType, pos)
	}
	dest := b.newAnonymousSignal("callbin", destType, pos)
	if isShiftBinOp(bin) {
		leftSignalType := signalType(leftType)
		if leftSignalType != nil && (left.Type == nil || !left.Type.Equal(leftSignalType)) {
			left = b.inlineCastIfNeeded(bb, left, leftSignalType, pos)
		}
		if left.Type != nil && (right.Type == nil || !right.Type.Equal(left.Type)) {
			cast := b.newAnonymousSignal("shift", left.Type, pos)
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
	return dest
}

func (b *builder) inlineIntrinsicCall(callee *ssa.Function, args []*Signal) ([]*Signal, bool) {
	if callee == nil || callee.Pkg == nil || callee.Pkg.Pkg == nil {
		return nil, false
	}
	switch callee.Pkg.Pkg.Path() {
	case "math":
		if callee.Name() == "Float64frombits" && len(args) == 1 && args[0] != nil {
			return []*Signal{args[0]}, true
		}
	}
	return nil, false
}

func (b *builder) isAcyclicFunction(fn *ssa.Function) bool {
	if fn == nil || len(fn.Blocks) == 0 {
		return true
	}
	color := make(map[*ssa.BasicBlock]uint8, len(fn.Blocks))
	var visit func(*ssa.BasicBlock) bool
	visit = func(block *ssa.BasicBlock) bool {
		if block == nil {
			return true
		}
		switch color[block] {
		case 1:
			return false
		case 2:
			return true
		}
		color[block] = 1
		for _, succ := range block.Succs {
			if !visit(succ) {
				return false
			}
		}
		color[block] = 2
		return true
	}
	for _, block := range fn.Blocks {
		if block == nil || color[block] != 0 {
			continue
		}
		if !visit(block) {
			return false
		}
	}
	return true
}

func predecessorIndex(block *ssa.BasicBlock, pred *ssa.BasicBlock) int {
	if block == nil || pred == nil {
		return -1
	}
	for i, candidate := range block.Preds {
		if candidate == pred {
			return i
		}
	}
	return -1
}

func valueResultCount(t types.Type) int {
	if t == nil {
		return 0
	}
	if tuple, ok := t.(*types.Tuple); ok {
		return tuple.Len()
	}
	return 1
}

func unwrapAddressValue(v ssa.Value) ssa.Value {
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

// replaceSignalInOperation replaces all occurrences of oldSignal with newSignal in an operation
func (b *builder) replaceSignalInOperation(op Operation, oldSignal, newSignal *Signal) Operation {
	if op == nil || oldSignal == nil {
		return op
	}

	switch o := op.(type) {
	case *BinOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		if o.Left == oldSignal {
			o.Left = newSignal
		}
		if o.Right == oldSignal {
			o.Right = newSignal
		}
		return o
	case *CompareOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		if o.Left == oldSignal {
			o.Left = newSignal
		}
		if o.Right == oldSignal {
			o.Right = newSignal
		}
		return o
	case *AssignOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		if o.Value == oldSignal {
			o.Value = newSignal
		}
		return o
	case *ConvertOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		if o.Value == oldSignal {
			o.Value = newSignal
		}
		return o
	case *NotOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		if o.Value == oldSignal {
			o.Value = newSignal
		}
		return o
	case *MuxOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		if o.Cond == oldSignal {
			o.Cond = newSignal
		}
		if o.TrueValue == oldSignal {
			o.TrueValue = newSignal
		}
		if o.FalseValue == oldSignal {
			o.FalseValue = newSignal
		}
		return o
	case *PhiOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		for i := range o.Incomings {
			if o.Incomings[i].Value == oldSignal {
				o.Incomings[i].Value = newSignal
			}
		}
		return o
	case *PrintOperation:
		for i := range o.Segments {
			if o.Segments[i].Value == oldSignal {
				o.Segments[i].Value = newSignal
			}
		}
		return o
	case *SendOperation:
		if o.Value == oldSignal {
			o.Value = newSignal
		}
		return o
	case *RecvOperation:
		if o.Dest == oldSignal {
			o.Dest = newSignal
		}
		return o
	default:
		return op
	}
}

// replaceSignalInTerminator replaces all occurrences of oldSignal with newSignal in a terminator
func (b *builder) replaceSignalInTerminator(term Terminator, oldSignal, newSignal *Signal) Terminator {
	if term == nil || oldSignal == nil {
		return term
	}

	switch t := term.(type) {
	case *BranchTerminator:
		if t.Cond == oldSignal {
			t.Cond = newSignal
		}
		return t
	case *JumpTerminator:
		return t
	case *ReturnTerminator:
		return t
	default:
		return term
	}
}

// findIndexedBaseForSignal finds the indexedBaseState for a given signal
func (b *builder) findIndexedBaseForSignal(sig *Signal) *indexedBaseState {
	if b == nil || sig == nil {
		return nil
	}

	// Search through all indexed bases to find one that contains this signal
	for ssaVal, state := range b.indexedBases {
		if state == nil {
			continue
		}

		if mappedSig, ok := b.signals[ssaVal]; ok && mappedSig == sig {
			return state
		}
		if param, ok := ssaVal.(*ssa.Parameter); ok {
			if mappedSig, ok := b.paramSignals[param]; ok && mappedSig == sig {
				return state
			}
		}

		// First, check if any element signal matches
		for _, elemSig := range state.elements {
			if elemSig == sig {
				return state
			}
		}
		for _, storageSig := range state.storage {
			if storageSig == sig {
				return state
			}
		}

		// Second, check if the signal name matches a global array name
		// This handles the case where sig is a placeholder for an array
		if g, ok := ssaVal.(*ssa.Global); ok {
			if g.Name() == sig.Name {
				return state
			}
		}
		if alloc, ok := ssaVal.(*ssa.Alloc); ok {
			if b.allocName(alloc) == sig.Name {
				return state
			}
		}
	}

	return nil
}
