package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func fixupUseChain(node Node) {
	if utils.IsNil(node) {
		return
	}
	if u, ok := node.(User); ok {
		for _, v := range u.GetValues() {
			if !utils.IsNil(v) {
				AddUser(v, u)
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

func EmitBefore(before, inst Instruction) {
	block := before.GetBlock()
	insts := block.Insts
	if index := slices.Index(insts, before); index > -1 {
		// Extend the slice
		insts = append(insts, nil)
		// Move elements to create a new space
		copy(insts[index+1:], insts[index:])
		// Insert new element
		insts[index] = inst
		block.Insts = insts
		block.Parent.SetReg(inst)
	}
}

func (f *FunctionBuilder) emit(i Instruction) {
	// if c, ok := i.(Value); ok {
	// 	if utils.IsNil(c.GetType()) {
	// 		c.SetType(BasicTypes[Any])
	// 	}
	// }

	f.CurrentBlock.Insts = append(f.CurrentBlock.Insts, i)
	f.SetReg(i)
}

func (f *FunctionBuilder) EmitConstInst(c *Const) *ConstInst {
	i := NewConstInst(c, f.CurrentBlock)
	fixupUseChain(i)
	f.emit(i)
	return i
}

func (f *FunctionBuilder) EmitUndefine(name string) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	u := NewUndefine(name, f.CurrentBlock)
	f.emit(u)
	return u
}

func (f *FunctionBuilder) EmitUnOp(op UnaryOpcode, v Value) Value {
	u := NewUnOp(op, v, f.CurrentBlock)
	if u, ok := u.(*UnOp); ok {
		fixupUseChain(u)
		f.emit(u)
	}
	return u
}

func (f *FunctionBuilder) EmitBinOp(op BinaryOpcode, x, y Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	v := NewBinOp(op, x, y, f.CurrentBlock)
	if b, ok := v.(*BinOp); ok {
		fixupUseChain(b)
		f.emit(b)
	}
	return v
}

func (f *FunctionBuilder) EmitIf(cond Value) *If {
	if f.CurrentBlock.finish {
		return nil
	}
	ifSSA := NewIf(cond, f.CurrentBlock)
	f.emit(ifSSA)
	f.CurrentBlock.finish = true
	return ifSSA
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

func (f *FunctionBuilder) EmitLoop(body, exit *BasicBlock, cond Value) *Loop {
	if f.CurrentBlock.finish {
		return nil
	}
	l := NewLoop(f.CurrentBlock, cond)
	l.Body = body
	l.Exit = exit
	f.CurrentBlock.AddSucc(body)
	f.CurrentBlock.AddSucc(exit)
	f.emit(l)
	f.CurrentBlock.finish = true
	return l
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
	f.Return = append(f.Return, r)
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

func (f *FunctionBuilder) EmitAssert(cond, msgValue Value, msg string) *Assert {
	if f.CurrentBlock.finish {
		return nil
	}
	a := NewAssert(cond, msgValue, msg, f.CurrentBlock)
	fixupUseChain(a)
	f.emit(a)
	return a
}

func (f *FunctionBuilder) emitMake(parentI User, typ Type, low, high, max, Len, Cap Value) *Make {
	i := NewMake(parentI, typ, low, high, max, Len, Cap, f.CurrentBlock)
	f.emit(i)
	return i
}

func (f *FunctionBuilder) EmitMakeBuildWithType(typ Type, Len, Cap Value) *Make {
	i := f.emitMake(nil, typ, nil, nil, nil, Len, Cap)
	i.IsNew = true
	return i
}
func (f *FunctionBuilder) EmitMakeWithoutType(Len, Cap Value) *Make {
	return f.emitMake(nil, nil, nil, nil, nil, Len, Cap)
}
func (f *FunctionBuilder) EmitMakeSlice(i User, low, high, max Value) *Make {
	return f.emitMake(i, i.GetType(), low, high, max, nil, nil)
}

func (f *FunctionBuilder) EmitField(i User, key Value) Value {
	return f.getFieldWithCreate(i, key, true)
}
func (f *FunctionBuilder) EmitFieldMust(i User, key Value) *Field {
	return f.GetField(i, key, true)
}

func (f *FunctionBuilder) EmitUpdate(address User, v Value) *Update {
	// CheckUpdateType(address.GetType(), v.GetType())
	s := NewUpdate(address, v, f.CurrentBlock)
	f.emit(s)
	return s
}

func (f *FunctionBuilder) EmitTypeCast(v Value, typ Type) *TypeCast {
	t := NewTypeCast(typ, v, f.CurrentBlock)
	fixupUseChain(t)
	f.emit(t)
	return t
}

func (f *FunctionBuilder) EmitNextOnly(iter Value) *Next {
	n := NewNext(iter, f.CurrentBlock)
	fixupUseChain(n)
	f.emit(n)
	return n
}

func (f *FunctionBuilder) EmitNext(iter Value) (key, field, ok Value) {
	n := f.EmitNextOnly(iter)
	// n iter-type: map[T]U   n-type {key: T, field: U, ok: bool}
	key = f.EmitField(n, NewConst("key"))
	field = f.EmitField(n, NewConst("field"))
	ok = f.EmitField(n, NewConst("ok"))
	return
}

func (f *FunctionBuilder) EmitErrorHandler(try, catch *BasicBlock) *ErrorHandler {
	e := NewErrorHandler(try, catch, f.CurrentBlock)
	f.emit(e)
	return e
}

func (f *FunctionBuilder) EmitPanic(info Value) *Panic {
	p := &Panic{
		anInstruction: newAnInstruction(f.CurrentBlock),
		Info:          info,
	}
	fixupUseChain(p)
	f.emit(p)
	return p
}

func (f *FunctionBuilder) EmitRecover() *Recover {
	r := &Recover{
		anInstruction: newAnInstruction(f.CurrentBlock),
		anNode:        NewNode(),
	}
	f.emit(r)
	return r
}
