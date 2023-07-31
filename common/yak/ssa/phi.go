package ssa

func NewPhi(f *Function, block *BasicBlock, variable string) *Phi {
	return &Phi{
		anInstruction: anInstruction{
			Parent: f,
			Block:  block,
		},
		Edge:     make([]Value, 0, len(block.Preds)),
		user:     make([]User, 0),
		variable: variable,
	}
}

func (phi *Phi) Build() Value {
	for _, p := range phi.Block.Preds {
		// phi.Edge[i] = phi.Parent.readVariableByBlock(phi.variable, p)
		phi.Edge = append(phi.Edge, phi.Parent.readVariableByBlock(phi.variable, p))
	}
	v := phi.triRemoveTrivialPhi()
	if v == phi {
		block := phi.Block
		block.Phis = append(block.Phis, phi)
	}
	return v
}

func (phi *Phi) triRemoveTrivialPhi() Value {
	var same Value
	same = nil
	for _, v := range phi.Edge {
		// pass same and phi self
		if v == same || v == phi {
			continue
		}

		// if have multiple value
		if same != nil {
			return phi
		}
		same = v
	}

	if same == nil {
		// The phi is in unreachable block or in the start block
		return nil
	}

	ReplaceValue(phi, same)

	for _, user := range phi.GetUser() {
		switch p := user.(type) {
		case *Phi:
			p.triRemoveTrivialPhi()
		}
	}

	return same
}
