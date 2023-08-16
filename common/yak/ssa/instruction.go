package ssa

import "fmt"

func (i *If) AddTrue(t *BasicBlock) {
	i.True = t
	i.Block.AddSucc(t)
}

func (i *If) AddFalse(f *BasicBlock) {
	i.False = f
	i.Block.AddSucc(f)
}

func (f *Field) GetLastValue() Value {
	if lenght := len(f.update); lenght != 0 {
		update, ok := f.update[lenght-1].(*Update)
		if !ok {
			panic("")
		}
		return update.value
	}
	return nil
}

func (f *Function) newCall(target Value, args []Value, isDropError bool) *Call {
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
	return c
}
