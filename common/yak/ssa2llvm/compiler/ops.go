package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
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
	case *ssa.Return:
		return c.compileReturn(op)
	case *ssa.ConstInst:
		return c.compileConst(op)
	case *ssa.Call:
		return c.compileCall(op)
	case *ssa.SideEffect:
		return c.compileSideEffect(op)
	case *ssa.Panic:
		return c.compilePanic(op)
	case *ssa.Recover:
		return c.compileRecover(op)
	case *ssa.Make:
		return c.compileMake(op)
	case *ssa.ParameterMember:
		return c.compileParameterMember(op)
	case *ssa.TypeCast:
		return c.compileTypeCast(op)
	default:
		// Ignore unimplemented instructions for now
		return nil
	}
}

// getValue resolves an SSA value ID to an LLVM value, performing lazy compilation
// for constants if they haven't been visited yet.
func (c *Compiler) getValue(contextInst ssa.Instruction, id int64) (llvm.Value, error) {
	// Exception values (try/catch `err`) are backed by the current function's panic slot.
	// These values can be referenced in multiple blocks, so do not cache the load.
	if c != nil && c.exceptionValueIDs != nil && !c.panicSlot.IsNil() {
		if _, ok := c.exceptionValueIDs[id]; ok {
			i64 := c.LLVMCtx.Int64Type()
			slotPtr := c.Builder.CreateIntToPtr(c.panicSlot, llvm.PointerType(i64, 0), fmt.Sprintf("yak_panic_ptr_%d", id))
			return c.Builder.CreateLoad(i64, slotPtr, fmt.Sprintf("yak_exc_%d", id)), nil
		}
	}

	// 1. Check cache
	if val, ok := c.Values[id]; ok {
		return val, nil
	}

	// 2. Not found, try to find in function and compile if it's a constant
	var fn *ssa.Function
	if contextInst != nil {
		fn = contextInst.GetFunc()
	} else {
		fn = c.CurrentFunction
	}

	if fn == nil {
		return llvm.Value{}, fmt.Errorf("getValue: cannot determine function (contextInst is nil and CurrentFunction is nil)")
	}

	valObj, ok := fn.GetValueById(id)
	if !ok {
		return llvm.Value{}, fmt.Errorf("getValue: value %d not found in function", id)
	}

	// 3. Lazy compile if ConstInst
	if constInst, ok := valObj.(*ssa.ConstInst); ok {
		if err := c.compileConst(constInst); err != nil {
			return llvm.Value{}, err
		}
		// Should be in cache now
		if val, ok := c.Values[id]; ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileConst succeded but value %d not cached", id)
	}

	// 4. Lazy compile if ParameterMember (Value, not Instruction)
	if pm, ok := valObj.(*ssa.ParameterMember); ok {
		if err := c.compileParameterMember(pm); err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.Values[id]; ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileParameterMember succeeded but value %d not cached", id)
	}

	// 5. Lazy compile if TypeCast
	if tc, ok := valObj.(*ssa.TypeCast); ok {
		if err := c.compileTypeCast(tc); err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.Values[id]; ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileTypeCast succeeded but value %d not cached", id)
	}

	// 6. Generic MemberCall
	if mc, ok := valObj.(ssa.MemberCall); ok && mc.IsMember() {
		if err := c.compileMemberCall(valObj, mc); err != nil {
			return llvm.Value{}, err
		}
		if val, ok := c.Values[id]; ok {
			return val, nil
		}
		return llvm.Value{}, fmt.Errorf("getValue: compileMemberCall succeeded but value %d not cached", id)
	}

	// 6. Return error if not found and not a constant
	// This usually means we are referencing an instruction that hasn't been compiled yet
	// (back-edge or dependency order issue) or not implemented.
	return llvm.Value{}, fmt.Errorf("getValue: value %d not found (dependency missing?)", id)
}

func (c *Compiler) compileBinOp(inst *ssa.BinOp, resultID int64) error {
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
		val = c.Builder.CreateAdd(lhs, rhs, name)
	case ssa.OpSub:
		val = c.Builder.CreateSub(lhs, rhs, name)
	case ssa.OpMul:
		val = c.Builder.CreateMul(lhs, rhs, name)
	case ssa.OpDiv:
		val = c.Builder.CreateSDiv(lhs, rhs, name)
	case ssa.OpMod:
		val = c.Builder.CreateSRem(lhs, rhs, name)
	case ssa.OpGt:
		val = buildZExt(c.Builder, c.Builder.CreateICmp(llvm.IntSGT, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpLt:
		val = buildZExt(c.Builder, c.Builder.CreateICmp(llvm.IntSLT, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpGtEq:
		val = buildZExt(c.Builder, c.Builder.CreateICmp(llvm.IntSGE, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpLtEq:
		val = buildZExt(c.Builder, c.Builder.CreateICmp(llvm.IntSLE, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpEq:
		val = buildZExt(c.Builder, c.Builder.CreateICmp(llvm.IntEQ, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	case ssa.OpNotEq:
		val = buildZExt(c.Builder, c.Builder.CreateICmp(llvm.IntNE, lhs, rhs, name), c.LLVMCtx.Int64Type(), name)
	default:
		return fmt.Errorf("unknown BinOp opcode: %v", inst.Op)
	}

	c.Values[resultID] = val
	if err := c.maybeEmitMemberSet(inst, inst, val); err != nil {
		return err
	}
	return nil
}

func (c *Compiler) compileConst(inst *ssa.ConstInst) error {
	id := inst.GetId()
	if _, ok := c.Values[id]; ok {
		return nil // Already compiled
	}

	// Handle different constant types
	// For now, assume int64 unless we can detect otherwise
	if inst.GetRawValue() == nil {
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		c.Values[id] = llvmVal
		if err := c.maybeEmitMemberSet(inst, inst, llvmVal); err != nil {
			return err
		}
		return nil
	}
	if inst.IsNumber() {
		// Use Int64 for simplicity as per Phase 1
		val := inst.Number()
		llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(val), true) // Signed
		c.Values[id] = llvmVal
		if err := c.maybeEmitMemberSet(inst, inst, llvmVal); err != nil {
			return err
		}
		return nil
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
		c.Values[id] = llvmVal
		if err := c.maybeEmitMemberSet(inst, inst, llvmVal); err != nil {
			return err
		}
		return nil
	} else if inst.IsString() {
		ptr := buildGlobalStringPtr(c.Builder, inst.VarString(), fmt.Sprintf("str_%d", id))
		// Represent pointers as i64 (uintptr) in LLVM IR.
		// NOTE: Do not tag here. Tagging is applied selectively at stdlib
		// call sites (e.g. print/println) so non-print stdlib calls can receive
		// raw C-string pointers.
		llvmVal := constPtrToInt(ptr, c.LLVMCtx.Int64Type())
		c.Values[id] = llvmVal
		if err := c.maybeEmitMemberSet(inst, inst, llvmVal); err != nil {
			return err
		}
		return nil
	}

	// Fallback/TODO: floats, nil
	// For now, log warning or create undef?
	// Return 0 for unknown to prevent crash?
	fmt.Printf("WARNING: Unsupported constant type for %v (ID: %d)\n", inst.GetRawValue(), id)
	llvmVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	c.Values[id] = llvmVal
	if err := c.maybeEmitMemberSet(inst, inst, llvmVal); err != nil {
		return err
	}
	return nil
}

func (c *Compiler) compileReturn(inst *ssa.Return) error {
	// If this function has a DeferBlock, route all returns through it.
	if c.CurrentFunction != nil && c.CurrentFunction.DeferBlock > 0 && !c.returnBlock.IsNil() {
		retVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		if len(inst.Results) > 0 {
			val, err := c.getValue(inst, inst.Results[0])
			if err != nil {
				return err
			}
			retVal = c.coerceToInt64(val)
		}
		if c.returnSlot.IsNil() {
			return fmt.Errorf("compileReturn: missing return slot for deferred function %s", c.CurrentFunction.GetName())
		}
		i64 := c.LLVMCtx.Int64Type()
		slotPtr := c.Builder.CreateIntToPtr(c.returnSlot, llvm.PointerType(i64, 0), fmt.Sprintf("yak_ret_ptr_%d", c.CurrentFunction.GetId()))
		c.Builder.CreateStore(retVal, slotPtr)

		deferBB, ok := c.Blocks[c.CurrentFunction.DeferBlock]
		if !ok {
			return fmt.Errorf("compileReturn: defer block %d not found", c.CurrentFunction.DeferBlock)
		}
		c.Builder.CreateBr(deferBB)
		return nil
	}

	// All compiled YakSSA functions currently use an `i64` return type.
	// Yak `return` without values maps to returning 0.
	if len(inst.Results) == 0 {
		c.Builder.CreateRet(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		return nil
	}

	// Only support single return value for now
	retID := inst.Results[0]
	val, err := c.getValue(inst, retID)
	if err != nil {
		return err
	}
	c.Builder.CreateRet(c.coerceToInt64(val))
	return nil
}

func (c *Compiler) compileTypeCast(inst *ssa.TypeCast) error {
	val, err := c.getValue(inst, inst.Value)
	if err != nil {
		return err
	}

	if inst.GetType() != nil && inst.GetType().GetTypeKind() == ssa.StringTypeKind {
		sourceKind := ssa.AnyTypeKind
		if fn := inst.GetFunc(); fn != nil {
			if sourceVal, ok := fn.GetValueById(inst.Value); ok && sourceVal != nil && sourceVal.GetType() != nil {
				sourceKind = sourceVal.GetType().GetTypeKind()
			}
		}
		if sourceKind == ssa.BytesTypeKind || sourceKind == ssa.StringTypeKind {
			fn, fnType := c.getOrInsertRuntimeToCString()
			argPtr := c.coerceToI8Ptr(val)
			val = c.Builder.CreateCall(fnType, fn, []llvm.Value{argPtr}, fmt.Sprintf("to_cstring_%d", inst.GetId()))
		}
	}

	val = c.coerceToInt64(val)
	c.Values[inst.GetId()] = val
	if err := c.maybeEmitMemberSet(inst, inst, val); err != nil {
		return err
	}
	return nil
}
