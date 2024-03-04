package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"golang.org/x/exp/slices"
)

func NewPhi(block *BasicBlock, variable string, create bool) *Phi {
	p := &Phi{
		anValue: NewValue(),
		Edge:    make([]Value, 0, len(block.Preds)),
		create:  create,
	}
	p.SetName(variable)
	p.SetBlock(block)
	p.SetFunc(block.GetFunc())
	return p
}

func SpinHandle(name string, phiValue, origin, latch Value) map[string]Value {
	// log.Infof("build phi: %s %v %v %v", name, phiVar, v1, v2)
	ret := make(map[string]Value)
	handler := func() {

		// step 1
		// this  value not change in this loop, should replace phi-value to origin value
		if phiValue == latch {
			ReplaceAllValue(phiValue, origin)
			DeleteInst(phiValue)

			for name, v := range ReplaceMemberCall(phiValue, origin) {
				ret[name] = v
			}

			ret[name] = origin
			return
		}

		// only this value change, create a Phi
		phi, ok := ToPhi(phiValue)
		if !ok {
			log.Errorf("phiValue is not a phi %s: %v", name, phiValue)
			return
		}

		// step 2
		if phi2, ok := ToPhi(latch); ok {
			if index := slices.Index(phi2.Edge, phiValue); index != -1 {
				phi2.Edge[index] = origin
				ret[name] = phi2
				DeleteInst(phiValue)
				return
			}
		}

		// step 3
		phi.Edge = append(phi.Edge, origin)
		phi.Edge = append(phi.Edge, latch)
		phi.SetName(name)
		phi.GetProgram().SetVirtualRegister(phi)
		ret[name] = phiValue
	}
	handler()
	return ret
}

var _ ssautil.SpinHandle[Value] = SpinHandle

// build phi
func generalPhi(builder *FunctionBuilder, block *BasicBlock) func(name string, t []Value) Value {
	return func(name string, t []Value) Value {
		if block != nil {
			recoverBlock := builder.CurrentBlock
			builder.CurrentBlock = block
			defer func() {
				builder.CurrentBlock = recoverBlock
			}()
		}

		phi := builder.EmitPhi(name, t)
		if phi == nil {
			return nil
		}
		phi.GetProgram().SetVirtualRegister(phi)
		phi.GetProgram().SetInstructionWithName(name, phi)
		return phi
	}
}

var _ ssautil.MergeHandle[Value] = generalPhi(nil, nil)
