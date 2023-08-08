package ssa

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func (f *Function) emit(i Instruction) {
	f.currentBlock.Instrs = append(f.currentBlock.Instrs, i)
}

func fixupUseChain(u User) {
	if u == nil {
		return
	}
	for _, v := range u.GetValues() {
		if v == nil {
			continue
		}
		v.AddUser(u)
	}
}

func (f *Function) emitArith(op yakvm.OpcodeFlag, x, y Value) *BinOp {
	if f.currentBlock.finish {
		return nil
	}
	b := &BinOp{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
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
	if f.currentBlock.finish {
		return nil
	}
	ifssa := &If{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Cond: cond,
	}
	fixupUseChain(ifssa)
	f.emit(ifssa)
	f.currentBlock.finish = true
	return ifssa
}

func (f *Function) emitJump(to *BasicBlock) *Jump {
	if f.currentBlock.finish {
		return nil
	}

	j := &Jump{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		To: to,
	}
	f.emit(j)
	f.currentBlock.AddSucc(to)
	f.currentBlock.finish = true
	return j
}

func (f *Function) emitReturn(vs []Value) *Return {
	if f.currentBlock.finish {
		return nil
	}
	r := &Return{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Results: vs,
	}
	fixupUseChain(r)
	f.emit(r)
	f.currentBlock.finish = true
	return r
}



func (f *Function) emitCall(target Value, args []Value, isDropError bool) *Call {
	if f.currentBlock.finish {
		return nil
	}
	c := &Call{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Method:      target,
		Args:        args,
		user:        []User{},
		isDropError: isDropError,
	}

	fixupUseChain(c)
	f.emit(c)
	return c
}

