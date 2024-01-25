package ssa

func NewCall(target Value, args, binding []Value, block *BasicBlock) *Call {
	c := &Call{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		Method:        target,
		Args:          args,
		binding:       binding,
		Async:         false,
		Unpack:        false,
		IsDropError:   false,
		IsEllipsis:    false,
	}
	return c
}

func (f *FunctionBuilder) NewCall(target Value, args []Value) *Call {
	call := NewCall(target, args, nil, f.CurrentBlock)
	return call
}

func (c *Call) HandleFreeValue(fvs []string, sideEffect []string) {
	builder := c.GetFunc().builder
	recoverBuilder := builder.SetCurrent(c)
	defer recoverBuilder()

	// parent := builder.parentBuilder

	for _, name := range fvs {
		// get current function variable for call this closure
		v := builder.ReadVariableBefore(name, false, c)
		// if not get, try build freeValue in parent function.
		if v == nil && builder.CanCaptureParentValue(name) {
			v = builder.BuildFreeValue(name)
		}
		if v != nil {
			c.binding = append(c.binding, v)
		} else {
			// mark error in freeValue.Variable
			// get freeValue
			fun, ok := ToFunction(c.Method)
			if !ok {
				continue
			}
			freeValue, ok := fun.FreeValues[name]
			if !ok {
				continue
			}
			if variable := freeValue.GetVariable(name); variable != nil {
				variable.NewError(Error, SSATAG, BindingNotFound(name, c.GetRange()))
				if len(fun.GetAllVariables()) != 0 {
					c.NewError(Error, SSATAG, BindingNotFoundInCall(name))
				}
			}
		}
	}

	for _, name := range sideEffect {
		v := builder.ReadVariableBefore(name, false, c)
		if v == nil {
			// if side effect not found, just skip
			continue
		}
		// handle side effect
		sideEffect := NewSideEffect(name, c)
		builder.EmitInstructionAfter(sideEffect, c)
		sideEffect.SetRange(c.GetRange())
		sideEffect.SetType(BasicTypes[Any])
		builder.WriteVariable(name, sideEffect)
		InsertValueReplaceOriginal(name, v, sideEffect)
	}

}
