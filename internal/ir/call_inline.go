package ir

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"

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
	state := f.builder.indexedStateForBase(addr.X, pos)
	if state == nil {
		return false
	}
	if idx, ok := constIndexValue(addr.Index); ok {
		if state.length >= 0 && idx >= state.length {
			return false
		}
		state.elements[idx] = value
		return true
	}
	index, ok := f.resolve(addr.Index)
	if !ok || index == nil || state.length <= 0 {
		return false
	}
	indexType := index.Type
	if indexType == nil {
		indexType = signalType(addr.Index.Type())
	}
	for i := 0; i < state.length; i++ {
		element := f.builder.indexedElementSignal(state, i, pos)
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
		state.elements[i] = next
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
	state := f.builder.indexedStateForBase(addr.X, addr.Pos())
	if state == nil {
		return nil
	}
	if idx, ok := constIndexValue(addr.Index); ok {
		sig := f.builder.indexedElementSignal(state, idx, addr.Pos())
		if sig != nil {
			f.values[addr] = sig
		}
		return sig
	}
	index, ok := f.resolve(addr.Index)
	if !ok || index == nil || state.length <= 0 {
		return nil
	}
	selected := f.builder.selectIndexedElement(f.bb, state, index, addr.Pos())
	if selected != nil {
		f.values[addr] = selected
	}
	return selected
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
				base, ok := unwrapIndexedBase(addr.X).(*ssa.Global)
				if !ok {
					continue
				}
				state := b.indexedStateForBase(base, store.Pos())
				if state == nil {
					continue
				}
				idx, ok := constIndexValue(addr.Index)
				if !ok {
					continue
				}
				if state.length >= 0 && idx >= state.length {
					continue
				}
				state.elements[idx] = value
			}
		}
	}
}

func (b *builder) signalForGlobal(g *ssa.Global) *Signal {
	if g == nil {
		return nil
	}
	if sig, ok := b.globalValues[g]; ok && sig != nil {
		return sig
	}
	ptrType, ok := g.Type().(*types.Pointer)
	if !ok {
		return nil
	}
	zero := b.newConstSignal(0, signalType(ptrType.Elem()), g.Pos())
	b.globalValues[g] = zero
	return zero
}

func (b *builder) handleCall(bb *BasicBlock, call *ssa.Call) {
	if bb == nil || call == nil {
		return
	}
	resultCount := valueResultCount(call.Type())
	callee := call.Call.StaticCallee()
	if callee == nil {
		if resultCount > 0 {
			b.reporter.Warning(call.Pos(), fmt.Sprintf("dynamic call result is not supported: %T", call.Call.Value))
		}
		return
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
			if resultCount > 0 {
				b.reporter.Warning(call.Pos(), fmt.Sprintf("call %s has unresolved argument %T", callee.String(), arg))
			}
			return
		}
		args = append(args, sig)
	}
	results, ok := b.inlineCall(bb, callee, args, make(map[*ssa.Function]struct{}), 0)
	if ok {
		if resultCount == 0 {
			return
		}
		if len(results) != resultCount {
			b.reporter.Warning(call.Pos(), fmt.Sprintf("call %s produced %d results, expected %d", callee.String(), len(results), resultCount))
			return
		}
		if resultCount == 1 {
			b.bindResolvedValue(bb, call, results[0])
			return
		}
		copied := make([]*Signal, len(results))
		copy(copied, results)
		b.tupleSignals[call] = copied
		return
	}

	// Fallback to explicit call lowering when inlining is unavailable.
	if resultCount > 1 {
		b.reporter.Warning(call.Pos(), fmt.Sprintf("multi-result call for %s is not supported", callee.String()))
		return
	}

	callOp := &CallOperation{
		Callee: callee.String(),
		Args:   args,
	}
	if resultCount == 1 {
		dest := b.ensureValueSignal(call)
		dest.Type = signalType(call.Type())
		callOp.Dest = dest
		b.signals[call] = dest
	}
	bb.Ops = append(bb.Ops, callOp)
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
	for i, param := range callee.Params {
		if i >= len(args) {
			frame.values[param] = b.newConstSignal(0, signalType(param.Type()), callee.Pos())
			continue
		}
		frame.values[param] = args[i]
	}
	return b.inlineEvalBlock(frame, callee.Blocks[0], nil, depth+1)
}

func (b *builder) inlineEvalBlock(frame *inlineFrame, block *ssa.BasicBlock, pred *ssa.BasicBlock, depth int) ([]*Signal, bool) {
	if frame == nil || block == nil {
		return nil, false
	}
	if depth >= inlineCallMaxDepth {
		return nil, false
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
	case *ssa.Phi, *ssa.DebugRef, *ssa.MakeInterface, *ssa.Slice:
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
