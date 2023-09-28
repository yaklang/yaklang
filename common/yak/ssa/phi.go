package ssa

func NewPhi(block *BasicBlock, variable string, create bool) *Phi {
	p := &Phi{
		anInstruction: newAnInstuction(block),
		anNode:        NewNode(),
		Edge:          make([]Value, 0, len(block.Preds)),
		create:        create,
	}
	p.SetVariable(variable)
	return p
}

func (b *BasicBlock) Sealed() {
	for _, p := range b.inCompletePhi {
		v := p.Build()
		if i, ok := v.(*Make); ok && i.buildField != nil {
			for _, user := range i.GetValues() {
				if f, ok := user.(*Field); ok {
					newf := i.buildField(f.Key.String())
					f.RemoveUser(i)
					ReplaceValue(f, newf)
					DeleteInst(f)
				}
			}
		} else if un, ok := v.(*Undefine); ok {
			if p.GetParent().builder.CanBuildFreeValue(p.variable) {
				v = p.GetParent().builder.BuildFreeValue(p.variable)
				// un.Replace(v)
				ReplaceValue(un, v)
				DeleteInst(un)
			}
		}
	}
	b.inCompletePhi = nil
	b.isSealed = true
}

func (phi *Phi) Name() string { return phi.GetVariable() }

func (phi *Phi) Build() Value {
	phi.Block.Skip = true
	for _, predBlock := range phi.Block.Preds {
		v := phi.Func.builder.readVariableByBlock(phi.GetVariable(), predBlock, phi.create)
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
	if v != nil {
		fixupUseChain(v)
	}
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
		} else {
			ret = w2
		}
	} else if w2 == nil {
		ret = w1
	}
	if ret != nil && ret != phi {
		phi.Replace(ret)
	}
	return ret
}

func (phi *Phi) Replace(to Value) {
	ReplaceValue(phi, to)
	for _, user := range phi.GetUsers() {
		switch p := user.(type) {
		case *Phi:
			p.tryRemoveTrivialPhi()
		}
	}
}
