package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) coerceToExternArgType(val llvm.Value, typ LLVMExternType) llvm.Value {
	switch typ {
	case ExternTypePtr:
		return c.coerceToI8Ptr(val)
	case ExternTypeI64:
		return c.coerceToInt64(val)
	default:
		return val
	}
}

func (c *Compiler) getOrInsertRuntimeAsyncCall() (llvm.Value, llvm.Type) {
	name := "yak_runtime_async_call"
	fn := c.Mod.NamedFunction(name)

	i64 := c.LLVMCtx.Int64Type()
	argvPtr := llvm.PointerType(i64, 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i64, i64, argvPtr}, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) compileAsyncLLVMCall(inst *ssa.Call, llvmFn llvm.Value, includeArgMember bool) error {
	asyncFn, asyncFnType := c.getOrInsertRuntimeAsyncCall()

	i64 := c.LLVMCtx.Int64Type()
	argvPtrType := llvm.PointerType(i64, 0)

	callArgs := make([]llvm.Value, 0, len(inst.Args)+len(inst.ArgMember))
	for i, argID := range inst.Args {
		argVal, err := c.getValue(inst, argID)
		if err != nil {
			return fmt.Errorf("compileAsyncLLVMCall: failed to resolve argument %d: %w", i, err)
		}
		callArgs = append(callArgs, c.coerceToInt64(argVal))
	}
	if includeArgMember && len(inst.ArgMember) > 0 {
		for _, memberID := range inst.ArgMember {
			memberVal, err := c.getValue(inst, memberID)
			if err != nil {
				return fmt.Errorf("compileAsyncLLVMCall: failed to resolve arg member %d: %w", memberID, err)
			}
			callArgs = append(callArgs, c.coerceToInt64(memberVal))
		}
	}

	argc := len(callArgs)
	argvPtr := llvm.ConstPointerNull(argvPtrType)
	if argc > 0 {
		mallocFn, mallocType := c.getOrInsertMalloc()
		sizeBytes := llvm.ConstInt(i64, uint64(argc*8), false)
		rawPtr := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{sizeBytes}, "yak_async_argv_mem")
		argvPtr = c.Builder.CreateIntToPtr(rawPtr, argvPtrType, "yak_async_argv_ptr")

		for i, argVal := range callArgs {
			idx := llvm.ConstInt(i64, uint64(i), false)
			elemPtr := c.Builder.CreateGEP(i64, argvPtr, []llvm.Value{idx}, "")
			c.Builder.CreateStore(c.coerceToInt64(argVal), elemPtr)
		}
	}

	fnPtr := c.Builder.CreatePtrToInt(llvmFn, i64, "yak_async_fn_ptr")
	argcVal := llvm.ConstInt(i64, uint64(argc), false)
	c.Builder.CreateCall(asyncFnType, asyncFn, []llvm.Value{fnPtr, argcVal, argvPtr}, "")

	if inst.GetId() > 0 {
		zero := llvm.ConstInt(i64, 0, false)
		c.Values[inst.GetId()] = zero
		if err := c.maybeEmitMemberSet(inst, inst, zero); err != nil {
			return err
		}
	}
	return nil
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
			llvmFn, llvmFnType := c.getOrDeclareLLVMFunction(ssaFn)
			if inst.Async {
				_ = llvmFnType
				return c.compileAsyncLLVMCall(inst, llvmFn, true)
			}
			return c.compileDirectLLVMCall(inst, llvmFn, llvmFnType, nil, false, true)
		}
		if calleeVal.IsMember() {
			if ft, ok := calleeVal.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
				llvmFn, llvmFnType := c.getOrDeclareLLVMFunction(ft.This)
				if inst.Async {
					_ = llvmFnType
					return c.compileAsyncLLVMCall(inst, llvmFn, true)
				}
				return c.compileDirectLLVMCall(inst, llvmFn, llvmFnType, nil, false, true)
			}
		}
	}

	calleeName := c.resolveCalleeName(fn, inst.Method)

	if binding, ok := c.getExternBinding(calleeName); ok && binding.DispatchID != 0 {
		if inst.Async {
			return c.compileAsyncStdlibDispatchCall(inst, binding)
		}
		return c.compileStdlibDispatchCall(inst, binding)
	}

	// 2. Get or declare LLVM function
	// Check externs first
	llvmFn := c.ensureExternDeclaration(calleeName)
	externBinding, hasExternBinding := c.getExternBinding(calleeName)

	if llvmFn.IsNil() {
		llvmFn = c.Mod.NamedFunction(calleeName)
		if llvmFn.IsNil() {
			// Function not found, create a declaration (prototype)
			// Default: all args and return are i64
			argTypes := make([]llvm.Type, len(inst.Args))
			for i := range argTypes {
				argTypes[i] = c.LLVMCtx.Int64Type()
			}
			funcType := llvm.FunctionType(c.LLVMCtx.Int64Type(), argTypes, false)
			llvmFn = llvm.AddFunction(c.Mod, calleeName, funcType)
		}
	}

	// 3. Prepare arguments
	args := make([]llvm.Value, 0, len(inst.Args))
	for i, argID := range inst.Args {
		argVal, err := c.getValue(inst, argID)
		if err != nil {
			return fmt.Errorf("compileCall: failed to resolve argument %d: %w", argID, err)
		}
		if hasExternBinding && i < len(externBinding.Params) {
			argVal = c.coerceToExternArgType(argVal, externBinding.Params[i])
		}
		args = append(args, argVal)
	}

	if inst.Async {
		return c.compileAsyncLLVMCall(inst, llvmFn, false)
	}

	// 4. Create call instruction
	// Get function type for CreateCall
	fnType := llvmFn.GlobalValueType()
	callResult := c.Builder.CreateCall(fnType, llvmFn, args, "")
	if hasExternBinding && externBinding.Return == ExternTypeVoid {
		callResult = llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	} else {
		callResult = c.coerceToInt64(callResult)
	}

	// 5. Register result if the call has users (returns a value)
	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = callResult
		if err := c.maybeEmitMemberSet(inst, inst, callResult); err != nil {
			return err
		}
	}

	return nil
}

func (c *Compiler) compileDirectLLVMCall(inst *ssa.Call, llvmFn llvm.Value, llvmFnType llvm.Type, externBinding *ExternBinding, hasExternBinding bool, includeArgMember bool) error {
	// Prepare arguments
	args := make([]llvm.Value, 0, len(inst.Args))
	for i, argID := range inst.Args {
		argVal, err := c.getValue(inst, argID)
		if err != nil {
			return fmt.Errorf("compileCall: failed to resolve argument %d: %w", argID, err)
		}
		if hasExternBinding && externBinding != nil && i < len(externBinding.Params) {
			argVal = c.coerceToExternArgType(argVal, externBinding.Params[i])
		}
		args = append(args, argVal)
	}

	if includeArgMember && len(inst.ArgMember) > 0 {
		for _, memberID := range inst.ArgMember {
			memberVal, err := c.getValue(inst, memberID)
			if err != nil {
				return fmt.Errorf("compileCall: failed to resolve arg member %d: %w", memberID, err)
			}
			args = append(args, c.coerceToInt64(memberVal))
		}
	}

	callResult := c.Builder.CreateCall(llvmFnType, llvmFn, args, "")
	if hasExternBinding && externBinding != nil && externBinding.Return == ExternTypeVoid {
		callResult = llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	} else {
		callResult = c.coerceToInt64(callResult)
	}

	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = callResult
		if err := c.maybeEmitMemberSet(inst, inst, callResult); err != nil {
			return err
		}
	}
	return nil
}
