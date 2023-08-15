package ssa

import (
	"fmt"

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

func (f *Function) emitCall(target Value, args []Value, isDropError bool) *Call {
	if f.currentBlock.finish {
		return nil
	}

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
		panic("call target is con't call: " + target.String())
	}

	if parent == nil {
		parent = f
	}
	getField := func(fun *Function, key string) bool {
		if v := fun.readField(key); v != nil {
			binding = append(binding, v)
			return true
		}
		return false
	}
	for index := range freevalue {
		if para, ok := freevalue[index].(*Parameter); ok { // not modify
			// find freevalue in parent function
			if v := parent.readVariable(para.variable); v != nil {
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
			if parent != f {
				// find freevalue in current function
				if v := f.readVariable(para.variable); v != nil {
					binding = append(binding, v)
					continue
				}
			}
			panic(fmt.Sprintf("call target clouse binding variable not found: %s", para))
		}

		if field, ok := freevalue[index].(*Field); ok { // will modify in function must field
			if getField(parent, field.Key.String()) {
				continue
			}
			if parent != f {
				if getField(f, field.Key.String()) {
					continue
				}
			}
			panic(fmt.Sprintf("call target clouse binding variable not found: %s", field))
		}
	}
	c := &Call{
		anInstruction: f.newAnInstuction(),
		Method:        target,
		Args:          args,
		user:          []User{},
		isDropError:   isDropError,
		binding:       binding,
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
