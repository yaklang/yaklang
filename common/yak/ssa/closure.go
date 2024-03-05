package ssa

// FunctionFreeValue is a free-value in a closure
type FunctionFreeValue struct {
	Name     string
	Variable *Variable

	HasDefault bool // this is mark is capture value
	Default    Value
}

func (s *FunctionType) SetFreeValue(fv map[string]*Parameter) {
	s.FreeValue = make([]*FunctionFreeValue, 0, len(fv))
	for name, p := range fv {
		v := &FunctionFreeValue{
			Name: name,
		}

		if variable := p.GetVariable(name); variable != nil {
			v.Variable = variable
		}
		if p.GetDefault() != nil {
			v.HasDefault = true
			v.Default = p.GetDefault()
		}
		s.FreeValue = append(s.FreeValue, v)
	}
}

// FunctionSideEffect is a side-effect in a closure
type FunctionSideEffect struct {
	Name   string
	Modify Value
	// only call-side Scope > this Scope-level, this side-effect can be create
	// Scope *Scope
	Variable *Variable

	// is modify parameter field
	IsMemberCall bool
	Parameter    int
	Key          Value
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
				Modify:       v,
				Name:         variable.GetName(),
				Variable:     variable,
				IsMemberCall: true,
				Parameter:    index,
				Key:          variable.key,
			})
		}
	}
}

func (s *FunctionType) SetSideEffect(se []*FunctionSideEffect) {
	s.SideEffects = se
}
