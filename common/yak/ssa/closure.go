package ssa

import "github.com/samber/lo"

func (s *FunctionType) SetFreeValue(fv map[string]*Parameter) {
	s.FreeValue = lo.MapToSlice(fv, func(name string, para *Parameter) *Parameter { return para })
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

	*parameterMember
}

func (f *Function) AddForceSideEffect(name string, v Value) {
	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:        name,
		Modify:      v,
		forceCreate: true,
		parameterMember: &parameterMember{
			MemberCallKind: NoMemberCall,
		},
	})
}
func (f *Function) AddSideEffect(name *Variable, v Value) {
	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:     name.GetName(),
		Modify:   v,
		Variable: name,
		parameterMember: &parameterMember{
			MemberCallKind: NoMemberCall,
		},
	})
}

func (f *Function) CheckAndSetSideEffect(variable *Variable, v Value) {
	if variable.IsMemberCall() {
		// if name is member call, it's modify parameter field
		para, ok := ToParameter(variable.object)
		if !ok {
			return
		}
		sideEffect := &FunctionSideEffect{
			Name:            variable.GetName(),
			VerboseName:     getMemberVerboseName(variable.object, variable.key),
			Modify:          v,
			Variable:        variable,
			forceCreate:     false,
			parameterMember: newParameterMember(para, variable.key),
		}
		f.SideEffects = append(f.SideEffects, sideEffect)
	}
}

func (s *FunctionType) SetSideEffect(se []*FunctionSideEffect) {
	s.SideEffects = se
}
