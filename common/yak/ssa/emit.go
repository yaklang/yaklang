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

func (f *Function) emitIf(cond Value) *If {
	ifssa := &If{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		Cond: cond,
	}
	cond.AddUser(ifssa)
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

func (f *Function) newPhi(block *BasicBlock, variable string) Value {
	phi := &Phi{
		anInstruction: anInstruction{
			Parent: f,
			Block:  block,
		},
		Edge:     make([]Value, len(block.Preds)),
		user:     make([]User, 0),
		variable: variable,
	}
	return phi.Build()
}
