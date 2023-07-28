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
	if len(block.Preds) == 1 {
		return f.readVariableByBlock(variable, block.Preds[0])
	} else {
		phi := &Phi{
			anInstruction: anInstruction{
				Parent: f,
				Block:  block,
			},
			Edge: []Value{},
		}
		f.wirteVariableByBlock(variable, phi, block)
		for _, p := range phi.Block.Preds {
			phi.Edge = append(phi.Edge, f.readVariableByBlock(variable, p))
		}
		block.Phis = append(block.Phis, phi)
		return phi
	}
	return nil
}
