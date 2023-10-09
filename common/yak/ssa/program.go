package ssa

import (
	"sync"
)

func NewProgram() *Program {
	prog := &Program{
		Packages:  make([]*Package, 0),
		buildOnce: sync.Once{},
	}
	return prog
}

func NewPackage(name string) *Package {
	pkg := &Package{
		Name:  name,
		Funcs: make([]*Function, 0),
	}
	return pkg
}

func (prog *Program) AddPackage(pkg *Package) {
	pkg.Prog = prog
	prog.Packages = append(prog.Packages, pkg)
}
