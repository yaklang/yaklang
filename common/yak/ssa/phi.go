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
	for _, predBlock := range phi.Block.Preds {
		// phi.Edge[i] = phi.Parent.readVariableByBlock(phi.variable, p)
		v := phi.Func.readVariableByBlock(phi.variable, predBlock)
		if v == nil {
			// warn!!! con't found this variable
			//TODO: if in left-expression is not warn
			fmt.Printf("warn!!! phi con't found this variable[%s]\n", phi.variable)
		}
		phi.Edge = append(phi.Edge, v)
	}
	v := phi.tryRemoveTrivialPhi()
	if v == phi {
		block := phi.Block
		block.Phis = append(block.Phis, phi)
	}
	fixupUseChain(phi)
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
