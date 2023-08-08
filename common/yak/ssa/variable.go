package ssa

// --------------- for assign
type LeftValue interface {
	Assign(Value)
	GetValue() Value
}

type IdentifierLV struct {
	variable string
	f        *Function
}

func (i *IdentifierLV) Assign(v Value) {
	i.f.wirteVariable(i.variable, v)
}

func (i *IdentifierLV) GetValue() Value {
	return i.f.readVariable(i.variable)
}

var _ LeftValue = (*IdentifierLV)(nil)





func (f *Function) wirteVariable(variable string, value Value) {
	f.wirteVariableByBlock(variable, value, f.currentBlock)
}

func (f *Function) readVariable(variable string) Value {
	return f.readVariableByBlock(variable, f.currentBlock)
}

func (f *Function) wirteVariableByBlock(variable string, value Value, block *BasicBlock) {
	_, ok := f.currentDef[variable]
	if !ok {
		f.currentDef[variable] = make(map[*BasicBlock]Value)
	}
	f.currentDef[variable][block] = value
}

func (f *Function) readVariableByBlock(variable string, block *BasicBlock) Value {
	if map2, ok := f.currentDef[variable]; ok {
		if value, ok := map2[block]; ok {
			return value
		}
	}
	return f.readVariableRecursive(variable, block)
}

func (f *Function) readVariableInParamAndFV(variable string) Value {
	// in enter block
	if para, ok := f.Param[variable]; ok {
		return para
	}
	if parent := f.parent; parent != nil {
		if v := parent.readVariable(variable); v != nil {
			freevalue := &Parameter{
				variable:    variable,
				Func:        f,
				isFreevalue: true,
				user:        []User{},
			}
			return freevalue
			// } else {
			// 	fmt.Printf("warn: con't found variable %s in function %s and parent-function %s\n", variable, f.name, parent.name)
		}
	}
	return nil
}

func (f *Function) readVariableRecursive(variable string, block *BasicBlock) Value {
	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		phi := NewPhi(f, block, variable)
		block.inCompletePhi[variable] = phi
		v = phi
	} else if len(block.Preds) == 0 {
		// this is enter block  in this function
		v = f.readVariableInParamAndFV(variable)
	} else if len(block.Preds) == 1 {
		v = f.readVariableByBlock(variable, block.Preds[0])
	} else {
		phi := NewPhi(f, block, variable)
		f.wirteVariableByBlock(variable, phi, block)
		v = phi.Build()
	}
	if v != nil {
		f.wirteVariableByBlock(variable, v, block)
	}
	return v
}

func (b *BasicBlock) Sealed() {
	for _, p := range b.inCompletePhi {
		p.Build()
	}
	b.isSealed = true
}
