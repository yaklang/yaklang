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
	return NewCall(target, args, nil, f.CurrentBlock)
}

func (c *Call) HandleFreeValue(fvs map[string]bool) {
	builder := c.GetFunc().builder
	recoverBuilder := builder.SetCurrent(c)
	defer recoverBuilder()

	// parent := builder.parentBuilder

	for name, modify := range fvs {
		_ = modify
		_ = name
		if v := builder.ReadVariableBefore(name, false, c); v != nil {
			if modify {
				field := builder.NewCaptureField(name)
				field.OutCapture = false
				// EmitBefore(c, field)
				builder.emitInstructionAfter(field, c)
				field.SetPosition(c.GetPosition())
				field.SetType(BasicTypes[Any])
				builder.WriteVariable(name, field)
				ReplaceValueInRange(v, field, func(inst Instruction) bool {
					if inst.GetPosition() == nil {
						return false
					}
					if inst.GetPosition().StartLine > c.GetPosition().StartLine {
						return true
					} else {
						return false
					}
				})
				//TODO: modify this binding
				c.binding = append(c.binding, v)
			} else {
				c.binding = append(c.binding, v)
			}
		} else {
			c.NewError(Error, SSATAG, BindingNotFound(name))
		}
	}
}
