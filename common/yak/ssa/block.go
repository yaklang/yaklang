package ssa

import (
	"fmt"
)

func (f *Function) GetDeferBlock() *BasicBlock {
	newDefer := func() *BasicBlock {
		block := f.NewBasicBlockNotAddBlocks("defer")
		f.DeferBlock = block
		// TODO: this Scope should be child Scope of other scope in function
		block.SetScope(NewScope(f, f.GetProgram().GetProgramName()))
		return block
	}
	if f.DeferBlock == nil {
		return newDefer()
	}
	block, ok := f.DeferBlock.(*BasicBlock)
	if !ok {
		log.Warnf("defer block is not a basic block")
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
		Preds:   make([]Value, 0),
		Succs:   make([]Value, 0),
		Insts:   make([]Instruction, 0),
		Phis:    make([]Value, 0),
		Handler: nil,
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
		log.Errorf("function$%v 's range is nil, missed block range (%v)", f.name, name)
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
	f.Blocks = append(f.Blocks, block)
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
			b.Parent = block
			block.Child = append(block.Child, b)
		}
	}
}

func (b *BasicBlock) HaveSubBlock(sub Value) bool {
	if b == nil || sub == nil {
		return false
	}

	for {
		subBlock, ok := ToBasicBlock(sub)
		if !ok {
			log.Errorf("BasicBlock.HaveSubBlock: sub %v is not a basic block", sub)
			return false
		}

		if b.GetId() == subBlock.GetId() {
			return true
		}

		sub = subBlock.Parent
	}
}

func (b *BasicBlock) Reachable() BasicBlockReachableKind {
	if b.canBeReached != BasicBlockUnknown {
		return b.canBeReached
	}

	if b.Condition != nil {
		return BasicBlockUnknown
	}

	if c, ok := b.Condition.(*ConstInst); ok {
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
	b.Succs = append(b.Succs, succ)
	succ.Preds = append(succ.Preds, b)
}

func (b *BasicBlock) LastInst() Instruction {
	return b.Insts[len(b.Insts)-1]
}
