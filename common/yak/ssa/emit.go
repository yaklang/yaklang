package ssa

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

func (f *FunctionBuilder) emit(i Instruction) {
	f.CurrentBlock.Instrs = append(f.CurrentBlock.Instrs, i)
	f.SetReg(i)
}

func (f *FunctionBuilder) newAnInstuction() anInstruction {
	return anInstruction{
		Func:  f.Function,
		Block: f.CurrentBlock,
		typs:  make(Types, 0),
		pos:   f.currtenPos,
	}
}

func (f *FunctionBuilder) EmitArith(op BinaryOpcode, x, y Value) *BinOp {
	if f.CurrentBlock.finish {
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

func (f *FunctionBuilder) EmitIf(cond Value) *If {
	if f.CurrentBlock.finish {
		return nil
	}
	ifssa := &If{
		anInstruction: f.newAnInstuction(),
		Cond:          cond,
	}
	fixupUseChain(ifssa)
	f.emit(ifssa)
	f.CurrentBlock.finish = true
	return ifssa
}

func (f *FunctionBuilder) EmitJump(to *BasicBlock) *Jump {
	if f.CurrentBlock.finish {
		return nil
	}

	j := &Jump{
		anInstruction: f.newAnInstuction(),
		To:            to,
	}
	j.anInstruction.pos = nil
	f.emit(j)
	f.CurrentBlock.AddSucc(to)
	f.CurrentBlock.finish = true
	return j
}

func (f *FunctionBuilder) EmitSwitch(cond Value, defaultb *BasicBlock, label []SwitchLabel) *Switch {
	if f.CurrentBlock.finish {
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
	f.CurrentBlock.finish = true
	return sw
}

func (f *FunctionBuilder) EmitReturn(vs []Value) *Return {
	if f.CurrentBlock.finish {
		return nil
	}
	r := &Return{
		anInstruction: f.newAnInstuction(),
		Results:       vs,
	}
	fixupUseChain(r)
	f.Return = append(f.Return, r)
	f.emit(r)
	f.CurrentBlock.finish = true
	return r
}

func (f *FunctionBuilder) EmitCall(c *Call) *Call {
	if f.CurrentBlock.finish {
		return nil
	}
	fixupUseChain(c)
	f.emit(c)
	return c
}

func (f *FunctionBuilder) emitInterface(parentI *Interface, typs Types, low, high, max, Len, Cap Value) *Interface {
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

func (f *FunctionBuilder) EmitInterfaceBuildWithType(typ Types, Len, Cap Value) *Interface {
	return f.emitInterface(nil, typ, nil, nil, nil, Len, Cap)
}
func (f *FunctionBuilder) EmitInterfaceSlice(i *Interface, low, high, max Value) *Interface {
	return f.emitInterface(i, i.typs, low, high, max, nil, nil)
}

func (b *FunctionBuilder) CreateInterfaceWithVs(keys []Value, vs []Value) *Interface {
	hasKey := true
	if len(keys) == 0 {
		hasKey = false
	}
	lValueLen := NewConst(len(vs))
	// typ := ParseInterfaceTypes(vs)
	typ := NewInterfaceType()
	itf := b.EmitInterfaceBuildWithType(Types{typ}, lValueLen, lValueLen)
	for i, rv := range vs {
		var key Value
		if hasKey {
			key = keys[i]
		} else {
			key = NewConst(i)
		}
		typ.AddField(key, rv.GetType())
		field := b.EmitField(itf, key)
		b.emitUpdate(field, rv)
	}
	typ.Transform()
	return itf
}

func (f *FunctionBuilder) EmitField(i Value, key Value) *Field {
	return f.getFieldWithCreate(i, key, true)
}

func (f *FunctionBuilder) emitUpdate(address *Field, v Value) *Update {
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
