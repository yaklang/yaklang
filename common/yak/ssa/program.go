package ssa

import (
	"sort"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/exp/slices"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func NewProgram(ProgramName string, enableDatabase bool, kind ProgramKind, fs filesys.FileSystem, programPath string) *Program {
	prog := &Program{
		Name:                    ProgramName,
		ProgramKind:             kind,
		UpStream:                make(map[string]*Program),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		Cache:                   NewDBCache(ProgramName, enableDatabase),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   make(map[string]*Function),
		ClassBluePrint:          make(map[string]*ClassBluePrint),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		editorMap:               omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		cacheExternInstance:     make(map[string]Value),
		externType:              make(map[string]Type),
		externBuildValueHandler: make(map[string]func(b *FunctionBuilder, id string, v any) (value Value)),
		ExternInstance:          make(map[string]any),
		ExternLib:               make(map[string]map[string]any),
	}
	prog.EnableDatabase = enableDatabase
	prog.Loader = ssautil.NewPackageLoader(
		ssautil.WithFileSystem(fs),
		ssautil.WithIncludePath(programPath),
	)
	return prog
}

func (prog *Program) HaveLibrary(name string) bool {
	if _, ok := prog.UpStream[name]; ok {
		return true
	}
	if prog.irProgram != nil {
		if slices.Contains(prog.irProgram.UpStream, name) {
			return true
		}
	}

	p, err := GetProgram(name, Library)
	if err != nil {
		return false
	}
	prog.UpStream[name] = p
	// update down stream
	p.DownStream[prog.Name] = prog
	return true
}

func (prog *Program) NewLibrary(name string, path []string) *Program {
	// create lib
	fs := prog.Loader.GetFilesysFileSystem()
	lib := NewProgram(name, prog.EnableDatabase, Library, fs, fs.Join(path...))
	prog.UpStream[name] = lib
	lib.DownStream[prog.Name] = prog
	lib.PushEditor(prog.getCurrentEditor())
	return lib
}

func NewProgramFromDB(p *ssadb.IrProgram) *Program {
	prog := &Program{
		Name:           p.ProgramName,
		ProgramKind:    ProgramKind(p.ProgramKind),
		UpStream:       make(map[string]*Program),
		DownStream:     make(map[string]*Program),
		Cache:          GetCacheFromPool(p.ProgramName),
		EnableDatabase: true,
		irProgram:      p,
	}
	// TODO: handler up and down stream
	return prog
}

func (prog *Program) GetProgramName() string {
	return prog.Name
}

func (prog *Program) GetAndCreateFunction(pkgName string, funcName string) *Function {
	fun := prog.GetFunction(funcName)
	if fun == nil {
		fun = prog.NewFunction(funcName)
	}

	if fun.GetRange() == nil {
		// if editor := prog.getCurrentEditor(); editor != nil {
		// 	fun.SetRangeInit(editor)
		// } else {
		log.Warnf("the program must contains a editor to init function range: %v", prog.Name)
		// }
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
		builder = NewBuilder(prog.getCurrentEditor(), fun, nil)
	}
	return builder
}

func (p *Program) GetFunction(name string) *Function {
	if f, ok := p.Funcs[name]; ok {
		return f
	}
	return nil
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
}

func (prog *Program) Finish() {
	for _, up := range prog.UpStream {
		up.Finish()
	}
	prog.Cache.SaveToDatabase()
	if prog.EnableDatabase {
		updateToDatabase(prog)
	}
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
		index -= 1
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
	p.editorMap.Set(e.GetUrl(), e)
}

func (p *Program) GetIncludeFiles() []string {
	return p.editorMap.Keys()
}

func (p *Program) getCurrentEditor() *memedit.MemEditor {
	if p.editorStack == nil || p.editorStack.Len() <= 0 {
		return nil
	}
	_, v, ok := p.editorStack.Last()
	if !ok {
		return nil
	}
	return v
}

func (p *Program) PopEditor() {
	if p.editorStack == nil || p.editorStack.Len() <= 0 {
		return
	}
	p.editorStack.Pop()
}
