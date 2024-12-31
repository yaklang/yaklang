package ssa

import (
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"path/filepath"
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

type ProjectConfigType int

const (
	PROJECT_CONFIG_YAML ProjectConfigType = iota
	PROJECT_CONFIG_JSON
	PROJECT_CONFIG_PROPERTIES
)

func NewProgram(ProgramName string, enableDatabase bool, kind ProgramKind, fs fi.FileSystem, programPath string) *Program {
	prog := &Program{
		Name:                    ProgramName,
		ProgramKind:             kind,
		LibraryFile:             make(map[string][]string),
		UpStream:                omap.NewEmptyOrderedMap[string, *Program](),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		Cache:                   NewDBCache(ProgramName, enableDatabase),
		astMap:                  make(map[string]struct{}),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   omap.NewEmptyOrderedMap[string, *Function](),
		Blueprint:               omap.NewEmptyOrderedMap[string, *Blueprint](),
		BlueprintStack:          utils.NewStack[*Blueprint](),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		editorMap:               omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
		FileList:                make(map[string]string),
		cacheExternInstance:     make(map[string]Value),
		externType:              make(map[string]Type),
		externBuildValueHandler: make(map[string]func(b *FunctionBuilder, id string, v any) (value Value)),
		ExternInstance:          make(map[string]any),
		ExternLib:               make(map[string]map[string]any),
		importDeclares:          omap.NewOrderedMap(make(map[string]*importDeclareItem)),
		ProjectConfig:           make(map[string]string),
		Template:                make(map[string]tl.TemplateGeneratedInfo),
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
	subProg.fixImportCallback = make([]func(), 0)
	return subProg
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
		if hash, ok := p.FileList[currentEditor.GetFilename()]; ok {
			if hash == currentEditor.SourceCodeMd5() {
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
	prog.UpStream.Set(sub.Name, sub)
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
		for _, s := range f.ChildFuncs {
			f, ok := ToFunction(s)
			if !ok {
				log.Warnf("function %s is not a ssa.Function", s.GetName())
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

	// only application need save and wait
	if prog.ProgramKind == Application {
		if prog.EnableDatabase { // save program
			updateToDatabase(prog)
		}
		// save instruction
		prog.Cache.SaveToDatabase()
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

func (p *Program) ParseProjectConfig(content string, typ ProjectConfigType) error {
	switch typ {
	case PROJECT_CONFIG_PROPERTIES:
		err := p.parsePropertiesProjectConfig(content)
		if err != nil {
			return err
		}
	}
	return utils.Errorf("not support project config type: %d", typ)
}

func (p *Program) parsePropertiesProjectConfig(content string) error {
	if p == nil {
		return utils.Errorf("program is nil")
	}
	lines := strings.Split(content, "\n")
	var errs error
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			errs = utils.JoinErrors(errs, utils.Errorf("bad properties line: %s", line))
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		p.ProjectConfig[key] = value
	}
	return nil
}

func (p *Program) GetProjectConfig(key string) string {
	if p == nil {
		return ""
	}
	return p.ProjectConfig[key]
}

func (p *Program) SetProjectConfig(key string, value string) {
	if p == nil {
		return
	}
	p.ProjectConfig[key] = value
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
