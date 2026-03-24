package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) getOrDeclareExternCallable(symbol string) llvm.Value {
	if symbol == "" {
		return llvm.Value{}
	}
	fn := c.Mod.NamedFunction(symbol)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, symbol, fnType)
	}
	return fn
}

func (c *Compiler) callArgIDs(inst *ssa.Call) []int64 {
	if inst == nil {
		return nil
	}
	argIDs := append([]int64{}, inst.Args...)
	return append(argIDs, inst.ArgMember...)
}

func (c *Compiler) newCallableContextCallSpec(inst *ssa.Call, llvmFn llvm.Value, argIDs []int64, debugName string) (contextCallSpec, error) {
	if inst == nil {
		return contextCallSpec{}, fmt.Errorf("newCallableContextCallSpec: missing call instruction")
	}
	if llvmFn.IsNil() {
		return contextCallSpec{}, fmt.Errorf("newCallableContextCallSpec: missing llvm function for call %d", inst.GetId())
	}

	targetName := debugName
	if targetName == "" {
		targetName = "yak_call_target"
	}

	return contextCallSpec{
		inst:      inst,
		kind:      abi.KindCallable,
		target:    c.Builder.CreatePtrToInt(llvmFn, c.LLVMCtx.Int64Type(), targetName),
		argIDs:    argIDs,
		async:     inst.Async,
		ctxName:   "yak_call_ctx",
		errPrefix: "emitCallableContextCall",
	}, nil
}

func (c *Compiler) newDispatchContextCallSpec(inst *ssa.Call, binding ExternBinding) (contextCallSpec, error) {
	if inst == nil {
		return contextCallSpec{}, fmt.Errorf("newDispatchContextCallSpec: missing call instruction")
	}
	if binding.DispatchID == 0 {
		return contextCallSpec{}, fmt.Errorf("newDispatchContextCallSpec: missing dispatch id for call %d", inst.GetId())
	}

	return contextCallSpec{
		inst:           inst,
		kind:           abi.KindDispatch,
		target:         llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(binding.DispatchID), false),
		argIDs:         append([]int64{}, inst.Args...),
		async:          inst.Async,
		tagPointerArgs: shouldTagStdlibArgPointers(binding.DispatchID),
		ctxName:        "yak_dispatch_ctx",
		errPrefix:      "emitDispatchContextCall",
	}, nil
}

// compileCall compiles a ssa.Call instruction to LLVM IR.
func (c *Compiler) compileCall(inst *ssa.Call) error {
	fn := inst.GetFunc()
	var calleeVal ssa.Value
	if fn != nil {
		if v, ok := fn.GetValueById(inst.Method); ok && v != nil {
			if vv, ok := v.(ssa.Value); ok {
				calleeVal = vv
			}
		}
	}

	// YakSSA uses function-typed member values (e.g. object-factor methods) where the
	// call target is an Undefined MemberCall but the FunctionType.This points at the
	// actual SSA function implementation. Prefer ID-based resolution to avoid name
	// collisions (e.g. duplicated "f$1").
	if calleeVal != nil {
		if ssaFn, ok := ssa.ToFunction(calleeVal); ok && ssaFn != nil && !ssaFn.IsExtern() {
			llvmFn, _ := c.getOrDeclareLLVMFunction(ssaFn)
			spec, err := c.newCallableContextCallSpec(inst, llvmFn, c.callArgIDs(inst), "yak_call_target")
			if err != nil {
				return err
			}
			return c.lowerResolvedContextCall(spec)
		}
		if calleeVal.IsMember() {
			if ft, ok := calleeVal.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
				llvmFn, _ := c.getOrDeclareLLVMFunction(ft.This)
				spec, err := c.newCallableContextCallSpec(inst, llvmFn, c.callArgIDs(inst), "yak_call_target")
				if err != nil {
					return err
				}
				return c.lowerResolvedContextCall(spec)
			}
		}
	}

	calleeName := c.resolveCalleeName(fn, inst.Method)

	// Stdlib dispatch calls.
	if binding, ok := c.getExternBinding(calleeName); ok && binding.DispatchID != 0 {
		spec, err := c.newDispatchContextCallSpec(inst, binding)
		if err != nil {
			return err
		}
		return c.lowerResolvedContextCall(spec)
	}

	// Context-ABI extern/hook calls.
	if binding, ok := c.getExternBinding(calleeName); ok && binding.Symbol != "" {
		llvmFn := c.getOrDeclareExternCallable(binding.Symbol)
		spec, err := c.newCallableContextCallSpec(inst, llvmFn, append([]int64{}, inst.Args...), "yak_extern_target")
		if err != nil {
			return err
		}
		return c.lowerResolvedContextCall(spec)
	}

	// Fallback: call a named function using the context ABI.
	llvmFn := c.getOrDeclareExternCallable(calleeName)
	if !llvmFn.IsNil() {
		spec, err := c.newCallableContextCallSpec(inst, llvmFn, append([]int64{}, inst.Args...), "yak_fallback_target")
		if err != nil {
			return err
		}
		return c.lowerResolvedContextCall(spec)
	}

	return fmt.Errorf("compileCall: unable to resolve callee %q", calleeName)
}
