package ir

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

const constEvalMaxSteps = 10000

type constEvalValue interface{}

type constEvalAddr struct {
	base ssa.Value
	path []int
}

type constEvaluator struct {
	values map[ssa.Value]constEvalValue
	addrs  map[ssa.Value]constEvalAddr
	memory map[ssa.Value]constEvalValue
}

func (b *builder) constEvalCall(callee *ssa.Function, callArgs []ssa.Value, loweredArgs []*Signal, pos token.Pos) ([]*Signal, bool) {
	if b == nil || callee == nil {
		return nil, false
	}
	if len(callArgs) < len(callee.Params) || len(loweredArgs) < len(callee.Params) {
		return nil, false
	}
	paramVals := make(map[*ssa.Parameter]constEvalValue, len(callee.Params))
	for i, param := range callee.Params {
		val, ok := b.constEvalArgValue(callArgs[i], loweredArgs[i], pos)
		if !ok {
			return nil, false
		}
		paramVals[param] = val
	}

	eval := &constEvaluator{
		values: make(map[ssa.Value]constEvalValue),
		addrs:  make(map[ssa.Value]constEvalAddr),
		memory: make(map[ssa.Value]constEvalValue),
	}
	for param, val := range paramVals {
		eval.values[param] = val
	}

	results, ok := eval.run(callee)
	if !ok {
		return nil, false
	}
	typesOut := resultTypes(callee)
	if len(results) != len(typesOut) {
		return nil, false
	}

	out := make([]*Signal, 0, len(results))
	for i := range results {
		sig, ok := b.constEvalValueToSignal(results[i], typesOut[i], pos)
		if !ok {
			return nil, false
		}
		out = append(out, sig)
	}
	return out, true
}

func (b *builder) constEvalArgValue(arg ssa.Value, lowered *Signal, pos token.Pos) (constEvalValue, bool) {
	if arg == nil {
		return nil, false
	}
	if _, length, ok := indexedElementInfo(arg.Type()); ok && length >= 0 {
		base := indexedBaseForConstEval(arg)
		if base == nil {
			return nil, false
		}
		state := b.indexedStateForBase(base, pos)
		if state == nil {
			return nil, false
		}
		values := make([]constEvalValue, length)
		for i := 0; i < length; i++ {
			elem := b.indexedElementSignal(state, i, pos)
			v, ok := constEvalSignalValue(elem)
			if !ok {
				return nil, false
			}
			values[i] = v
		}
		return values, true
	}
	return constEvalSignalValue(lowered)
}

func (b *builder) constEvalValueToSignal(v constEvalValue, t types.Type, pos token.Pos) (*Signal, bool) {
	switch val := v.(type) {
	case bool:
		return b.newConstSignal(val, signalType(t), pos), true
	case int64:
		return b.newConstSignal(val, signalType(t), pos), true
	default:
		return nil, false
	}
}

func indexedBaseForConstEval(v ssa.Value) ssa.Value {
	for v != nil {
		switch val := v.(type) {
		case *ssa.ChangeType:
			v = val.X
		case *ssa.Convert:
			v = val.X
		case *ssa.UnOp:
			if val.Op == token.MUL {
				return unwrapIndexedBase(unwrapAddressValue(val.X))
			}
			return nil
		case *ssa.IndexAddr:
			return unwrapIndexedBase(val.X)
		default:
			if _, _, ok := indexedElementInfo(v.Type()); ok {
				return unwrapIndexedBase(v)
			}
			return nil
		}
	}
	return nil
}

func constEvalSignalValue(sig *Signal) (constEvalValue, bool) {
	if sig == nil {
		return nil, false
	}
	switch v := sig.Value.(type) {
	case bool:
		return v, true
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
		return nil, false
	}
}

func resultTypes(fn *ssa.Function) []types.Type {
	if fn == nil || fn.Signature == nil || fn.Signature.Results() == nil {
		return nil
	}
	results := fn.Signature.Results()
	out := make([]types.Type, 0, results.Len())
	for i := 0; i < results.Len(); i++ {
		out = append(out, results.At(i).Type())
	}
	return out
}

func (e *constEvaluator) run(fn *ssa.Function) ([]constEvalValue, bool) {
	if e == nil || fn == nil || len(fn.Blocks) == 0 {
		return nil, false
	}
	block := fn.Blocks[0]
	var pred *ssa.BasicBlock

	for steps := 0; steps < constEvalMaxSteps; steps++ {
		if block == nil {
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
			value, ok := e.resolveValue(phi.Edges[edge])
			if !ok {
				return nil, false
			}
			e.values[phi] = value
			i++
		}

		for ; i < len(instrs); i++ {
			switch instr := instrs[i].(type) {
			case *ssa.Jump:
				if len(block.Succs) == 0 {
					return []constEvalValue{}, true
				}
				pred = block
				block = block.Succs[0]
				goto nextBlock
			case *ssa.If:
				if len(block.Succs) < 2 {
					return nil, false
				}
				cond, ok := e.resolveBool(instr.Cond)
				if !ok {
					return nil, false
				}
				pred = block
				if cond {
					block = block.Succs[0]
				} else {
					block = block.Succs[1]
				}
				goto nextBlock
			case *ssa.Return:
				out := make([]constEvalValue, 0, len(instr.Results))
				for _, rv := range instr.Results {
					value, ok := e.resolveValue(rv)
					if !ok {
						return nil, false
					}
					out = append(out, value)
				}
				return out, true
			default:
				if !e.execInstr(instr) {
					return nil, false
				}
			}
		}

		switch len(block.Succs) {
		case 0:
			return []constEvalValue{}, true
		case 1:
			pred = block
			block = block.Succs[0]
		default:
			return nil, false
		}
	nextBlock:
	}
	return nil, false
}

func (e *constEvaluator) execInstr(instr ssa.Instruction) bool {
	switch v := instr.(type) {
	case *ssa.Alloc:
		ptrType, ok := v.Type().(*types.Pointer)
		if !ok {
			return false
		}
		zero, ok := constEvalZero(ptrType.Elem())
		if !ok {
			return false
		}
		e.memory[v] = zero
		e.addrs[v] = constEvalAddr{base: v}
		return true
	case *ssa.IndexAddr:
		base, ok := e.resolveAddr(v.X)
		if !ok {
			return false
		}
		idx, ok := e.resolveInt(v.Index)
		if !ok || idx < 0 {
			return false
		}
		path := append([]int{}, base.path...)
		path = append(path, int(idx))
		e.addrs[v] = constEvalAddr{base: base.base, path: path}
		return true
	case *ssa.Store:
		addr, ok := e.resolveAddr(v.Addr)
		if !ok {
			return false
		}
		value, ok := e.resolveValue(v.Val)
		if !ok {
			return false
		}
		return e.store(addr, value)
	case *ssa.UnOp:
		switch v.Op {
		case token.MUL:
			addr, ok := e.resolveAddr(v.X)
			if !ok {
				return false
			}
			value, ok := e.load(addr)
			if !ok {
				return false
			}
			e.values[v] = value
			return true
		case token.NOT:
			value, ok := e.resolveBool(v.X)
			if !ok {
				return false
			}
			e.values[v] = !value
			return true
		case token.SUB:
			value, ok := e.resolveInt(v.X)
			if !ok {
				return false
			}
			e.values[v] = -value
			return true
		case token.ADD:
			value, ok := e.resolveInt(v.X)
			if !ok {
				return false
			}
			e.values[v] = value
			return true
		default:
			return false
		}
	case *ssa.BinOp:
		value, ok := e.evalBinOp(v)
		if !ok {
			return false
		}
		e.values[v] = value
		return true
	case *ssa.Convert:
		value, ok := e.resolveValue(v.X)
		if !ok {
			return false
		}
		e.values[v] = value
		return true
	case *ssa.ChangeType:
		value, ok := e.resolveValue(v.X)
		if !ok {
			return false
		}
		e.values[v] = value
		return true
	case *ssa.Phi, *ssa.DebugRef, *ssa.MakeInterface, *ssa.Slice:
		return true
	case *ssa.Extract:
		value, ok := e.resolveValue(v.Tuple)
		if !ok {
			return false
		}
		tuple, ok := value.([]constEvalValue)
		if !ok || v.Index < 0 || v.Index >= len(tuple) {
			return false
		}
		e.values[v] = tuple[v.Index]
		return true
	case *ssa.Call:
		return valueResultCount(v.Type()) == 0
	default:
		return false
	}
}

func (e *constEvaluator) resolveValue(v ssa.Value) (constEvalValue, bool) {
	if v == nil {
		return nil, false
	}
	if val, ok := e.values[v]; ok {
		return val, true
	}
	switch val := v.(type) {
	case *ssa.Const:
		return constEvalFromSSAConst(val)
	case *ssa.Global:
		addr := constEvalAddr{base: val}
		e.addrs[val] = addr
		loaded, ok := e.load(addr)
		return loaded, ok
	case *ssa.Convert:
		return e.resolveValue(val.X)
	case *ssa.ChangeType:
		return e.resolveValue(val.X)
	}
	return nil, false
}

func (e *constEvaluator) resolveAddr(v ssa.Value) (constEvalAddr, bool) {
	if v == nil {
		return constEvalAddr{}, false
	}
	if addr, ok := e.addrs[v]; ok {
		return addr, true
	}
	switch val := unwrapAddressValue(v).(type) {
	case *ssa.Alloc:
		addr := constEvalAddr{base: val}
		e.addrs[val] = addr
		return addr, true
	case *ssa.Global:
		addr := constEvalAddr{base: val}
		e.addrs[val] = addr
		return addr, true
	}
	return constEvalAddr{}, false
}

func (e *constEvaluator) load(addr constEvalAddr) (constEvalValue, bool) {
	base, ok := e.memory[addr.base]
	if !ok {
		var zeroType types.Type
		switch v := addr.base.(type) {
		case *ssa.Alloc:
			ptr, ok := v.Type().(*types.Pointer)
			if !ok {
				return nil, false
			}
			zeroType = ptr.Elem()
		case *ssa.Global:
			ptr, ok := v.Type().(*types.Pointer)
			if !ok {
				return nil, false
			}
			zeroType = ptr.Elem()
		default:
			return nil, false
		}
		base, ok = constEvalZero(zeroType)
		if !ok {
			return nil, false
		}
		e.memory[addr.base] = base
	}
	return constEvalIndexGet(base, addr.path)
}

func (e *constEvaluator) store(addr constEvalAddr, value constEvalValue) bool {
	base, ok := e.memory[addr.base]
	if !ok {
		loaded, ok := e.load(constEvalAddr{base: addr.base})
		if !ok {
			return false
		}
		base = loaded
	}
	next, ok := constEvalIndexSet(base, addr.path, value)
	if !ok {
		return false
	}
	e.memory[addr.base] = next
	return true
}

func (e *constEvaluator) resolveInt(v ssa.Value) (int64, bool) {
	value, ok := e.resolveValue(v)
	if !ok {
		return 0, false
	}
	switch n := value.(type) {
	case int64:
		return n, true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func (e *constEvaluator) resolveBool(v ssa.Value) (bool, bool) {
	value, ok := e.resolveValue(v)
	if !ok {
		return false, false
	}
	switch b := value.(type) {
	case bool:
		return b, true
	case int64:
		return b != 0, true
	default:
		return false, false
	}
}

func (e *constEvaluator) evalBinOp(op *ssa.BinOp) (constEvalValue, bool) {
	if op == nil {
		return nil, false
	}
	switch op.Op {
	case token.EQL, token.NEQ:
		left, lok := e.resolveValue(op.X)
		right, rok := e.resolveValue(op.Y)
		if !lok || !rok {
			return nil, false
		}
		if lb, ok := left.(bool); ok {
			rb, ok := right.(bool)
			if !ok {
				return nil, false
			}
			if op.Op == token.EQL {
				return lb == rb, true
			}
			return lb != rb, true
		}
		li, ok := left.(int64)
		if !ok {
			return nil, false
		}
		ri, ok := right.(int64)
		if !ok {
			return nil, false
		}
		if op.Op == token.EQL {
			return li == ri, true
		}
		return li != ri, true
	default:
		left, lok := e.resolveInt(op.X)
		right, rok := e.resolveInt(op.Y)
		if !lok || !rok {
			return nil, false
		}
		switch op.Op {
		case token.ADD:
			return left + right, true
		case token.SUB:
			return left - right, true
		case token.MUL:
			return left * right, true
		case token.QUO:
			if right == 0 {
				return nil, false
			}
			return left / right, true
		case token.REM:
			if right == 0 {
				return nil, false
			}
			return left % right, true
		case token.AND:
			return left & right, true
		case token.OR:
			return left | right, true
		case token.XOR:
			return left ^ right, true
		case token.AND_NOT:
			return left &^ right, true
		case token.SHL:
			if right < 0 {
				return nil, false
			}
			return left << uint(right), true
		case token.SHR:
			if right < 0 {
				return nil, false
			}
			if isSignedType(op.X.Type()) {
				return left >> uint(right), true
			}
			return int64(uint64(left) >> uint(right)), true
		case token.LSS:
			return left < right, true
		case token.LEQ:
			return left <= right, true
		case token.GTR:
			return left > right, true
		case token.GEQ:
			return left >= right, true
		default:
			return nil, false
		}
	}
}

func constEvalFromSSAConst(c *ssa.Const) (constEvalValue, bool) {
	if c == nil || c.Value == nil {
		return nil, false
	}
	switch c.Value.Kind() {
	case constant.Bool:
		return constant.BoolVal(c.Value), true
	case constant.Int:
		if i, ok := constant.Int64Val(c.Value); ok {
			return i, true
		}
		if u, ok := constant.Uint64Val(c.Value); ok {
			return int64(u), true
		}
		return nil, false
	default:
		return nil, false
	}
}

func constEvalZero(t types.Type) (constEvalValue, bool) {
	if t == nil {
		return nil, false
	}
	switch tt := t.Underlying().(type) {
	case *types.Basic:
		if tt.Info()&types.IsBoolean != 0 {
			return false, true
		}
		return int64(0), true
	case *types.Array:
		length := int(tt.Len())
		values := make([]constEvalValue, length)
		for i := 0; i < length; i++ {
			zero, ok := constEvalZero(tt.Elem())
			if !ok {
				return nil, false
			}
			values[i] = zero
		}
		return values, true
	default:
		return nil, false
	}
}

func constEvalIndexGet(value constEvalValue, path []int) (constEvalValue, bool) {
	if len(path) == 0 {
		return value, true
	}
	array, ok := value.([]constEvalValue)
	if !ok {
		return nil, false
	}
	idx := path[0]
	if idx < 0 || idx >= len(array) {
		return nil, false
	}
	return constEvalIndexGet(array[idx], path[1:])
}

func constEvalIndexSet(value constEvalValue, path []int, next constEvalValue) (constEvalValue, bool) {
	if len(path) == 0 {
		return next, true
	}
	array, ok := value.([]constEvalValue)
	if !ok {
		return nil, false
	}
	idx := path[0]
	if idx < 0 || idx >= len(array) {
		return nil, false
	}
	updated, ok := constEvalIndexSet(array[idx], path[1:], next)
	if !ok {
		return nil, false
	}
	array[idx] = updated
	return array, true
}
