package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// freeValueCaptureMode decides how a closure captures a free value.
//
//   - ByValue keeps the current behavior: the value at closure creation is
//     copied into the closure object. It is correct for read-only captures
//     that are re-materialized at each call site.
//   - ByRefFresh gives the closure its own heap slot initialized with the
//     current value. Mutable per-iteration captures (for i := ...) and
//     per-call locals (counter factories) need this so state persists across
//     calls without leaking between closures.
//   - ByRefShared points the closure at the parent's existing slot (a loop
//     phi's alloca). Shared loop variables (for k = ...) need this so every
//     closure observes the same final value.
type freeValueCaptureMode int

const (
	freeValueCaptureByValue freeValueCaptureMode = iota
	freeValueCaptureByRefFresh
	freeValueCaptureByRefShared
)

func (c *Compiler) freeValueCaptureMode(fn *ssa.Function, binding callframe.FreeValueBinding) freeValueCaptureMode {
	if fn == nil || binding.ValueID <= 0 {
		return freeValueCaptureByValue
	}
	param, ok := fn.GetValueById(binding.ValueID)
	if !ok {
		return freeValueCaptureByValue
	}
	p, ok := ssa.ToParameter(param)
	if !ok || p == nil || p.GetDefault() == nil {
		return freeValueCaptureByValue
	}
	if phi, ok := p.GetDefault().(*ssa.Phi); ok && phi != nil {
		if lv := phi.GetLastVariable(); lv != nil && !lv.GetLocal() {
			// A non-local loop phi is the shared loop variable: all closures
			// created in the loop must observe the same slot.
			return freeValueCaptureByRefShared
		}
	}
	// Per-iteration loop variables, function locals, and globals all get a
	// private heap slot so mutable captures persist per closure.
	return freeValueCaptureByRefFresh
}

func (c *Compiler) getOrInsertRuntimeMakeCallable() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.MakeCallableSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64, i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) functionValueForArg(fn *ssa.Function, valueID int64) (*ssa.Function, bool) {
	if fn == nil || valueID <= 0 {
		return nil, false
	}
	value, ok := fn.GetValueById(valueID)
	if !ok || value == nil {
		return nil, false
	}
	return c.resolveFunctionValue(value)
}

func (c *Compiler) resolveFunctionValue(value ssa.Value) (*ssa.Function, bool) {
	if value == nil {
		return nil, false
	}
	if inst, ok := value.(ssa.Instruction); ok && inst.IsLazy() {
		if self, ok := inst.Self().(ssa.Value); ok && self != nil {
			value = self
		}
	}
	if param, ok := ssa.ToParameter(value); ok && param != nil && param.GetDefault() != nil {
		value = param.GetDefault()
	}
	if ssaFn, ok := ssa.ToFunction(value); ok && ssaFn != nil && !ssaFn.IsExtern() {
		return ssaFn, true
	}
	if ft, ok := value.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
		return ft.This, true
	}
	return nil, false
}

func (c *Compiler) materializeCallableClosure(contextInst ssa.Instruction, ssaFn *ssa.Function) (llvm.Value, error) {
	if ssaFn == nil {
		return llvm.Value{}, fmt.Errorf("materializeCallableClosure: missing function")
	}
	llvmFn, _ := c.getOrDeclareLLVMFunction(ssaFn)
	if llvmFn.IsNil() {
		return llvm.Value{}, fmt.Errorf("materializeCallableClosure: failed to declare %s", ssaFn.GetName())
	}
	c.enterMaterializingCallable(ssaFn)
	defer c.leaveMaterializingCallable(ssaFn)

	i64 := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	target := c.Builder.CreatePtrToInt(llvmFn, i64, "yak_callable_fn")
	callerFn := c.currentFunction()
	if contextInst != nil && contextInst.GetFunc() != nil {
		callerFn = contextInst.GetFunc()
	}

	bindings := callframe.OrderedFreeValueBindings(ssaFn)
	resolvedIDs := c.callableClosureFreeValueIDs(contextInst, ssaFn)
	freeValuesPtr := llvm.ConstPointerNull(i8Ptr)
	if len(bindings) > 0 {
		mallocFn, mallocType := c.getOrInsertMalloc()
		sizeBytes := llvm.ConstInt(i64, uint64(len(bindings)*8), false)
		raw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{sizeBytes}, "yak_callable_free_mem")
		i64Ptr := llvm.PointerType(i64, 0)
		freeI64Ptr := c.Builder.CreateIntToPtr(raw, i64Ptr, "yak_callable_free_i64p")
		// Read and write free values for the same variable must share one
		// slot; otherwise a closure's n++ writes to a different slot than the
		// one its next call reads from.
		freshSlots := make(map[string]llvm.Value)
		for index, binding := range bindings {
			valueID := int64(0)
			if index < len(resolvedIDs) {
				valueID = resolvedIDs[index]
			}
			value := llvm.ConstInt(i64, 0, false)
			if valueID > 0 {
				if capturedFn, ok := c.functionValueForArg(callerFn, valueID); ok && c.isMaterializingCallable(capturedFn) {
					capturedLLVMFn, _ := c.getOrDeclareLLVMFunction(capturedFn)
					if !capturedLLVMFn.IsNil() {
						value = c.Builder.CreatePtrToInt(capturedLLVMFn, i64, "yak_callable_cycle_fn")
					}
				} else {
					resolved, err := c.resolveCallableCaptureValue(contextInst, valueID)
					if err != nil {
						return llvm.Value{}, fmt.Errorf("materializeCallableClosure: free value %d: %w", valueID, err)
					}
					value = c.coerceToInt64(resolved)
				}
			}
			mode := c.freeValueCaptureMode(ssaFn, binding)
			switch mode {
			case freeValueCaptureByRefShared:
				// Point the closure at the parent's phi slot so every closure
				// created in the loop observes the same shared variable. The
				// resolved valueID is the phi in the caller's function; the
				// closure's own parameter only carries the default.
				if p, ok := callerFn.GetValueById(valueID); ok {
					if phi, ok := p.(*ssa.Phi); ok && phi != nil {
						if slot := c.ensureValueSlot(phi.GetId()); !slot.IsNil() {
							value = c.Builder.CreatePtrToInt(slot, i64, "yak_free_shared_ptr")
						}
					}
				}
			case freeValueCaptureByRefFresh:
				// Give the closure its own heap slot initialized with the
				// current value; mutable captures persist per closure. Reuse
				// the slot for read/write free values of the same variable.
				name := binding.Variable.GetName()
				if existing, ok := freshSlots[name]; ok && !existing.IsNil() {
					value = c.coerceToInt64(existing)
					break
				}
				freshRaw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{llvm.ConstInt(i64, 8, false)}, "yak_free_slot_mem")
				freshPtr := c.Builder.CreateIntToPtr(freshRaw, i64Ptr, "yak_free_slot_i64p")
				c.Builder.CreateStore(value, freshPtr)
				freshSlots[name] = freshPtr
				value = freshRaw
				// A per-iteration variable mutated in the loop body (e.g.
				// b++ after the closure is created) must be captured with the
				// value at the END of the body, not at closure creation. The
				// front end's phi carries that value as its loop-back edge;
				// when the edge lives in the body block, re-initialize the
				// fresh slot at the end of that block.
				if phiVal, ok := callerFn.GetValueById(valueID); ok {
					if phi, ok := phiVal.(*ssa.Phi); ok && phi != nil && len(phi.Edge) >= 2 {
						latchEdgeID := phi.Edge[len(phi.Edge)-1]
						if latchEdge, ok := callerFn.GetValueById(latchEdgeID); ok {
							if latchInst, ok := latchEdge.(ssa.Instruction); ok && latchInst.GetBlock() != nil &&
								contextInst != nil && contextInst.GetBlock() != nil &&
								latchInst.GetBlock().GetId() == contextInst.GetBlock().GetId() {
								_ = c.withInstructionInsertPoint(latchInst, func() error {
									latchVal, err := c.getValue(latchInst, latchEdgeID)
									if err != nil {
										return err
									}
									c.Builder.CreateStore(c.coerceToInt64(latchVal), freshPtr)
									return nil
								})
							}
						}
					}
				}
			}
			idx := llvm.ConstInt(i64, uint64(index), false)
			slot := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{idx}, "")
			c.Builder.CreateStore(value, slot)
		}
		freeValuesPtr = c.Builder.CreateBitCast(freeI64Ptr, i8Ptr, "yak_callable_free_i8p")
	}

	makeFn, makeType := c.getOrInsertRuntimeMakeCallable()
	return c.Builder.CreateCall(makeType, makeFn, []llvm.Value{
		target,
		llvm.ConstInt(i64, uint64(len(ssaFn.ParameterMembers)), false),
		llvm.ConstInt(i64, uint64(len(bindings)), false),
		freeValuesPtr,
	}, "yak_callable_closure"), nil
}

func (c *Compiler) enterMaterializingCallable(fn *ssa.Function) {
	if c == nil || fn == nil || fn.GetId() <= 0 {
		return
	}
	if c.materializingCallableIDs == nil {
		c.materializingCallableIDs = make(map[int64]int)
	}
	c.materializingCallableIDs[fn.GetId()]++
}

func (c *Compiler) leaveMaterializingCallable(fn *ssa.Function) {
	if c == nil || fn == nil || fn.GetId() <= 0 || c.materializingCallableIDs == nil {
		return
	}
	c.materializingCallableIDs[fn.GetId()]--
	if c.materializingCallableIDs[fn.GetId()] <= 0 {
		delete(c.materializingCallableIDs, fn.GetId())
	}
}

func (c *Compiler) isMaterializingCallable(fn *ssa.Function) bool {
	if c == nil || fn == nil || fn.GetId() <= 0 || c.materializingCallableIDs == nil {
		return false
	}
	return c.materializingCallableIDs[fn.GetId()] > 0
}

func (c *Compiler) resolveCallableCaptureValue(contextInst ssa.Instruction, valueID int64) (llvm.Value, error) {
	tagPointerArg := false
	if call, ok := contextInst.(*ssa.Call); ok && call != nil {
		tagPointerArg = c.shouldTagDirectCallArg(call, valueID)
	}
	value, _, err := c.resolveContextCallArg(contextInst, valueID, tagPointerArg)
	if err != nil {
		return llvm.Value{}, err
	}
	return value, nil
}

func (c *Compiler) callableClosureFreeValueIDs(contextInst ssa.Instruction, calleeFn *ssa.Function) []int64 {
	bindings := callframe.OrderedFreeValueBindings(calleeFn)
	if len(bindings) == 0 {
		return nil
	}

	callerFn := c.currentFunction()
	if contextInst != nil && contextInst.GetFunc() != nil {
		callerFn = contextInst.GetFunc()
	}
	call, _ := contextInst.(*ssa.Call)

	out := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, c.resolveCallableFreeValueID(callerFn, call, binding))
	}
	return out
}

func (c *Compiler) resolveCallableFreeValueID(callerFn *ssa.Function, call *ssa.Call, binding callframe.FreeValueBinding) int64 {
	name := binding.Name
	if call != nil && name != "" {
		if actualID, ok := call.Binding[name]; ok && actualID > 0 && valueBelongsToFunction(callerFn, actualID) {
			return actualID
		}
	}
	// Prefer the captured variable's own value (the free-value parameter's
	// default, e.g. the loop phi). The call-site scope can hold a stale
	// same-named variable from an earlier loop, which would capture the wrong
	// value.
	if binding.Variable != nil {
		value := binding.Variable.GetValue()
		if value != nil {
			// The variable's value is the closure's free-value parameter; its
			// default is the captured value in the caller (e.g. the loop phi).
			if param, ok := ssa.ToParameter(value); ok && param != nil && param.GetDefault() != nil {
				def := param.GetDefault()
				if callerFn == nil || def.GetFunc() == callerFn {
					return def.GetId()
				}
			}
			if value.GetId() > 0 && (callerFn == nil || value.GetFunc() == callerFn) {
				return value.GetId()
			}
		}
	}
	if call != nil && name != "" {
		if actualID := valueIDFromCallScope(call, name, callerFn); actualID > 0 {
			return actualID
		}
	}
	if callerFn != nil && name != "" {
		for variable, valueID := range callerFn.FreeValues {
			if variable != nil && variable.GetName() == name && valueID > 0 {
				return valueID
			}
		}
	}
	if valueBelongsToFunction(callerFn, binding.ValueID) {
		return binding.ValueID
	}
	return 0
}

func valueIDFromCallScope(call *ssa.Call, name string, callerFn *ssa.Function) int64 {
	if call == nil || name == "" || call.GetBlock() == nil || call.GetBlock().ScopeTable == nil {
		return 0
	}
	variable := ssa.ReadVariableFromScopeAndParent(call.GetBlock().ScopeTable, name)
	if variable == nil || variable.GetValue() == nil || variable.GetValue().GetId() <= 0 {
		return 0
	}
	value := variable.GetValue()
	if callerFn != nil && value.GetFunc() != callerFn {
		return 0
	}
	return value.GetId()
}

func valueBelongsToFunction(fn *ssa.Function, valueID int64) bool {
	if fn == nil || valueID <= 0 {
		return false
	}
	value, ok := fn.GetValueById(valueID)
	return ok && value != nil && value.GetFunc() == fn
}
