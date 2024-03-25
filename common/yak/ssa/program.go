package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func NewProgram(dbProgramName string) *Program {
	prog := &Program{
		Packages:           make(map[string]*Package),
		ConstInstruction:   omap.NewEmptyOrderedMap[int, *ConstInst](),
		NameToInstructions: omap.NewEmptyOrderedMap[string, []Instruction](),
		IdToInstructionMap: omap.NewEmptyOrderedMap[int, Instruction](),
		errors:             make([]*SSAError, 0),
		buildOnce:          sync.Once{},
		ClassBluePrint:     make(map[string]*ClassBluePrint),
	}
	if dbProgramName != "" {
		prog.persistentBackendMutex = new(sync.Mutex)
		prog.persistentBackend = func() (int, func(Instruction) error) {
			db := consts.GetGormProjectDatabase()
			code, codeIns := ssadb.RequireIrCode(db, dbProgramName)
			return int(code), func(i Instruction) error {
				defer func() {
					db.Save(codeIns)
				}()
				return FitIRCode(codeIns, i)
			}
		}
	}
	return prog
}

func (prog *Program) GetAndCreateMainFunction() *Function {
	pkg := prog.GetPackage("main")
	if pkg == nil {
		pkg = NewPackage("main")
		prog.AddPackage(pkg)
	}
	fun := pkg.GetFunction("main")
	if fun == nil {
		fun = pkg.NewFunction("main")
	}
	return fun
}

// create or get main function builder
func (prog *Program) GetAndCreateMainFunctionBuilder() *FunctionBuilder {
	fun := prog.GetAndCreateMainFunction()
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

func (p *Program) GetFunctionFast(paths ...string) *Function {
	if len(paths) > 1 {
		pkg := p.GetPackage(paths[0])
		if pkg != nil {
			return pkg.GetFunction(paths[1])
		}
	} else if len(paths) == 1 {
		if ret := p.GetPackage("main"); ret != nil {
			return ret.GetFunction(paths[0])
		}
	} else {
		if ret := p.GetPackage("main"); ret != nil {
			return ret.GetFunction("main")
		}
	}
	return nil
}

func (prog *Program) EachFunction(handler func(*Function)) {
	var handFunc func(*Function)
	handFunc = func(f *Function) {
		handler(f)
		for _, s := range f.ChildFuncs {
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
