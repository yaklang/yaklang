package ssa

func (phi *Phi) Build() Value {
	for i, p := range phi.Block.Preds {
		phi.Edge[i] = phi.Parent.readVariableByBlock(phi.variable, p)
	}
	v := triRemoveTrivialPhi(phi)
	if v == phi {
		block := phi.Block
		block.Phis = append(block.Phis, phi)
	}
	return v
}

func triRemoveTrivialPhi(phi *Phi) Value {
	var same Value
	same = nil
	for _, v := range phi.Edge {
		if same == v || same == phi {
			continue
		}

		if same != nil {
			return phi
		}
		same = v
	}
	for _, user := range phi.GetUser() {
		switch p := user.(type) {
		case *Phi:
			triRemoveTrivialPhi(p)
		}
	}

	return same
}

func (i *If) AddTrue(t *BasicBlock) {
	i.True = t
	i.Block.AddSucc(t)
}

func (i *If) AddFalse(f *BasicBlock) {
	i.False = f
	i.Block.AddSucc(f)
}
