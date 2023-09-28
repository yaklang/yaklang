package ssa

import "github.com/yaklang/yaklang/common/utils"

func NewCall(target Value, args, binding []Value, isDropError bool, block *BasicBlock) *Call {
	c := &Call{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
		Method:        target,
		Args:          args,
		IsDropError:   isDropError,
		binding:       binding,
	}
	c.AddValue(target)
	for _, v := range args {
		c.AddValue(v)
	}
	for _, v := range binding {
		c.AddValue(v)
	}

	return c
}

func (f *FunctionBuilder) NewCall(target Value, args []Value, isDropError bool) *Call {
	var freevalue []Value
	var parent *Function
	binding := make([]Value, 0, len(freevalue))

	switch inst := target.(type) {
	case *Field:
		// field
		if v, ok := inst.Obj.(Value); ok && inst.IsMethod {
			args = append([]Value{v}, args...)
		}
		fun, ok := inst.GetLastValue().(*Function)
		if ok {
			freevalue = fun.FreeValues
			parent = fun.parent
		}
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
		// f.NewError(Error, SSATAG, "call target is con't call: "+target.String())
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
			if v := parent.builder.ReadVariable(para.variable, false); !utils.IsNil(v) {
				switch v := v.(type) {
				case *Parameter:
					if !v.IsFreevalue {
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
				if v := f.ReadVariable(para.variable, false); !utils.IsNil(v) {
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
	// if t := target.GetType(); !utils.IsNil(t) {
	// 	if ft, ok := t.(*FunctionType); ok {
	// c.SetType(ft.ReturnType)
	// 	}
	// }
	return NewCall(target, args, binding, isDropError, f.CurrentBlock)
}
