package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
)

func (f *Function) GetDeferBlock() *BasicBlock {
	if f.DeferBlock == nil {
		f.DeferBlock = f.NewBasicBlockNotAddBlocks("defer")
	}
	return f.DeferBlock
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
		anValue:    NewValue(),
		Preds:      make([]Value, 0),
		Succs:      make([]Value, 0),
		Insts:      make([]Instruction, 0),
		Phis:       make([]Value, 0),
		Handler:    nil,
		finish:     false,
		ScopeTable: NewScope(f.GetProgram().GetProgramName()),
	}
	b.SetName(name)
	b.SetFunc(f)
	b.SetBlock(b)
	if !nodAddToBlocks {
		addToBlocks(b)
	}
	if functionRange := f.GetRange(); functionRange != nil {
		b.SetRange(functionRange)
	} else {
		log.Infof("function$%v 's range is nil, set entry block range (%v) to nil", f.name, name)
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
	block.GetProgram().SetVirtualRegister(block)
}

/*
	if condition is true  :  1 reach
	if condition is false : -1 unreachable
	if condition need calc: 0  unknown
*/

func (b *BasicBlock) Reachable() int {
	if b.setReachable {
		return b.canBeReached
	}

	if b.Condition == nil {
		return 0
	}

	if c, ok := b.Condition.(*ConstInst); ok {
		if c.IsBoolean() {
			if c.Boolean() {
				return 1
			} else {
				return -1
			}
		}
	}

	return 0
}

func (b *BasicBlock) AddSucc(succ *BasicBlock) {
	b.Succs = append(b.Succs, succ)
	succ.Preds = append(succ.Preds, b)
}

func (b *BasicBlock) LastInst() Instruction {
	return b.Insts[len(b.Insts)-1]
}
