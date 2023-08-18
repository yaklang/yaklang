package ssa

func (f *Function) AddAnonymous(anon *Function) {
	f.AnonFuncs = append(f.AnonFuncs, anon)
	anon.parent = f
	anon.symbol.parentI = f.symbol
}

func (f *Function) NewParam(name string) {
	p := &Parameter{
		variable: name,
		Func:     f,
		user:     []User{},
		typs:     make(Types, 0),
	}
	f.Param = append(f.Param, p)
	f.writeVariable(name, p)
}

func (f *Function) ReturnValue() []Value {
	ret := f.ExitBlock.LastInst().(*Return)
	return ret.Results
}
