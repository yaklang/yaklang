package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func EmitInst(i Instruction) {
	block := i.GetBlock()
	if block == nil {
		// println("void block!! %s")
		return
	}
	if index := slices.Index(block.Instrs, i); index != -1 {
		return
	}
	if len(block.Instrs) == 0 {
		b := block.Parent.builder
		current := b.CurrentBlock
		b.CurrentBlock = block
		b.emit(i)
		b.CurrentBlock = current
	} else {
		EmitBefore(block.LastInst(), i)
	}
}

func Insert(i Instruction, b *BasicBlock) {
	b.Instrs = append(b.Instrs, i)
}

func DeleteInst(i Instruction) {
	b := i.GetBlock()
	if phi, ok := i.(*Phi); ok {
		b.Phis = utils.Remove(b.Phis, phi)
	} else {
		b.Instrs = utils.Remove(b.Instrs, i)
	}
	f := i.GetParent()
	delete(f.InstReg, i)
	// if v, ok := i.(Value); ok {
	// 	f := i.GetParent()
	// 	f.symbolTable[v.GetVariable()] = remove(f.symbolTable[v.GetVariable()], v)
	// }
}

func newAnInstuction(block *BasicBlock) anInstruction {
	return anInstruction{
		Func:     block.Parent,
		Block:    block,
		typs:     nil,
		variable: "",
		pos:      block.Parent.builder.CurrtenPos,
	}
}

func NewJump(to *BasicBlock, block *BasicBlock) *Jump {
	j := &Jump{
		anInstruction: newAnInstuction(block),
		To:            to,
	}
	return j
}

func NewLoop(block *BasicBlock, cond Value) *Loop {
	l := &Loop{
		anInstruction: newAnInstuction(block),
		Cond:          cond,
	}
	return l
}

func NewConstInst(c *Const, block *BasicBlock) *ConstInst {
	v := &ConstInst{
		Const:         *c,
		anInstruction: newAnInstuction(block),
	}
	return v
}

func NewUndefine(name string, block *BasicBlock) *Undefine {
	u := &Undefine{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
	}
	u.SetVariable(name)
	block.Parent.builder.writeVariableByBlock(name, u, block)
	return u
}

func NewBinOpOnly(op BinaryOpcode, x, y Value, block *BasicBlock) *BinOp {
	b := &BinOp{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
		Op:            op,
		X:             x,
		Y:             y,
	}
	b.AddValue(x)
	b.AddValue(y)
	if op >= OpGt && op <= OpIn {
		b.SetType(BasicTypes[Boolean])
	}
	// fixupUseChain(b)
	return b
}

func NewBinOp(op BinaryOpcode, x, y Value, block *BasicBlock) Value {
	v := HandlerBinOp(NewBinOpOnly(op, x, y, block))
	return v
}

func NewUnOpOnly(op UnaryOpcode, x Value, block *BasicBlock) *UnOp {
	u := &UnOp{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
		Op:            op,
		X:             x,
	}
	u.AddValue(x)
	return u
}

func NewUnOp(op UnaryOpcode, x Value, block *BasicBlock) Value {
	b := HandlerUnOp(NewUnOpOnly(op, x, block))
	fixupUseChain(b)
	return b
}

func NewIf(cond Value, block *BasicBlock) *If {
	ifssa := &If{
		anInstruction: newAnInstuction(block),
		Cond:          cond,
	}
	fixupUseChain(ifssa)
	return ifssa
}

func NewSwitch(cond Value, defaultb *BasicBlock, label []SwitchLabel, block *BasicBlock) *Switch {
	sw := &Switch{
		anInstruction: newAnInstuction(block),
		Cond:          cond,
		DefaultBlock:  defaultb,
		Label:         label,
	}
	fixupUseChain(sw)
	return sw
}

func NewReturn(vs []Value, block *BasicBlock) *Return {
	r := &Return{
		anInstruction: newAnInstuction(block),
		Results:       vs,
	}
	fixupUseChain(r)
	r.SetType(CalculateType(lo.Map(vs, func(v Value, _ int) Type { return v.GetType() })))
	return r
}

func NewTypeCast(typ Type, v Value, block *BasicBlock) *TypeCast {
	t := &TypeCast{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
		Value:         v,
	}
	t.AddValue(v)
	t.SetType(typ)
	return t
}

func NewAssert(cond, msgValue Value, msg string, block *BasicBlock) *Assert {
	a := &Assert{
		anInstruction: newAnInstuction(block),
		Cond:          cond,
		Msg:           msg,
		MsgValue:      msgValue,
	}
	return a
}

func NewNext(iter Value, block *BasicBlock) *Next {
	n := &Next{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
		Iter:          iter,
	}
	n.AddValue(iter)

	/*
		next map[T]U
			{
				key: T
				field: U
				ok: bool
			}
	*/
	typ := NewObjectType()
	typ.Kind = Struct
	typ.AddField(NewConst("ok"), BasicTypes[Boolean])
	if it, ok := iter.GetType().(*ObjectType); ok {
		if keytyp := it.keyTyp; keytyp != nil {
			typ.AddField(NewConst("key"), keytyp)
		} else {
			typ.AddField(NewConst("key"), BasicTypes[Any])
		}
		if fieldtyp := it.fieldType; fieldtyp != nil {
			typ.AddField(NewConst("field"), fieldtyp)
		} else {
			typ.AddField(NewConst("field"), BasicTypes[Any])
		}
	} else {
		typ.AddField(NewConst("key"), BasicTypes[Any])
		typ.AddField(NewConst("field"), BasicTypes[Any])
	}

	n.SetType(typ)
	return n
}

func NewErrorHandler(try, catch, block *BasicBlock) *ErrorHandler {
	e := &ErrorHandler{
		anInstruction: newAnInstuction(block),
		try:           try,
		catch:         catch,
	}
	block.AddSucc(try)
	try.Handler = e
	block.AddSucc(catch)
	catch.Handler = e
	return e
}

func NewParam(variable string, isFreevalue bool, fun *Function) *Parameter {
	p := &Parameter{
		anNode:      NewNode(),
		variable:    variable,
		Func:        fun,
		IsFreevalue: isFreevalue,
		typs:        nil,
	}
	return p
}

func (i *If) AddTrue(t *BasicBlock) {
	i.True = t
	i.Block.AddSucc(t)
}

func (i *If) AddFalse(f *BasicBlock) {
	i.False = f
	i.Block.AddSucc(f)
}

func (l *Loop) Finish(init, step []Value) {
	// check cond
	check := func(v Value) bool {
		if _, ok := v.(*Phi); ok {
			return true
		} else {
			return false
		}
	}

	if b, ok := l.Cond.(*BinOp); ok {
		if b.Op < OpGt || b.Op > OpNotEq {
			l.NewError(Error, SSATAG, "this condition not compare")
		}
		if check(b.X) {
			l.Key = b.X.(*Phi)
		} else if check(b.Y) {
			l.Key = b.Y.(*Phi)
		} else {
			l.NewError(Error, SSATAG, "this condition not change")
		}
	}

	if l.Key == nil {
		return
	}
	tmp := lo.SliceToMap(l.Key.Edge, func(v Value) (Value, struct{}) { return v, struct{}{} })

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

func (f *Field) GetLastValue() Value {
	if lenght := len(f.Update); lenght != 0 {
		update, ok := f.Update[lenght-1].(*Update)
		if !ok {
			panic("")
		}
		return update.Value
	}
	return nil
}

func (e *ErrorHandler) AddFinal(f *BasicBlock) {
	e.final = f
	e.GetBlock().AddSucc(f)
	f.Handler = e
}

func (e *ErrorHandler) AddDone(d *BasicBlock) {
	e.done = d
	e.GetBlock().AddSucc(d)
	d.Handler = e
}
