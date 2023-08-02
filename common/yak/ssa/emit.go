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
	if f.currentBlock.finish {
		return nil
	}
	ifssa := &If{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
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
			Parent: f,
			Block:  f.currentBlock,
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
			Parent: f,
			Block:  f.currentBlock,
		},
		Results: vs,
	}
	fixupUseChain(r)
	f.emit(r)
	f.currentBlock.finish = true
	return r
}

func (f *Function) emitClosure(target *Function) *Closure {
	if f.currentBlock.finish {
		return nil
	}
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

func (f *Function) emitCall(target Value, args []Value, isDropError bool) *Call {
	if f.currentBlock.finish {
		return nil
	}
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

func (f *Function) emitAlloc(name string) *Alloc {
	if f.currentBlock.finish {
		return nil
	}
	alloc := &Alloc{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		variable: name,
		user:     []User{},
	}
	f.emit(alloc)
	return alloc
}

func (f *Function) emitStore(alloc *Alloc, v Value) *Store {
	if f.currentBlock.finish {
		return nil
	}
	store := &Store{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		alloc: alloc,
		value: v,
	}
	alloc.v = v
	f.emit(store)
	fixupUseChain(store)
	return store
}
func (f *Function) emitLoad(alloc *Alloc) Value {
	if f.currentBlock.finish {
		return nil
	}
	load := &Load{
		anInstruction: anInstruction{
			Parent: f,
			Block:  f.currentBlock,
		},
		user:  []User{},
		alloc: alloc,
	}
	fixupUseChain(load)
	f.emit(load)
	return load
}
