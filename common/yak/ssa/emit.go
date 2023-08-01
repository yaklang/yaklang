package ssa

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func (f *Function) emit(i Instruction) {
	f.currentBlock.Instrs = append(f.currentBlock.Instrs, i)
}

func fixupUseChain(u User) {
	for _, v := range u.GetValue() {
		v.AddUser(u)
	}
}

func (f *Function) emitArith(op yakvm.OpcodeFlag, x, y Value) *BinOp {
	b := &BinOp{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		Op:   op,
		X:    x,
		Y:    y,
		user: []User{},
	}
	fixupUseChain(b)
	f.emit(b)
	return b
}

func (f *Function) emitIf(cond Value) *If {
	ifssa := &If{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		Cond: cond,
	}
	fixupUseChain(ifssa)
	f.emit(ifssa)
	return ifssa
}

func (f *Function) emitJump(to *BasicBlock) *Jump {
	j := &Jump{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		To: to,
	}
	f.currentBlock.AddSucc(to)
	f.emit(j)
	return j
}
