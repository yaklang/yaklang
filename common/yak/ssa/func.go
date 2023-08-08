package ssa

// use in for/switch
type target struct {
	tail      *target // the stack
	_break    *BasicBlock
	_continue *BasicBlock
}

func (f *Function) AddAnonymous(anon *Function) {
	f.AnonFuncs = append(f.AnonFuncs, anon)
	anon.parent = f
}

func (f *Function) NewParam(name string, add bool) *Parameter {
	p := &Parameter{
		variable: name,
		Func:     f,
		user:     []User{},
	}
	if add {
		// f.Param = append(f.Param, p)
		f.Param[name] = p
	}
	return p
}
