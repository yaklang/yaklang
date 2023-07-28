package ssa

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func (f *Function) emit(i Instruction) {
	f.currentBlock.Instrs = append(f.currentBlock.Instrs, i)
}

func (f *Function) emitArith(op yakvm.OpcodeFlag, x, y Value) *BinOp {
	ret := &BinOp{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		Op:   op,
		X:    x,
		Y:    y,
		user: []User{},
	}
	x.AddUser(ret)
	y.AddUser(ret)
	f.emit(ret)
	return ret
}
