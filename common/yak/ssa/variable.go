package ssa

func (f *Function) wirteVariable(variable string, value Value) {
	f.wirteVariableByBlock(variable, value, f.currentBlock)
}
func (f *Function) wirteVariableByBlock(variable string, value Value, block *BasicBlock) {
	_, ok := f.currentDef[variable]
	if !ok {
		f.currentDef[variable] = make(map[*BasicBlock]Value)
	}
	f.currentDef[variable][f.currentBlock] = value
}

func (f *Function) readVariable(variable string) Value {
	return f.readVariableByBlock(variable, f.currentBlock)
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
	// if block in sealedBlock
	if len(block.Preds) == 1 {
		return f.readVariableByBlock(variable, block.Preds[0])
	} else {
		v := f.newPhi(block, variable)
		f.wirteVariableByBlock(variable, v, block)
		return v
	}
	return nil
}
