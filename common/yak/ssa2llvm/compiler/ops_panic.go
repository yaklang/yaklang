package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// emitCheckedIntDiv lowers integer division and remainder. A zero divisor
// traps in the CPU (SIGFPE) before any defer/recover can run, so the zero
// case stores a panic and leaves through the function's defer block instead
// of emitting sdiv/srem.
func (c *Compiler) emitCheckedIntDiv(inst *ssa.BinOp, lhs, rhs llvm.Value, name string, isMod bool) (llvm.Value, error) {
	fn := c.currentFunction()
	if fn == nil || c.function == nil || c.function.llvmFn.IsNil() || inst == nil {
		if isMod {
			return c.Builder.CreateSRem(lhs, rhs, name), nil
		}
		return c.Builder.CreateSDiv(lhs, rhs, name), nil
	}

	i64 := c.LLVMCtx.Int64Type()
	zero := llvm.ConstInt(i64, 0, false)
	isZero := c.Builder.CreateICmp(llvm.IntEQ, rhs, zero, name+"_zero")

	origBB := c.currentInsertBlock()
	if origBB.IsNil() {
		if isMod {
			return c.Builder.CreateSRem(lhs, rhs, name), nil
		}
		return c.Builder.CreateSDiv(lhs, rhs, name), nil
	}
	okBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, name+"_ok")
	panicBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, name+"_div0")

	c.Builder.SetInsertPointAtEnd(panicBB)
	msg := "division by zero"
	if isMod {
		msg = "integer modulo by zero"
	}
	msgPtr := c.Builder.CreateGlobalStringPtr(msg, name+"_div0_msg")
	tagged := c.Builder.CreateOr(
		llvm.ConstPtrToInt(msgPtr, i64),
		llvm.ConstInt(i64, yakTaggedPointerMask, false),
		name+"_div0_panic",
	)
	if err := c.storeContextPanic(tagged, abi.FlagPanicTaggedPointer); err != nil {
		return llvm.Value{}, err
	}
	if fn.DeferBlock > 0 && !c.function.returnBlock.IsNil() {
		deferBB, ok := c.ssaBlockEntry(fn.DeferBlock)
		if !ok {
			return llvm.Value{}, fmt.Errorf("emitCheckedIntDiv: defer block %d not found", fn.DeferBlock)
		}
		c.Builder.CreateBr(deferBB)
	} else {
		c.Builder.CreateRetVoid()
	}

	c.Builder.SetInsertPointAtEnd(okBB)
	var val llvm.Value
	if isMod {
		val = c.Builder.CreateSRem(lhs, rhs, name)
	} else {
		val = c.Builder.CreateSDiv(lhs, rhs, name)
	}

	c.Builder.SetInsertPointAtEnd(origBB)
	c.Builder.CreateCondBr(isZero, panicBB, okBB)
	c.Builder.SetInsertPointAtEnd(okBB)
	if block := inst.GetBlock(); block != nil && block.GetId() > 0 {
		c.Blocks[block.GetId()] = okBB
		c.function.activeBlockID = block.GetId()
	}
	return val, nil
}

func (c *Compiler) compilePanic(inst *ssa.Panic) error {
	if inst == nil {
		return nil
	}

	infoVal, err := c.getValue(inst, inst.Info)
	if err != nil {
		return err
	}
	infoVal = c.coerceToInt64(infoVal)

	// Persist the panic value for catch/recover paths and for propagation to callers.
	if err := c.storeContextPanic(infoVal, c.panicValueFlags(inst)); err != nil {
		return err
	}
	return c.branchAfterContextPanic(inst.GetBlock())
}

// continuePastDropErrorPanic splits the current block after `f()~`.
// yak_runtime_drop_error leaves a non-nil error in the call context's panic
// word. That path must enter try/catch (or leave the function) instead of
// executing the rest of the statement.
func (c *Compiler) continuePastDropErrorPanic(inst ssa.Instruction, ctxI64 llvm.Value) error {
	if c == nil || c.function == nil || c.function.llvmFn.IsNil() || inst == nil || ctxI64.IsNil() {
		return nil
	}
	panicVal, err := c.loadCtxWordFrom(ctxI64, abi.WordPanic, "yak_drop_panic")
	if err != nil {
		return err
	}
	i64 := c.LLVMCtx.Int64Type()
	isPanic := c.Builder.CreateICmp(llvm.IntNE, panicVal, llvm.ConstInt(i64, 0, false), "yak_drop_has_panic")

	origBB := c.currentInsertBlock()
	if origBB.IsNil() {
		return fmt.Errorf("continuePastDropErrorPanic: missing insert block")
	}
	contBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("yak_drop_ok_%d", inst.GetId()))
	panicBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("yak_drop_panic_%d", inst.GetId()))

	c.Builder.SetInsertPointAtEnd(panicBB)
	if err := c.storeContextPanic(panicVal, abi.FlagPanicTaggedPointer); err != nil {
		return err
	}
	if err := c.branchAfterContextPanic(inst.GetBlock()); err != nil {
		return err
	}

	c.Builder.SetInsertPointAtEnd(origBB)
	c.Builder.CreateCondBr(isPanic, panicBB, contBB)
	c.Builder.SetInsertPointAtEnd(contBB)
	if block := inst.GetBlock(); block != nil && block.GetId() > 0 {
		c.Blocks[block.GetId()] = contBB
		c.function.activeBlockID = block.GetId()
	}
	return nil
}

// branchAfterContextPanic leaves the current block once a panic is stored on
// the function context: into the active catch, through defer, or back to the caller.
func (c *Compiler) branchAfterContextPanic(block *ssa.BasicBlock) error {
	if block == nil {
		return fmt.Errorf("branchAfterContextPanic: missing block")
	}
	handlerID := int64(0)
	if c.function != nil && c.function.activeHandlerByBlock != nil {
		handlerID = c.function.activeHandlerByBlock[block.GetId()]
	}
	if handlerID == 0 {
		return c.leaveFunctionOnPanic()
	}
	catchBodyID := int64(0)
	if c.function != nil && c.function.catchBodyByHandler != nil {
		catchBodyID = c.function.catchBodyByHandler[handlerID]
	}
	if catchBodyID == 0 {
		return c.leaveFunctionOnPanic()
	}
	catchBB, ok := c.ssaBlockEntry(catchBodyID)
	if !ok {
		return fmt.Errorf("branchAfterContextPanic: catch body block %d not found", catchBodyID)
	}
	c.Builder.CreateBr(catchBB)
	return nil
}

func (c *Compiler) leaveFunctionOnPanic() error {
	currentFunction := c.currentFunction()
	if currentFunction != nil && currentFunction.DeferBlock > 0 && c.function != nil && !c.function.returnBlock.IsNil() {
		deferBB, ok := c.ssaBlockEntry(currentFunction.DeferBlock)
		if !ok {
			return fmt.Errorf("leaveFunctionOnPanic: defer block %d not found", currentFunction.DeferBlock)
		}
		c.Builder.CreateBr(deferBB)
		return nil
	}
	c.Builder.CreateRetVoid()
	return nil
}

func (c *Compiler) panicValueFlags(inst *ssa.Panic) uint64 {
	if c == nil || inst == nil {
		return 0
	}
	fn := inst.GetFunc()
	if fn == nil {
		return 0
	}
	value, ok := fn.GetValueById(inst.Info)
	if !ok || value == nil {
		return 0
	}
	if c.ssaValueIsPointer(value, fn) {
		return abi.FlagPanicTaggedPointer
	}
	return 0
}

func (c *Compiler) compileRecover(inst *ssa.Recover) error {
	if inst == nil {
		return nil
	}

	val, err := c.loadContextPanic(fmt.Sprintf("yak_panic_load_%d", inst.GetId()))
	if err != nil {
		return err
	}
	// recover() both returns the captured panic value and clears it: the
	// function resumes normally and the panic does not propagate to callers.
	// Without this, `defer recover()` in a closure still propagates the panic
	// (e.g. retry's die(111) escapes and the loop never observes count>=4).
	if err := c.clearContextPanic(); err != nil {
		return err
	}
	if inst.GetId() > 0 {
		c.cacheValue(inst.GetId(), c.coerceToInt64(val))
	}
	return nil
}

// compileAssert lowers a yak `assert` SSA instruction by splitting the current
// LLVM block: on a false condition it stores the panic value in the invoke
// context and aborts (through defer if present), mirroring an unhandled panic;
// on a true condition it falls through to a continuation block that carries the
// rest of the enclosing block's instructions. The panic then propagates to the
// entry context and (via the main wrapper) a non-zero exit, so a failing assert
// is observable instead of being silently swallowed.
func (c *Compiler) compileAssert(inst *ssa.Assert) error {
	if inst == nil {
		return nil
	}
	fn := c.currentFunction()
	if fn == nil || c.function == nil || c.function.llvmFn.IsNil() {
		return fmt.Errorf("compileAssert: no active function context")
	}

	condVal, err := c.getValue(inst, inst.Cond)
	if err != nil {
		return err
	}
	condVal = c.coerceToI1(condVal, "assert_cond")

	// Panic value: prefer the runtime MsgValue if present, else the static Msg.
	i64 := c.LLVMCtx.Int64Type()
	panicVal := llvm.Value{}
	panicFlags := uint64(0)
	msg := inst.Msg
	if msg == "" {
		msg = "assert error! no description"
	}
	msgPtr := c.Builder.CreateGlobalStringPtr(msg, fmt.Sprintf("yak_assert_msg_%d", inst.GetId()))
	tagged := c.Builder.CreateOr(llvm.ConstPtrToInt(msgPtr, i64), llvm.ConstInt(i64, yakTaggedPointerMask, false), "yak_assert_panic_str")
	panicVal = tagged
	panicFlags = abi.FlagPanicTaggedPointer

	curBlockID := int64(0)
	if inst.GetBlock() != nil {
		curBlockID = inst.GetBlock().GetId()
	}
	// Capture the enclosing block (the current insert point) BEFORE switching
	// to the panic/continuation blocks below.
	origBB := c.currentInsertBlock()
	if origBB.IsNil() {
		return fmt.Errorf("compileAssert: cannot determine current insert block")
	}
	contBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("yak_assert_cont_%d", inst.GetId()))
	panicBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("yak_assert_panic_%d", inst.GetId()))

	// Failure path: store the panic value and abort (through defer if present).
	// This mirrors the unhandled-panic path in compilePanic so the panic slot is
	// set and the value propagates to callers / the entry context.
	c.Builder.SetInsertPointAtEnd(panicBB)
	if err := c.storeContextPanic(panicVal, panicFlags); err != nil {
		return err
	}
	if fn.DeferBlock > 0 && !c.function.returnBlock.IsNil() {
		deferBB, ok := c.ssaBlockEntry(fn.DeferBlock)
		if !ok {
			return fmt.Errorf("compileAssert: defer block %d not found", fn.DeferBlock)
		}
		c.Builder.CreateBr(deferBB)
	} else {
		c.Builder.CreateRetVoid()
	}

	// The enclosing block currently ends with the assert's condition in flight.
	// End it with a conditional branch: cond true -> continuation (which carries
	// the remaining instructions of the enclosing block), false -> panic block.
	// Emit the branch into origBB, then move the insert point to contBB so the
	// remaining instructions of the enclosing block and its CFG terminator are
	// emitted into the continuation.
	c.Builder.SetInsertPointAtEnd(origBB)
	c.Builder.CreateCondBr(condVal, contBB, panicBB)

	// Subsequent instructions / the block terminator must land in contBB.
	// Re-point the SSA block -> LLVM block mapping so the pass-2 terminator
	// emitter (ensureAllBlockTerminators) emits the succ-branch into contBB.
	c.Builder.SetInsertPointAtEnd(contBB)
	if curBlockID > 0 {
		c.Blocks[curBlockID] = contBB
		c.function.activeBlockID = curBlockID
	}
	return nil
}
