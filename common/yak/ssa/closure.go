package ssa

import (
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type SideEffectKind string

const (
	NormalSideEffect  SideEffectKind = "NormalSideEffect"
	PointerSideEffect SideEffectKind = "PointerSideEffect"
)

func SwitchSideEffectKind(kind string) SideEffectKind {
	switch kind {
	case string(NormalSideEffect):
		return NormalSideEffect
	case string(PointerSideEffect):
		return PointerSideEffect
	}
	return NormalSideEffect
}

func (s *FunctionType) SetFreeValue(fv map[*Variable]*Parameter) {
	s.FreeValue = lo.MapToSlice(fv, func(name *Variable, para *Parameter) *Parameter { return para })
}

// FunctionSideEffect is a side-effect in a closure
type FunctionSideEffect struct {
	Name        string
	VerboseName string
	Modify      int64
	// only call-side Scope > this Scope-level, this side-effect can be create
	// Scope *Scope
	Variable *Variable

	forceCreate bool
	Kind        SideEffectKind

	*parameterMemberInner
}

func (f *Function) AddForceSideEffect(variable *Variable, value Value, index int, kind SideEffectKind) {
	if variable.IsMemberCall() {
		para, ok := ToParameter(variable.object)
		if !ok {
			return
		}
		f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
			Name:                 variable.GetName(),
			VerboseName:          getMemberVerboseName(variable.object, variable.key),
			Modify:               value.GetId(),
			Variable:             variable,
			forceCreate:          false,
			Kind:                 kind,
			parameterMemberInner: newParameterMember(para, variable.key),
		})
	} else {
		f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
			Name:        variable.GetName(),
			VerboseName: variable.GetName(),
			Modify:      value.GetId(),
			Variable:    variable,
			forceCreate: true,
			Kind:        kind,
			parameterMemberInner: &parameterMemberInner{
				MemberCallKind:        ParameterCall,
				MemberCallObjectIndex: index,
			},
		})
	}
}

func (f *Function) AddSideEffect(variable *Variable, v Value) {
	var bind *Variable = variable

	for p := f.builder.parentBuilder; p != nil; p = p.builder.parentBuilder {
		parentScope := p.CurrentBlock.ScopeTable
		if find := ReadVariableFromScopeAndParent(parentScope, variable.GetName()); find != nil {
			bind = find
			break
		}
	}

	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:        variable.GetName(),
		VerboseName: variable.GetName(),
		Modify:      v.GetId(),
		Variable:    bind,
		Kind:        NormalSideEffect,
		parameterMemberInner: &parameterMemberInner{
			MemberCallKind: NoMemberCall,
		},
	})
}

func (f *FunctionBuilder) CheckMemberSideEffect(variable *Variable, v Value) {
	var bind *Variable = variable

	for p := f.builder.parentBuilder; p != nil; p = p.builder.parentBuilder {
		parentScope := p.CurrentBlock.ScopeTable
		if find := ReadVariableFromScopeAndParent(parentScope, variable.GetName()); find != nil {
			bind = find
			break
		} else if obj := variable.object; obj != nil {
			if find := ReadVariableFromScopeAndParent(parentScope, obj.GetName()); find != nil {
				bind = find
				break
			}
		}
	}

	if variable.IsMemberCall() {
		// if name is member call, it's modify parameter field
		para, ok := ToParameter(variable.object)
		if !ok {
			return
		}

		sideEffect := &FunctionSideEffect{
			Name:                 variable.GetName(),
			VerboseName:          getMemberVerboseName(variable.object, variable.key),
			Modify:               v.GetId(),
			Variable:             bind,
			forceCreate:          false,
			Kind:                 NormalSideEffect,
			parameterMemberInner: newParameterMember(para, variable.key),
		}
		f.SideEffects = append(f.SideEffects, sideEffect)

		if f.MarkedThisObject != nil &&
			para.GetDefault() != nil &&
			f.MarkedThisObject.GetId() == para.GetDefault().GetId() {
			f.SetMethod(true, para.GetType())
		}
	}
}

func (s *FunctionType) SetSideEffect(se []*FunctionSideEffect) {
	s.SideEffects = se
}

func handleSideEffect(c *Call, funcTyp *FunctionType, buildPointer bool) {
	currentScope := c.GetBlock().ScopeTable
	function := c.GetFunc()
	builder := function.builder

	for _, se := range funcTyp.SideEffects {
		if se.Kind == NormalSideEffect && buildPointer {
			continue
		}

		modify, ok := c.GetValueById(se.Modify)
		if !ok || modify == nil {
			continue
		}
		if p, ok := ToParameter(modify); ok && !p.IsFreeValue {
			id := c.Args[p.FormalParameterIndex]
			if id > 0 && se.Kind == PointerSideEffect {
				if v, ok := c.GetValueById(id); ok {
					modify = v
				}
			}
		}

		var variable *Variable
		modifyScope := modify.GetBlock().ScopeTable

		switch se.MemberCallKind {
		case NoMemberCall:
			if ret := GetFristLocalVariableFromScopeAndParent(currentScope, se.Name); ret != nil {
				if modifyScope.IsSameOrSubScope(ret.GetScope()) {
					continue
				}
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterCall:
			val, exists := se.Get(c)
			if !exists || utils.IsNil(val) {
				continue
			}
			if val.GetType().GetTypeKind() == PointerKind {
				se.Name = builder.GetOriginPointerName(val)
			} else {
				if val.GetName() != "" {
					se.Name = val.GetName()
				} else {
					se.Name = val.GetLastVariable().GetName()
				}
			}
			if v := currentScope.ReadVariable(se.Name); v != nil {
				se.Variable = v.(*Variable)
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterMemberCall:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			if obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				variable = builder.CreateMemberCallVariable(obj, key)
			} else {
				continue
			}
		case CallMemberCall:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			if obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				variable = builder.CreateMemberCallVariable(obj, key)
			} else {
				continue
			}
		default:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			if obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				variable = builder.CreateMemberCallVariable(obj, key)
			} else {
				continue
			}
		}

		if sideEffect := builder.EmitSideEffect(se.Name, c, modify); sideEffect != nil {
			// TODO: handle side effect in loop scope,
			// will replace value in scope and create new phi
			sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
			if v := ReadVariableFromScopeAndParent(currentScope, se.Name); v != nil {
				variable.SetCaptured(v)
			}

			builder.AssignVariable(variable, sideEffect)
			if strings.Contains(se.VerboseName, "this") {
				sideEffect.SetVerboseName(se.VerboseName)
			}
			c.SideEffectValue[se.VerboseName] = sideEffect.GetId()
		}
	}
}

func handleSideEffectBind(c *Call, funcTyp *FunctionType) {
	currentScope := c.GetBlock().ScopeTable
	function := c.GetFunc()
	builder := function.builder

	for _, se := range funcTyp.SideEffects {
		if se.Kind == PointerSideEffect {
			continue
		}

		modify, ok := c.GetValueById(se.Modify)
		if !ok || modify == nil {
			continue
		}
		var variable, bindVariable *Variable
		var bindScope, modifyScope ScopeIF
		if se.Variable != nil {
			bindScope = se.Variable.GetScope()
			bindVariable = se.Variable
		} else {
			bindScope = currentScope
		}
		modifyScope = modify.GetBlock().ScopeTable
		_ = modifyScope
		_ = bindScope

		// is object
		switch se.MemberCallKind {
		case NoMemberCall:
			if ret := GetFristLocalVariableFromScopeAndParent(currentScope, se.Name); ret != nil {
				if modifyScope.IsSameOrSubScope(ret.GetScope()) {
					continue
				}
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterCall:
			val, exists := se.Get(c)
			if !exists || utils.IsNil(val) {
				continue
			}
			if val.GetType().GetTypeKind() == PointerKind {
				se.Name = builder.GetOriginPointerName(val)
			} else {
				if val.GetName() != "" {
					se.Name = val.GetName()
				} else {
					se.Name = val.GetLastVariable().GetName()
				}
			}
			if v := currentScope.ReadVariable(se.Name); v != nil {
				se.Variable = v.(*Variable)
			}
			variable = builder.CreateVariableForce(se.Name)
		case ParameterMemberCall:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			if obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				variable = builder.CreateMemberCallVariable(obj, key)
			} else {
				continue
			}
		case CallMemberCall:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			if obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				variable = builder.CreateMemberCallVariable(obj, key)
			} else {
				continue
			}
			if p, ok := ToParameter(modify); ok {
				if len(c.Args) > p.FormalParameterIndex {
					if arg, ok := c.GetValueById(c.Args[p.FormalParameterIndex]); ok && arg != nil {
						Point(modify, arg)
					}
				}
			}
		default:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			if obj.GetType().GetTypeKind() == PointerKind {
				obj = builder.GetOriginValue(obj)
			}
			// is object
			if key, ok := c.GetValueById(se.MemberCallKey); ok && key != nil {
				variable = builder.CreateMemberCallVariable(obj, key)
			} else {
				continue
			}
		}

		if sideEffect := builder.EmitSideEffect(se.Name, c, modify); sideEffect != nil {
			if builder.SupportClosure {
				if parentValue, ok := builder.getParentFunctionVariable(se.Name); ok && se.Variable != nil {
					// the ret variable should be FreeValue
					para := builder.BuildFreeValueByVariable(se.Variable)
					para.SetDefault(parentValue)
					para.SetType(parentValue.GetType())
					parentValue.AddOccultation(para)
				}
			}

			AddSideEffect := func() {
				// TODO: handle side effect in loop scope,
				// will replace value in scope and create new phi
				sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
				if se.Variable != nil {
					variable.SetCaptured(se.Variable)
				}
				builder.AssignVariable(variable, sideEffect)
				sideEffect.SetVerboseName(se.VerboseName)
				c.SideEffectValue[se.VerboseName] = sideEffect.GetId()
			}

			SetCapturedSideEffect := func() {
				err := variable.Assign(sideEffect)
				if err != nil {
					log.Warnf("BUG: variable.Assign error: %v", err)
					return
				}
				if strings.Contains(se.VerboseName, "this") {
					sideEffect.SetVerboseName(se.VerboseName)
				}
				currentScope.SetCapturedSideEffect(se.VerboseName, variable, se.Variable)

				function.SideEffects = append(function.SideEffects, se)
			}

			CheckSideEffect := func(find *Variable) {
				// Check := func(scope ScopeIF) {
				// 	if bindScope.IsSameOrSubScope(scope) {
				// 		AddSideEffect()
				// 	} else {
				// 		SetCapturedSideEffect()
				// 	}
				// }

				if bindVariable == nil || find.GetCaptured() == bindVariable.GetCaptured() {
					AddSideEffect()
				} else {
					SetCapturedSideEffect()
				}
			}

			var GetScope func(ScopeIF, string, *FunctionBuilder) *Variable
			GetScope = func(scope ScopeIF, name string, builder *FunctionBuilder) *Variable {
				var ret *Variable
				if vairable := GetFristLocalVariableFromScopeAndParent(scope, name); vairable != nil {
					ret = vairable
				} else if vairable := GetFristVariableFromScopeAndParent(scope, name); vairable != nil {
					ret = vairable
				}
				if ret == nil {
					return nil
				}
				if _, ok := ToParameter(ret.GetValue()); ok {
					parentBuilder := builder.parentBuilder
					if parentBuilder != nil {
						parentScope := parentBuilder.CurrentBlock.ScopeTable
						return GetScope(parentScope, name, parentBuilder)
					}
				}

				return ret
			}

			if _, ok := modify.(*Parameter); ok {
				AddSideEffect()
				continue
			}

			obj := se.parameterMemberInner
			if ret := GetScope(currentScope, se.Name, builder); ret != nil {
				CheckSideEffect(ret)
				continue
			} else if ret := GetScope(currentScope, obj.ObjectName, builder); ret != nil {
				CheckSideEffect(ret)
				continue
			} else if obj.ObjectName == "this" {
				AddSideEffect()
				continue
			}

			if obj.MemberCallKind == ParameterMemberCall || obj.MemberCallKind == CallMemberCall {
				AddSideEffect()
				continue
			}

			// 处理跨闭包的side-effect
			if block := function.GetBlock(); block != nil {
				functionScope := block.ScopeTable
				if ret := GetScope(functionScope, se.Name, builder); ret != nil {
					CheckSideEffect(ret)
					continue
				} else if obj := se.parameterMemberInner; obj.ObjectName != "" { // 处理object
					if ret := GetScope(functionScope, obj.ObjectName, builder); ret != nil {
						CheckSideEffect(ret)
						continue
					} else {
						AddSideEffect()
						continue
					}
				}
			}
		}
	}
}

func (f *FunctionBuilder) SwitchFreevalueInSideEffectFromScope(name string, se *SideEffect, scope ScopeIF) *SideEffect {
	vs := make(Values, 0)
	if scope == nil {
		return se
	}
	if seValue, ok := f.GetValueById(se.Value); ok && seValue != nil {
		if phi, ok := ToPhi(seValue); ok {
			for i, id := range phi.Edge {
				edgeValue, ok := f.GetValueById(id)
				if !ok || edgeValue == nil {
					continue
				}
				vs = append(vs, edgeValue)
				if p, ok := ToParameter(edgeValue); ok && p.IsFreeValue {
					if value := scope.ReadValue(name); value != nil {
						vs[i] = value
					}
				}
			}
			phit := &Phi{
				anValue:            phi.anValue,
				CFGEntryBasicBlock: phi.CFGEntryBasicBlock,
				Edge:               vs.GetIds(),
			}

			if callSite, ok := f.GetValueById(se.CallSite); ok && callSite != nil {
				if call, ok := callSite.(*Call); ok {
					sideEffect := f.EmitSideEffect(name, call, phit)
					return sideEffect
				}
			}
		}
	}
	return se
}
