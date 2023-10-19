package ssa

func NewCall(target Value, args, binding []Value, block *BasicBlock) *Call {
	c := &Call{
		anInstruction: newAnInstruction(block),
		anNode:        NewNode(),
		Method:        target,
		Args:          args,
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

func (f *FunctionBuilder) NewCall(target Value, args []Value) *Call {
	return NewCall(target, args, nil, f.CurrentBlock)
}

func (c *Call) HandleFreeValue(fvs map[string]bool) {
	builder := c.GetParent().builder

	recoverBlock := builder.CurrentBlock
	recoverSymbol := builder.blockSymbolTable
	builder.CurrentBlock = c.Block
	builder.blockSymbolTable = c.symbolTable
	defer func() {
		builder.CurrentBlock = recoverBlock
		builder.blockSymbolTable = recoverSymbol
	}()

	// parent := builder.parentBuilder

	for name, modify := range fvs {
		// v := parent.readVariableByBlock(name, c.Block)
		if v := builder.GetVariableBefore(name, c); v != nil {
			// if v := builder.ReadVariable(name, false); v != nil {
			if modify {
				field := builder.NewCaptureField(name)
				field.OutCapture = false
				// EmitBefore(c, field)
				EmitAfter(c, field)
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
