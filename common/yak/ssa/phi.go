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

func SpinHandle(name string, phiValue, header, latch Value) map[string]Value {
	/*
		loop-header:
			init

		loop-condition:
			loop[init, condition,] jump loop-body

		loop-body:
			...
			jump loop-latch;

		loop-latch:
			...
			jump loop-header;
	*/
	ret := make(map[string]Value)
	for true {
		// step 1
		// this  value not change in this loop, should replace phi-value to header value
		if phiValue == latch {
			ReplaceAllValue(phiValue, header)
			DeleteInst(phiValue)

			for name, v := range ReplaceMemberCall(phiValue, header) {
				ret[name] = v
			}

			ret[name] = header
			break
		}

		// only this value change, create a Phi
		phi, ok := ToPhi(phiValue)
		if !ok {
			log.Errorf("phiValue is not a phi %s: %v", name, phiValue)
			break
		}

		// step 2
		if phi2, ok := ToPhi(latch); ok {
			if index := slices.Index(phi2.Edge, phiValue); index != -1 {
				phi2.Edge[index] = header
				ret[name] = phi2
				DeleteInst(phiValue)
				break
			}
		}

		// step 3
		phi.Edge = append(phi.Edge, header)
		phi.Edge = append(phi.Edge, latch)
		phi.SetName(name)
		phi.GetProgram().SetVirtualRegister(phi)
		ret[name] = phiValue
		break
	}
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
		phi.SetVerboseName(t[0].GetVerboseName())
		return phi
	}
}

var _ ssautil.MergeHandle[Value] = generalPhi(nil, nil)
