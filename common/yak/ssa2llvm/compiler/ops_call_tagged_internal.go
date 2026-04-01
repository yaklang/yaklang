package compiler

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	obfcore "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

func (c *Compiler) instructionTag(id int64) string {
	if c == nil || id <= 0 || len(c.InstrTags) == 0 {
		return ""
	}
	return c.InstrTags[id]
}

func (c *Compiler) compileTaggedObfCall(inst *ssa.Call) (bool, error) {
	if inst == nil {
		return false, nil
	}

	lowering, ok := obfcore.LookupTaggedCallLowering(c.instructionTag(inst.GetId()))
	if !ok {
		return false, nil
	}

	fn, fnType := c.getOrDeclareTaggedCallPlaceholder(lowering.Symbol, lowering.Arity)
	args := make([]llvm.Value, 0, len(inst.Args))
	for _, argID := range inst.Args {
		argVal, err := c.resolveSSAValueAsInt64(inst, argID, "yak_obf_fn")
		if err != nil {
			return true, err
		}
		args = append(args, argVal)
	}

	callResult := c.Builder.CreateCall(fnType, fn, args, "")
	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = callResult
	}
	return true, nil
}

func (c *Compiler) getOrDeclareTaggedCallPlaceholder(name string, arity int) (llvm.Value, llvm.Type) {
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()

	var paramTypes []llvm.Type
	if arity > 0 {
		paramTypes = make([]llvm.Type, 0, arity)
		for i := 0; i < arity; i++ {
			paramTypes = append(paramTypes, i64)
		}
	}

	fnType := llvm.FunctionType(i64, paramTypes, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}
