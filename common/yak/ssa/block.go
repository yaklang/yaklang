package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

func (f *Function) GetDeferBlock() *BasicBlock {
	newDefer := func() *BasicBlock {
		block := f.NewBasicBlockNotAddBlocks("defer")
		f.DeferBlock = block.GetId()
		// TODO: this Scope should be child Scope of other scope in function
		block.SetScope(NewScope(f, f.GetProgram().GetProgramName()))
		return block
	}
	if f.DeferBlock <= 0 {
		return newDefer()
	}
	block, ok := f.GetBasicBlockByID(f.DeferBlock)
	if !ok || block == nil {
		return newDefer()
	}
	return block
}

func (f *Function) NewBasicBlock(name string) *BasicBlock {
	return f.newBasicBlockEx(name, true, false)
}
func (f *Function) NewBasicBlockUnSealed(name string) *BasicBlock {
	return f.newBasicBlockEx(name, false, false)
}

func (f *Function) NewBasicBlockNotAddBlocks(name string) *BasicBlock {
	return f.newBasicBlockEx(name, true, true)
}
func (f *Function) NewBasicBlockNotAddUnSealed(name string) *BasicBlock {
	return f.newBasicBlockEx(name, false, true)
}

func (f *Function) newBasicBlockEx(name string, isSealed bool, nodAddToBlocks bool) *BasicBlock {
	b := &BasicBlock{
		anValue: NewValue(),
		Preds:   make([]int64, 0),
		Succs:   make([]int64, 0),
		Insts:   make([]int64, 0),
		Phis:    make([]int64, 0),
		Handler: 0,
		finish:  false,
	}
	b.SetName(name)
	b.SetFunc(f)
	b.SetBlock(b)
	b.GetProgram().SetVirtualRegister(b)
	if !nodAddToBlocks {
		addToBlocks(b)
	}
	if functionRange := f.GetRange(); functionRange != nil {
		b.SetRange(functionRange)
	} else if name == "entry" {
		log.Debugf("func$%v entry 's range is nil, set entry block range to empty in first building", f.name)
	} else {
		log.Warnf("function$%v 's range is nil, missed block range (%v)", f.name, name)
	}
	return b
}

func addToBlocks(block *BasicBlock) {
	f := block.GetFunc()
	index := len(f.Blocks)

	name := block.GetName()
	if name != "" {
		name = fmt.Sprintf("%s-%d", name, index)
	} else {
		name = fmt.Sprintf("b-%d", index)
	}
	block.SetName(name)

	block.Index = index
	f.Blocks = append(f.Blocks, block.GetId())
}

func (b *BasicBlock) SetScope(s ScopeIF) {
	// If block already has a scope, check if we need to create a new sub scope
	if b.ScopeTable != nil {
		// If the scope is already associated with another block, create a new sub scope
		if existingBlock := GetBlockByScope(s); existingBlock != nil && existingBlock != b {
			log.Warnf("block %v already has scope %v, but trying to set scope %v which is associated with block %v, creating new sub scope",
				b.GetName(), b.ScopeTable.GetScopeName(), s.GetScopeName(), existingBlock.GetName())
			// Create a new sub scope to avoid conflict
			s = s.CreateSubScope()
			// Update the block's scope
			b.ScopeTable = s
			s.SetExternInfo("block", b)
			{
				if block := GetBlockByScope(s.GetParent()); block != nil {
					b.Parent = block.GetId()
					block.Child = append(block.Child, b.GetId())
				}
			}
			return
		}
		// If the scope is the same or already associated with this block, just return
		if existingBlock := GetBlockByScope(s); existingBlock == b {
			return
		}
		// Otherwise, log error and return
		log.Errorf("block %v already has a scope %v, cannot set scope %v", b.GetName(), b.ScopeTable.GetScopeName(), s.GetScopeName())
		return
	}

	// If the scope is already associated with another block, create a new sub scope
	if existingBlock := GetBlockByScope(s); existingBlock != nil && existingBlock != b {
		log.Warnf("block %v set scope %v, but this scope already has block %v, creating new sub scope", b.GetName(), s.GetScopeName(), existingBlock.GetName())
		// Create a new sub scope to avoid conflict
		s = s.CreateSubScope()
	}

	b.ScopeTable = s
	s.SetExternInfo("block", b)

	{
		if block := GetBlockByScope(s.GetParent()); block != nil {
			b.Parent = block.GetId()
			block.Child = append(block.Child, b.GetId())
		}
	}
}

func (b *BasicBlock) HaveSubBlock(sub Value) bool {
	if b == nil || sub == nil {
		return false
	}

	for {
		subBlock, ok := ToBasicBlock(sub)
		if !ok || utils.IsNil(subBlock) {
			log.Warnf("BasicBlock.HaveSubBlock: sub %v is not a basic block", sub)
			return false
		}

		if b.GetId() == subBlock.GetId() {
			return true
		}

		sub, _ = subBlock.GetBasicBlockByID(subBlock.Parent)
	}
}

func (b *BasicBlock) Reachable() BasicBlockReachableKind {
	if b.canBeReached != BasicBlockUnknown {
		return b.canBeReached
	}

	if b.Condition > 0 {
		return BasicBlockUnknown
	}

	inst, ok := b.GetInstructionById(b.Condition)
	if !ok {
		return BasicBlockUnknown
	}
	if c, ok := ToConstInst(inst); ok {
		if c.IsBoolean() {
			if c.Boolean() {
				return BasicBlockReachable
			} else {
				return BasicBlockUnReachable
			}
		}
	}
	return BasicBlockUnknown
}

func (b *BasicBlock) SetConditionFromValue(v Value, source string) {
	if b == nil || utils.IsNil(v) {
		return
	}
	// Do not assign b.Condition here: that field is owned by ssa4analyze.BlockCondition
	// (merged reachability formula). Early ids break getCondition(), which ANDs edgeCond
	// with GetValueById(from.Condition) and can call NewBinOp with a nil operand when the
	// id is not yet a resolvable Value. Use ConditionValues / ConditionInst / ConditionMeta
	// for CFG and block condition summaries instead.
	b.ConditionValues = []int64{v.GetId()}
	if b.ConditionMeta == nil {
		b.ConditionMeta = map[string]any{}
	}
	if source != "" {
		b.ConditionMeta["source"] = source
	}
	if _, ok := b.ConditionMeta["schema_version"]; !ok {
		b.ConditionMeta["schema_version"] = 1
	}
}

func (b *BasicBlock) SetConditionInstID(instID int64) {
	if b == nil || instID <= 0 {
		return
	}
	b.ConditionInst = instID
}

func (b *BasicBlock) GetConditionValues() []int64 {
	if b == nil {
		return nil
	}
	if len(b.ConditionValues) > 0 {
		out := make([]int64, 0, len(b.ConditionValues))
		for _, id := range b.ConditionValues {
			if id > 0 {
				out = append(out, id)
			}
		}
		return out
	}
	if b.Condition > 0 {
		return []int64{b.Condition}
	}
	return nil
}

func (b *BasicBlock) BlockConditionSummary() BlockConditionSummary {
	if b == nil {
		return BlockConditionSummary{}
	}
	meta := map[string]any{}
	for k, v := range b.ConditionMeta {
		meta[k] = v
	}
	vals := b.GetConditionValues()
	instID := b.ConditionInst
	if instID <= 0 && len(vals) > 0 {
		instID = vals[0]
	}
	summary := BlockConditionSummary{
		BlockID:     b.GetId(),
		CondInstID:  instID,
		CondValueID: vals,
		Meta:        meta,
	}
	if fn := b.GetFunc(); fn != nil {
		summary.FuncID = fn.GetId()
	}
	return summary
}

func (b *BasicBlock) AddSucc(succ *BasicBlock) {
	b.Succs = append(b.Succs, succ.GetId())
	succ.Preds = append(succ.Preds, b.GetId())
}

func (b *BasicBlock) LastInst() Instruction {
	if inst, ok := b.GetInstructionById(b.Insts[len(b.Insts)-1]); ok {
		return inst
	}
	return nil
}
