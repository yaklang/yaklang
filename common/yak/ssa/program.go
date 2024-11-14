package ssa

import (
	"github.com/samber/lo"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func NewProgram(ProgramName string, enableDatabase bool, kind ProgramKind, fs fi.FileSystem, programPath string) *Program {
	prog := &Program{
		Name:                    ProgramName,
		ProgramKind:             kind,
		LibraryFile:             make(map[string][]string),
		UpStream:                make(map[string]*Program),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		Cache:                   NewDBCache(ProgramName, enableDatabase),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   make(map[string]*Function),
		ClassBluePrint:          omap.NewEmptyOrderedMap[string, *Blueprint](),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		editorMap:               omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		FileList:                make(map[string]string),
		cacheExternInstance:     make(map[string]Value),
		externType:              make(map[string]Type),
		externBuildValueHandler: make(map[string]func(b *FunctionBuilder, id string, v any) (value Value)),
		ExternInstance:          make(map[string]any),
		ExternLib:               make(map[string]map[string]any),
		importDeclares:          omap.NewOrderedMap(make(map[string]*importDeclareItem)),
	}
	if kind == Application {
		prog.Application = prog
	}
	prog.EnableDatabase = enableDatabase
	prog.Loader = ssautil.NewPackageLoader(
		ssautil.WithFileSystem(fs),
		ssautil.WithIncludePath(programPath),
		ssautil.WithBasePath(programPath),
	)
	return prog
}
func (prog *Program) createSubProgram(name string, kind ProgramKind, path ...string) *Program {
	fs := prog.Loader.GetFilesysFileSystem()
	fullPath := prog.GetCurrentEditor().GetFilename()
	endPath := fs.Join(path...)
	programPath, _, _ := strings.Cut(fullPath, endPath)
	subProg := NewProgram(name, prog.EnableDatabase, kind, fs, programPath)
	subProg.Application = prog.Application

	subProg.Loader.AddIncludePath(prog.Loader.GetIncludeFiles()...)
	subProg.Language = prog.Language

	subProg.LibraryFile = prog.LibraryFile
	subProg.FileList = prog.FileList
	subProg.editorStack = prog.editorStack.Copy()
	// subProg.editorStack = prog.editorStack
	subProg.externType = prog.externType
	subProg.externBuildValueHandler = prog.externBuildValueHandler
	subProg.ExternInstance = prog.ExternInstance
	subProg.ExternLib = prog.ExternLib
	subProg.VirtualImport = prog.VirtualImport
	subProg.ExportType = make(map[string]Type)
	subProg.ExportValue = make(map[string]Value)

	//todo: 这里需要加一个测试
	subProg.GlobalScope = prog.GlobalScope

	// up-down stream and application
	prog.AddUpStream(subProg)
	prog.Application.AddUpStream(subProg)
	subProg.Application = prog.Application
	subProg.Cache = prog.Cache
	return subProg
}

func (prog *Program) GetSubProgram(name string, path ...string) *Program {
	child, ok := prog.UpStream[name]
	if !ok {
		child = prog.createSubProgram(name, Library, path...)
	}
	return child
}

func (prog *Program) NewLibrary(name string, path []string) *Program {
	return prog.createSubProgram(name, Library, path...)
}

func (prog *Program) GetLibrary(name string, virtualImport ...bool) (*Program, bool) {
	if prog == nil || utils.IsNil(prog) || prog.Application == nil || utils.IsNil(prog.Application) {
		return nil, false
	}
	// get lib from application
	app := prog.Application
	currentEditor := prog.GetCurrentEditor()
	// this program has current file
	hasFile := func(p *Program) bool {
		if hash, ok := p.FileList[currentEditor.GetFilename()]; ok {
			if hash == currentEditor.SourceCodeMd5() {
				return true
			}
		}
		return false
	}

	// contain in memory
	if p, ok := app.UpStream[name]; ok {
		return p, hasFile(p)
	}

	if p, ok := prog.UpStream[name]; ok {
		app.AddUpStream(p)
		return p, hasFile(p)
	}
	if len(virtualImport) > 0 && virtualImport[0] {
		lib, err := prog.GenerateVirtualLib(name)
		if err != nil {
			log.Warnf("generate virtual lib fail: %s", err)
			return nil, false
		}
		return lib, true
	}
	if !app.EnableDatabase {
		return nil, false
	}
	version := ""
	if p := app.GetSCAPackageByName(name); p != nil {
		version = p.Version
	} else {
		return nil, false
	}
	// library in  database, load and set relation
	p, err := GetLibrary(name, version)
	if err != nil {
		return nil, false
	}
	app.AddUpStream(p)
	if !slices.Contains(p.irProgram.UpStream, name) {
		// update up-down stream
		prog.AddUpStream(p)
	}
	return p, hasFile(p)
}

func (prog *Program) AddUpStream(sub *Program) {
	prog.UpStream[sub.Name] = sub
	sub.DownStream[prog.Name] = prog
}

func (prog *Program) GetProgramName() string {
	return prog.Name
}

func (prog *Program) GetAndCreateFunction(pkgName string, funcName string) *Function {
	fun := prog.GetFunction(funcName, pkgName)
	if fun == nil {
		fun = prog.NewFunction(funcName)
	}
	return fun
}

func (prog *Program) GetCacheExternInstance(name string) (Value, bool) {
	v, ok := prog.cacheExternInstance[name]
	return v, ok
}

func (prog *Program) SetCacheExternInstance(name string, v Value) {
	prog.cacheExternInstance[name] = v
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

func (prog *Program) EachFunction(handler func(*Function)) {
	var handFunc func(*Function)
	handFunc = func(f *Function) {
		handler(f)
		for _, s := range f.ChildFuncs {
			f, ok := ToFunction(s)
			if !ok {
				log.Warnf("function %s is not a ssa.Function", s.GetName())
				continue
			}
			handFunc(f)
		}
	}

	for _, f := range prog.Funcs {
		handFunc(f)
	}
	for _, up := range prog.UpStream {
		for _, f := range up.Funcs {
			handFunc(f)
		}
	}
}

func (prog *Program) Finish() {
	if prog.ProgramKind == Application && prog.EnableDatabase {
		prog.Cache.SaveToDatabase()
		updateToDatabase(prog)
	}
}
func (p *Program) LazyBuild() {
	buildLazyFunction := func(program *Program) {
		for k := 0; k < program.ClassBluePrint.Len(); k++ {
			if blueprint, exits := program.ClassBluePrint.GetByIndex(k); exits {
				blueprint.Build()
			}
		}
		for _, function := range program.Funcs {
			function.Build()
		}
		for k := 0; k < program.ClassBluePrint.Len(); k++ {
			if blueprint, exits := program.ClassBluePrint.GetByIndex(k); exits {
				blueprint.BuildConstructorAndDestructor()
			}
		}
	}
	buildMoreProg := func(prog ...*Program) {
		for _, program := range prog {
			buildLazyFunction(program)
		}
	}
	buildMoreProg(append(append(lo.Values(p.UpStream), lo.Values(p.DownStream)...), p)...)
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

func (prog *Program) GetFrontValueByOffset(searchOffset int) (offset int, value Value) {
	index, offset := prog.SearchIndexAndOffsetByOffset(searchOffset)
	// 如果二分查找的结果是大于目标值的，那么就需要回退一个
	if offset > searchOffset {
		if index > 0 {
			index -= 1
		}
		offset = prog.OffsetSortedSlice[index]
	}
	if item, ok := prog.OffsetMap[offset]; ok {
		value = item.GetValue()
	}
	return offset, value
}

func (p *Program) GetEditor(url string) (*memedit.MemEditor, bool) {
	return p.editorMap.Get(url)
}

func (p *Program) PushEditor(e *memedit.MemEditor) {
	p.editorStack.Push(e)
	if !p.PreHandler() {
		p.editorMap.Set(p.GetCurrentEditor().GetFilename(), p.GetCurrentEditor())
	}
}

func (p *Program) GetIncludeFiles() []string {
	return p.editorMap.Keys()
}
func (p *Program) GetIncludeFileNum() int {
	return p.editorMap.Len()
}

func (p *Program) GetCurrentEditor() *memedit.MemEditor {
	if p.editorStack == nil || p.editorStack.Len() <= 0 {
		return nil
	}
	_, v, ok := p.editorStack.Last()
	if !ok {
		return nil
	}
	return v
}

func (p *Program) PopEditor(save bool) {
	if p.editorStack == nil || p.editorStack.Len() <= 0 {
		return
	}
	e := p.editorStack.Pop()
	if save && e != nil {
		p.FileList[e.GetFilename()] = e.SourceCodeMd5()
	}
}

func (p *Program) GetSCAPackageByName(name string) *dxtypes.Package {
	if p == nil {
		return nil
	}
	for _, pkg := range p.SCAPackages {
		if strings.Contains(pkg.Name, name) {
			return pkg
		}
	}
	return nil
}

func (p *Program) GetApplication() *Program {
	if p == nil {
		return nil
	}
	return p.Application
}
