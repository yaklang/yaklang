package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

func fixupUseChain(node Node) {
	if utils.IsNil(node) {
		return
	}
	if u, ok := node.(User); ok {
		for _, v := range u.GetValues() {
			if !utils.IsNil(v) {
				v.AddUser(u)
			}
		}
	}

	if v, ok := node.(Value); ok {
		for _, user := range v.GetUsers() {
			if !utils.IsNil(user) {
				user.AddValue(v)
			}
		}

	}
}

func (f *FunctionBuilder) emit(i Instruction) {
	f.CurrentBlock.Instrs = append(f.CurrentBlock.Instrs, i)
	f.SetReg(i)
}

func (f *FunctionBuilder) EmitUndefine(name string) *Undefine {
	if f.CurrentBlock.finish {
		return nil
	}
	u := NewUndefine(name, f.CurrentBlock)
	u.NewError(Error, SSATAG, "this variable undefine")
	f.emit(u)
	return u
}

func (f *FunctionBuilder) EmitArith(op BinaryOpcode, x, y Value) *BinOp {
	if f.CurrentBlock.finish {
		return nil
	}
	b := NewBinOp(op, x, y, f.CurrentBlock)
	f.emit(b)
	return b
}

func (f *FunctionBuilder) EmitIf(cond Value) *If {
	if f.CurrentBlock.finish {
		return nil
	}
	ifssa := NewIf(cond, f.CurrentBlock)
	f.emit(ifssa)
	f.CurrentBlock.finish = true
	return ifssa
}

func (f *FunctionBuilder) EmitJump(to *BasicBlock) *Jump {
	if f.CurrentBlock.finish {
		return nil
	}
	j := NewJump(to, f.CurrentBlock)
	f.emit(j)
	f.CurrentBlock.AddSucc(to)
	f.CurrentBlock.finish = true
	return j
}

func (f *FunctionBuilder) EmitSwitch(cond Value, defaultb *BasicBlock, label []SwitchLabel) *Switch {
	if f.CurrentBlock.finish {
		return nil
	}
	sw := NewSwitch(cond, defaultb, label, f.CurrentBlock)
	f.emit(sw)
	f.CurrentBlock.finish = true
	return sw
}

func (f *FunctionBuilder) EmitReturn(vs []Value) *Return {
	if f.CurrentBlock.finish {
		return nil
	}
	r := NewReturn(vs, f.CurrentBlock)
	f.emit(r)
	f.CurrentBlock.finish = true
	return r
}

func (f *FunctionBuilder) EmitCall(c *Call) *Call {
	if f.CurrentBlock.finish || c == nil {
		return nil
	}
	fixupUseChain(c)
	f.emit(c)
	return c
}

func (f *FunctionBuilder) emitInterface(parentI *Interface, typs Types, low, high, max, Len, Cap Value) *Interface {
	i := NewInterface(parentI, typs, low, high, max, Len, Cap, f.CurrentBlock)
	f.emit(i)
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
	itf := b.EmitInterfaceBuildWithType(nil, lValueLen, lValueLen)
	for i, rv := range vs {
		var key Value
		if hasKey {
			key = keys[i]
		} else {
			key = NewConst(i)
		}
		field := b.EmitField(itf, key)
		b.emitUpdate(field, rv)
	}
	return itf
}

func (f *FunctionBuilder) EmitField(i User, key Value) *Field {
	return f.getFieldWithCreate(i, key, true)
}

func (f *FunctionBuilder) emitUpdate(address *Field, v Value) *Update {
	// CheckUpdateType(address.GetType(), v.GetType())
	s := NewUpdate(address, v, f.CurrentBlock)
	f.emit(s)
	return s
}
