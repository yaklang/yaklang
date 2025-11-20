package ssa

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"golang.org/x/exp/slices"
)

func NewPhi(block *BasicBlock, variable string) *Phi {
	p := &Phi{
		anValue: NewValue(),
		Edge:    make([]int64, 0, len(block.Preds)),
	}
	p.SetName(variable)
	p.SetBlock(block)
	p.SetFunc(block.GetFunc())
	p.GetProgram().SetVirtualRegister(p)
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
	retT := make(map[string]Value)
	func() {
		// step 1
		// this  value not change in this loop, should replace phi-value to header value
		if phiValue == latch {
			ReplaceAllValue(phiValue, header)
			DeleteInst(phiValue)

			for name, v := range ReplaceMemberCall(phiValue, header) {
				ret[name] = v
			}

			var CreatePhi func(Value)
			pass := make(map[Value]struct{})
			CreatePhi = func(start Value) {
				if _, ok := pass[start]; ok { // Avoid loops
					return
				} else {
					pass[start] = struct{}{}
				}

				for k, v := range start.GetAllMember() {
					res := checkCanMemberCallExist(start, k)
					if find, ok := ret[res.name]; ok {
						phi, ok := ToPhi(phiValue)
						if !ok {
							log.Errorf("phiValue is not a phi %s: %v", name, phiValue)
							return
						}
						if find != v {
							phit := NewPhi(phi.GetBlock(), res.name)
							phit.Edge = append(phit.Edge, find.GetId())
							phit.Edge = append(phit.Edge, v.GetId())
							phit.SetName(res.name)
							phit.GetProgram().SetVirtualRegister(phit)
							if block := phi.GetBlock(); block != nil && !slices.Contains(block.Phis, phit.GetId()) {
								block.Phis = append(block.Phis, phit.GetId())
							}
							phit.CFGEntryBasicBlock = phi.CFGEntryBasicBlock
							retT[res.name] = phit
						}
					}
				}
				for _, v := range start.GetAllMember() {
					CreatePhi(v)
				}
			}

			CreatePhi(header)

			for n, r := range retT {
				ret[n] = r
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
			if index := slices.Index(phi2.Edge, phiValue.GetId()); index != -1 {
				phi2.Edge[index] = header.GetId()
				ret[name] = phi2
				ReplaceAllValue(phiValue, phi2)
				for name, v := range ReplaceMemberCall(phiValue, phi2) {
					ret[name] = v
				}
				DeleteInst(phiValue)
				return
			}
		}

		// step 3
		if headerID := header.GetId(); !slices.Contains(phi.Edge, headerID) {
			phi.Edge = append(phi.Edge, headerID)
		}
		if latchID := latch.GetId(); !slices.Contains(phi.Edge, latchID) {
			phi.Edge = append(phi.Edge, latchID)
		}
		phi.SetName(name)
		phi.GetProgram().SetVirtualRegister(phi)
		ret[name] = phiValue
	}()
	return ret
}

var _ ssautil.SpinHandle[Value] = SpinHandle

// build phi
func generatePhi(builder *FunctionBuilder, block *BasicBlock, cfgEntryBlock Value) func(name string, t []Value) Value {
	return func(name string, vst []Value) Value {
		defer func() {
			if msg := recover(); msg != nil {
				log.Errorf("generatePhi panic: %s", msg)
				fmt.Println("name: " + name)
				for _, value := range vst {
					fmt.Println("verbose: " + value.GetShortVerboseName())
				}
				utils.PrintCurrentGoroutineRuntimeStack()

			}
		}()
		if !utils.IsNil(block) {
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
			//if _, ok := ToParameter(v); ok {
			//	continue
			//}
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
		if utils.IsNil(phi) {
			return nil
		}
		phi.GetProgram().SetVirtualRegister(phi)
		phi.GetProgram().SetInstructionWithName(name, phi)
		phi.SetVerboseName(vs[0].GetVerboseName())
		// 不能直接进行cfgEntryBlock.GetId()，即便GetId会判断receiver为nil
		// 但是cfgEntryBlock是接口类型，会产生interface panic
		if !utils.IsNil(cfgEntryBlock) {
			phi.CFGEntryBasicBlock = cfgEntryBlock.GetId()
		} else {
			phi.CFGEntryBasicBlock = -1
		}
		return phi
	}
}

var _ ssautil.MergeHandle[Value] = generatePhi(nil, nil, nil)
