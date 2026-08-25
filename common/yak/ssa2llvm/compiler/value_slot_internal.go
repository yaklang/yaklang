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

func (c *Compiler) currentInsertBlock() llvm.BasicBlock {
	if c == nil {
		return llvm.BasicBlock{}
	}
	cb := (C.LLVMBuilderRef)(unsafe.Pointer(c.Builder.C))
	res := C.LLVMGetInsertBlock(cb)
	var out llvm.BasicBlock
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
	entryBB := c.entryBlockFor(fn)
	if entryBB.IsNil() {
		return llvm.Value{}
	}

	restoreBB := c.currentInsertBlock()
	if restoreBB.IsNil() {
		restoreBB = c.restoreInsertBlock(nil)
	}
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

func (c *Compiler) hasValueSlot(id int64) bool {
	if c == nil || c.function == nil || c.function.valueSlots == nil || id <= 0 {
		return false
	}
	slot, ok := c.function.valueSlots[id]
	return ok && !slot.IsNil()
}

func (c *Compiler) loadSSAValue(id int64) llvm.Value {
	if c != nil && c.function != nil && c.function.redirectedSlots != nil {
		if src, ok := c.function.redirectedSlots[id]; ok {
			if ptr, ok := c.redirectedSlotPtr(id, src); ok && !ptr.IsNil() {
				return c.Builder.CreateLoad(c.LLVMCtx.Int64Type(), ptr, fmt.Sprintf("yak_redir_load_%d", id))
			}
		}
	}
	if c != nil && c.function != nil && c.function.freeValuePointers != nil {
		if ptr, ok := c.function.freeValuePointers[id]; ok && !ptr.IsNil() {
			return c.Builder.CreateLoad(c.LLVMCtx.Int64Type(), ptr, fmt.Sprintf("yak_free_load_%d", id))
		}
	}
	slot := c.ensureValueSlot(id)
	if slot.IsNil() {
		return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	}
	return c.Builder.CreateLoad(c.LLVMCtx.Int64Type(), slot, fmt.Sprintf("yak_load_%d", id))
}

// redirectedSlotPtr resolves the closure heap free-value slot pointer for a
// redirected SSA value at the current insert point. The closure object is
// reloaded from its entry-block alloca (so it dominates this use) and the
// runtime fetch is emitted in the using block, so the produced pointer
// dominates the load/store that follows even when the redirecting side-effect
// lives elsewhere.
func (c *Compiler) redirectedSlotPtr(id int64, src redirectedSlotSource) (llvm.Value, bool) {
	if src.closureSlot.IsNil() {
		return llvm.Value{}, false
	}
	closureRaw := c.Builder.CreateLoad(c.LLVMCtx.Int64Type(), src.closureSlot, fmt.Sprintf("yak_redir_closure_%d", id))
	slotFn, slotType := c.getOrInsertRuntimeGetClosureFreeSlot()
	idx := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(src.index), false)
	slotRaw := c.Builder.CreateCall(slotType, slotFn, []llvm.Value{
		closureRaw, idx,
	}, fmt.Sprintf("yak_closure_fv_slot_%d", id))
	i64Ptr := llvm.PointerType(c.LLVMCtx.Int64Type(), 0)
	return c.Builder.CreateIntToPtr(c.coerceToInt64(slotRaw), i64Ptr, fmt.Sprintf("yak_closure_fv_slotp_%d", id)), true
}

// ensureRedirectedClosureAlloca returns (creating on first use) an
// entry-block alloca that parks the closure object word for a redirected SSA
// value. Storing at the redirecting side-effect and reloading at each use
// keeps the closure word dominant over every redirected load/store.
func (c *Compiler) ensureRedirectedClosureAlloca(id int64) llvm.Value {
	if c == nil || c.function == nil {
		return llvm.Value{}
	}
	if c.function.redirectedClosureAllocas == nil {
		c.function.redirectedClosureAllocas = make(map[int64]llvm.Value)
	}
	if slot, ok := c.function.redirectedClosureAllocas[id]; ok && !slot.IsNil() {
		return slot
	}
	fn := c.function.current
	if fn == nil || fn.EnterBlock <= 0 {
		return llvm.Value{}
	}
	entryBB := c.entryBlockFor(fn)
	if entryBB.IsNil() {
		return llvm.Value{}
	}
	restoreBB := c.currentInsertBlock()
	if restoreBB.IsNil() {
		restoreBB = c.restoreInsertBlock(nil)
	}
	prevActive := c.function.activeBlockID
	c.function.activeBlockID = fn.EnterBlock
	if first := entryBB.FirstInstruction(); first.IsNil() {
		c.Builder.SetInsertPointAtEnd(entryBB)
	} else {
		c.Builder.SetInsertPointBefore(first)
	}
	name := fmt.Sprintf("yak_redir_closure_alloca_%d", id)
	slot := buildAlloca(c.Builder, c.LLVMCtx.Int64Type(), name)
	c.function.redirectedClosureAllocas[id] = slot
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

func (c *Compiler) storeSSAValue(id int64, val llvm.Value) {
	if c == nil || id <= 0 || val.IsNil() {
		return
	}
	if c.function != nil && c.function.redirectedSlots != nil {
		if src, ok := c.function.redirectedSlots[id]; ok {
			if ptr, ok := c.redirectedSlotPtr(id, src); ok && !ptr.IsNil() {
				c.Builder.CreateStore(c.coerceToInt64(val), ptr)
				if c.function.storedValues == nil {
					c.function.storedValues = make(map[int64]struct{})
				}
				c.function.storedValues[id] = struct{}{}
				return
			}
		}
	}
	if c.function != nil && c.function.freeValuePointers != nil {
		if ptr, ok := c.function.freeValuePointers[id]; ok && !ptr.IsNil() {
			c.Builder.CreateStore(c.coerceToInt64(val), ptr)
			if c.function.storedValues == nil {
				c.function.storedValues = make(map[int64]struct{})
			}
			c.function.storedValues[id] = struct{}{}
			return
		}
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

func llvmIsZeroValue(v llvm.Value) bool {
	if v.IsNil() {
		return true
	}
	rv := (C.LLVMValueRef)(unsafe.Pointer(v.C))
	if rv == nil {
		return true
	}
	return C.LLVMIsNull(rv) != 0
}
