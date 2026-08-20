package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

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

	block := inst.GetBlock()
	if block == nil {
		return fmt.Errorf("compilePanic: panic %d has no block", inst.GetId())
	}
	handlerID := int64(0)
	if c.function != nil && c.function.activeHandlerByBlock != nil {
		handlerID = c.function.activeHandlerByBlock[block.GetId()]
	}
	if handlerID == 0 {
		// Unhandled panic: propagate to caller (through defer if present).
		currentFunction := c.currentFunction()
		if currentFunction != nil && currentFunction.DeferBlock > 0 && c.function != nil && !c.function.returnBlock.IsNil() {
			deferBB, ok := c.Blocks[currentFunction.DeferBlock]
			if !ok {
				return fmt.Errorf("compilePanic: defer block %d not found", currentFunction.DeferBlock)
			}
			c.Builder.CreateBr(deferBB)
			return nil
		}
		c.Builder.CreateRetVoid()
		return nil
	}

	catchBodyID := int64(0)
	if c.function != nil && c.function.catchBodyByHandler != nil {
		catchBodyID = c.function.catchBodyByHandler[handlerID]
	}
	if catchBodyID == 0 {
		// No catch block; propagate to caller (through defer if present).
		currentFunction := c.currentFunction()
		if currentFunction != nil && currentFunction.DeferBlock > 0 && c.function != nil && !c.function.returnBlock.IsNil() {
			deferBB, ok := c.Blocks[currentFunction.DeferBlock]
			if !ok {
				return fmt.Errorf("compilePanic: defer block %d not found", currentFunction.DeferBlock)
			}
			c.Builder.CreateBr(deferBB)
			return nil
		}
		c.Builder.CreateRetVoid()
		return nil
	}

	catchBB, ok := c.Blocks[catchBodyID]
	if !ok {
		return fmt.Errorf("compilePanic: catch body block %d not found", catchBodyID)
	}
	c.Builder.CreateBr(catchBB)
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
		deferBB, ok := c.Blocks[fn.DeferBlock]
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
