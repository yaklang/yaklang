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
	if b.ScopeTable != nil {
		log.Errorf("block %v already has a scope", b.GetName())
	}
	b.ScopeTable = s
	{
		if block := GetBlockByScope(s); block != nil {
			log.Errorf("block %v set scope %v, but this scope already has block %v", b.GetName(), s.GetScopeName(), block.GetName())
		}
		s.SetExternInfo("block", b)
	}
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
			log.Errorf("BasicBlock.HaveSubBlock: sub %v is not a basic block", sub)
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
