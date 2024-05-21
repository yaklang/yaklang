package ssa

import (
	"github.com/samber/lo"
)

func NewJump(to *BasicBlock) *Jump {
	j := &Jump{
		anInstruction: NewInstruction(),
		To:            to,
	}
	return j
}

func NewLoop(cond Value) *Loop {
	l := &Loop{
		anInstruction: NewInstruction(),
		Cond:          cond,
	}
	return l
}

func NewUndefined(name string) *Undefined {
	u := &Undefined{
		anValue: NewValue(),
	}
	u.SetName(name)
	return u
}

func NewBinOp(op BinaryOpcode, x, y Value) *BinOp {
	b := &BinOp{
		anValue: NewValue(),
		Op:      op,
		X:       x,
		Y:       y,
	}
	if op >= OpGt && op <= OpIn {
		b.SetType(BasicTypes[BooleanTypeKind])
	}
	return b
}

func NewUnOp(op UnaryOpcode, x Value) *UnOp {
	u := &UnOp{
		anValue: NewValue(),
		Op:      op,
		X:       x,
	}
	return u
}

func NewIf() *If {
	ifSSA := &If{
		anInstruction: NewInstruction(),
	}
	return ifSSA
}

func NewSwitch(cond Value, defaultb *BasicBlock, label []SwitchLabel) *Switch {
	sw := &Switch{
		anInstruction: NewInstruction(),
		Cond:          cond,
		DefaultBlock:  defaultb,
		Label:         label,
	}
	return sw
}

func NewReturn(vs []Value) *Return {
	r := &Return{
		anValue: NewValue(),
		Results: vs,
	}
	return r
}

func NewTypeCast(typ Type, v Value) *TypeCast {
	t := &TypeCast{
		anValue: NewValue(),
		Value:   v,
	}
	t.SetType(typ)
	return t
}

func NewTypeValue(typ Type) *TypeValue {
	t := &TypeValue{
		anValue: NewValue(),
	}
	t.SetType(typ)
	return t
}

func NewAssert(cond, msgValue Value, msg string) *Assert {
	a := &Assert{
		anInstruction: NewInstruction(),
		Cond:          cond,
		Msg:           msg,
		MsgValue:      msgValue,
	}
	return a
}

func NewNext(iter Value, isIn bool) *Next {
	n := &Next{
		anValue: NewValue(),
		Iter:    iter,
		InNext:  isIn,
	}
	typ := newNextType(iter.GetType(), isIn)
	n.SetType(typ)
	return n
}

func NewErrorHandler(try, catch *BasicBlock) *ErrorHandler {
	e := &ErrorHandler{
		anInstruction: NewInstruction(),
		try:           try,
		catch:         catch,
	}
	// block.AddSucc(try)
	try.Handler = e
	// block.AddSucc(catch)
	catch.Handler = e
	return e
}

func NewExternLib(variable string, builder *FunctionBuilder, table map[string]any) *ExternLib {
	e := &ExternLib{
		anValue:   NewValue(),
		table:     table,
		builder:   builder,
		MemberMap: make(map[string]Value),
		Member:    make([]Value, 0),
	}
	e.SetName(variable)
	e.SetFunc(builder.Function)
	e.SetBlock(builder.EnterBlock)
	e.SetRange(builder.CurrentRange)
	e.GetProgram().SetVirtualRegister(e)
	e.GetProgram().SetInstructionWithName(variable, e)
	return e
}

func NewParam(variable string, isFreeValue bool, builder *FunctionBuilder) *Parameter {
	p := &Parameter{
		anValue:      NewValue(),
		IsFreeValue:  isFreeValue,
		defaultValue: nil,
	}
	p.SetName(variable)
	p.SetFunc(builder.Function)
	p.SetBlock(builder.EnterBlock)
	p.SetRange(builder.CurrentRange)
	p.GetProgram().SetVirtualRegister(p)
	p.GetProgram().SetInstructionWithName(variable, p)
	return p
}

func (p *Parameter) Copy() *Parameter {
	new := NewParam(p.GetName(), p.IsFreeValue, p.GetFunc().builder)
	new.FormalParameterIndex = p.FormalParameterIndex
	return new
}

func NewParamMember(variable string, builder *FunctionBuilder, obj *Parameter, key Value) *ParameterMember {
	p := &ParameterMember{
		anValue:              NewValue(),
		parameterMemberInner: newParameterMember(obj, key),
	}
	p.SetName(variable)
	p.SetFunc(builder.Function)
	p.SetBlock(builder.EnterBlock)
	p.SetRange(builder.CurrentRange)
	p.GetProgram().SetVirtualRegister(p)
	return p
}

func NewSideEffect(variable string, call *Call, value Value) *SideEffect {
	s := &SideEffect{
		anValue:  NewValue(),
		CallSite: call,
		Value:    value,
	}
	s.SetName(variable)
	s.SetType(value.GetType())
	return s
}

func (i *If) SetCondition(t Value) {
	i.Cond = t
	fixupUseChain(i)
}

func (i *If) AddTrue(t *BasicBlock) {
	i.True = t
	i.GetBlock().AddSucc(t)
}

func (i *If) AddFalse(f *BasicBlock) {
	i.False = f
	i.GetBlock().AddSucc(f)
}

func (l *Loop) Finish(init, step []Value) {
	// check cond
	check := func(v Value) bool {
		if _, ok := ToPhi(v); ok {
			return true
		} else {
			return false
		}
	}

	if b, ok := l.Cond.(*BinOp); ok {
		// if b.Op < OpGt || b.Op > OpNotEq {
		// 	l.NewError(Error, SSATAG, "this condition not compare")
		// }
		if check(b.X) {
			l.Key = b.X
		} else if check(b.Y) {
			l.Key = b.Y
			// } else {
			// l.NewError(Error, SSATAG, "this condition not change")
		}
	}

	if l.Key == nil {
		return
	}
	tmp := lo.SliceToMap(l.Key.GetValues(), func(v Value) (Value, struct{}) { return v, struct{}{} })

	set := func(vs []Value) Value {
		for _, v := range vs {
			if _, ok := tmp[v]; ok {
				return v
			}
		}
		return nil
	}

	l.Init = set(init)
	l.Step = set(step)

	fixupUseChain(l)
}

func (e *ErrorHandler) AddFinal(f *BasicBlock) {
	e.final = f
	f.Handler = e
}

func (e *ErrorHandler) AddDone(d *BasicBlock) {
	e.done = d
	d.Handler = e
}
