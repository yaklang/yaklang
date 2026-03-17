package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
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

	// Persist the panic value for catch/recover paths.
	if !c.panicSlot.IsNil() {
		i64 := c.LLVMCtx.Int64Type()
		slotPtr := c.Builder.CreateIntToPtr(c.panicSlot, llvm.PointerType(i64, 0), fmt.Sprintf("yak_panic_ptr_%d", inst.GetId()))
		c.Builder.CreateStore(infoVal, slotPtr)
	}

	block := inst.GetBlock()
	if block == nil {
		return fmt.Errorf("compilePanic: panic %d has no block", inst.GetId())
	}
	handlerID := int64(0)
	if c.activeHandlerByBlock != nil {
		handlerID = c.activeHandlerByBlock[block.GetId()]
	}
	if handlerID == 0 {
		// Unhandled panic: for now, terminate the function.
		c.Builder.CreateRet(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		return nil
	}

	catchBodyID := int64(0)
	if c.catchBodyByHandler != nil {
		catchBodyID = c.catchBodyByHandler[handlerID]
	}
	if catchBodyID == 0 {
		// No catch block; route to final/done if available, else terminate.
		c.Builder.CreateRet(llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		return nil
	}

	catchBB, ok := c.Blocks[catchBodyID]
	if !ok {
		return fmt.Errorf("compilePanic: catch body block %d not found", catchBodyID)
	}
	c.Builder.CreateBr(catchBB)
	return nil
}

func (c *Compiler) compileRecover(inst *ssa.Recover) error {
	if inst == nil {
		return nil
	}

	val := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	if !c.panicSlot.IsNil() {
		i64 := c.LLVMCtx.Int64Type()
		slotPtr := c.Builder.CreateIntToPtr(c.panicSlot, llvm.PointerType(i64, 0), fmt.Sprintf("yak_panic_ptr_%d", inst.GetId()))
		val = c.Builder.CreateLoad(i64, slotPtr, fmt.Sprintf("yak_panic_load_%d", inst.GetId()))
	}
	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = c.coerceToInt64(val)
	}
	return nil
}

