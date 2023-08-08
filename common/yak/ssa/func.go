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

func (f *Function) NewParam(name string) {
	p := &Parameter{
		variable: name,
		Func:     f,
		user:     []User{},
	}
	f.Param = append(f.Param, p)
	f.writeVariable(name, p)
	}



