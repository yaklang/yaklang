package ssa

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func NewProgram(dbProgramName string) *Program {
	prog := &Program{
		Packages:          make(map[string]*Package),
		errors:            make([]*SSAError, 0),
		ClassBluePrint:    make(map[string]*ClassBluePrint),
		Cache:             NewDBCache(dbProgramName),
		OffsetMap:         make(map[int]*OffsetItem),
		OffsetSortedSlice: make([]int, 0),
		loader:            ssautil.NewPackageLoader(),
	}
	return prog
}

func (prog *Program) GetProgramName() string {
	return prog.Cache.ProgramName
}

func (prog *Program) GetAndCreateFunction(pkgName string, funcName string) *Function {
	pkg := prog.GetPackage(pkgName)
	if pkg == nil {
		pkg = NewPackage(pkgName)
		prog.AddPackage(pkg)
	}
	fun := pkg.GetFunction(funcName)
	if fun == nil {
		fun = pkg.NewFunction(funcName)
	}
	return fun
}

// create or get main function builder
func (prog *Program) GetAndCreateFunctionBuilder(pkgName string, funcName string) *FunctionBuilder {
	fun := prog.GetAndCreateFunction(pkgName, funcName)
	builder := fun.builder
	if builder == nil {
		builder = NewBuilder(prog.GetCurrentEditor(), fun, nil)
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

func (prog *Program) Finish() {
	prog.Cache.SaveToDatabase()
}

func (prog *Program) SearchIndexAndOffsetByOffset(searchOffset int) (index int, offset int) {
	index = sort.Search(len(prog.OffsetSortedSlice), func(i int) bool {
		return prog.OffsetSortedSlice[i] >= searchOffset
	})
	if index >= len(prog.OffsetSortedSlice) && len(prog.OffsetSortedSlice) > 0 {
		index = len(prog.OffsetSortedSlice) - 1
	}
	if len(prog.OffsetSortedSlice) > 0 {
		offset = prog.OffsetSortedSlice[index]
	}
	return
}

func (prog *Program) GetFrontValueByOffset(searchOffset int) (value Value) {
	index, offset := prog.SearchIndexAndOffsetByOffset(searchOffset)
	// 如果二分查找的结果是大于目标值的，那么就需要回退一个
	if offset > searchOffset {
		index -= 1
		offset = prog.OffsetSortedSlice[index]
	}
	if item, ok := prog.OffsetMap[offset]; ok {
		value = item.GetValue()
	}
	return value
}

func (prog *Program) IsPackagePathInList(pkgName string) bool {
	for _, pkgPath := range prog.packagePathList {
		name := strings.Join(pkgPath, ".")
		if name == pkgName {
			return true
		}
	}
	return false
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
