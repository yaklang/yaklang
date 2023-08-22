package ssa

import (
	"sync"
)

func NewProgram() *Program {
	prog := &Program{
		Packages:   make([]*Package, 0),
		buildOnece: sync.Once{},
	}
	return prog
}

func (prog *Program) NewPackage(name string) *Package {
	pkg := &Package{
		Name:  name,
		Prog:  prog,
		Funcs: make([]*Function, 0),
	}
	prog.Packages = append(prog.Packages, pkg)
	return pkg
}
