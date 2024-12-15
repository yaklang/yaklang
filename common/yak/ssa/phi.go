package ssa

import (
	"fmt"
	"runtime"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"golang.org/x/exp/slices"
)

func NewPhi(block *BasicBlock, variable string) *Phi {
	p := &Phi{
		anValue: NewValue(),
		Edge:    make([]Value, 0, len(block.Preds)),
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
	func() {
		// step 1
		// this  value not change in this loop, should replace phi-value to header value
		if phiValue == latch {
			ReplaceAllValue(phiValue, header)

			DeleteInst(phiValue)

			for name, v := range ReplaceMemberCall(phiValue, header) {
				ret[name] = v
			}

			ret[name] = header
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
				phi2.Edge[index] = header
				ret[name] = phi2
				DeleteInst(phiValue)
				ReplaceAllValue(phiValue, phi2)
				for name, v := range ReplaceMemberCall(phiValue, phi2) {
					ret[name] = v
				}
				return
			}
		}

		// step 3
		phi.Edge = append(phi.Edge, header)
		phi.Edge = append(phi.Edge, latch)
		phi.SetName(name)
		phi.GetProgram().SetVirtualRegister(phi)
		ret[name] = phiValue
		return
	}()
	return ret
}

var _ ssautil.SpinHandle[Value] = SpinHandle

// build phi
func generatePhi(builder *FunctionBuilder, block *BasicBlock, cfgEntryBlock Value) func(name string, t []Value) Value {
	return func(name string, vst []Value) Value {
		defer func() {
			if msg := recover(); msg != nil {
				var buffer = make([]byte, 4096)
				stack := runtime.Stack(buffer, false)
				fmt.Println("报错原因：" + string(buffer[:stack]))
				fmt.Println("name: " + name)
				for _, value := range vst {
					fmt.Println("verbose: " + value.GetShortVerboseName())
				}
			}
		}()
		if block != nil {
			recoverBlock := builder.CurrentBlock
			builder.CurrentBlock = block
			defer func() {
				builder.CurrentBlock = recoverBlock
			}()
		}

		var t Type
		var vs []Value
		typeMerge := make(map[Type]struct{})

		for _, v := range vst {
			vs = append(vs, v)
		}
		if len(vs) == 0 {
			return nil
		}
		for _, v := range vs {
			if v.GetType().GetTypeKind() == AnyTypeKind {
				continue
			}
			if _, ok := ToParameter(v); ok {
				continue
			}
			if _, ok := typeMerge[v.GetType()]; ok {
				continue
			}
			typeMerge[v.GetType()] = struct{}{}
		}
		switch len(typeMerge) {
		case 0:
			t = CreateAnyType()
		case 1:
			for t2, _ := range typeMerge {
				t = t2
			}
		default:
			// import the or type
			t = NewOrType(lo.Keys(typeMerge)...)
		}
		phi := builder.EmitPhi(name, vs)
		phi.SetType(t)
		if phi == nil {
			return nil
		}
		phi.GetProgram().SetVirtualRegister(phi)
		phi.GetProgram().SetInstructionWithName(name, phi)
		phi.SetVerboseName(vs[0].GetVerboseName())
		phi.CFGEntryBasicBlock = cfgEntryBlock
		return phi
	}
}

var _ ssautil.MergeHandle[Value] = generatePhi(nil, nil, nil)
