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

func (f *Function) emitReturn(vs []Value) *Return {
	r := &Return{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		Results: vs,
	}
	fixupUseChain(r)
	f.emit(r)
	return r
}

func (f *Function) emitClosure(target *Function) *Closure {
	m := &Closure{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		Fn:       target,
		Bindings: make([]Value, 0, len(target.FreeValue)),
		user:     make([]User, 0),
	}
	//TODO: handler binding with target.freeValue
	m.Bindings = append(m.Bindings, target.FreeValue...)
	// for _, v := range target.FreeValue {
	// 	m.Bindings = append(m.Bindings, v)
	// }

	// assert
	if len(m.Bindings) != len(target.FreeValue) {
		panic("bingding variable length error")
	}

	fixupUseChain(m)
	f.emit(m)
	return m
}

func (f *Function) emitCall(target *MakeClosure, args []Value, isDropError bool) *Call {
	c := &Call{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
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
