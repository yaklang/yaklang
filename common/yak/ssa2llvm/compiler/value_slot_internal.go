package compiler

/*
#include <llvm-c/Core.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func buildAlloca(b llvm.Builder, t llvm.Type, name string) llvm.Value {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	ct := (C.LLVMTypeRef)(unsafe.Pointer(t.C))
	res := C.LLVMBuildAlloca(cb, ct, cname)
	var out llvm.Value
	*(*unsafe.Pointer)(unsafe.Pointer(&out)) = unsafe.Pointer(res)
	return out
}

func (c *Compiler) isSlotBackedValue(id int64) bool {
	if c == nil || id <= 0 || c.function == nil || c.function.current == nil {
		return false
	}
	valObj, ok := c.function.current.GetValueById(id)
	if !ok || valObj == nil {
		return false
	}
	switch valObj.(type) {
	case *ssa.ConstInst, *ssa.Undefined:
		return false
	}
	if _, ok := ssa.ToFunction(valObj); ok {
		return false
	}
	return true
}

func (c *Compiler) ensureValueSlot(id int64) llvm.Value {
	if c == nil || c.function == nil {
		return llvm.Value{}
	}
	if c.function.valueSlots == nil {
		c.function.valueSlots = make(map[int64]llvm.Value)
	}
	if slot, ok := c.function.valueSlots[id]; ok && !slot.IsNil() {
		return slot
	}

	fn := c.function.current
	if fn == nil || fn.EnterBlock <= 0 {
		return llvm.Value{}
	}
	entryBB, ok := c.Blocks[fn.EnterBlock]
	if !ok || entryBB.IsNil() {
		return llvm.Value{}
	}

	restoreBB := c.restoreInsertBlock(nil)
	prevActive := c.function.activeBlockID
	c.function.activeBlockID = fn.EnterBlock
	if first := entryBB.FirstInstruction(); first.IsNil() {
		c.Builder.SetInsertPointAtEnd(entryBB)
	} else {
		c.Builder.SetInsertPointBefore(first)
	}

	name := fmt.Sprintf("yak_slot_%d", id)
	slot := buildAlloca(c.Builder, c.LLVMCtx.Int64Type(), name)
	c.function.valueSlots[id] = slot

	if !restoreBB.IsNil() {
		if c.blockHasTerminator(restoreBB) {
			c.setInsertPointBeforeTerminator(restoreBB)
		} else {
			c.Builder.SetInsertPointAtEnd(restoreBB)
		}
	}
	c.function.activeBlockID = prevActive
	return slot
}

func (c *Compiler) loadSSAValue(id int64) llvm.Value {
	slot := c.ensureValueSlot(id)
	if slot.IsNil() {
		return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	}
	return c.Builder.CreateLoad(c.LLVMCtx.Int64Type(), slot, fmt.Sprintf("yak_load_%d", id))
}

func (c *Compiler) storeSSAValue(id int64, val llvm.Value) {
	if c == nil || id <= 0 || val.IsNil() {
		return
	}
	slot := c.ensureValueSlot(id)
	if slot.IsNil() {
		return
	}
	c.Builder.CreateStore(c.coerceToInt64(val), slot)
	if c.function != nil {
		if c.function.storedValues == nil {
			c.function.storedValues = make(map[int64]struct{})
		}
		c.function.storedValues[id] = struct{}{}
	}
}

func (c *Compiler) isSSAValueStored(id int64) bool {
	if c == nil || c.function == nil || c.function.storedValues == nil {
		return false
	}
	_, ok := c.function.storedValues[id]
	return ok
}
