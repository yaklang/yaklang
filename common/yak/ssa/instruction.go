package ssa

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
