package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

func NewMake(parentI Value, typ Type, low, high, step, Len, Cap Value) *Make {
	i := &Make{}
	i.initValue()
	if !utils.IsNil(low) {
		i.low = low.GetId()
	}
	if !utils.IsNil(high) {
		i.high = high.GetId()
	}
	if !utils.IsNil(step) {
		i.step = step.GetId()
	}
	if !utils.IsNil(parentI) {
		i.parentI = parentI.GetId()
	}
	if !utils.IsNil(Len) {
		i.Len = Len.GetId()
	}
	if !utils.IsNil(Cap) {
		i.Cap = Cap.GetId()
	}

	i.SetType(typ)
	return i
}

func NewJump(to *BasicBlock) *Jump {
	j := &Jump{
		anInstruction: newAnInstruction(),
		To:            to.GetId(),
	}
	return j
}

func NewLoop(cond Value) *Loop {
	l := &Loop{
		anInstruction: newAnInstruction(),
		Cond:          cond.GetId(),
	}
	return l
}

func NewUndefined(name string) *Undefined {
	u := &Undefined{}
	u.initValue()
	u.SetName(name)
	return u
}

func NewBinOp(op BinaryOpcode, x, y Value) *BinOp {
	if x == nil || y == nil {
		return nil
	}
	b := &BinOp{
		Op: op,
		X:  x.GetId(),
		Y:  y.GetId(),
	}
	b.initValue()
	if op >= OpGt && op <= OpIn {
		b.SetType(CreateBooleanType())
	}
	return b
}

func NewUnOp(op UnaryOpcode, x Value) *UnOp {
	u := &UnOp{
		Op: op,
		X:  x.GetId(),
	}
	u.initValue()
	return u
}

func NewIf() *If {
	ifSSA := &If{
		anInstruction: newAnInstruction(),
	}
	return ifSSA
}

func NewSwitch(cond Value, defaultb *BasicBlock, label []SwitchLabel) *Switch {
	sw := &Switch{
		anInstruction: newAnInstruction(),
		// Cond:          cond.GetId(),
		DefaultBlock: defaultb,
		Label:        label,
	}
	if !utils.IsNil(cond) {
		sw.Cond = cond.GetId()
	}
	return sw
}

func NewReturn(vs Values) *Return {
	r := &Return{
		Results: vs.GetIds(),
	}
	r.initValue()
	return r
}

func NewTypeCast(typ Type, v Value) *TypeCast {
	if v == nil {
		return nil
	}
	t := &TypeCast{
		Value: v.GetId(),
	}
	t.initValue()
	t.SetType(typ)
	return t
}

func NewTypeValue(typ Type) *TypeValue {
	t := &TypeValue{}
	t.initValue()
	t.SetType(typ)
	return t
}

func NewAssert(cond, msgValue Value, msg string) *Assert {
	a := &Assert{
		anInstruction: newAnInstruction(),
		Cond:          cond.GetId(),
		Msg:           msg,
	}
	if !utils.IsNil(msgValue) {
		a.MsgValue = msgValue.GetId()
	}
	return a
}

func NewNext(iter Value, isIn bool) *Next {
	if utils.IsNil(iter) {
		return nil
	}
	n := &Next{
		Iter:   iter.GetId(),
		InNext: isIn,
	}
	n.initValue()
	typ := newNextType(iter.GetType(), isIn)
	n.SetType(typ)
	return n
}

func NewErrorHandler(try *BasicBlock) *ErrorHandler {
	e := &ErrorHandler{
		anInstruction: newAnInstruction(),
		Try:           try.GetId(),
	}
	return e
}

func NewErrorCatch(try *ErrorHandler, catch *BasicBlock, exception Value) *ErrorCatch {
	e := &ErrorCatch{
		CatchBody: catch.GetId(),
		Exception: exception.GetId(),
	}
	e.initValue()
	return e
}

func NewExternLib(variable string, builder *FunctionBuilder, table map[string]any) *ExternLib {
	e := &ExternLib{
		table:   table,
		builder: builder,
	}
	e.initValue()
	e.SetName(variable)
	e.SetFunc(builder.Function)
	block, ok := builder.GetBasicBlockByID(builder.EnterBlock)
	if ok && block != nil {
		e.SetBlock(block)
	} else {
		log.Warnf("ExternLib block cannot convert to BasicBlock: %v", builder.EnterBlock)
	}
	e.SetRange(builder.CurrentRange)
	e.GetProgram().SetVirtualRegister(e)
	e.GetProgram().SetInstructionWithName(variable, e)
	return e
}

func NewParam(variable string, isFreeValue bool, builder *FunctionBuilder) *Parameter {
	p := &Parameter{
		IsFreeValue: isFreeValue,
	}
	p.initValue()
	p.SetName(variable)
	p.SetFunc(builder.Function)

	block, ok := builder.GetBasicBlockByID(builder.EnterBlock)
	if ok && block != nil {
		p.SetBlock(block)
	} else {
		log.Warnf("Parameter block cannot convert to BasicBlock: %v", builder.EnterBlock)
	}

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
		parameterMemberInner: newParameterMember(obj, key),
	}
	p.initValue()
	p.SetName(variable)
	p.SetFunc(builder.Function)

	block, ok := builder.GetBasicBlockByID(builder.EnterBlock)
	if ok && block != nil {
		p.SetBlock(block)
	} else {
		log.Warnf("NewParamMember block cannot convert to BasicBlock: %v", builder.EnterBlock)
	}

	p.SetRange(builder.CurrentRange)
	p.GetProgram().SetVirtualRegister(p)
	return p
}
func NewMoreParamMember(variable string, builder *FunctionBuilder, member *ParameterMember, key Value) *ParameterMember {
	p := &ParameterMember{
		parameterMemberInner: newMoreParameterMember(member, key),
	}
	p.initValue()
	p.SetName(variable)
	p.SetFunc(builder.Function)
	block, ok := builder.GetBasicBlockByID(builder.EnterBlock)
	if ok && block != nil {
		p.SetBlock(block)
	} else {
		log.Warnf("NewParamMember block cannot convert to BasicBlock: %v", builder.EnterBlock)
	}
	p.SetRange(builder.CurrentRange)
	p.GetProgram().SetVirtualRegister(p)
	return p
}

func NewSideEffect(variable string, call *Call, value Value) *SideEffect {
	s := &SideEffect{
		CallSite: call.GetId(),
		Value:    value.GetId(),
	}
	s.initValue()
	s.SetName(variable)
	return s
}

func (i *If) SetCondition(t Value) {
	defer i.markPersistenceDirty()
	i.Cond = t.GetId()
	if b := i.GetBlock(); b != nil {
		b.SetConditionInstID(i.GetId())
		b.SetConditionFromValue(t, "if")
	}
	fixupUseChain(i)
}

func (i *If) AddTrue(t *BasicBlock) {
	defer i.markPersistenceDirty()
	if t == nil {
		return
	}
	i.True = t.GetId()
	if block := i.GetBlock(); block != nil {
		block.AddSucc(t)
	}
}

func (i *If) AddFalse(f *BasicBlock) {
	defer i.markPersistenceDirty()
	if f == nil {
		return
	}
	i.False = f.GetId()
	if block := i.GetBlock(); block != nil {
		block.AddSucc(f)
	}
}

func (l *Loop) Finish(init, step []Value) {
	defer l.markPersistenceDirty()
	// check cond
	check := func(id int64) bool {
		v, ok := l.GetValueById(id)
		if !ok || v == nil {
			return false
		}
		if _, ok := ToPhi(v); ok {
			return true
		} else {
			return false
		}
	}

	cond, ok := l.GetValueById(l.Cond)
	if !ok || cond == nil {
		return
	}
	if b, ok := cond.(*BinOp); ok {
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

	if l.Key > 0 {
		return
	}
	keyValue, ok := l.GetValueById(l.Key)
	if !ok || keyValue == nil {
		return
	}
	tmp := lo.SliceToMap(keyValue.GetValues(), func(v Value) (Value, struct{}) { return v, struct{}{} })

	set := func(vs []Value) int64 {
		for _, v := range vs {
			if _, ok := tmp[v]; ok {
				return v.GetId()
			}
		}
		return 0
	}

	l.Init = set(init)
	l.Step = set(step)

	fixupUseChain(l)
}

func (e *ErrorHandler) AddFinal(f *BasicBlock) {
	defer e.markPersistenceDirty()
	defer f.markPersistenceDirty()
	e.Final = f.GetId()
	f.Handler = e.GetId()
}

func (e *ErrorHandler) AddDone(d *BasicBlock) {
	defer e.markPersistenceDirty()
	defer d.markPersistenceDirty()
	e.Done = d.GetId()
	d.Handler = e.GetId()
}
