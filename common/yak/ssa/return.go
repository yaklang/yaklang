package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
		return handleType(r.Results[0])
	default:
		newObjTyp := NewObjectType()
		for i, v := range r.Results {
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
	para := b.Params[0]
	if para == nil || para.IsObject() || para.HasUsers() {
		return
	}

	// remove from param
	b.Params = utils.RemoveSliceItem(b.Params, para)
	// fix other field in function
	b.ParamLength--
	// fix other parameter index
	for i, p := range b.Params {
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
		for name, fv := range fun.FreeValues {
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

	// set defer function
	if deferLen := len(b.deferExpr); deferLen > 0 {
		endBlock := b.CurrentBlock

		deferBlock := b.GetDeferBlock()
		b.CurrentBlock = deferBlock
		for _, i := range b.deferExpr {
			b.EmitCall(i)
		}
		b.deferExpr = []*Call{}

		b.CurrentBlock = endBlock
	}

	// set program offsetMap
	// skip const
	// skip no variable value
	// skip return
	for _, blockRaw := range b.Blocks {
		block, ok := ToBasicBlock(blockRaw)
		if !ok {
			log.Warnf("function %s has a non-block instruction: %s", b.Function.GetName(), blockRaw.GetName())
			continue
		}

		for _, inst := range block.Insts {
			value, ok := ToValue(inst)
			if !ok {
				continue
			}
			if _, ok := ToConst(value); ok {
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
func handlerReturnType(rs []*Return) Type {
	tmp := make(map[Type]struct{}, len(rs))
	for _, r := range rs {
		typs := r.calcType()

		if _, ok := tmp[typs]; !ok {
			tmp[typs] = struct{}{}
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

	if f.Type == nil {
		f.Type = NewFunctionType("", nil, nil, false)
	}
	funType := f.Type

	funType.Parameter = lo.Map(f.Params, func(p Value, _ int) Type {
		t := p.GetType()
		return t
	})
	funType.ReturnType = handlerReturnType(lo.FilterMap(f.Return, func(i Value, _ int) (*Return, bool) {
		return ToReturn(i)
	}))
	funType.IsVariadic = f.hasEllipsis
	funType.This = f
	funType.ReturnValue = lo.FilterMap(f.Return, func(i Value, _ int) (*Return, bool) {
		return ToReturn(i)
	})
	funType.ParameterLen = f.ParamLength
	funType.ParameterValue = lo.FilterMap(f.Params, func(i Value, _ int) (*Parameter, bool) {
		return ToParameter(i)
	})
	funType.ParameterMember = lo.FilterMap(f.ParameterMembers, func(i Value, _ int) (*ParameterMember, bool) {
		return ToParameterMember(i)
	})
	var result = make(map[string]*Parameter)
	for n, p := range f.FreeValues {
		if param, ok := ToParameter(p); ok {
			result[n] = param
		} else {
			log.Warnf("free value %s is not a parameter", n)
		}
	}
	funType.SetFreeValue(result)
	funType.SetSideEffect(f.SideEffects)
	f.SetType(funType)
}
