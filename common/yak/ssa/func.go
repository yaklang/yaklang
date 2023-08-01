package ssa


func (f *Function) NewParam(name string, add bool) *Parameter {
	p := &Parameter{
		variable: name,
		parent:   f,
		user:     []User{},
	}
	if add {
		f.Param = append(f.Param, p)
	}
	return p
}
