package compiler

import (
	"fmt"
	"reflect"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

const yakTaggedPointerMask uint64 = 1 << 62

type contextCallSpec struct {
	inst      ssa.Instruction
	kind      uint64
	target    llvm.Value
	args      []contextCallArg
	async     bool
	ctxName   string
	errPrefix string
}

type contextCallArg struct {
	ssaID         int64
	value         llvm.Value
	root          llvm.Value
	tagPointerArg bool
	// tagBytes reports a comparison operand. Byte literals are raw C strings,
	// like string literals, but their SSA type is bytes so the pointer check
	// leaves them untagged. Equality then compares the address with a []byte
	// shadow (`body != b"a"`) instead of the bytes.
	tagBytes bool
}

func (c *Compiler) getOrInsertRuntimeInvoke() (llvm.Value, llvm.Type) {
	sym := c.runtimeSymName(abi.InvokeSymbol)
	fn := c.Mod.NamedFunction(sym)
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, sym, fnType)
	}
	return fn, fnType
}

func (c *Compiler) emitRuntimeInvoke(ctxI8 llvm.Value) {
	invokeFn, invokeType := c.getOrInsertRuntimeInvoke()
	c.Builder.CreateCall(invokeType, invokeFn, []llvm.Value{ctxI8}, "")
}

func (c *Compiler) resolveContextCallArg(inst ssa.Instruction, argID int64, tagPointerArgs bool, tagBytes ...bool) (llvm.Value, llvm.Value, error) {
	i64 := c.LLVMCtx.Int64Type()
	if inst != nil {
		if fn := inst.GetFunc(); fn != nil {
			if ssaFn, ok := c.functionValueForArg(fn, argID); ok && ssaFn != nil {
				// A call result that is a function value was already
				// materialized as a closure by the callee's return (see
				// compileReturn). Re-materializing here would resolve the
				// free values from the caller's scope, where the callee's
				// locals do not exist.
				alreadyClosure := false
				if val, ok := fn.GetValueById(argID); ok {
					if _, isCall := val.(*ssa.Call); isCall {
						alreadyClosure = true
					}
				}
				if !alreadyClosure {
					closure, err := c.materializeCallableClosure(inst, ssaFn)
					if err != nil {
						return llvm.Value{}, llvm.Value{}, err
					}
					// Record the closure argument so SideEffect instructions
					// after this call can read captured variables back from
					// its free-value slots (e.g. retry(100, f) where f
					// mutates count).
					if c.function != nil && inst != nil {
						if c.function.materializedClosureArgs == nil {
							c.function.materializedClosureArgs = make(map[int64][]llvm.Value)
						}
						c.function.materializedClosureArgs[inst.GetId()] = append(
							c.function.materializedClosureArgs[inst.GetId()], closure)
					}
					if tagPointerArgs {
						tag := llvm.ConstInt(i64, yakTaggedPointerMask, false)
						return c.Builder.CreateOr(closure, tag, "yak_ctx_callable_tag"), closure, nil
					}
					return closure, closure, nil
				}
			}
		}
	}

	argVal, err := c.resolveSSAValueAsInt64(inst, argID, "yak_ctx_fn")
	if err != nil {
		return llvm.Value{}, llvm.Value{}, err
	}

	argI64 := argVal
	root := llvm.ConstInt(i64, 0, false)
	if !tagPointerArgs || inst == nil {
		return argI64, root, nil
	}

	fn := inst.GetFunc()
	if fn == nil {
		return argI64, root, nil
	}

	ssaVal, ok := fn.GetValueById(argID)
	if !ok || ssaVal == nil {
		return argI64, root, nil
	}
	// Byte literals share the C-string representation of string literals, but
	// ssaValueIsPointer does not treat the bytes type as a pointer. Comparison
	// has to tag them so the runtime reads the bytes instead of the address.
	tagByteLiteral := len(tagBytes) > 0 && tagBytes[0] && c.ssaValueIsBytes(ssaVal)
	if !c.ssaValueIsPointer(ssaVal, fn) && !tagByteLiteral {
		return argI64, root, nil
	}

	root = argI64
	tag := llvm.ConstInt(i64, yakTaggedPointerMask, false)
	argI64 = c.Builder.CreateOr(argI64, tag, "yak_ctx_arg_tag")
	return argI64, root, nil
}

func (c *Compiler) resolveContextCallArgValue(inst ssa.Instruction, arg contextCallArg) (llvm.Value, llvm.Value, error) {
	if arg.ssaID > 0 {
		return c.resolveContextCallArg(inst, arg.ssaID, arg.tagPointerArg, arg.tagBytes)
	}

	i64 := c.LLVMCtx.Int64Type()
	value := arg.value
	if value.IsNil() {
		value = llvm.ConstInt(i64, 0, false)
	}
	value = c.coerceToInt64(value)
	root := arg.root
	if root.IsNil() {
		root = llvm.ConstInt(i64, 0, false)
	} else {
		root = c.coerceToInt64(root)
	}
	return value, root, nil
}

func (c *Compiler) emitContextCall(spec contextCallSpec) (llvm.Value, error) {
	if spec.inst == nil {
		return llvm.Value{}, fmt.Errorf("emitContextCall: missing call instruction")
	}
	if spec.target.IsNil() {
		return llvm.Value{}, fmt.Errorf("emitContextCall: missing target for call %d", spec.inst.GetId())
	}

	callBB := c.restoreInsertBlock(spec.inst)
	callBlockID := int64(0)
	if spec.inst.GetBlock() != nil {
		callBlockID = spec.inst.GetBlock().GetId()
	}
	restoreCallInsertPoint := func() {
		if c.function != nil && callBlockID > 0 {
			c.function.activeBlockID = callBlockID
		}
		if !callBB.IsNil() {
			c.restoreInsertPoint(callBB)
		}
	}

	argc := len(spec.args)
	ctxName := spec.ctxName
	if ctxName == "" {
		ctxName = "yak_call_ctx"
	}

	// Allocate and initialize the invoke context at the final insert point
	// (the call instruction's own block), not at the position the caller had
	// when lazy compilation started. Otherwise a forward-referenced call is
	// split: ctx malloc lands in the caller's block while argument stores and
	// the invoke land in the callee instruction's block, reusing a context
	// whose parameter values were computed in the wrong place.
	restoreCallInsertPoint()
	ctxI8, ctxI64, err := c.allocInvokeContext(argc, ctxName)
	if err != nil {
		return llvm.Value{}, err
	}
	if err := c.initInvokeContext(ctxI64, spec.kind, spec.target, argc); err != nil {
		return llvm.Value{}, err
	}
	flags := uint64(0)
	if spec.async {
		flags |= abi.FlagAsync
	}
	if call, ok := spec.inst.(*ssa.Call); ok && call != nil && call.IsEllipsis {
		flags |= abi.FlagEllipsis
	}
	if flags != 0 {
		i64 := c.LLVMCtx.Int64Type()
		if err := c.storeCtxWordFrom(ctxI64, abi.WordFlags, llvm.ConstInt(i64, flags, false)); err != nil {
			return llvm.Value{}, err
		}
	}
	// A defer body runs as its own call. Seed its context with the panicking
	// frame's panic so recover() inside the defer observes it; after the
	// call, copy the (possibly cleared) panic back so a recovered panic does
	// not keep propagating.
	if err := c.inheritDeferPanic(ctxI64, callBlockID); err != nil {
		return llvm.Value{}, err
	}

	for index, arg := range spec.args {
		argI64, root, err := c.resolveContextCallArgValue(spec.inst, arg)
		if err != nil {
			prefix := spec.errPrefix
			if prefix == "" {
				prefix = "emitContextCall"
			}
			restoreCallInsertPoint()
			return llvm.Value{}, fmt.Errorf("%s: failed to resolve argument %d: %w", prefix, index, err)
		}
		restoreCallInsertPoint()
		if err := c.storeInvokeContextArg(ctxI64, index, argI64); err != nil {
			return llvm.Value{}, err
		}
		if err := c.storeInvokeContextRoot(ctxI64, argc, index, root); err != nil {
			return llvm.Value{}, err
		}
	}

	restoreCallInsertPoint()
	c.emitRuntimeInvoke(ctxI8)
	if err := c.propagateDeferPanic(ctxI64, callBlockID); err != nil {
		return llvm.Value{}, err
	}

	zero := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	if spec.async {
		if spec.inst.GetId() > 0 {
			c.setActiveBlockFromInstruction(spec.inst)
			c.storeSSAValue(spec.inst.GetId(), zero)
		}
		return zero, nil
	}

	ret, err := c.loadCtxWordFrom(ctxI64, abi.WordRet, "")
	if err != nil {
		return llvm.Value{}, err
	}
	ret = c.coerceToInt64(ret)
	if spec.inst.GetId() > 0 {
		c.setActiveBlockFromInstruction(spec.inst)
		// A `call~` keeps the first result. Extern yaklib multi-returns are a
		// bare []any, which drop-error unwraps. A yak function's multi-return
		// is a *[]any slice, so index "0" instead. Unpacked calls
		// (a, b = f()~) keep the tuple so later member reads can index it.
		if call, ok := spec.inst.(*ssa.Call); ok && call.IsDropError && !call.Unpack {
			if c.calleeReturnsTuple(call) {
				ret = c.emitRuntimeGetField(ret, "0", call.GetId())
			} else {
				ret = c.emitRuntimeDropError(ctxI8, ret)
				if err := c.continuePastDropErrorPanic(spec.inst, ctxI64); err != nil {
					return llvm.Value{}, err
				}
			}
		}
		c.storeSSAValue(spec.inst.GetId(), ret)
	}
	return ret, nil
}

func (c *Compiler) callInheritsDeferPanic(blockID int64) bool {
	fn := c.currentFunction()
	return fn != nil && c.function != nil && fn.DeferBlock > 0 && blockID == fn.DeferBlock
}

func (c *Compiler) inheritDeferPanic(ctxI64 llvm.Value, blockID int64) error {
	if !c.callInheritsDeferPanic(blockID) {
		return nil
	}
	panicVal, err := c.loadContextWord(abi.WordPanic, "yak_defer_panic")
	if err != nil {
		return err
	}
	if err := c.storeCtxWordFrom(ctxI64, abi.WordPanic, panicVal); err != nil {
		return err
	}
	flagVal, err := c.loadContextWord(abi.WordFlags, "yak_defer_flags")
	if err != nil {
		return err
	}
	i64 := c.LLVMCtx.Int64Type()
	bit := c.Builder.CreateAnd(flagVal, llvm.ConstInt(i64, abi.FlagPanicTaggedPointer, false), "yak_defer_panic_flag")
	existing, err := c.loadCtxWordFrom(ctxI64, abi.WordFlags, "yak_call_flags")
	if err != nil {
		return err
	}
	return c.storeCtxWordFrom(ctxI64, abi.WordFlags, c.Builder.CreateOr(existing, bit, "yak_call_flags_panic"))
}

func (c *Compiler) propagateDeferPanic(ctxI64 llvm.Value, blockID int64) error {
	if !c.callInheritsDeferPanic(blockID) {
		return nil
	}
	panicVal, err := c.loadCtxWordFrom(ctxI64, abi.WordPanic, "yak_defer_panic_back")
	if err != nil {
		return err
	}
	if err := c.storeContextWord(abi.WordPanic, panicVal); err != nil {
		return err
	}
	flagVal, err := c.loadCtxWordFrom(ctxI64, abi.WordFlags, "yak_defer_flags_back")
	if err != nil {
		return err
	}
	parentFlags, err := c.loadContextWord(abi.WordFlags, "yak_parent_flags")
	if err != nil {
		return err
	}
	i64 := c.LLVMCtx.Int64Type()
	keep := llvm.ConstInt(i64, ^abi.FlagPanicTaggedPointer, false)
	bit := llvm.ConstInt(i64, abi.FlagPanicTaggedPointer, false)
	cleared := c.Builder.CreateAnd(parentFlags, keep, "yak_parent_flags_clear_panic")
	calleeBit := c.Builder.CreateAnd(flagVal, bit, "yak_defer_panic_flag_back")
	return c.storeContextWord(abi.WordFlags, c.Builder.CreateOr(cleared, calleeBit, "yak_parent_flags_panic"))
}

// calleeReturnsTuple reports whether a direct yak function returns more than
// one value. Those returns are packed into a slice; `f()~` keeps index "0".
// Extern yaklib calls stay on yak_runtime_drop_error, which unwraps []any.
func (c *Compiler) calleeReturnsTuple(call *ssa.Call) bool {
	if call == nil {
		return false
	}
	fn := call.GetFunc()
	if fn == nil {
		return false
	}
	calleeVal, ok := fn.GetValueById(call.Method)
	if !ok || calleeVal == nil {
		return false
	}
	ssaFn, ok := ssa.ToFunction(calleeVal)
	if !ok || ssaFn == nil || ssaFn.IsExtern() {
		return false
	}
	for _, id := range ssaFn.Return {
		inst, ok := ssaFn.GetValueById(id)
		if !ok || inst == nil {
			continue
		}
		ret, ok := ssa.ToReturn(inst)
		if ok && ret != nil && len(ret.Results) > 1 {
			return true
		}
	}
	return false
}

// callReturnCount reports how many Go values the callee returns. It is used to
// unpack the runtime's multi-return tuple for drop-error calls.
func (c *Compiler) callReturnCount(inst ssa.Instruction) int {
	call, ok := inst.(*ssa.Call)
	if !ok || call == nil {
		return 1
	}
	fn := call.GetFunc()
	if fn == nil {
		return 1
	}
	calleeVal, ok := fn.GetValueById(call.Method)
	if !ok || calleeVal == nil {
		return 1
	}
	if ssaFn, ok := ssa.ToFunction(calleeVal); ok && ssaFn != nil && ssaFn.IsExtern() {
		if pkg, method, ok := splitQualifiedName(ssaFn.GetName()); ok {
			if exported, ok := yaklang.LookupExport(pkg, method); ok && exported != nil {
				t := reflect.TypeOf(exported)
				if t != nil && t.Kind() == reflect.Func {
					return t.NumOut()
				}
			}
		}
	}
	return 1
}

func (c *Compiler) setActiveBlockFromInstruction(inst ssa.Instruction) {
	if c == nil || c.function == nil || inst == nil || inst.GetBlock() == nil {
		return
	}
	c.function.activeBlockID = inst.GetBlock().GetId()
}

func (c *Compiler) finishContextCall(inst ssa.Instruction, result llvm.Value) error {
	if inst == nil || inst.GetId() <= 0 {
		return nil
	}
	result = c.coerceToInt64(result)
	if !c.isSSAValueStored(inst.GetId()) {
		c.cacheValue(inst.GetId(), result)
	}
	if val, ok := inst.(ssa.Value); ok {
		if err := c.maybeEmitMemberSet(inst, val, inst.GetId()); err != nil {
			return err
		}
		if err := c.emitMemberVariableSetsForCompiledObject(inst, val); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) lowerResolvedContextCall(spec contextCallSpec) error {
	result, err := c.emitContextCall(spec)
	if err != nil {
		return err
	}
	return c.finishContextCall(spec.inst, result)
}

func shouldTagStdlibArgPointers(id abi.FuncID) bool {
	return id != 0
}

func ssaArgs(argIDs []int64, tagPointerArgs bool) []contextCallArg {
	args := make([]contextCallArg, 0, len(argIDs))
	for _, argID := range argIDs {
		args = append(args, contextCallArg{
			ssaID:         argID,
			tagPointerArg: tagPointerArgs,
		})
	}
	return args
}
