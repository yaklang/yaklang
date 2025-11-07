package ssa

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/utils"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func NewProgram(
	ProgramName string, databaseKind ProgramCacheKind, kind ssadb.ProgramKind,
	fs fi.FileSystem, programPath string, fileSize int,
	ttl ...time.Duration,
) *Program {
	prog := &Program{
		Name:                    ProgramName,
		ProgramKind:             kind,
		LibraryFile:             make(map[string][]string),
		UpStream:                omap.NewEmptyOrderedMap[string, *Program](),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		astMap:                  make(map[string]struct{}),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   omap.NewEmptyOrderedMap[string, *Function](),
		Blueprint:               omap.NewEmptyOrderedMap[string, *Blueprint](),
		BlueprintStack:          utils.NewStack[*Blueprint](),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		editorMap:               omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		FileList:                make(map[string]string),
		LineCount:               0,
		cacheExternInstance:     make(map[string]Value),
		externType:              make(map[string]Type),
		externBuildValueHandler: make(map[string]func(b *FunctionBuilder, id string, v any) (value Value)),
		ExternInstance:          make(map[string]any),
		ExternLib:               make(map[string]map[string]any),
		importDeclares:          omap.NewOrderedMap(make(map[string]*importDeclareItem)),
		ProjectConfig:           make(map[string]*ProjectConfig),
		Template:                make(map[string]tl.TemplateGeneratedInfo),
		CurrentIncludingStack:   utils.NewStack[string](),
		config:                  NewLanguageConfig(),
	}

	prog.GlobalVariablesBlueprint = NewBlueprint("__GlobalVariables__")
	prog.GlobalVariablesBlueprint.SetKind(BlueprintClass)
	prog.Blueprint.Set("__GlobalVariables__", prog.GlobalVariablesBlueprint)

	if kind == Application {
		prog.Application = prog
		prog.Cache = NewDBCache(prog, databaseKind, fileSize, ttl...)
	}
	prog.DatabaseKind = databaseKind
	prog.Loader = ssautil.NewPackageLoader(
		ssautil.WithFileSystem(fs),
		ssautil.WithIncludePath(programPath),
		ssautil.WithBasePath(programPath),
	)
	return prog
}

func NewTmpProgram(ProgramName string) *Program {
	prog := &Program{
		Name:                    ProgramName,
		ProgramKind:             Application,
		LibraryFile:             make(map[string][]string),
		UpStream:                omap.NewEmptyOrderedMap[string, *Program](),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		astMap:                  make(map[string]struct{}),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   omap.NewEmptyOrderedMap[string, *Function](),
		Blueprint:               omap.NewEmptyOrderedMap[string, *Blueprint](),
		BlueprintStack:          utils.NewStack[*Blueprint](),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		editorMap:               omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		FileList:                make(map[string]string),
		LineCount:               0,
		cacheExternInstance:     make(map[string]Value),
		externType:              make(map[string]Type),
		externBuildValueHandler: make(map[string]func(b *FunctionBuilder, id string, v any) (value Value)),
		ExternInstance:          make(map[string]any),
		ExternLib:               make(map[string]map[string]any),
		importDeclares:          omap.NewOrderedMap(make(map[string]*importDeclareItem)),
		ProjectConfig:           make(map[string]*ProjectConfig),
		Template:                make(map[string]tl.TemplateGeneratedInfo),
		CurrentIncludingStack:   utils.NewStack[string](),
		config:                  NewLanguageConfig(),
	}
	prog.Application = prog
	prog.DatabaseKind = ProgramCacheMemory
	return prog
}
func (prog *Program) createSubProgram(name string, kind ssadb.ProgramKind, path ...string) *Program {
	fs := prog.Loader.GetFilesysFileSystem()
	fullPath := prog.GetCurrentEditor().GetFilename()
	endPath := fs.Join(path...)
	programPath, _, _ := strings.Cut(fullPath, endPath)
	subProg := NewProgram(name, prog.DatabaseKind, kind, fs, programPath, 0)
	subProg.Application = prog.Application
	subProg.config = prog.config

	subProg.Loader.AddIncludePath(prog.Loader.GetIncludeFiles()...)
	subProg.Language = prog.Language

	subProg.LibraryFile = prog.LibraryFile
	subProg.FileList = prog.FileList
	subProg.LineCount = prog.LineCount
	subProg.editorStack = prog.editorStack.Copy()
	// subProg.editorStack = prog.editorStack
	subProg.externType = prog.externType
	subProg.externBuildValueHandler = prog.externBuildValueHandler
	subProg.ExternInstance = prog.ExternInstance
	subProg.ExternLib = prog.ExternLib
	subProg.ExportType = make(map[string]Type)
	subProg.ExportValue = make(map[string]Value)
	subProg.ReExportTable = make(map[string]*ReExportInfo)

	// up-down stream and application
	prog.AddUpStream(subProg)
	prog.Application.AddUpStream(subProg)
	subProg.Application = prog.Application
	subProg.Cache = prog.Cache
	subProg.fixImportCallback = make([]func(), 0)
	return subProg
}
func (prog *Program) IsVirtualImport() bool {
	return prog.config.VirtualImport
}

func (prog *Program) GetSubProgram(name string, path ...string) *Program {
	child, ok := prog.UpStream.Get(name)
	if !ok {
		child = prog.createSubProgram(name, Library, path...)
	}
	return child
}

func (prog *Program) NewLibrary(name string, path []string) *Program {
	return prog.createSubProgram(name, Library, path...)
}

func (prog *Program) GetOrCreateLibrary(name string) (*Program, error) {
	library, _ := prog.GetLibrary(name)
	if library != nil {
		return library, nil
	}
	lib, err := prog.GenerateVirtualLib(name)
	if err != nil {
		log.Warnf("generate virtual lib fail: %s", err)
		return nil, err
	}
	return lib, nil
}
func (prog *Program) GetLibrary(name string) (*Program, bool) {
	if prog == nil || utils.IsNil(prog) || prog.Application == nil || utils.IsNil(prog.Application) {
		return nil, false
	}
	// get lib from application
	app := prog.Application
	currentEditor := prog.GetCurrentEditor()
	// this program has current file
	hasFile := func(p *Program) bool {
		if hash, ok := p.FileList[currentEditor.GetUrl()]; ok {
			if hash == currentEditor.GetIrSourceHash() {
				return true
			}
		}
		return false
	}

	// contain in memory
	if p, ok := app.UpStream.Get(name); ok {
		return p, hasFile(p)
	}

	if p, ok := prog.UpStream.Get(name); ok {
		app.AddUpStream(p)
		return p, hasFile(p)
	}
	// if !app.EnableDatabase {
	return nil, false
	// }
	// version := ""
	// if p := app.GetSCAPackageByName(name); p != nil {
	// 	version = p.Version
	// } else {
	// 	return nil, false
	// }
	// library in  database, load and set relation
	// p, err := GetLibrary(name, version)
	// if err != nil {
	// 	return nil, false
	// }
	// app.AddUpStream(p)
	// if !slices.Contains(p.irProgram.UpStream, name) {
	// 	// update up-down stream
	// 	prog.AddUpStream(p)
	// }
	// return p, hasFile(p)
}

func (prog *Program) AddUpStream(sub *Program) {
	prog.UpStream.Set(sub.Name, sub)
	sub.DownStream[prog.Name] = prog
}

func (prog *Program) GetProgramName() string {
	if prog == nil {
		return ""
	}
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
	prog.SetPackageName(pkgName)
	fun := prog.GetAndCreateFunction(pkgName, funcName)
	builder := fun.builder
	if builder == nil {
		builder = NewBuilder(prog.GetCurrentEditor(), fun, nil)
	}

	return builder
}
func (prog *Program) SetPackageName(name string) {
	prog.PkgName = name
}

func (prog *Program) EachFunction(handler func(*Function)) {
	var handFunc func(*Function)
	handFunc = func(f *Function) {
		handler(f)
		for _, id := range f.ChildFuncs {
			child, ok := f.GetValueById(id)
			if !ok || child == nil {
				log.Warnf("function %s child %d not found in function %s", f.GetName(), id, f.GetName())
				continue
			}
			f, ok := ToFunction(child)
			if !ok {
				log.Warnf("function %s is not a ssa.Function", child.GetName())
				continue
			}
			handFunc(f)
		}
	}

	prog.Funcs.ForEach(func(i string, v *Function) bool {
		handFunc(v)
		return true
	})
	// for _, f := range prog.Funcs {
	// 	handFunc(f)
	// }
	prog.UpStream.ForEach(func(i string, v *Program) bool {
		v.Funcs.ForEach(func(i string, v *Function) bool {
			handFunc(v)
			return true
		})
		return true
	})
}

func (prog *Program) Finish() {
	// only run once and not wait
	if prog.finished {
		return
	}
	prog.finished = true

	// check instruction build
	// if len(prog.astMap) != 0 {
	/* in end this program not delete all astMap item,
	this mean some file build in preHandler but not build in Build
	*/
	// log.Errorf("BUG!! program %s has not finish ast", prog.Name)
	prog.LazyBuild() // finish
	// }
	prog.UpStream.ForEach(func(i string, v *Program) bool {
		v.Finish()
		return true
	})
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

func (p *Program) ShouldVisit(path string) bool {
	return p.editorMap.Have(path)
}

func (p *Program) GetEditor(url string) (*memedit.MemEditor, bool) {
	return p.editorMap.Get(url)
}

func (p *Program) SetEditor(url string, me *memedit.MemEditor) {
	p.editorMap.Set(url, me)
}

func (p *Program) GetIncludeFiles() []string {
	return p.editorMap.Keys()
}
func (p *Program) GetIncludeFileNum() int {
	return p.editorMap.Len()
}

func (p *Program) PushEditor(e *memedit.MemEditor) {
	p.editorStack.Push(e)
	if !p.PreHandler() {
		p.SetEditor(e.GetUrl(), e)
	}
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
		p.FileList[e.GetUrl()] = e.GetIrSourceHash()
		p.LineCount += e.GetLineCount()
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

func (p *Program) GetTemplate(path string) tl.TemplateGeneratedInfo {
	if p == nil {
		return nil
	}
	return p.Template[path]
}

func (p *Program) TryGetTemplate(path string) tl.TemplateGeneratedInfo {
	if p == nil {
		return nil
	}
	if t := p.GetTemplate(path); t != nil {
		return t
	}
	fileName := filepath.Base(path)
	var rets []tl.TemplateGeneratedInfo
	for tp, t := range p.Template {
		if strings.Contains(tp, fileName) {
			rets = append(rets, t)
		}
	}
	if len(rets) == 1 {
		return rets[0]
	}
	return nil
}

func (p *Program) SetTemplate(path string, info tl.TemplateGeneratedInfo) {
	if p == nil {
		return
	}
	p.Template[path] = info
}
