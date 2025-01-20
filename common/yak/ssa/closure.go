package ssa

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (s *FunctionType) SetFreeValue(fv map[*Variable]*Parameter) {
	s.FreeValue = lo.MapToSlice(fv, func(name *Variable, para *Parameter) *Parameter { return para })
}

// FunctionSideEffect is a side-effect in a closure
type FunctionSideEffect struct {
	Name        string
	VerboseName string
	Modify      Value
	// only call-side Scope > this Scope-level, this side-effect can be create
	// Scope *Scope
	Variable *Variable

	forceCreate bool

	*parameterMemberInner
}

func (f *Function) AddForceSideEffect(name string, v Value, index int) {
	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:        name,
		Modify:      v,
		forceCreate: true,
		parameterMemberInner: &parameterMemberInner{
			MemberCallKind:        ParameterCall,
			MemberCallObjectIndex: index,
		},
	})
}

func (f *Function) AddSideEffect(variable *Variable, v Value) {
	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:        variable.GetName(),
		VerboseName: variable.GetName(),
		Modify:      v,
		Variable:    variable,
		parameterMemberInner: &parameterMemberInner{
			MemberCallKind: NoMemberCall,
		},
	})
}

func (f *FunctionBuilder) CheckAndSetSideEffect(variable *Variable, v Value) {
	if variable.IsMemberCall() {
		// if name is member call, it's modify parameter field
		para, ok := ToParameter(variable.object)
		if !ok {
			return
		}
		// todo: 各个语言的处理不一样，需要统一
		// if _, isPointer := ToPointerType(para.GetType()); !isPointer {
		// 	return
		// }

		sideEffect := &FunctionSideEffect{
			Name:                 variable.GetName(),
			VerboseName:          getMemberVerboseName(variable.object, variable.key),
			Modify:               v,
			Variable:             variable,
			forceCreate:          false,
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

func handleSideEffect(c *Call, funcTyp *FunctionType) {
	currentScope := c.GetBlock().ScopeTable
	function := c.GetFunc()
	builder := function.builder

	for _, se := range funcTyp.SideEffects {
		var variable *Variable
		var modifyScope ScopeIF
		var bindVariable *Variable
		modifyScope = se.Modify.GetBlock().ScopeTable
		_ = modifyScope

		bindVariable = se.Variable

		// is object
		if se.MemberCallKind == NoMemberCall {
			if ret := GetFristLocalVariableFromScopeAndParent(currentScope, se.Name); ret != nil {
				if modifyScope.IsSameOrSubScope(ret.GetScope()) {
					continue
				}
			}
			variable = builder.CreateVariableForce(se.Name)
			if bindVariable != nil {
				variable.SetCaptured(bindVariable)
			}
		} else {
			// is object
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			variable = builder.CreateMemberCallVariable(obj, se.MemberCallKey)
			if bindVariable != nil {
				variable.SetCaptured(bindVariable)
			}
		}

		if sideEffect := builder.EmitSideEffect(se.Name, c, se.Modify); sideEffect != nil {
			// TODO: handle side effect in loop scope,
			// will replace value in scope and create new phi
			sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
			builder.AssignVariable(variable, sideEffect)
			sideEffect.SetVerboseName(se.VerboseName)
			c.SideEffectValue[se.VerboseName] = sideEffect
		}
	}
}

func handleSideEffectBind(c *Call, funcTyp *FunctionType) {
	currentScope := c.GetBlock().ScopeTable
	function := c.GetFunc()
	builder := function.builder

	for _, se := range funcTyp.SideEffects {
		var bindVariable, createVariable *Variable
		var bindScope, modifyScope ScopeIF
		var findName string = se.Name

		if se.Variable != nil {
			bindVariable = se.Variable
			bindScope = bindVariable.GetScope()
			if o := bindVariable.object; o != nil {
				if p, ok := ToParameter(o); ok && p.IsFreeValue {
					if defaul := p.GetDefault(); defaul != nil {
						// 对于member而言default为外部object
						// bindVariable = defaul.GetLastVariable()
						// if vam := defaul.GetVariableMemory(); vam != nil {
						// 	if variable, ok := vam.GetVariableByName(o.GetName()); ok {
						// 		bindVariable = variable.(*Variable)
						// 	}
						// }
						// bindScope = bindVariable.GetScope()

						findName = fmt.Sprintf("#%d.%s", bindVariable.GetId(), se.Variable.key.String())
						if member := bindScope.ReadVariable(findName); member != nil {
							bindVariable = member.(*Variable)
						} else {
							// todo
						}
					}
				}
			}
		} else {
			bindVariable = nil
			bindScope = currentScope
		}
		modifyScope = se.Modify.GetBlock().ScopeTable
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
			createVariable = builder.CreateVariableForce(se.Name)
			if bindVariable != nil {
				createVariable.SetCaptured(bindVariable)
			}
		case ParameterCall:
			val, exists := se.Get(c)
			if !exists || utils.IsNil(val) {
				continue
			}
			//直接找到variable来生成sideEffect
			//modify side-effect name
			if val.GetName() != "" {
				se.Name = val.GetName()
			} else {
				se.Name = val.GetLastVariable().GetName()
			}
			findName = se.Name
			se.Variable = val.GetLastVariable()
			createVariable = builder.CreateVariable(se.Name)
		default:
			obj, ok := se.Get(c)
			if !ok {
				continue
			}
			// is object
			createVariable = builder.CreateMemberCallVariable(obj, se.MemberCallKey)
			if bindVariable != nil {
				createVariable.SetCaptured(bindVariable)
			}
		}

		if sideEffect := builder.EmitSideEffect(se.Name, c, se.Modify); sideEffect != nil {
			if builder.SupportClosure {
				if parentValue, ok := builder.getParentFunctionVariable(se.Name); ok && bindVariable != nil {
					// the ret variable should be FreeValue
					para := builder.BuildFreeValueByVariable(bindVariable)
					para.SetDefault(parentValue)
					para.SetType(parentValue.GetType())
					parentValue.AddOccultation(para)
				}
			}

			AddSideEffect := func() {
				// TODO: handle side effect in loop scope,
				// will replace value in scope and create new phi
				sideEffect = builder.SwitchFreevalueInSideEffect(se.Name, sideEffect)
				builder.AssignVariable(createVariable, sideEffect)
				sideEffect.SetVerboseName(se.VerboseName)
				c.SideEffectValue[se.VerboseName] = sideEffect
			}

			SetCapturedSideEffect := func() {
				err := createVariable.Assign(sideEffect)
				if err != nil {
					log.Warnf("BUG: variable.Assign error: %v", err)
					return
				}
				sideEffect.SetVerboseName(se.VerboseName)
				currentScope.SetCapturedSideEffect(se.VerboseName, createVariable, bindVariable)

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

			if _, ok := se.Modify.(*Parameter); ok {
				AddSideEffect()
				continue
			}

			obj := se.parameterMemberInner
			if ret := GetScope(currentScope, findName, builder); ret != nil {
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
	vs := make([]Value, 0)
	if scope == nil {
		return se
	}
	if phi, ok := ToPhi(se.Value); ok {
		for i, e := range phi.Edge {
			vs = append(vs, e)
			if p, ok := ToParameter(e); ok && p.IsFreeValue {
				if value := scope.ReadValue(name); value != nil {
					vs[i] = value
				}
			}
		}
		phit := &Phi{
			anValue:            phi.anValue,
			CFGEntryBasicBlock: phi.CFGEntryBasicBlock,
			Edge:               vs,
		}

		sideEffect := f.EmitSideEffect(name, se.CallSite.(*Call), phit)
		return sideEffect
	}
	return se
}
