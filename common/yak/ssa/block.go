package ssa

import (
	"fmt"
)

func (f *Function) NewBasicBlock(name string) *BasicBlock {
	return f.newBasicBlockWithSealed(name, true)
}
func (f *Function) NewBasicBlockUnSealed(name string) *BasicBlock {
	return f.newBasicBlockWithSealed(name, false)
}

func (f *Function) newBasicBlockWithSealed(name string, isSealed bool) *BasicBlock {
	index := len(f.Blocks)
	if name != "" {
		name = fmt.Sprintf("%s%d", name, index)
	} else {
		name = fmt.Sprintf("b%d", index)
	}
	b := &BasicBlock{
		Index:         index,
		Name:          name,
		Parent:        f,
		Preds:         make([]*BasicBlock, 0),
		Succs:         make([]*BasicBlock, 0),
		Instrs:        make([]Instruction, 0),
		Phis:          make([]*Phi, 0),
		isSealed:      isSealed,
		inCompletePhi: make([]*Phi, 0),
		user:          make([]User, 0),
	}
	f.Blocks = append(f.Blocks, b)
	return b
}

/*
	if condition is true  :  1 reach
	if condition is false : -1 unreach
	if condition need calc: 0  unknow
*/

func (b *BasicBlock) Reachable() int {
	if c, ok := b.Condition.(*Const); ok {
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
	return b.Instrs[len(b.Instrs)-1]
}
