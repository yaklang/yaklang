package ssa

func NewPhi(block *BasicBlock, variable string) *Phi {
	p := &Phi{
		anInstruction: newAnInstuction(block),
		Edge:          make([]Value, 0, len(block.Preds)),
		user:          make([]User, 0),
	}
	p.SetVariable(variable)
	return p
}

func (phi *Phi) Name() string { return phi.GetVariable() }

func (phi *Phi) Build() Value {
	phi.Block.Skip = true
	for _, predBlock := range phi.Block.Preds {
		// phi.Edge[i] = phi.Parent.readVariableByBlock(phi.variable, p)
		v := phi.Func.builder.readVariableByBlock(phi.GetVariable(), predBlock)
		phi.Edge = append(phi.Edge, v)
	}
	phi.Block.Skip = false
	v := phi.tryRemoveTrivialPhi()
	if v == phi {
		block := phi.Block
		block.Phis = append(block.Phis, phi)
		phi.Func.SetReg(phi)
		phi.Func.WriteSymbolTable(phi.GetVariable(), phi)
	}
	fixupUseChain(v)
	return v
}

func (phi *Phi) tryRemoveTrivialPhi() Value {
	w1, w2 := phi.wit1, phi.wit2
	getValue := func(pass Value) Value {
		for _, v := range phi.Edge {
			if v == phi || v == pass {
				continue
			}
			return v
		}
		return nil
	}
	if w1 == nil || w2 == nil {
		// init w1 w2
		w1 = getValue(nil)
		w2 = getValue(w1)
	} else {
		if w1 == phi || w1 == w2 {
			w1 = getValue(w2)
		}
		if w2 == phi || w2 == w1 {
			w2 = getValue(w1)
		}
	}

	var ret Value
	ret = phi
	if w1 == nil {
		if w2 == nil {
			ret = nil
		}
		ret = w2
	}
	if w2 == nil {
		ret = w1
	}
	if ret != nil && ret != phi {
		ReplaceValue(phi, ret)
		for _, user := range phi.GetUsers() {
			switch p := user.(type) {
			case *Phi:
				p.tryRemoveTrivialPhi()
			}
		}
	}
	return ret
}
