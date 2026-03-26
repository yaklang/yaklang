package compiler

import (
	"fmt"

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

	// Persist the panic value for catch/recover paths and for propagation to callers.
	if err := c.storeContextPanic(infoVal); err != nil {
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

func (c *Compiler) compileRecover(inst *ssa.Recover) error {
	if inst == nil {
		return nil
	}

	val, err := c.loadContextPanic(fmt.Sprintf("yak_panic_load_%d", inst.GetId()))
	if err != nil {
		return err
	}
	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = c.coerceToInt64(val)
	}
	return nil
}
