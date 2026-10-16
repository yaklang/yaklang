package compiler

import (
	"fmt"
	"sort"

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

// captureValueIsLocalPhi reports a per-iteration loop variable (`for i :=`).
// Each closure needs a private slot. Recording that slot under the variable
// name makes every later `i` share it, and the pointer is born in the loop
// body, which does not dominate the code after the loop.
func (c *Compiler) captureValueIsLocalPhi(fn *ssa.Function, valueID int64) bool {
	if fn == nil || valueID <= 0 {
		return false
	}
	val, ok := fn.GetValueById(valueID)
	if !ok || val == nil {
		return false
	}
	phi, ok := val.(*ssa.Phi)
	if !ok || phi == nil {
		return false
	}
	lv := phi.GetLastVariable()
	return lv != nil && lv.GetLocal()
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

// ownedNestedClosure is a free-value parameter whose default is a closure
// defined by callerFn. It has to be materialized at this insert point so
// its free values bind to callerFn's cells.
func (c *Compiler) ownedNestedClosure(callerFn *ssa.Function, binding callframe.FreeValueBinding) *ssa.Function {
	if c == nil || callerFn == nil || binding.Variable == nil {
		return nil
	}
	param, ok := ssa.ToParameter(binding.Variable.GetValue())
	if !ok || param == nil {
		return nil
	}
	nested, ok := c.resolveFunctionValue(param.GetDefault())
	if !ok || nested == nil || !c.closureNeedsFreeValues(nested) {
		return nil
	}
	parent := nested.GetParent()
	if parent == nil || parent.GetId() != callerFn.GetId() {
		return nil
	}
	return nested
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
	extras := c.extraCaptures(ssaFn)
	freeCount := len(bindings) + len(extras)
	freeValuesPtr := llvm.ConstPointerNull(i8Ptr)
	// A named function captures its own name so the body can call it. That
	// slot cannot be filled while the object is still being built; the cell
	// is patched with the object after make_callable returns.
	var selfCells []llvm.Value
	if freeCount > 0 {
		mallocFn, mallocType := c.getOrInsertMalloc()
		sizeBytes := llvm.ConstInt(i64, uint64(freeCount*8), false)
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
				if ptr := c.captureSlotPointer(valueID); !ptr.IsNil() {
					// This function already captured the variable by reference.
					// A nested closure (a defer body) must share that slot:
					// copying the current word sees the zero phi, and a fresh
					// slot would not accumulate appends.
					idx := llvm.ConstInt(i64, uint64(index), false)
					slot := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{idx}, "")
					c.Builder.CreateStore(c.Builder.CreatePtrToInt(ptr, i64, "yak_free_parent_ptr"), slot)
					continue
				}
			}
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
			// The binding names a closure defined by this caller, but the
			// resolver found no value here (its default function belongs to
			// itself, not to the caller). Build that closure now, while the
			// caller's capture cells dominate this insert point, and pass the
			// object down. A nested goroutine that instead rebuilds the
			// function initializes the captured scalars to zero.
			if valueID <= 0 {
				if nested := c.ownedNestedClosure(callerFn, binding); nested != nil && !c.isMaterializingCallable(nested) {
					closure, err := c.materializeCallableClosure(contextInst, nested)
					if err != nil {
						return llvm.Value{}, fmt.Errorf("materializeCallableClosure: nested %s: %w", nested.GetName(), err)
					}
					value = c.coerceToInt64(closure)
				}
			}
			mode := c.freeValueCaptureMode(ssaFn, binding)
			isSelf := bindingNamesSelf(ssaFn, binding)
			var selfCell llvm.Value
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
				// the slot for read/write free values of the same variable,
				// and reuse this function's cell so a later assignment and a
				// callback invoked indirectly both see the same memory.
				// A per-iteration loop phi does not join that cell: the next
				// iteration's closure must keep its own value.
				name := ""
				if binding.Variable != nil {
					name = binding.Variable.GetName()
				}
				perIter := c.captureValueIsLocalPhi(callerFn, valueID)
				if !perIter {
					if existing := c.capturedCell(name); !existing.IsNil() {
						value = c.Builder.CreatePtrToInt(existing, i64, "yak_free_cell_ptr")
						break
					}
				}
				if existing, ok := freshSlots[name]; ok && name != "" && !existing.IsNil() {
					value = c.coerceToInt64(existing)
					break
				}
				freshRaw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{llvm.ConstInt(i64, 8, false)}, "yak_free_slot_mem")
				freshPtr := c.Builder.CreateIntToPtr(freshRaw, i64Ptr, "yak_free_slot_i64p")
				c.Builder.CreateStore(value, freshPtr)
				if isSelf {
					selfCell = freshPtr
				}
				if name != "" {
					freshSlots[name] = freshPtr
				}
				if !perIter {
					c.rememberCapturedCell(name, freshPtr)
				}
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
			if isSelf && !selfCell.IsNil() {
				selfCells = append(selfCells, selfCell)
			}
			idx := llvm.ConstInt(i64, uint64(index), false)
			slot := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{idx}, "")
			c.Builder.CreateStore(value, slot)
		}
		if err := c.storeExtraClosureCaptures(contextInst, callerFn, extras, freeI64Ptr, len(bindings), freshSlots, mallocFn, mallocType); err != nil {
			return llvm.Value{}, err
		}
		freeValuesPtr = c.Builder.CreateBitCast(freeI64Ptr, i8Ptr, "yak_callable_free_i8p")
	}

	makeFn, makeType := c.getOrInsertRuntimeMakeCallable()
	closure := c.Builder.CreateCall(makeType, makeFn, []llvm.Value{
		target,
		llvm.ConstInt(i64, uint64(len(ssaFn.ParameterMembers)), false),
		llvm.ConstInt(i64, uint64(freeCount), false),
		freeValuesPtr,
	}, "yak_callable_closure")
	// make_callable copies the slot pointer, not the cell word. Filling the
	// cell now makes a recursive call through this free value see the object.
	for _, cell := range selfCells {
		c.Builder.CreateStore(c.coerceToInt64(closure), cell)
	}
	return closure, nil
}

func bindingNamesSelf(fn *ssa.Function, binding callframe.FreeValueBinding) bool {
	if fn == nil || binding.Variable == nil {
		return false
	}
	param, ok := ssa.ToParameter(binding.Variable.GetValue())
	if !ok || param == nil || param.GetDefault() == nil {
		return false
	}
	self, ok := ssa.ToFunction(param.GetDefault())
	return ok && self != nil && self.GetId() == fn.GetId()
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
	// The default above is the ancestor's original value (the make in the
	// outer function), and the call-site scope holds the final side-effect
	// instruction, which is not a readable word yet. The live cell is this
	// function's by-ref free-value parameter: nested closures, including
	// defer bodies, have to share it so appends accumulate.
	if id := c.callerFreeValueID(callerFn, name); id > 0 {
		return id
	}
	if call != nil && name != "" {
		if actualID := valueIDFromCallScope(call, name, callerFn); actualID > 0 {
			return actualID
		}
	}
	if valueBelongsToFunction(callerFn, binding.ValueID) {
		return binding.ValueID
	}
	// The parent function may have been given this cell only because a nested
	// callback uses it. That slot is keyed by the ancestor value id.
	if origin := freeValueOrigin(binding); origin != nil && origin.GetId() > 0 {
		if ptr := c.captureSlotPointer(origin.GetId()); !ptr.IsNil() {
			return origin.GetId()
		}
	}
	return 0
}

// extraCapture is a free value of a nested function that this closure must
// carry even though its own body never names the variable.
type extraCapture struct {
	Name     string
	OriginID int64
}

func (c *Compiler) closureNeedsFreeValues(fn *ssa.Function) bool {
	return fn != nil && (len(fn.FreeValues) > 0 || len(c.extraCaptures(fn)) > 0)
}

func (c *Compiler) capturedCell(name string) llvm.Value {
	if c == nil || c.function == nil || name == "" || c.function.capturedCells == nil {
		return llvm.Value{}
	}
	info, ok := c.function.capturedCells[name]
	if !ok || info.alloca.IsNil() {
		return llvm.Value{}
	}
	// The pointer store lives in the capturing block. An earlier loop that
	// reuses the name has not executed that store; loading here dereferences
	// the null initializer.
	if !c.capturedCellStoreDominatesUse(info.blockID) {
		return llvm.Value{}
	}
	i64Ptr := llvm.PointerType(c.LLVMCtx.Int64Type(), 0)
	return c.Builder.CreateLoad(i64Ptr, info.alloca, "yak_captured_cell")
}

// capturedCellStoreDominatesUse reports whether the block that publishes the
// capture pointer dominates the block currently being compiled.
func (c *Compiler) capturedCellStoreDominatesUse(storeBlockID int64) bool {
	if c == nil || c.function == nil || c.function.current == nil || storeBlockID <= 0 {
		return false
	}
	useBlockID := c.function.activeBlockID
	if useBlockID <= 0 {
		return false
	}
	if storeBlockID == useBlockID {
		return true
	}
	fn := c.function.current
	storeVal, ok := fn.GetValueById(storeBlockID)
	if !ok || storeVal == nil {
		return false
	}
	useVal, ok := fn.GetValueById(useBlockID)
	if !ok || useVal == nil {
		return false
	}
	storeBB, ok := ssa.ToBasicBlock(storeVal)
	if !ok || storeBB == nil {
		return false
	}
	useBB, ok := ssa.ToBasicBlock(useVal)
	if !ok || useBB == nil {
		return false
	}
	return blockDominatesInFunction(fn, storeBB, useBB)
}

// prepareEntryCaptureCells allocates by-ref cells for closures that capture
// an entry-block constant. The creating call often sits in a loop body, and
// block order compiles the join after that loop before the body, so a cell
// born at the call does not exist yet when the parent read is lowered. The
// cell is published from the entry block and therefore dominates the read.
func (c *Compiler) prepareEntryCaptureCells(fn *ssa.Function) error {
	if c == nil || fn == nil || c.function == nil || len(fn.ChildFuncs) == 0 {
		return nil
	}
	seen := map[int64]struct{}{}
	var children []*ssa.Function
	var walk func(*ssa.Function)
	walk = func(cur *ssa.Function) {
		if cur == nil {
			return
		}
		for _, id := range cur.ChildFuncs {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			child := childFunctionByID(cur, id)
			if child == nil {
				child = childFunctionByID(fn, id)
			}
			if child == nil || child == fn {
				continue
			}
			children = append(children, child)
			walk(child)
		}
	}
	walk(fn)
	if len(children) == 0 {
		return nil
	}
	return c.withEntryInsertPoint(fn, func() error {
		for _, child := range children {
			if err := c.prepareChildCaptureCells(fn, child); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Compiler) prepareChildCaptureCells(parent, child *ssa.Function) error {
	if parent == nil || child == nil {
		return nil
	}
	for _, binding := range callframe.OrderedFreeValueBindings(child) {
		if c.freeValueCaptureMode(child, binding) != freeValueCaptureByRefFresh {
			continue
		}
		valueID := entryCaptureValueID(parent, binding)
		if valueID <= 0 || !c.captureValueDefinedInEntry(parent, valueID) || c.captureValueIsLocalPhi(parent, valueID) {
			continue
		}
		val, ok := parent.GetValueById(valueID)
		if !ok || val == nil {
			continue
		}
		if _, isConst := val.(*ssa.ConstInst); !isConst {
			continue
		}
		name := binding.Name
		if name == "" && binding.Variable != nil {
			name = binding.Variable.GetName()
		}
		if name == "" {
			continue
		}
		if existing := c.capturedCell(name); !existing.IsNil() {
			continue
		}
		mallocFn, mallocType := c.getOrInsertMalloc()
		i64 := c.LLVMCtx.Int64Type()
		raw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{llvm.ConstInt(i64, 8, false)}, "yak_free_slot_mem")
		ptr := c.Builder.CreateIntToPtr(raw, llvm.PointerType(i64, 0), "yak_free_slot_i64p")
		init, err := c.getValue(nil, valueID)
		if err != nil {
			return err
		}
		c.Builder.CreateStore(c.coerceToInt64(init), ptr)
		c.rememberCapturedCell(name, ptr)
	}
	return nil
}

func entryCaptureValueID(parent *ssa.Function, binding callframe.FreeValueBinding) int64 {
	if parent == nil || binding.Variable == nil {
		return 0
	}
	param, ok := ssa.ToParameter(binding.Variable.GetValue())
	if !ok || param == nil || param.GetDefault() == nil {
		return 0
	}
	def := param.GetDefault()
	if def.GetFunc() != parent {
		return 0
	}
	return def.GetId()
}

func (c *Compiler) captureValueDefinedInEntry(fn *ssa.Function, valueID int64) bool {
	if c == nil || fn == nil || valueID <= 0 || fn.EnterBlock <= 0 {
		return false
	}
	val, ok := fn.GetValueById(valueID)
	if !ok || val == nil {
		return false
	}
	block := val.GetBlock()
	if block == nil {
		return true
	}
	return block.GetId() == fn.EnterBlock
}

func (c *Compiler) rememberCapturedCell(name string, ptr llvm.Value) {
	if c == nil || c.function == nil || name == "" || ptr.IsNil() {
		return
	}
	if c.function.capturedCells == nil {
		c.function.capturedCells = make(map[string]capturedCellSlot)
	}
	if existing, ok := c.function.capturedCells[name]; ok && !existing.alloca.IsNil() {
		return
	}
	fn := c.currentFunction()
	if fn == nil || c.entryBlockFor(fn).IsNil() {
		return
	}
	i64Ptr := llvm.PointerType(c.LLVMCtx.Int64Type(), 0)
	var slot llvm.Value
	if err := c.withEntryInsertPoint(fn, func() error {
		slot = buildAlloca(c.Builder, i64Ptr, "yak_captured_cell_slot")
		c.Builder.CreateStore(llvm.ConstPointerNull(i64Ptr), slot)
		return nil
	}); err != nil || slot.IsNil() {
		return
	}
	c.Builder.CreateStore(ptr, slot)
	blockID := c.function.activeBlockID
	c.function.capturedCells[name] = capturedCellSlot{alloca: slot, blockID: blockID}
}

// capturedCellForPhi returns the heap cell of the variable this phi merges.
// Only the phi's own name is consulted: a loop phi can carry extra variable
// bindings that belong to other values.
func (c *Compiler) capturedCellForPhi(phi *ssa.Phi) llvm.Value {
	if c == nil || phi == nil {
		return llvm.Value{}
	}
	if last := phi.GetLastVariable(); last != nil && last.GetName() != "" {
		if ptr := c.capturedCell(last.GetName()); !ptr.IsNil() {
			return ptr
		}
	}
	if name := phi.GetName(); name != "" {
		if ptr := c.capturedCell(name); !ptr.IsNil() {
			return ptr
		}
	}
	return llvm.Value{}
}

func (c *Compiler) capturedCellPointer(val ssa.Value) llvm.Value {
	if c == nil || val == nil {
		return llvm.Value{}
	}
	// GetName is the variable the instruction was created for. A later member
	// use (the count inside a format list) becomes the last variable and
	// would hide the cell.
	if name := val.GetName(); name != "" {
		if ptr := c.capturedCell(name); !ptr.IsNil() {
			return ptr
		}
	}
	if last := val.GetLastVariable(); last != nil {
		if ptr := c.capturedCell(last.GetName()); !ptr.IsNil() {
			return ptr
		}
	}
	for name := range val.GetAllVariables() {
		if ptr := c.capturedCell(name); !ptr.IsNil() {
			return ptr
		}
	}
	return llvm.Value{}
}

// publishCapturedCell stores an assignment into the shared capture cell.
// Side effects and phis are observations of a write the closure already
// performed; storing them again would clobber the callback's update.
func (c *Compiler) publishCapturedCell(inst ssa.Instruction) {
	if c == nil || inst == nil {
		return
	}
	switch inst.(type) {
	case *ssa.SideEffect, *ssa.Phi, *ssa.Jump, *ssa.If, *ssa.Loop, *ssa.Switch, *ssa.Return:
		return
	}
	val, ok := inst.(ssa.Value)
	if !ok || val == nil {
		return
	}
	ptr := c.capturedCellPointer(val)
	if ptr.IsNil() {
		return
	}
	word, ok := c.rawDefinedWord(inst.GetId())
	if !ok || word.IsNil() {
		return
	}
	c.Builder.CreateStore(c.coerceToInt64(word), ptr)
}

func (c *Compiler) rawDefinedWord(id int64) (llvm.Value, bool) {
	if c == nil || id <= 0 {
		return llvm.Value{}, false
	}
	if c.Values != nil {
		if v, ok := c.Values[id]; ok && !v.IsNil() {
			return v, true
		}
	}
	if c.function != nil && c.function.valueSlots != nil {
		if slot, ok := c.function.valueSlots[id]; ok && !slot.IsNil() {
			return c.Builder.CreateLoad(c.LLVMCtx.Int64Type(), slot, fmt.Sprintf("yak_cell_src_%d", id)), true
		}
	}
	return llvm.Value{}, false
}

func (c *Compiler) captureSlotPointer(id int64) llvm.Value {
	if c == nil || c.function == nil || id <= 0 {
		return llvm.Value{}
	}
	if c.function.freeValuePointers != nil {
		if ptr, ok := c.function.freeValuePointers[id]; ok && !ptr.IsNil() {
			return ptr
		}
	}
	if c.function.transitiveCapturePointers != nil {
		if ptr, ok := c.function.transitiveCapturePointers[id]; ok && !ptr.IsNil() {
			return ptr
		}
	}
	return llvm.Value{}
}

func (c *Compiler) extraCaptures(fn *ssa.Function) []extraCapture {
	if c == nil || fn == nil || fn.GetId() <= 0 {
		return nil
	}
	if c.extraCaptureCache == nil {
		c.extraCaptureCache = make(map[int64][]extraCapture)
	}
	if cached, ok := c.extraCaptureCache[fn.GetId()]; ok {
		return cached
	}
	if c.extraCaptureComputing != nil && c.extraCaptureComputing[fn.GetId()] {
		return nil
	}
	if c.extraCaptureComputing == nil {
		c.extraCaptureComputing = make(map[int64]bool)
	}
	c.extraCaptureComputing[fn.GetId()] = true
	defer delete(c.extraCaptureComputing, fn.GetId())

	seen := make(map[string]struct{})
	for _, binding := range callframe.OrderedFreeValueBindings(fn) {
		if binding.Name != "" {
			seen[binding.Name] = struct{}{}
		}
	}
	visited := map[int64]struct{}{fn.GetId(): {}}
	var out []extraCapture
	var walk func(*ssa.Function)
	walk = func(child *ssa.Function) {
		if child == nil || child.GetId() <= 0 {
			return
		}
		if _, ok := visited[child.GetId()]; ok {
			return
		}
		visited[child.GetId()] = struct{}{}
		for _, binding := range callframe.OrderedFreeValueBindings(child) {
			if binding.Name == "" {
				continue
			}
			if _, ok := seen[binding.Name]; ok {
				continue
			}
			origin := freeValueOrigin(binding)
			if origin == nil || origin.GetId() <= 0 {
				continue
			}
			if !c.functionSuppliesCapture(fn.GetParent(), binding.Name, origin.GetId()) {
				continue
			}
			seen[binding.Name] = struct{}{}
			out = append(out, extraCapture{Name: binding.Name, OriginID: origin.GetId()})
		}
		for _, id := range child.ChildFuncs {
			walk(functionByID(child, id))
		}
	}
	for _, id := range fn.ChildFuncs {
		walk(functionByID(fn, id))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].OriginID < out[j].OriginID
		}
		return out[i].Name < out[j].Name
	})
	c.extraCaptureCache[fn.GetId()] = out
	return out
}

func (c *Compiler) functionSuppliesCapture(fn *ssa.Function, name string, originID int64) bool {
	if fn == nil || originID <= 0 {
		return false
	}
	if valueBelongsToFunction(fn, originID) {
		return true
	}
	for variable, id := range fn.FreeValues {
		if id > 0 && variable != nil && variable.GetName() == name {
			return true
		}
	}
	for _, extra := range c.extraCaptures(fn) {
		if extra.OriginID == originID || (name != "" && extra.Name == name) {
			return true
		}
	}
	return false
}

func childFunctionByID(fn *ssa.Function, id int64) *ssa.Function {
	if fn == nil || id <= 0 {
		return nil
	}
	value, ok := fn.GetValueById(id)
	if !ok || value == nil {
		return nil
	}
	if value.IsLazy() {
		if self := value.Self(); self != nil {
			if unwrapped, ok := self.(ssa.Value); ok && unwrapped != nil {
				value = unwrapped
			}
		}
	}
	child, ok := ssa.ToFunction(value)
	if !ok || child == nil {
		return nil
	}
	return child
}

func functionByID(fn *ssa.Function, id int64) *ssa.Function {
	if fn == nil || id <= 0 {
		return nil
	}
	value, ok := fn.GetValueById(id)
	if !ok || value == nil {
		return nil
	}
	child, ok := ssa.ToFunction(value)
	if !ok {
		return nil
	}
	return child
}

func freeValueOrigin(binding callframe.FreeValueBinding) ssa.Value {
	var value ssa.Value
	if binding.Variable != nil {
		value = binding.Variable.GetValue()
	}
	seen := make(map[int64]struct{})
	for value != nil && value.GetId() > 0 {
		id := value.GetId()
		if _, ok := seen[id]; ok {
			return value
		}
		seen[id] = struct{}{}
		param, ok := ssa.ToParameter(value)
		if !ok || param == nil || !param.IsFreeValue || param.GetDefault() == nil {
			return value
		}
		next := param.GetDefault()
		if next == nil || next.GetId() == id {
			return value
		}
		value = next
	}
	return nil
}

func (c *Compiler) storeExtraClosureCaptures(contextInst ssa.Instruction, callerFn *ssa.Function, extras []extraCapture, freeI64Ptr llvm.Value, base int, freshSlots map[string]llvm.Value, mallocFn llvm.Value, mallocType llvm.Type) error {
	if len(extras) == 0 {
		return nil
	}
	i64 := c.LLVMCtx.Int64Type()
	i64Ptr := llvm.PointerType(i64, 0)
	for i, extra := range extras {
		word, shared, err := c.extraCaptureWord(contextInst, callerFn, extra)
		if err != nil {
			return err
		}
		if !shared {
			name := extra.Name
			if existing, ok := freshSlots[name]; ok && !existing.IsNil() {
				word = c.Builder.CreatePtrToInt(existing, i64, "yak_free_extra_reuse")
			} else {
				freshRaw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{llvm.ConstInt(i64, 8, false)}, "yak_free_slot_mem")
				freshPtr := c.Builder.CreateIntToPtr(freshRaw, i64Ptr, "yak_free_slot_i64p")
				c.Builder.CreateStore(word, freshPtr)
				freshSlots[name] = freshPtr
				// This is the only slot for the name. Later assignments in
				// this function publish here, and a nested callback writes
				// the same pointer. A per-iteration loop variable stays
				// private to this closure.
				if !c.captureValueIsLocalPhi(callerFn, extra.OriginID) {
					c.rememberCapturedCell(name, freshPtr)
				}
				word = freshRaw
				c.reinitExtraSlotAtPhiLatch(contextInst, callerFn, extra.OriginID, freshPtr)
			}
		}
		idx := llvm.ConstInt(i64, uint64(base+i), false)
		slot := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{idx}, "")
		c.Builder.CreateStore(word, slot)
	}
	return nil
}

// extraCaptureWord returns the i64 to store and whether it is already a slot
// pointer (shared) rather than the current value.
func (c *Compiler) extraCaptureWord(contextInst ssa.Instruction, callerFn *ssa.Function, extra extraCapture) (llvm.Value, bool, error) {
	i64 := c.LLVMCtx.Int64Type()
	// A named capture in this function already owns the heap cell. An extra
	// capture of the same variable has to carry that pointer, not a snapshot.
	// Per-iteration loop phis are not that shared cell.
	if !c.captureValueIsLocalPhi(callerFn, extra.OriginID) {
		if ptr := c.capturedCell(extra.Name); !ptr.IsNil() {
			return c.Builder.CreatePtrToInt(ptr, i64, "yak_free_cell_ptr"), true, nil
		}
	}
	if ptr := c.captureSlotPointer(extra.OriginID); !ptr.IsNil() {
		return c.Builder.CreatePtrToInt(ptr, i64, "yak_free_parent_ptr"), true, nil
	}
	if id := c.callerFreeValueID(callerFn, extra.Name); id > 0 {
		if ptr := c.captureSlotPointer(id); !ptr.IsNil() {
			return c.Builder.CreatePtrToInt(ptr, i64, "yak_free_parent_ptr"), true, nil
		}
	}
	if callerFn != nil {
		if origin, ok := callerFn.GetValueById(extra.OriginID); ok && origin != nil {
			if phi, ok := origin.(*ssa.Phi); ok && phi != nil && phi.GetFunc() == callerFn {
				if lv := phi.GetLastVariable(); lv != nil && !lv.GetLocal() {
					if slot := c.ensureValueSlot(phi.GetId()); !slot.IsNil() {
						return c.Builder.CreatePtrToInt(slot, i64, "yak_free_shared_ptr"), true, nil
					}
				}
			}
			if constInst, ok := origin.(*ssa.ConstInst); ok && constInst != nil {
				if err := c.compileConst(constInst); err != nil {
					return llvm.Value{}, false, err
				}
				if val, ok := c.getCachedValue(contextInst, constInst.GetId()); ok {
					return c.coerceToInt64(val), false, nil
				}
			}
		}
	}
	if !valueBelongsToFunction(callerFn, extra.OriginID) {
		return llvm.Value{}, false, fmt.Errorf("extra capture %s origin %d is not visible", extra.Name, extra.OriginID)
	}
	resolved, err := c.resolveCallableCaptureValue(contextInst, extra.OriginID)
	if err != nil {
		return llvm.Value{}, false, fmt.Errorf("extra capture %s: %w", extra.Name, err)
	}
	return c.coerceToInt64(resolved), false, nil
}

func (c *Compiler) reinitExtraSlotAtPhiLatch(contextInst ssa.Instruction, callerFn *ssa.Function, valueID int64, freshPtr llvm.Value) {
	if callerFn == nil || valueID <= 0 || freshPtr.IsNil() {
		return
	}
	phiVal, ok := callerFn.GetValueById(valueID)
	if !ok || phiVal == nil {
		return
	}
	phi, ok := phiVal.(*ssa.Phi)
	if !ok || phi == nil || len(phi.Edge) < 2 {
		return
	}
	latchEdgeID := phi.Edge[len(phi.Edge)-1]
	latchEdge, ok := callerFn.GetValueById(latchEdgeID)
	if !ok || latchEdge == nil {
		return
	}
	latchInst, ok := latchEdge.(ssa.Instruction)
	if !ok || latchInst.GetBlock() == nil || contextInst == nil || contextInst.GetBlock() == nil {
		return
	}
	if latchInst.GetBlock().GetId() != contextInst.GetBlock().GetId() {
		return
	}
	_ = c.withInstructionInsertPoint(latchInst, func() error {
		latchVal, err := c.getValue(latchInst, latchEdgeID)
		if err != nil {
			return err
		}
		c.Builder.CreateStore(c.coerceToInt64(latchVal), freshPtr)
		return nil
	})
}

// callerFreeValueID returns this function's free-value parameter for name.
// Same-named bindings share one cell; prefer the one already lowered to a
// by-ref slot so a nested closure can point at it.
func (c *Compiler) callerFreeValueID(fn *ssa.Function, name string) int64 {
	if fn == nil || name == "" || len(fn.FreeValues) == 0 {
		return 0
	}
	best := int64(0)
	bestBound := false
	for variable, id := range fn.FreeValues {
		if variable == nil || variable.GetName() != name || id <= 0 {
			continue
		}
		bound := c != nil && c.function != nil && c.function.freeValuePointers != nil
		if bound {
			if ptr, ok := c.function.freeValuePointers[id]; ok && !ptr.IsNil() {
				if !bestBound || best == 0 || id < best {
					best = id
					bestBound = true
				}
				continue
			}
		}
		if bestBound {
			continue
		}
		if best == 0 || id < best {
			best = id
		}
	}
	return best
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
