package ssa

func NewPhi(block *BasicBlock, variable string, create bool) *Phi {
	p := &Phi{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		Edge:          make([]Value, 0, len(block.Preds)),
		create:        create,
	}
	p.SetName(variable)
	p.SetBlock(block)
	p.SetFunc(block.GetFunc())
	return p
}

func SpinHandle(name string, phiValue, origin, latch Value) Value {
	// log.Infof("build phi: %s %v %v %v", name, phiVar, v1, v2)
	if phiValue == latch {
		ReplaceAllValue(phiValue, origin)
		DeleteInst(phiValue)
		return origin
	}
	if phi, ok := ToPhi(phiValue); ok {
		phi.Edge = append(phi.Edge, origin)
		phi.Edge = append(phi.Edge, latch)
		phi.SetName(name)
		phi.GetProgram().SetVirtualRegister(phi)
		return phiValue
	}
	return nil
}

// build phi
func generalPhi(builder *FunctionBuilder) func(name string, t []Value) Value {
	return func(name string, t []Value) Value {
		phi := builder.EmitPhi(name, t)
		if phi == nil {
			return nil
		}
		phi.GetProgram().SetVirtualRegister(phi)
		phi.GetProgram().SetInstructionWithName(name, phi)
		return phi
	}
}
