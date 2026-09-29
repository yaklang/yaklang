package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) compileUnOp(inst *ssa.UnOp, resultID int64) error {
	if _, ok := c.getCachedValue(inst, resultID); ok {
		return nil
	}
	x, err := c.getValue(inst, inst.X)
	if err != nil {
		return err
	}
	x = c.coerceToInt64(x)

	var val llvm.Value
	name := fmt.Sprintf("unop_%d", resultID)
	i64 := c.LLVMCtx.Int64Type()

	switch inst.Op {
	case ssa.OpNot:
		one := llvm.ConstInt(i64, 1, false)
		val = c.Builder.CreateXor(x, one, name)
	case ssa.OpNeg:
		if c.valueCarriesFloatBits(inst.GetFunc(), inst.X, 0) {
			// A float operand is a float64 bit pattern in an i64 word, so
			// `0 - pattern` would negate the two's-complement word instead:
			// -1.5 (0xbff8...) came out as -3.0 (0xc008...). Negating a float
			// is exactly flipping its sign bit.
			sign := llvm.ConstInt(i64, 1<<63, false)
			val = c.Builder.CreateXor(x, sign, name)
		} else {
			zero := llvm.ConstInt(i64, 0, false)
			val = c.Builder.CreateSub(zero, x, name)
		}
	case ssa.OpPlus:
		val = x
	case ssa.OpBitwiseNot:
		minusOne := llvm.ConstInt(i64, ^uint64(0), true)
		val = c.Builder.CreateXor(x, minusOne, name)
	case ssa.OpChan:
		spec := contextCallSpec{
			inst: inst,
			kind: abi.KindDispatch,
			target: llvm.ConstInt(
				c.LLVMCtx.Int64Type(),
				uint64(abi.IDRuntimeChanRecv),
				false,
			),
			args: []contextCallArg{
				{value: x, tagPointerArg: true},
			},
			ctxName:   "yak_chan_recv_ctx",
			errPrefix: "emitRuntimeChanRecv",
		}
		result, err := c.emitContextCall(spec)
		if err != nil {
			return err
		}
		val = c.coerceToInt64(result)
	default:
		return fmt.Errorf("compileUnOp: unsupported opcode %v", inst.Op)
	}

	c.cacheValue(resultID, val)
	if err := c.maybeEmitMemberSet(inst, inst, resultID); err != nil {
		return err
	}
	return nil
}

// valueCarriesFloatBits reports whether an SSA value holds a float64 bit
// pattern rather than an integer. Unary negation and the float binops need this
// because both kinds share one i64 ABI word: -1.5 is the pattern
// 0xbff8000000000000, so integer arithmetic on it silently produces a different
// float instead of failing loudly.
func (c *Compiler) valueCarriesFloatBits(fn *ssa.Function, id int64, depth int) bool {
	if fn == nil || id <= 0 || depth > 8 {
		return false
	}
	v, ok := fn.GetValueById(id)
	if !ok || v == nil {
		return false
	}
	return c.valueDefinesFloat(fn, v, depth)
}

// valueDefinesFloat walks the definition chain because the frontend types most
// values as the shared "number" kind, which cannot tell an int from a float.
// The float() cast is the only place a distinct "float" type name appears; a
// literal, a negation of a float, or arithmetic on one all keep that meaning.
func (c *Compiler) valueDefinesFloat(fn *ssa.Function, v ssa.Value, depth int) bool {
	if v == nil {
		return false
	}
	if t := v.GetType(); t != nil && t.String() == "float" {
		return true
	}
	switch val := v.(type) {
	case *ssa.ConstInst:
		return val != nil && val.IsFloat()
	case *ssa.TypeCast:
		if val == nil {
			return false
		}
		return c.valueCarriesFloatBits(fn, val.Value, depth+1)
	case *ssa.UnOp:
		if val == nil || val.Op != ssa.OpNeg {
			return false
		}
		return c.valueCarriesFloatBits(fn, val.X, depth+1)
	case *ssa.BinOp:
		if val == nil {
			return false
		}
		switch val.Op {
		case ssa.OpAdd, ssa.OpSub, ssa.OpMul, ssa.OpDiv:
			return c.valueCarriesFloatBits(fn, val.X, depth+1) ||
				c.valueCarriesFloatBits(fn, val.Y, depth+1)
		}
	}
	return false
}
