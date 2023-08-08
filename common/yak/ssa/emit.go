package ssa

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func (f *Function) emit(i Instruction) {
	f.currentBlock.Instrs = append(f.currentBlock.Instrs, i)
}

func fixupUseChain(u User) {
	if u == nil {
		return
	}
	for _, v := range u.GetValues() {
		if v != nil {
			v.AddUser(u)
		}
	}
	for _, user := range u.GetUsers() {
		if user != nil {
			user.AddValue(u)
		}
	}
}

func (f *Function) emitArith(op yakvm.OpcodeFlag, x, y Value) *BinOp {
	if f.currentBlock.finish {
		return nil
	}
	b := &BinOp{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Op:   op,
		X:    x,
		Y:    y,
		user: []User{},
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
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Cond: cond,
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
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		To: to,
	}
	f.emit(j)
	f.currentBlock.AddSucc(to)
	f.currentBlock.finish = true
	return j
}

func (f *Function) emitReturn(vs []Value) *Return {
	if f.currentBlock.finish {
		return nil
	}
	r := &Return{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Results: vs,
	}
	fixupUseChain(r)
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
			fmt.Printf("debug %v\n", para.variable)
			panic("call target clouse binding variable not found")
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
			panic("call target clouse binding field not found")
		}
	}
	c := &Call{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
		},
		Method:      target,
		Args:        args,
		user:        []User{},
		isDropError: isDropError,
		binding:     binding,
	}

	fixupUseChain(c)
	f.emit(c)
	return c
}


func (f *Function) emitUpdate(address User, v Value) *Update {
	//use-value-chain: address -> update -> value
	s := &Update{
		anInstruction: anInstruction{
			Func:  f,
			Block: f.currentBlock,
			typ:   nil,
		},
		value:   v,
		address: address,
	}
	f.emit(s)
	fixupUseChain(s)
	return s
}
