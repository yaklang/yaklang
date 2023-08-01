package ssa

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
	map2, ok := f.currentDef[variable]
	if !ok {
		return nil
	}
	value, ok := map2[block]
	if !ok {
		return f.readVariableRecursive(variable, block)
	}
	return value
}

func (f *Function) readVariableRecursive(variable string, block *BasicBlock) Value {
	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		phi := NewPhi(f, block, variable)
		block.inCompletePhi[variable] = phi
		v = phi
	} else if len(block.Preds) == 1 {
		v = f.readVariableByBlock(variable, block.Preds[0])
	} else {
		phi := NewPhi(f, block, variable)
		f.wirteVariableByBlock(variable, phi, block)
		v = phi.Build()
	}
	f.wirteVariableByBlock(variable, v, block)
	return v
}

func (b *BasicBlock) Sealed() {
	for _, p := range b.inCompletePhi {
		p.Build()
	}
	b.isSealed = true
}
