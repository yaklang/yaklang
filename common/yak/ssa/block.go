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
		Insts:         make([]Instruction, 0),
		Phis:          make([]*Phi, 0),
		isSealed:      isSealed,
		inCompletePhi: make([]*Phi, 0),
		user:          make([]User, 0),
	}
	f.Blocks = append(f.Blocks, b)
	return b
}

func (b *BasicBlock) SetPosition(pos *Position) {
	b.pos = pos
}

/*
	if condition is true  :  1 reach
	if condition is false : -1 unreachable
	if condition need calc: 0  unknown
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
	return b.Insts[len(b.Insts)-1]
}
