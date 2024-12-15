package ssa

import "github.com/samber/lo"

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
	Variable     *Variable
	BindVariable *Variable

	forceCreate bool

	*parameterMemberInner
}

func (f *Function) AddForceSideEffect(name string, v Value) {
	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:        name,
		Modify:      v,
		forceCreate: true,
		parameterMemberInner: &parameterMemberInner{
			MemberCallKind: NoMemberCall,
		},
	})
}

func (f *Function) AddSideEffect(variable *Variable, v Value) {
	var bind *Variable

	for p := f.builder.parentBuilder; p != nil; p = p.builder.parentBuilder {
		parentScope := p.CurrentBlock.ScopeTable
		if find := ReadVariableFromScopeAndParent(parentScope, variable.GetName()); find != nil {
			bind = find
			break
		}
	}
	if bind == nil {
		bind = variable
	}

	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:         variable.GetName(),
		VerboseName:  variable.GetName(),
		Modify:       v,
		Variable:     variable,
		BindVariable: bind,
		parameterMemberInner: &parameterMemberInner{
			MemberCallKind: NoMemberCall,
		},
	})
}

func (f *FunctionBuilder) CheckAndSetSideEffect(variable *Variable, v Value) {
	var bind *Variable

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
	if bind == nil {
		bind = variable
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
			Modify:               v,
			Variable:             variable,
			BindVariable:         bind,
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
