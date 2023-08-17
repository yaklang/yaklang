package ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func fixupUseChain(node Node) {
	if node == nil {
		return
	}
	if u, ok := node.(User); ok {
		for _, v := range u.GetValues() {
			if v != nil {
				v.AddUser(u)
			}
		}
	}

	if v, ok := node.(Value); ok {
		for _, user := range v.GetUsers() {
			if user != nil {
				user.AddValue(v)
			}
		}

	}
}

func (f *Function) emit(i Instruction) {
	f.currentBlock.Instrs = append(f.currentBlock.Instrs, i)
	f.SetReg(i)
}

func (f *Function) newAnInstuction() anInstruction {
	return anInstruction{
		Func:  f,
		Block: f.currentBlock,
		typs:  make(Types, 0),
		pos:   f.currtenPos,
	}
}

func (f *Function) emitArith(op yakvm.OpcodeFlag, x, y Value) *BinOp {
	if f.currentBlock.finish {
		return nil
	}
	b := &BinOp{
		anInstruction: f.newAnInstuction(),
		Op:            op,
		X:             x,
		Y:             y,
		user:          []User{},
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
		anInstruction: f.newAnInstuction(),
		Cond:          cond,
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
		anInstruction: f.newAnInstuction(),
		To:            to,
	}
	j.anInstruction.pos = nil
	f.emit(j)
	f.currentBlock.AddSucc(to)
	f.currentBlock.finish = true
	return j
}

func (f *Function) emitSwitch(cond Value, defaultb *BasicBlock, label []switchlabel) *Switch {
	if f.currentBlock.finish {
		return nil
	}

	sw := &Switch{
		anInstruction: f.newAnInstuction(),
		cond:          cond,
		defaultBlock:  defaultb,
		label:         label,
	}
	fixupUseChain(sw)
	f.emit(sw)
	f.currentBlock.finish = true
	return sw
}

func (f *Function) emitReturn(vs []Value) *Return {
	if f.currentBlock.finish {
		return nil
	}
	r := &Return{
		anInstruction: f.newAnInstuction(),
		Results:       vs,
	}
	fixupUseChain(r)
	f.Return = append(f.Return, r)
	f.emit(r)
	f.currentBlock.finish = true
	return r
}

func (f *Function) emitCall(c *Call) *Call {
	if f.currentBlock.finish {
		return nil
	}
	fixupUseChain(c)
	f.emit(c)
	return c
}

func (f *Function) emitInterface(parentI *Interface, typs Types, low, high, max, Len, Cap Value) *Interface {
	i := &Interface{
		anInstruction: f.newAnInstuction(),
		parentI:       parentI,
		low:           low,
		high:          high,
		max:           max,
		field:         make(map[Value]*Field, 0),
		Len:           Len,
		Cap:           Cap,
		users:         make([]User, 0),
	}
	if typs != nil {
		i.anInstruction.typs = typs
	}
	f.emit(i)
	fixupUseChain(i)
	return i
}

func (f *Function) emitInterfaceBuildWithType(typ Types, Len, Cap Value) *Interface {
	return f.emitInterface(nil, typ, nil, nil, nil, Len, Cap)
}
func (f *Function) emitInterfaceSlice(i *Interface, low, high, max Value) *Interface {
	return f.emitInterface(i, i.typs, low, high, max, nil, nil)
}

func (b *builder) CreateInterfaceWithVs(keys []Value, vs []Value) *Interface {
	hasKey := true
	if len(keys) == 0 {
		hasKey = false
	}
	lValueLen := NewConst(len(vs))
	// typ := ParseInterfaceTypes(vs)
	typ := NewInterfaceType()
	itf := b.emitInterfaceBuildWithType(Types{typ}, lValueLen, lValueLen)
	for i, rv := range vs {
		var key Value
		if hasKey {
			key = keys[i]
		} else {
			key = NewConst(i)
		}
		typ.AddField(key, rv.GetType())
		field := b.emitField(itf, key)
		b.emitUpdate(field, rv)
	}
	typ.Transform()
	return itf
}

func (f *Function) emitField(i Value, key Value) *Field {
	return f.getFieldWithCreate(i, key, true)
}

func (f *Function) emitUpdate(address *Field, v Value) *Update {
	//use-value-chain: address -> update -> value
	CheckUpdateType(address.GetType(), v.GetType())
	s := &Update{
		anInstruction: f.newAnInstuction(),
		value:         v,
		address:       address,
	}
	f.emit(s)
	fixupUseChain(s)
	return s
}
