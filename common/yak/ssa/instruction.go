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
		typs:     make(Types, 0),
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

func NewBinOp(op BinaryOpcode, x, y Value, block *BasicBlock) *BinOp {
	b := &BinOp{
		anInstruction: newAnInstuction(block),
		Op:            op,
		X:             x,
		Y:             y,
		user:          []User{},
	}
	fixupUseChain(b)
	return b
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
	block.Parent.Return = append(block.Parent.Return, r)
	return r
}

func NewInterface(parentI *Interface, typs Types, low, high, max, Len, Cap Value, block *BasicBlock) *Interface {

	i := &Interface{
		anInstruction: newAnInstuction(block),
		parentI:       parentI,
		low:           low,
		high:          high,
		max:           max,
		Field:         make(map[Value]*Field, 0),
		Len:           Len,
		Cap:           Cap,
		users:         make([]User, 0),
	}
	if typs != nil {
		i.anInstruction.typs = typs
	}
	fixupUseChain(i)
	return i
}

func NewUpdate(address *Field, v Value, block *BasicBlock) *Update {
	s := &Update{
		anInstruction: newAnInstuction(block),
		Value:         v,
		Address:       address,
	}
	fixupUseChain(s)
	return s
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

func (f *FunctionBuilder) NewCall(target Value, args []Value, isDropError bool) *Call {
	var freevalue []Value
	var parent *Function
	binding := make([]Value, 0, len(freevalue))

	switch inst := target.(type) {
	case *Field:
		// field
		fun := inst.GetLastValue().(*Function)
		freevalue = fun.FreeValues
		parent = fun.parent
	case *Function:
		// Function
		freevalue = inst.FreeValues
	case *Parameter:
		// is a freevalue, pass
	case *Call:
		// call, check the function
		switch method := inst.Method.(type) {
		case *Function:
			fun := method.ReturnValue()[0].(*Function)
			freevalue = fun.FreeValues
			parent = fun.parent
		}
	default:
		// other
		// con't call
		f.NewError(Error, SSATAG, "call target is con't call: "+target.String())
	}

	if parent == nil {
		parent = f.Function
	}
	getField := func(fun *Function, key string) bool {
		if v := fun.builder.ReadField(key); v != nil {
			binding = append(binding, v)
			return true
		}
		return false
	}
	for index := range freevalue {
		if para, ok := freevalue[index].(*Parameter); ok { // not modify
			// find freevalue in parent function
			if v := parent.builder.ReadVariable(para.variable); !utils.IsNil(v) {
				switch v := v.(type) {
				case *Parameter:
					if !v.isFreevalue {
						// is parameter, just abort
						continue
					}
					// is freevalue, find in current function
				default:
					binding = append(binding, v)
					continue
				}
			}
			if parent != f.Function {
				// find freevalue in current function
				if v := f.ReadVariable(para.variable); !utils.IsNil(v) {
					binding = append(binding, v)
					continue
				}
			}
			f.NewError(Error, SSATAG, "call target clouse binding variable not found: %s", para)
		}

		if field, ok := freevalue[index].(*Field); ok { // will modify in function must field
			if getField(parent, field.Key.String()) {
				continue
			}
			if parent != f.Function {
				if getField(f.Function, field.Key.String()) {
					continue
				}
			}
			f.NewError(Error, SSATAG, "call target clouse binding variable not found: %s", field)
		}
	}
	c := &Call{
		anInstruction: newAnInstuction(f.CurrentBlock),
		Method:        target,
		Args:          args,
		user:          []User{},
		isDropError:   isDropError,
		binding:       binding,
	}
	return c
}
