package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// prepareDeferGuards allocates one entry-block slot per deferred instruction.
// EmitDefer always places the call in the function defer block, including when
// the defer statement sat inside a branch. The slot is set only when that
// branch runs, and the defer block skips the call while the slot is still 0.
func (c *Compiler) prepareDeferGuards(fn *ssa.Function) error {
	if c == nil || c.function == nil || fn == nil || fn.DeferBlock <= 0 {
		return nil
	}
	block, ok := fn.GetBasicBlockByID(fn.DeferBlock)
	if !ok || block == nil {
		return nil
	}
	entryBB := c.entryBlockFor(fn)
	if entryBB.IsNil() {
		return nil
	}

	type armedDefer struct {
		id    int64
		block int64
		after int64
	}
	armed := make([]armedDefer, 0)
	for _, instID := range block.Insts {
		inst, ok := fn.GetInstructionById(instID)
		if !ok || inst == nil {
			continue
		}
		blockID, after, guarded := deferGuardOf(inst)
		if !guarded {
			continue
		}
		armed = append(armed, armedDefer{id: inst.GetId(), block: blockID, after: after})
	}
	if len(armed) == 0 {
		return nil
	}

	c.Builder.SetInsertPointAtEnd(entryBB)
	if c.blockHasTerminator(entryBB) {
		return fmt.Errorf("prepareDeferGuards: entry block already terminated")
	}
	i64 := c.LLVMCtx.Int64Type()
	zero := llvm.ConstInt(i64, 0, false)
	c.function.deferGuardSlots = make(map[int64]llvm.Value, len(armed))
	c.function.deferArmAtStart = make(map[int64][]int64)
	c.function.deferArmAfter = make(map[int64][]int64)
	for _, item := range armed {
		slot := buildAlloca(c.Builder, i64, fmt.Sprintf("defer_arm_%d", item.id))
		c.Builder.CreateStore(zero, slot)
		c.function.deferGuardSlots[item.id] = slot
		if item.after == 0 {
			c.function.deferArmAtStart[item.block] = append(c.function.deferArmAtStart[item.block], item.id)
			continue
		}
		c.function.deferArmAfter[item.after] = append(c.function.deferArmAfter[item.after], item.id)
	}
	return nil
}

func deferGuardOf(inst ssa.Instruction) (blockID, after int64, ok bool) {
	type guarded interface {
		DeferGuard() (int64, int64, bool)
	}
	g, isGuarded := inst.(guarded)
	if !isGuarded || g == nil {
		return 0, 0, false
	}
	return g.DeferGuard()
}

func (c *Compiler) armDeferGuards(ids []int64) {
	if c == nil || c.function == nil || len(ids) == 0 || c.function.deferGuardSlots == nil {
		return
	}
	bb := c.currentInsertBlock()
	if c.function.activeBlockID > 0 {
		if cur, ok := c.Blocks[c.function.activeBlockID]; ok && !cur.IsNil() {
			bb = cur
		}
	}
	if bb.IsNil() || c.blockHasTerminator(bb) {
		return
	}
	c.Builder.SetInsertPointAtEnd(bb)
	one := llvm.ConstInt(c.LLVMCtx.Int64Type(), 1, false)
	for _, id := range ids {
		slot, ok := c.function.deferGuardSlots[id]
		if !ok || slot.IsNil() {
			continue
		}
		c.Builder.CreateStore(one, slot)
	}
}

// instructionRunsInDeferBlock reports a deferred recover/panic that EmitDefer
// left in the source block's instruction list. The id stays there so the
// defer arms when that statement is reached. The body belongs to the
// function defer block; emitting it in the source block retargets that
// block and leaves the pre-created block without a terminator.
func (c *Compiler) instructionRunsInDeferBlock(fn *ssa.Function, blockID int64, inst ssa.Instruction) bool {
	if c == nil || fn == nil || inst == nil || fn.DeferBlock <= 0 || blockID == fn.DeferBlock {
		return false
	}
	owner := inst.GetBlock()
	if owner == nil || owner.GetId() != fn.DeferBlock {
		return false
	}
	id := inst.GetId()
	for _, listed := range owner.Insts {
		if listed == id {
			return true
		}
	}
	return false
}

// compileInstWithDeferGuard runs inst, then publishes its capture writes.
// A deferred instruction is skipped when its origin block never stored the arm slot.
func (c *Compiler) compileInstWithDeferGuard(fn *ssa.Function, inst ssa.Instruction) error {
	emit := func() error {
		if err := c.compileInstruction(inst); err != nil {
			return err
		}
		c.publishCapturedCell(inst)
		return c.storeClosureCaptureAtDefinition(fn, inst)
	}
	if c == nil || c.function == nil || inst == nil || c.function.deferGuardSlots == nil {
		return emit()
	}
	slot, ok := c.function.deferGuardSlots[inst.GetId()]
	if !ok || slot.IsNil() {
		return emit()
	}
	return c.emitDeferGuard(inst, slot, emit)
}

func (c *Compiler) emitDeferGuard(inst ssa.Instruction, slot llvm.Value, emit func() error) error {
	if c == nil || c.function == nil || c.function.llvmFn.IsNil() || inst == nil {
		return emit()
	}
	cur := c.currentInsertBlock()
	if cur.IsNil() || c.blockHasTerminator(cur) {
		return fmt.Errorf("defer guard: insert block missing or already terminated for inst %d", inst.GetId())
	}
	id := inst.GetId()
	doBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("defer_do_%d", id))
	skipBB := c.LLVMCtx.AddBasicBlock(c.function.llvmFn, fmt.Sprintf("defer_skip_%d", id))
	i64 := c.LLVMCtx.Int64Type()
	flag := c.Builder.CreateLoad(i64, slot, fmt.Sprintf("defer_flag_%d", id))
	cond := c.Builder.CreateICmp(llvm.IntNE, flag, llvm.ConstInt(i64, 0, false), fmt.Sprintf("defer_armed_%d", id))
	c.Builder.CreateCondBr(cond, doBB, skipBB)

	c.Builder.SetInsertPointAtEnd(doBB)
	blockID := int64(0)
	if block := inst.GetBlock(); block != nil {
		blockID = block.GetId()
		c.Blocks[blockID] = doBB
	}
	if err := emit(); err != nil {
		return err
	}

	seen := map[llvm.BasicBlock]struct{}{}
	closeBlock := func(bb llvm.BasicBlock) {
		if bb.IsNil() {
			return
		}
		if _, ok := seen[bb]; ok {
			return
		}
		seen[bb] = struct{}{}
		if c.blockHasTerminator(bb) {
			return
		}
		c.Builder.SetInsertPointAtEnd(bb)
		c.Builder.CreateBr(skipBB)
	}
	if blockID > 0 {
		if bb, ok := c.Blocks[blockID]; ok {
			closeBlock(bb)
		}
	}
	closeBlock(c.currentInsertBlock())
	if blockID > 0 {
		c.Blocks[blockID] = skipBB
	}
	c.Builder.SetInsertPointAtEnd(skipBB)
	return nil
}
