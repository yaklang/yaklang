package compiler

import (
	"fmt"
	"math"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func (c *Compiler) compileInstruction(inst ssa.Instruction) error {
	id := inst.GetId()

	switch op := inst.(type) {
	case *ssa.BinOp:
		return c.compileBinOp(op, id)
	case *ssa.Jump:
		return c.compileJump(op)
	case *ssa.If:
		return c.compileIf(op)
	case *ssa.Loop:
		return c.compileLoop(op)
	case *ssa.Switch:
		return c.compileSwitch(op)
	case *ssa.Return:
		return c.compileReturn(op)
	case *ssa.ConstInst:
		return c.compileConst(op)
	case *ssa.Call:
		return c.compileCall(op)
	case *ssa.SideEffect:
		return c.compileSideEffectInstruction(op)
	case *ssa.Panic:
		return c.compilePanic(op)
	case *ssa.Assert:
		return c.compileAssert(op)
	case *ssa.Recover:
		return c.compileRecover(op)
	case *ssa.Make:
		return c.compileMake(op)
	case *ssa.ParameterMember:
		return c.compileParameterMember(op)
	case *ssa.Undefined:
		return c.compileUndefined(op)
	case *ssa.TypeCast:
		return c.compileTypeCast(op)
	case *ssa.UnOp:
		return c.compileUnOp(op, id)
	case *ssa.Next:
		return c.compileNext(op)
	default:
		// Ignore unimplemented instructions for now
		return nil
	}
}

func (c *Compiler) finishGetValue(contextInst ssa.Instruction, id int64) (llvm.Value, error) {
	if val, ok := c.getCachedValue(contextInst, id); ok {
		return val, nil
	}
	return llvm.Value{}, fmt.Errorf("getValue: value %d not materialized", id)
}

// getValue resolves an SSA value ID to an LLVM value, performing lazy compilation
// for constants if they haven't been visited yet.
func (c *Compiler) getValue(contextInst ssa.Instruction, id int64) (llvm.Value, error) {
	if id == 0 {
		return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false), nil
	}

	// Exception values (try/catch `err`) are backed by the current function's panic slot.
	// These values can be referenced in multiple blocks, so do not cache the load.
	if c != nil && c.function != nil && c.function.exceptionValueIDs != nil {
		if _, ok := c.function.exceptionValueIDs[id]; ok {
			return c.loadContextPanic(fmt.Sprintf("yak_exc_%d", id))
		}
	}

	// 1. Find the SSA value. Dynamic member reads are intentionally handled
	// before the cache below because field writes can mutate their backing
	// runtime object after the member value was first materialized.
	var fn *ssa.Function
	if contextInst != nil {
		fn = contextInst.GetFunc()
	} else {
		fn = c.currentFunction()
	}

	if fn == nil {
		return llvm.Value{}, fmt.Errorf("getValue: cannot determine function (contextInst is nil and current function is nil)")
	}

	valObj, ok := fn.GetValueById(id)
	if !ok {
		return llvm.Value{}, fmt.Errorf("getValue: value %d not found in function", id)
	}
	c.ensureOwnerValueIDs(fn)
	if _, owned := c.function.ownerValueIDs[id]; !owned {
		// Cross-function reference (front-end artifact): never compile a value
		// owned by another function from this function's context. The front end
		// can misplace loop-body values (e.g. "#<next>.field") into the entry
		// function while the referenced Next belongs to the callee; allow those
		// only when their object/key chain points back into the current
		// function so the callee can compile them correctly.
		if !c.valueHasLocalDependency(fn, valObj) {
			return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false), nil
		}
	}
	if se, ok := valObj.(*ssa.SideEffect); ok {
		err := c.withLazyCompileInsertPoint(contextInst, se, func() error {
			return c.compileSideEffectValue(se)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileSideEffect succeeded but value %d not cached", id)
	}
	if inst, ok := valObj.(ssa.Instruction); ok && inst.IsLazy() {
		if self := inst.Self(); self != nil {
			if materialized, ok := self.(ssa.Value); ok && materialized != nil {
				valObj = materialized
			}
		}
	}

	if memberVal, ok := valObj.(ssa.Value); ok && c.shouldReadMemberValueDynamically(memberVal, id) {
		if err := c.compileDynamicMemberValue(contextInst, memberVal); err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileMemberCall succeeded but value %d not cached", id)
	}

	// 2. Check cache for non-dynamic values.
	if val, ok := c.getCachedValue(contextInst, id); ok {
		return val, nil
	}

	// ExternLib values are compile-time module handles; they are not runtime objects.
	if extern, ok := ssa.ToExternLib(valObj); ok && extern != nil {
		val := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		c.cacheValue(id, val)
		return c.finishGetValue(contextInst, id)
	}

	// 3. Lazy compile if ConstInst
	if constInst, ok := valObj.(*ssa.ConstInst); ok {
		if err := c.compileConst(constInst); err != nil {
			return llvm.Value{}, err
		}
		// Should be in cache now
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileConst succeded but value %d not cached", id)
	}

	// 3. Lazy compile if Phi (slot-backed; incoming stores emitted in resolvePhi)
	if phi, ok := valObj.(*ssa.Phi); ok && phi != nil {
		if err := c.ensurePhiNode(phi); err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return c.loadSSAValue(id), nil
	}

	// 4. Lazy compile if Parameter (function argument / closure binding)
	if param, ok := ssa.ToParameter(valObj); ok && param != nil {
		if param.GetFunc() != nil && c.currentFunction() != nil && param.GetFunc() != c.currentFunction() {
			return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false), nil
		}
		var loadedOK bool
		err := c.withEntryInsertPoint(fn, func() error {
			val, ok := c.loadBoundParameterValue(fn, param)
			if ok {
				loadedOK = true
				c.cacheValue(id, val)
			}
			return nil
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if loadedOK {
			return c.finishGetValue(contextInst, id)
		}
		if def := param.GetDefault(); def != nil {
			if ssaFn, ok := ssa.ToFunction(def); ok && ssaFn != nil {
				llvmFn, _ := c.getOrDeclareLLVMFunction(ssaFn)
				val := c.Builder.CreatePtrToInt(llvmFn, c.LLVMCtx.Int64Type(), "yak_fn_i64")
				c.cacheValue(id, val)
				return c.finishGetValue(contextInst, id)
			}
			if defConst, ok := ssa.ToConstInst(def); ok {
				if err := c.compileConst(defConst); err == nil {
					if val, ok := c.Values[id]; ok {
						return val, nil
					}
				}
			}
		}
		val := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		c.cacheValue(id, val)
		return c.finishGetValue(contextInst, id)
	}

	// 5. Treat unresolved globals / placeholders as nil i64 for compile-through.
	// Extern member values (e.g. ssa.ModeAll) are Undefined placeholders; lower them via
	// MemberCall / yaklib export instead of folding to zero here.
	if undef, ok := valObj.(*ssa.Undefined); ok && undef != nil {
		isMember := false
		if mc, ok := valObj.(ssa.MemberCall); ok && mc.IsMember() {
			isMember = true
		}
		if undef.IsExtern() {
			if isMember {
				// fall through to MemberCall lowering
			} else if pkg, key, ok := splitExternValueName(undef.GetName()); ok {
				if err := c.compileYaklibExportMember(contextInst, undef, pkg, key); err == nil {
					if val, ok := c.getCachedValue(contextInst, id); ok {
						return val, nil
					}
				}
			} else {
				switch undef.Kind {
				case ssa.UndefinedValueValid, ssa.UndefinedValueInValid, ssa.UndefinedMemberInValid:
					val := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
					c.cacheValue(id, val)
					return val, nil
				}
			}
		} else if !isMember {
			switch undef.Kind {
			case ssa.UndefinedValueValid, ssa.UndefinedValueInValid, ssa.UndefinedMemberInValid:
				val := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
				c.cacheValue(id, val)
				return val, nil
			}
		}
	}

	// 6. Lazy compile if ParameterMember (Value, not Instruction)
	if pm, ok := valObj.(*ssa.ParameterMember); ok {
		err := c.withLazyCompileInsertPoint(contextInst, pm, func() error {
			return c.compileParameterMember(pm)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileParameterMember succeeded but value %d not cached", id)
	}

	// 7. Lazy compile if TypeCast
	if tc, ok := valObj.(*ssa.TypeCast); ok {
		if err := c.compileTypeCast(tc); err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileTypeCast succeeded but value %d not cached", id)
	}

	// 9. Lazy compile if Make
	if mk, ok := valObj.(*ssa.Make); ok {
		err := c.withLazyCompileInsertPoint(contextInst, mk, func() error {
			return c.compileMake(mk)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileMake succeeded but value %d not cached", id)
	}

	// 10. Lazy compile if BinOp
	if binOp, ok := valObj.(*ssa.BinOp); ok {
		err := c.withLazyCompileInsertPoint(contextInst, binOp, func() error {
			return c.compileBinOp(binOp, id)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileBinOp succeeded but value %d not cached", id)
	}

	// 11. Lazy compile if Call
	if callInst, ok := valObj.(*ssa.Call); ok {
		err := c.withLazyCompileInsertPoint(contextInst, callInst, func() error {
			return c.compileCall(callInst)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileCall succeeded but value %d not cached", id)
	}

	// 12. Lazy compile if UnOp
	if unOp, ok := valObj.(*ssa.UnOp); ok {
		err := c.withLazyCompileInsertPoint(contextInst, unOp, func() error {
			return c.compileUnOp(unOp, id)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileUnOp succeeded but value %d not cached", id)
	}

	// 13. Lazy compile if Recover
	if rec, ok := valObj.(*ssa.Recover); ok {
		err := c.withLazyCompileInsertPoint(contextInst, rec, func() error {
			return c.compileRecover(rec)
		})
		if err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.getCachedValue(contextInst, id); ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileRecover succeeded but value %d not cached", id)
	}

	// 14. Lazy compile if Next
	if next, ok := valObj.(*ssa.Next); ok {
		if next.GetFunc() != nil && c.currentFunction() != nil && next.GetFunc() != c.currentFunction() {
			return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false), nil
		}
		if !c.isSSAValueStored(id) {
			err := c.withLazyCompileInsertPoint(contextInst, next, func() error {
				return c.compileNext(next)
			})
			if err != nil {
				return llvm.Value{}, err
			}
			if val, ok := c.getCachedValue(contextInst, id); ok {
				return val, nil
			}
			return llvm.Value{}, fmt.Errorf("getValue: compileNext succeeded but value %d not cached", id)
		}
		return c.loadSSAValue(id), nil
	}

	// 15. Generic MemberCall
	if mc, ok := valObj.(ssa.MemberCall); ok && mc.IsMember() {
		if _, isFn := ssa.ToFunction(valObj); isFn {
			// A function value stored as an object member is a closure/function
			// pointer, not a member read: fall through to case 16 so it is
			// materialized directly instead of being read back through the object.
		} else {
			if err := c.compileMemberCall(contextInst, valObj, mc); err != nil {
				return llvm.Value{}, err
			}
			if val, ok := c.getCachedValue(contextInst, id); ok {
				return val, nil
			}
			return llvm.Value{}, fmt.Errorf("getValue: compileMemberCall succeeded but value %d not cached", id)
		}
	}

	// 16. Function values are materialized as i64 function pointers in the
	// unified InvokeContext representation. A yaklib export used as a value is
	// materialized as a dispatch closure instead of an undefined symbol.
	if ssaFn, ok := ssa.ToFunction(valObj); ok && ssaFn != nil {
		if ssaFn.IsExtern() {
			if pkg, method, ok := splitQualifiedName(ssaFn.GetName()); ok {
				if _, ok := yaklang.LookupExport(pkg, method); ok {
					c.recordYaklibDependency(pkg, method)
					val := c.materializeYaklibExportCallable(valObj, pkg, method)
					c.cacheValue(id, val)
					return val, nil
				}
				// Modules registered only in the AOT runtime table (e.g. poc,
				// http via runtimeRegisterYaklibModule) are still yaklib
				// dependencies for tier selection and per-module DCE.
				c.recordYaklibDependency(pkg, method)
			}
		}
		llvmFn, _ := c.getOrDeclareLLVMFunction(ssaFn)
		val := c.Builder.CreatePtrToInt(llvmFn, c.LLVMCtx.Int64Type(), "yak_fn_i64")
		c.cacheValue(id, val)
		return val, nil
	}

	// 17. Return error if not found and not a constant
	// This usually means we are referencing an instruction that hasn't been compiled yet
	// (back-edge or dependency order issue) or not implemented.
	return llvm.Value{}, fmt.Errorf("getValue: value %d (%T) not found (dependency missing?)", id, valObj)
}

func (c *Compiler) compileBinOp(inst *ssa.BinOp, resultID int64) error {
	if _, ok := c.getCachedValue(inst, resultID); ok {
		return nil
	}
	lhs, err := c.getValue(inst, inst.X)
	if err != nil {
		return err
	}
	rhs, err := c.getValue(inst, inst.Y)
	if err != nil {
		return err
	}

	var val llvm.Value
	name := fmt.Sprintf("val_%d", resultID)

	switch inst.Op {
	case ssa.OpAdd:
		if inst.GetType() != nil && inst.GetType().GetTypeKind() == ssa.StringTypeKind {
			val = c.emitRuntimeConcat(lhs, rhs, name)
		} else {
			val = c.Builder.CreateAdd(lhs, rhs, name)
		}
	case ssa.OpSub:
		val = c.Builder.CreateSub(lhs, rhs, name)
	case ssa.OpMul:
		val = c.Builder.CreateMul(lhs, rhs, name)
	case ssa.OpDiv:
		val = c.Builder.CreateSDiv(lhs, rhs, name)
	case ssa.OpMod:
		val = c.Builder.CreateSRem(lhs, rhs, name)
	case ssa.OpGt:
		val = c.Builder.CreateZExt(c.Builder.CreateICmp(llvm.IntSGT, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpLt:
		val = c.Builder.CreateZExt(c.Builder.CreateICmp(llvm.IntSLT, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpGtEq:
		val = c.Builder.CreateZExt(c.Builder.CreateICmp(llvm.IntSGE, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpLtEq:
		val = c.Builder.CreateZExt(c.Builder.CreateICmp(llvm.IntSLE, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpEq:
		spec, err := c.newRuntimeEqDispatchSpec(inst, false)
		if err != nil {
			return err
		}
		return c.lowerResolvedContextCall(spec)
	case ssa.OpNotEq:
		spec, err := c.newRuntimeEqDispatchSpec(inst, true)
		if err != nil {
			return err
		}
		return c.lowerResolvedContextCall(spec)
	case ssa.OpIn:
		spec, err := c.newRuntimeInDispatchSpec(inst)
		if err != nil {
			return err
		}
		return c.lowerResolvedContextCall(spec)
	case ssa.OpShl:
		val = c.Builder.CreateShl(lhs, rhs, name)
	case ssa.OpShr:
		val = c.Builder.CreateAShr(lhs, rhs, name)
	case ssa.OpAnd:
		val = c.Builder.CreateAnd(lhs, rhs, name)
	case ssa.OpLogicAnd:
		val = c.Builder.CreateAnd(lhs, rhs, name)
	case ssa.OpOr:
		val = c.Builder.CreateOr(lhs, rhs, name)
	case ssa.OpLogicOr:
		val = c.Builder.CreateOr(lhs, rhs, name)
	case ssa.OpXor:
		val = c.Builder.CreateXor(lhs, rhs, name)
	default:
		return fmt.Errorf("unknown BinOp opcode: %v", inst.Op)
	}

	c.cacheValue(resultID, val)
	if err := c.maybeEmitMemberSet(inst, inst, resultID); err != nil {
		return err
	}
	return nil
}

func (c *Compiler) compileConst(inst *ssa.ConstInst) error {
	id := inst.GetId()
	if _, ok := c.getCachedValue(inst, id); ok {
		return c.finishConstValue(inst, id)
	}

	// Handle different constant types
	// For now, assume int64 unless we can detect otherwise
	if inst.GetRawValue() == nil {
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		c.cacheValue(id, llvmVal)
		return c.finishConstValue(inst, id)
	}
	if inst.IsNumber() {
		// Use Int64 for simplicity as per Phase 1
		val := inst.Number()
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(val), true) // Signed
		c.cacheValue(id, llvmVal)
		return c.finishConstValue(inst, id)
	} else if inst.IsFloat() {
		bits := math.Float64bits(inst.Float())
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), bits, false)
		c.cacheValue(id, llvmVal)
		return c.finishConstValue(inst, id)
	} else if inst.IsBoolean() {
		// Represent bool as i64 0 or 1 for compatibility with mixed ops,
		// or handle strictly.
		// NOTE: BinOps expect i64 operands in our current implementation.
		// If explicit bool type needed, we might need zext/sext.
		// Let's use i64 0/1 for now.
		bVal := inst.Boolean()
		iVal := uint64(0)
		if bVal {
			iVal = 1
		}
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), iVal, false)
		c.cacheValue(id, llvmVal)
		return c.finishConstValue(inst, id)
	} else if inst.IsString() || (inst.GetType() != nil && inst.GetType().GetTypeKind() == ssa.BytesTypeKind) {
		ptr := c.Builder.CreateGlobalStringPtr(inst.VarString(), fmt.Sprintf("str_%d", id))
		// Represent pointers as i64 (uintptr) in LLVM IR.
		// NOTE: Do not tag here. Tagging is applied selectively at stdlib
		// call sites (e.g. print/println) so non-print stdlib calls can receive
		// raw C-string pointers. Bytes constants use the same C-string
		// representation; the runtime converts them to []byte on demand.
		llvmVal := llvm.ConstPtrToInt(ptr, c.LLVMCtx.Int64Type())
		c.cacheValue(id, llvmVal)
		return c.finishConstValue(inst, id)
	} else if rawBool, ok := inst.GetRawValue().(bool); ok {
		// A boolean whose SSA type is Any (e.g. true written through a
		// dynamic map key) still must compile to 0/1.
		iVal := uint64(0)
		if rawBool {
			iVal = 1
		}
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), iVal, false)
		c.cacheValue(id, llvmVal)
		return c.finishConstValue(inst, id)
	}

	// Fallback/TODO: floats, nil
	// For now, log warning or create undef?
	// Return 0 for unknown to prevent crash?
	fmt.Printf("WARNING: Unsupported constant type for %v (ID: %d)\n", inst.GetRawValue(), id)
	llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	c.cacheValue(id, llvmVal)
	return c.finishConstValue(inst, id)
}

func (c *Compiler) compileReturn(inst *ssa.Return) error {
	retVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	if len(inst.Results) > 0 {
		val, err := c.getValue(inst, inst.Results[0])
		if err != nil {
			return err
		}
		retVal = c.coerceToInt64(val)
		// A returned closure must be materialized in the function that owns its
		// free values (e.g. a counter factory returning func() { n++; return n }).
		// Materializing at the caller's call site would resolve the captured
		// variable from the caller's scope, where it does not exist.
		if fn := inst.GetFunc(); fn != nil {
			if ssaFn, ok := c.functionValueForArg(fn, inst.Results[0]); ok && ssaFn != nil && !ssaFn.IsExtern() && len(ssaFn.FreeValues) > 0 {
				closure, err := c.materializeCallableClosure(inst, ssaFn)
				if err != nil {
					return fmt.Errorf("compileReturn: materialize returned closure: %w", err)
				}
				retVal = c.coerceToInt64(closure)
			}
		}
	}
	// Apply the closure's return side effects: writes to captured variables
	// must go through the by-reference free-value slots so mutable state
	// persists across calls (counter factories, i++ in per-iteration loop
	// closures). This must also run for implicit exits (functions without an
	// explicit ret), so closures like `func() { count += 1 }` still write back.
	if fn := inst.GetFunc(); fn != nil {
		if err := c.applyClosureSideEffectWriteback(fn, inst); err != nil {
			return err
		}
	}
	if err := c.storeContextReturn(retVal); err != nil {
		return err
	}

	// If this function has a DeferBlock, route all returns through it.
	currentFunction := c.currentFunction()
	if currentFunction != nil && currentFunction.DeferBlock > 0 && c.function != nil && !c.function.returnBlock.IsNil() {
		deferBB, ok := c.Blocks[currentFunction.DeferBlock]
		if !ok {
			return fmt.Errorf("compileReturn: defer block %d not found", currentFunction.DeferBlock)
		}
		c.Builder.CreateBr(deferBB)
		return nil
	}

	c.Builder.CreateRetVoid()
	return nil
}

// applyClosureSideEffectWriteback stores the closure's modified captured
// variables back into their by-reference free-value slots. It is shared by
// explicit returns and implicit function exits so mutable captures persist
// across calls even when the closure body has no ret statement.
func (c *Compiler) applyClosureSideEffectWriteback(fn *ssa.Function, contextInst ssa.Instruction) error {
	if fn == nil || c.function == nil || c.function.freeValuePointers == nil {
		return nil
	}
	var retBlock *ssa.BasicBlock
	if contextInst != nil {
		retBlock = contextInst.GetBlock()
	}
	if retBlock == nil && c.function != nil && c.function.activeBlockID > 0 {
		if blockInst, ok := fn.GetInstructionById(c.function.activeBlockID); ok {
			if blk, ok := ssa.ToBasicBlock(blockInst); ok {
				retBlock = blk
			}
		}
	}
	for _, ser := range fn.SideEffectsReturn {
		for variable, se := range ser {
			if variable == nil || se == nil || se.Modify <= 0 {
				continue
			}
			// Only write back on return paths where the modification actually
			// executed: an early-return branch that skips the assignment (e.g.
			// `if dir { return }` before `files = append(files, x)`) must not
			// clobber the free-value slot with an uninitialized value. The
			// modified value's defining block dominates the return iff every
			// path to the return executed the side effect.
			if !c.sideEffectModifyDominates(fn, se, retBlock) {
				continue
			}
			for _, binding := range callframe.OrderedFreeValueBindings(fn) {
				// Match the side-effect's variable to the free-value binding by
				// name: the binding's variable value is the free-value
				// parameter, which differs from the modified value when the
				// captured variable is reassigned (e.g. files = append(files, x)).
				if se.Variable != nil && binding.Variable != nil {
					if se.Variable.GetName() != binding.Variable.GetName() {
						continue
					}
				} else if se.Name != binding.Name {
					continue
				}
				ptr, ok := c.function.freeValuePointers[binding.ValueID]
				if !ok || ptr.IsNil() {
					continue
				}
				modifyVal, err := c.getValue(contextInst, se.Modify)
				if err != nil {
					continue
				}
				c.Builder.CreateStore(c.coerceToInt64(modifyVal), ptr)
				break
			}
		}
	}
	return nil
}

// sideEffectModifyDominates reports whether the block defining the side
// effect's modified value dominates the return instruction's block, i.e. the
// modification is guaranteed to have executed on every path reaching the
// return. A nil defining block (e.g. a constant) is treated as dominating so
// constant assignments keep their previous writeback behavior.
func (c *Compiler) sideEffectModifyDominates(fn *ssa.Function, se *ssa.FunctionSideEffect, retBlock *ssa.BasicBlock) bool {
	if fn == nil || se == nil || se.Modify <= 0 || retBlock == nil {
		return false
	}
	modifyVal, ok := fn.GetValueById(se.Modify)
	if !ok || modifyVal == nil {
		return false
	}
	modifyBlock := modifyVal.GetBlock()
	if modifyBlock == nil {
		return true
	}
	return blockDominatesInFunction(fn, modifyBlock, retBlock)
}

// blockDominatesInFunction reports whether block a dominates block b in fn's
// CFG: every path from the function entry to b passes through a.
func blockDominatesInFunction(fn *ssa.Function, a, b *ssa.BasicBlock) bool {
	if fn == nil || a == nil || b == nil {
		return false
	}
	if a.GetId() == b.GetId() {
		return true
	}
	// Classic iterative dominator dataflow over the function's block graph:
	// dom[b] = {b} ∪ (∩ dom[p] for p in preds(b)), iterated to a fixpoint.
	entryID := fn.EnterBlock
	blockIDs := fn.Blocks
	dom := make(map[int64]map[int64]struct{}, len(blockIDs))
	for _, id := range blockIDs {
		dom[id] = make(map[int64]struct{})
	}
	if d, ok := dom[entryID]; ok {
		d[entryID] = struct{}{}
	}
	changed := true
	for changed {
		changed = false
		for _, id := range blockIDs {
			if id == entryID {
				continue
			}
			blockInst, ok := fn.GetInstructionById(id)
			if !ok || blockInst == nil {
				continue
			}
			block, ok := ssa.ToBasicBlock(blockInst)
			if !ok || block == nil {
				continue
			}
			newDom := make(map[int64]struct{})
			newDom[id] = struct{}{}
			first := true
			for _, pred := range block.Preds {
				predDom, ok := dom[pred]
				if !ok {
					continue
				}
				if first {
					for k := range predDom {
						newDom[k] = struct{}{}
					}
					first = false
				} else {
					for k := range newDom {
						if _, ok := predDom[k]; !ok {
							delete(newDom, k)
						}
					}
				}
			}
			if !domSetEqual(dom[id], newDom) {
				dom[id] = newDom
				changed = true
			}
		}
	}
	domB, ok := dom[b.GetId()]
	if !ok {
		return false
	}
	_, dominates := domB[a.GetId()]
	return dominates
}

func domSetEqual(a, b map[int64]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func (c *Compiler) compileTypeCast(inst *ssa.TypeCast) error {
	val, err := c.getValue(inst, inst.Value)
	if err != nil {
		return err
	}

	if inst.GetType() != nil {
		sourceKind := ssa.AnyTypeKind
		if fn := inst.GetFunc(); fn != nil {
			if sourceVal, ok := fn.GetValueById(inst.Value); ok && sourceVal != nil && sourceVal.GetType() != nil {
				sourceKind = sourceVal.GetType().GetTypeKind()
			}
		}
		switch inst.GetType().GetTypeKind() {
		case ssa.StringTypeKind:
			if sourceKind == ssa.BytesTypeKind || sourceKind == ssa.StringTypeKind {
				fn, fnType := c.getOrInsertRuntimeToCString()
				argWord := c.coerceToInt64(val)
				val = c.Builder.CreateCall(fnType, fn, []llvm.Value{argWord}, fmt.Sprintf("to_cstring_%d", inst.GetId()))
			} else if sourceKind == ssa.BooleanTypeKind {
				castFn, castType := c.getOrInsertRuntimeBoolToString()
				val = c.Builder.CreateCall(castType, castFn, []llvm.Value{c.coerceToInt64(val)}, fmt.Sprintf("bool_to_str_%d", inst.GetId()))
			} else {
				// Number/null/any source: format through the runtime, which
				// recognizes float bit patterns and keeps nil as "".
				castFn, castType := c.getOrInsertRuntimeToString()
				val = c.Builder.CreateCall(castType, castFn, []llvm.Value{c.coerceToInt64(val)}, fmt.Sprintf("to_string_%d", inst.GetId()))
			}
		case ssa.NumberTypeKind:
			// int()/float() casts share the NumberTypeKind; the float() target
			// carries the "float" type name so the conversion direction is
			// distinguishable at compile time. String sources are parsed.
			if sourceKind == ssa.StringTypeKind {
				if inst.GetType().String() == "float" {
					castFn, castType := c.getOrInsertRuntimeParseFloat()
					val = c.Builder.CreateCall(castType, castFn, []llvm.Value{c.coerceToInt64(val)}, fmt.Sprintf("parse_float_%d", inst.GetId()))
				} else {
					castFn, castType := c.getOrInsertRuntimeParseInt()
					val = c.Builder.CreateCall(castType, castFn, []llvm.Value{c.coerceToInt64(val)}, fmt.Sprintf("parse_int_%d", inst.GetId()))
				}
			} else if inst.GetType().String() == "float" {
				castFn, castType := c.getOrInsertRuntimeToFloat()
				val = c.Builder.CreateCall(castType, castFn, []llvm.Value{c.coerceToInt64(val)}, fmt.Sprintf("to_float_%d", inst.GetId()))
			} else {
				castFn, castType := c.getOrInsertRuntimeToInt()
				val = c.Builder.CreateCall(castType, castFn, []llvm.Value{c.coerceToInt64(val)}, fmt.Sprintf("to_int_%d", inst.GetId()))
			}
		case ssa.BooleanTypeKind:
			// bool(x): any non-zero word is true (float bit patterns included).
			cond := c.coerceToI1(c.coerceToInt64(val), "bool_cast_cond")
			val = c.Builder.CreateZExt(cond, c.LLVMCtx.Int64Type(), "bool_cast")
		}
	}

	val = c.coerceToInt64(val)
	c.cacheValue(inst.GetId(), val)
	if err := c.maybeEmitMemberSet(inst, inst, inst.GetId()); err != nil {
		return err
	}
	return nil
}

func (c *Compiler) getOrInsertRuntimeToInt() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeToIntSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeToFloat() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeToFloatSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeToString() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeToStringSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeBoolToString() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeBoolToStringSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeParseInt() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeParseIntSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeParseFloat() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeParseFloatSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) valueHasLocalDependency(fn *ssa.Function, val ssa.Value) bool {
	if c == nil || c.function == nil || c.function.ownerValueIDs == nil || val == nil {
		return false
	}
	for _, pair := range val.GetObjectKeyPairs() {
		if pair.Object != nil {
			if _, ok := c.function.ownerValueIDs[pair.Object.GetId()]; ok {
				return true
			}
		}
		if pair.Key != nil {
			if _, ok := c.function.ownerValueIDs[pair.Key.GetId()]; ok {
				return true
			}
		}
	}
	for _, variable := range val.GetAllVariables() {
		if variable == nil {
			continue
		}
		if value := variable.GetValue(); value != nil {
			if _, ok := c.function.ownerValueIDs[value.GetId()]; ok {
				return true
			}
		}
	}
	return false
}
