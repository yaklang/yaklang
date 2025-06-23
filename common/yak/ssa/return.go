package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func (r *Return) calcType() Type {
	handleType := func(v Value) Type {
		if v == nil {
			log.Errorf("Return[%s: %s] value is nil", r.String(), r.GetRange())
			return BasicTypes[NullTypeKind]
		}
		t := v.GetType()
		if objTyp, ok := ToObjectType(t); ok {
			t = ParseClassBluePrint(v, objTyp)
		}
		return t
	}

	switch len(r.Results) {
	case 0:
		return BasicTypes[NullTypeKind]
	case 1:
		return handleType(r.GetValueById(r.Results[0]))
	default:
		newObjTyp := NewObjectType()
		for i, v := range r.Results {
			v := r.GetValueById(v)
			newObjTyp.AddField(NewConst(i), handleType(v))
		}
		newObjTyp.Finish()
		newObjTyp.Kind = TupleTypeKind
		newObjTyp.Len = len(r.Results)
		return newObjTyp
	}
}

func (b *FunctionBuilder) fixupParameterWithThis() {
	// has this value, in first parameter
	if b.MarkedThisObject == nil {
		return
	}
	if len(b.Params) <= 0 {
		return
	}
	// if this value is not object, and not user, should remove it.
	paraId := b.Params[0]
	para := b.GetValueById(paraId)
	if para == nil || para.IsObject() || para.HasUsers() {
		return
	}

	// remove from param
	b.Params = utils.RemoveSliceItem(b.Params, paraId)
	// fix other field in function
	b.ParamLength--
	// fix other parameter index
	for i, p := range b.Params {
		p := b.GetValueById(p)
		param, ok := ToParameter(p)
		if !ok {
			log.Warnf("fixupParameterWithThis: parameter is not a Parameter, but is %s", p.GetName())
			continue
		}
		param.FormalParameterIndex = i
	}
	// fixup side effect,
	// if this side-effect is member call, the index just "--"
	for _, se := range b.SideEffects {
		if se.MemberCallKind == ParameterMemberCall {
			se.MemberCallObjectIndex--
		}
	}
}

// Finish current function builder
func (b *FunctionBuilder) Finish() {
	b.fixupParameterWithThis()

	for _, fun := range b.MarkedFunctions {
		for variable, fv := range fun.FreeValues {
			fv := b.GetValueById(fv)
			name := variable.GetName()
			param, ok := ToParameter(fv)
			if ok {
				if param.GetDefault() != nil {
					continue
				}
			} else {
				log.Warnf("free value %s is not a parameter", name)
				continue
			}

			if b.PeekValue(name) == nil {
				fv.NewError(Error, SSATAG, ValueUndefined(name))
			}
		}
	}

	// set program offsetMap
	// skip const
	// skip no variable value
	// skip return
	for _, blockRaw := range b.Blocks {
		block := b.GetInstructionById(blockRaw)
		if block == nil {
			log.Warnf("function %s has a non-block instruction: %v", b.Function.GetName(), blockRaw)
			continue
		}

		basicBlock, ok := ToBasicBlock(block)
		if !ok {
			log.Warnf("function %s has a non-block instruction: %s", b.Function.GetName(), block.GetName())
			continue
		}

		for _, inst := range basicBlock.Insts {
			value := b.GetValueById(inst)
			if value == nil {
				continue
			}
			if _, ok := ToConstInst(value); ok {
				continue
			}
			if value.GetOpcode() == SSAOpcodeReturn {
				continue
			}

			if len(value.GetAllVariables()) == 0 {
				b.GetProgram().SetOffsetValue(value, value.GetRange())
			}
		}
	}

	// function finish
	b.Function.Finish()
}

// calculate all return instruction in function, get return type
func handlerReturnType(rs []*Return, functionType *FunctionType) Type {
	tmp := make(map[Type]struct{}, len(rs))
	for _, r := range rs {
		typs := r.calcType()

		if _, ok := tmp[typs]; !ok {
			tmp[typs] = struct{}{}
		}
		var opcode = []Opcode{SSAOpcodeParameter, SSAOpcodeFreeValue, SSAOpcodeParameterMember, SSAOpcodeSideEffect}
		for _, result := range r.Results {
			if result <= 0 {
				continue
			}
			result := r.GetValueById(result)
			if !slices.Contains(opcode, result.GetOpcode()) {
				if utils.IsNil(result.GetType()) {
					log.Errorf("[BUG]: result type is null,check it: %v  name: %s", result.GetOpcode(), result.GetVerboseName())
					continue
				}
				if result.GetType().GetTypeKind() == ClassBluePrintTypeKind {
					for key, value := range result.GetAllMember() {
						variable := value.GetLastVariable()
						functionType.SideEffects = append(functionType.SideEffects, &FunctionSideEffect{
							Name:        variable.GetName(),
							VerboseName: getMemberVerboseName(result, key),
							Variable:    variable,
							Modify:      value.GetId(),
							parameterMemberInner: &parameterMemberInner{
								MemberCallKind: CallMemberCall,
								MemberCallKey:  key.GetId(),
							},
						})
					}
				}
			}
		}
	}

	typs := lo.Keys(tmp)
	if len(typs) == 0 {
		return BasicTypes[NullTypeKind]
	} else if len(typs) == 1 {
		return typs[0]
	} else {
		// TODO: how handler this? multiple return with different type
		// should set Warn!!
		// and ?? Type ??
		return GetAnyType()
	}
}

// Finish the function, set FunctionType, set EnterBlock/ExitBlock
func (f *Function) Finish() {
	f.EnterBlock = f.Blocks[0]
	f.ExitBlock = f.Blocks[len(f.Blocks)-1]

	if f.DeferBlock != 0 {
		block := f.GetInstructionById(f.DeferBlock)
		if block != nil {
			if basicBlock, ok := ToBasicBlock(block); ok {
				addToBlocks(basicBlock)
			}
		}
	}

	if f.Type == nil {
		f.Type = NewFunctionType("", nil, nil, false)
	}
	funType := f.Type

	funType.Parameter = lo.Map(f.Params, func(id int64, _ int) Type {
		p := f.GetValueById(id)
		t := p.GetType()
		return t
	})
	funType.ReturnType = handlerReturnType(lo.FilterMap(f.Return, func(i int64, _ int) (*Return, bool) {
		inst := f.GetValueById(i)
		return ToReturn(inst)
	}), funType)
	funType.IsVariadic = f.hasEllipsis
	funType.This = f
	funType.ParameterLen = f.ParamLength
	funType.ParameterValue = lo.FilterMap(f.Params, func(i int64, _ int) (*Parameter, bool) {
		inst := f.GetValueById(i)
		return ToParameter(inst)
	})
	funType.ParameterMember = lo.FilterMap(f.ParameterMembers, func(i int64, _ int) (*ParameterMember, bool) {
		inst := f.GetValueById(i)
		return ToParameterMember(inst)
	})
	result := make(map[*Variable]*Parameter)
	for n, p := range f.FreeValues {
		p := f.GetValueById(p)
		if param, ok := ToParameter(p); ok {
			result[n] = param
		} else {
			log.Warnf("free value %s is not a parameter", n)
		}
	}
	funType.SetFreeValue(result)
	f.builder.SetReturnSideEffects()
	ses := funType.SideEffects
	tmpSideEffects := make(map[*Variable]*FunctionSideEffect)

	for _, seReturn := range f.SideEffectsReturn {
		for v, se := range seReturn {
			tmpSideEffects[v] = se
		}
	}

	for variable, tse := range tmpSideEffects {
		vs := []Value{}
		for _, ses := range f.SideEffectsReturn {
			if value, ok := ses[variable]; ok {
				modifyValue := f.GetValueById(value.Modify)
				if modifyValue != nil {
					vs = append(vs, modifyValue)
				}
			}
		}
		if len(vs) > 1 {
			phi := f.builder.EmitPhi(variable.GetName(), vs)
			if phi != nil {
				tse.Modify = phi.GetId()
			}
		}
	}

	for _, se := range tmpSideEffects {
		modifyValue := f.GetValueById(se.Modify)
		if modifyValue != nil && modifyValue.GetBlock() != nil {
			scope := modifyValue.GetBlock().ScopeTable
			if ret := GetFristLocalVariableFromScopeAndParent(scope, se.Name); ret != nil {
				if ret.GetLocal() {
					continue
				}
			}
		}

		ses = append(ses, se)
	}
	funType.SetSideEffect(ses)
	f.SetType(funType)
}
