package ssa

import "fmt"

func NewPhi(f *Function, block *BasicBlock, variable string) *Phi {
	return &Phi{
		anInstruction: anInstruction{
			Func:  f,
			Block: block,
			typs:  make(Types, 0),
			pos:   &Position{},
		},
		Edge:     make([]Value, 0, len(block.Preds)),
		user:     make([]User, 0),
		variable: variable,
	}
}

func (phi *Phi) Build() Value {
	for _, p := range phi.Block.Preds {
		// phi.Edge[i] = phi.Parent.readVariableByBlock(phi.variable, p)
		v := phi.Func.readVariableByBlock(phi.variable, p)
		if v == nil {
			// warn!!! con't found this variable
			//TODO: if in left-expression is not warn
			fmt.Printf("warn!!! phi con't found this variable[%s]\n", phi.variable)
		}
		phi.Edge = append(phi.Edge, v)
	}
	v := phi.triRemoveTrivialPhi()
	if v == phi {
		block := phi.Block
		block.Phis = append(block.Phis, phi)
	}
	fixupUseChain(phi)
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

	for _, user := range phi.GetUsers() {
		switch p := user.(type) {
		case *Phi:
			p.triRemoveTrivialPhi()
		}
	}

	return same
}
