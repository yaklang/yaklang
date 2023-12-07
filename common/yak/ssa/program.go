package ssa

import (
	"sync"
)

func NewProgram() *Program {
	prog := &Program{
		Packages:  make(map[string]*Package),
		buildOnce: sync.Once{},
	}
	return prog
}

// create or get main function builder
func (prog *Program) GetAndCreateMainFunctionBuilder() *FunctionBuilder {
	pkg := prog.GetPackage("main")
	if pkg == nil {
		pkg = NewPackage("main")
		prog.AddPackage(pkg)
	}
	fun := pkg.GetFunction("main")
	if fun == nil {
		fun = pkg.NewFunction("main")
	}
	builder := fun.builder
	if builder == nil {
		builder = NewBuilder(fun, nil)
	}
	return builder
}

func (prog *Program) AddPackage(pkg *Package) {
	pkg.Prog = prog
	prog.Packages[pkg.Name] = pkg
}
func (prog *Program) GetPackage(name string) *Package {
	if p, ok := prog.Packages[name]; ok {
		return p
	} else {
		return nil
	}
}

func (prog *Program) EachFunction(handler func(*Function)) {
	var handFunc func(*Function)
	handFunc = func(f *Function) {
		handler(f)
		for _, s := range f.AnonFuncs {
			handFunc(s)
		}
	}

	for _, pkg := range prog.Packages {
		for _, f := range pkg.Funcs {
			handFunc(f)
		}
	}

}

func NewPackage(name string) *Package {
	pkg := &Package{
		Name:  name,
		Funcs: make(map[string]*Function, 0),
	}
	return pkg
}

func (pkg *Package) GetFunction(name string) *Function {
	if f, ok := pkg.Funcs[name]; ok {
		return f
	} else {
		return nil
	}
}
