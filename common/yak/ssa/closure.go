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

	// is modify parameter field
	IsMemberCall   bool
	ParameterIndex int
	Key            Value
}

func (f *Function) AddSideEffect(name *Variable, v Value) {
	f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
		Name:     name.GetName(),
		Modify:   v,
		Variable: name,
	})
}

func (f *Function) CheckAndSetSideEffect(variable *Variable, v Value) {
	if variable.IsMemberCall() {
		// if name is member call, it's modify parameter field
		if index, ok := f.paramMap[variable.object]; ok {
			f.SideEffects = append(f.SideEffects, &FunctionSideEffect{
				Modify:         v,
				Name:           variable.GetName(),
				VerboseName:    getMemberVerboseName(variable.object, variable.key),
				Variable:       variable,
				IsMemberCall:   true,
				ParameterIndex: index,
				Key:            variable.key,
			})
		}
	}
}

func (s *FunctionType) SetSideEffect(se []*FunctionSideEffect) {
	s.SideEffects = se
}
