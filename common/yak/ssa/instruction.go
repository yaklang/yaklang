package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

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
		pos:      block.Parent.builder.currtenPos,
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

func NewUndefine(name string, block *BasicBlock) *Undefine {
	u := &Undefine{
		anInstruction: newAnInstuction(block),
		user:          []User{},
		values:        []Value{},
	}
	u.SetVariable(name)
	block.Parent.WriteVariable(name, u)
	return u
}

func NewBinOpOnly(op BinaryOpcode, x, y Value, block *BasicBlock) *BinOp {
	b := &BinOp{
		anInstruction: newAnInstuction(block),
		Op:            op,
		X:             x,
		Y:             y,
		user:          []User{},
	}
	if op >= OpGt && op <= OpNotEq {
		b.SetType(BasicTypes[Boolean])
	}
	// fixupUseChain(b)
	return b
}

func NewBinOp(op BinaryOpcode, x, y Value, block *BasicBlock) Value {
	v := HandlerBinOp(NewBinOpOnly(op, x, y, block))
	return v
}

func NewUnOp(op UnaryOpcode, x Value, block *BasicBlock) *UnOp {
	b := &UnOp{
		anInstruction: newAnInstuction(block),
		Op:            op,
		X:             x,
		user:          []User{},
	}
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
		Value:         v,
		user:          make([]User, 0),
	}
	t.SetType(typ)
	return t
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
