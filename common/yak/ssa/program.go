package ssa

import (
	"path/filepath"
	"sort"
	"strings"

	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// initConcurrentMaps publishes the program maps that several compile workers
// read and write. SafeMap owns the locking; callers must not add another mutex.
func (p *Program) initConcurrentMaps() {
	if p == nil {
		return
	}
	p.mapsOnce.Do(func() {
		if p.variableRanges == nil {
			p.variableRanges = utils.NewSafeMapWithKey[*memedit.MemEditor, *utils.SafeMapWithKey[variableRangePosition, *memedit.Range]]()
		}
		if p.ExportType == nil {
			p.ExportType = utils.NewSafeMapWithKey[string, Type]()
		}
		if p.ExportValue == nil {
			p.ExportValue = utils.NewSafeMapWithKey[string, Value]()
		}
		if p.ReExportTable == nil {
			p.ReExportTable = utils.NewSafeMapWithKey[string, *ReExportInfo]()
		}
	})
}

func NewProgram(
	cfg *ssaconfig.Config,
	databaseKind ProgramCacheKind,
	kind ssadb.ProgramKind,
	fs fi.FileSystem,
	programPath string,
	fileSize int,
) *Program {
	cfg = ensureProgramConfig(cfg)
	programName := cfg.GetProgramName()
	prog := &Program{
		Name:                    programName,
		ProgramKind:             kind,
		LibraryFile:             make(map[string][]string),
		UpStream:                omap.NewEmptyOrderedMap[string, *Program](),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   omap.NewEmptyOrderedMap[string, *Function](),
		Blueprint:               omap.NewEmptyOrderedMap[string, *Blueprint](),
		BlueprintStack:          utils.NewStack[*Blueprint](),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
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
		NameCache:               ssadb.NewNameCache(programName, databaseKind != ProgramCacheMemory),
		compileConfig:           cfg,
		deferredBuilds:          omap.NewEmptyOrderedMap[string, *deferredBuildTask](),
	}

	prog.GlobalVariablesBlueprint = NewBlueprint("__GlobalVariables__")
	prog.GlobalVariablesBlueprint.SetKind(BlueprintClass)
	prog.Blueprint.Set("__GlobalVariables__", prog.GlobalVariablesBlueprint)

	if kind == Application {
		prog.Application = prog
		prog.Cache = NewDBCache(cfg, prog, databaseKind, fileSize)
	}
	prog.DatabaseKind = databaseKind
	prog.Loader = ssautil.NewPackageLoader(
		ssautil.WithFileSystem(fs),
		ssautil.WithIncludePath(programPath),
		ssautil.WithBasePath(programPath),
	)
	prog.ctx = cfg.GetContext()
	return prog
}

// Compile-unit lifecycle (BeginCompileUnit/EndCompileUnit/CurrentCompileUnit,
// ProgramForCompileUnit), ReleaseCompletedUnitMemory, CheckMemoryPressure and
// helpers live in program_unit.go.

func NewTmpProgram(ProgramName string) *Program {
	prog := &Program{
		Name:                    ProgramName,
		ProgramKind:             Application,
		LibraryFile:             make(map[string][]string),
		UpStream:                omap.NewEmptyOrderedMap[string, *Program](),
		DownStream:              make(map[string]*Program),
		errors:                  make([]*SSAError, 0),
		OffsetMap:               make(map[int]*OffsetItem),
		OffsetSortedSlice:       make([]int, 0),
		Funcs:                   omap.NewEmptyOrderedMap[string, *Function](),
		Blueprint:               omap.NewEmptyOrderedMap[string, *Blueprint](),
		BlueprintStack:          utils.NewStack[*Blueprint](),
		editorStack:             omap.NewOrderedMap(make(map[string]*memedit.MemEditor)),
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
		NameCache:               ssadb.NewNameCache(ProgramName, false),
		deferredBuilds:          omap.NewEmptyOrderedMap[string, *deferredBuildTask](),
	}
	prog.Application = prog
	prog.DatabaseKind = ProgramCacheMemory
	return prog
}
func (prog *Program) createSubProgram(name string, kind ssadb.ProgramKind, path ...string) *Program {
	fs := prog.Loader.GetFilesysFileSystem()
	currentEditor := prog.GetCurrentEditor()
	if currentEditor == nil && prog.Application != nil && prog.Application != prog {
		currentEditor = prog.Application.GetCurrentEditor()
	}
	fullPath := prog.Loader.GetBasePath()
	if currentEditor != nil {
		fullPath = currentEditor.GetFilename()
	}
	endPath := fs.Join(path...)
	programPath, _, _ := strings.Cut(fullPath, endPath)
	if programPath == "" {
		programPath = prog.Loader.GetBasePath()
	}
	subProg := NewProgram(cloneProgramConfig(prog.compileConfig, name), prog.DatabaseKind, kind, fs, programPath, 0)
	subProg.Application = prog.Application
	subProg.config = prog.config
	subProg.SetDiagnosticsRecorder(prog.diagnosticsRecorderForChild())

	subProg.Loader.AddIncludePath(prog.Loader.GetIncludeFiles()...)
	subProg.Language = prog.Language

	subProg.LibraryFile = prog.LibraryFile
	fileOwner := prog.GetApplication()
	if fileOwner == nil {
		fileOwner = prog
	}
	fileOwner.fileListMu.Lock()
	subProg.FileList = fileOwner.FileList
	subProg.LineCount = fileOwner.LineCount
	fileOwner.fileListMu.Unlock()
	prog.editorStackMu.RLock()
	subProg.editorStack = prog.editorStack.Copy()
	prog.editorStackMu.RUnlock()
	// subProg.editorStack = prog.editorStack
	subProg.externType = prog.externType
	subProg.externMethodBuilder = prog.externMethodBuilder
	subProg.externBuildValueHandler = prog.externBuildValueHandler
	subProg.ExternInstance = prog.ExternInstance
	subProg.ExternLib = prog.ExternLib
	subProg.ExportType = utils.NewSafeMapWithKey[string, Type]()
	subProg.ExportValue = utils.NewSafeMapWithKey[string, Value]()
	subProg.ReExportTable = utils.NewSafeMapWithKey[string, *ReExportInfo]()
	subProg.Application = prog.Application
	if prog.Application != nil && prog.Application.Build != nil {
		subProg.Build = prog.Application.Build
	} else {
		subProg.Build = prog.Build
	}
	subProg.Cache = prog.Cache
	subProg.NameCache = prog.NameCache
	subProg.fixImportCallback = make([]func(), 0)
	if subProg.PkgName == "" {
		subProg.PkgName = name
	}

	// Publish only after the fields above are set. Concurrent importers
	// observe this library through UpStream.
	prog.AddUpStream(subProg)
	if prog.Application != nil {
		prog.Application.AddUpStream(subProg)
	}
	return subProg
}

func ensureProgramConfig(cfg *ssaconfig.Config) *ssaconfig.Config {
	if cfg != nil {
		return cfg
	}
	ret, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	if err != nil {
		return &ssaconfig.Config{}
	}
	return ret
}

func cloneProgramConfig(base *ssaconfig.Config, programName string) *ssaconfig.Config {
	opts := []ssaconfig.Option{
		ssaconfig.WithSetProgramName(programName),
	}
	if base != nil {
		opts = append(opts,
			ssaconfig.WithContext(base.GetContext()),
			ssaconfig.WithCompileIrCacheTTL(base.GetCompileIrCacheTTL()),
			ssaconfig.WithCompileIrCacheMax(base.GetCompileIrCacheMax()),
		)
	}
	ret, err := ssaconfig.New(ssaconfig.ModeSSACompile, opts...)
	if err != nil {
		return ensureProgramConfig(base)
	}
	if base != nil {
		ret.SetCompileProjectBytes(base.GetCompileProjectBytes())
		ret.SetCompileUnitSplit(base.GetCompileUnitSplit())
	}
	return ret
}
func (prog *Program) IsVirtualImport() bool {
	return prog.config.VirtualImport
}

func (prog *Program) GetSubProgram(name string, path ...string) *Program {
	return prog.NewLibrary(name, path)
}

func (prog *Program) NewLibrary(name string, path []string) *Program {
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	app.libraryCreateMu.Lock()
	defer app.libraryCreateMu.Unlock()
	if existing, ok := app.UpStream.Get(name); ok && existing != nil {
		return existing
	}
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
		if currentEditor == nil || p == nil {
			return false
		}
		owner := app
		if owner == nil {
			owner = p
		}
		owner.fileListMu.Lock()
		hash, ok := p.FileList[currentEditor.GetUrl()]
		owner.fileListMu.Unlock()
		return ok && hash == currentEditor.GetIrSourceHash()
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
	if prog == nil {
		return nil
	}
	// Publish the builder only. NewBuilder does not Emit, and this must
	// not call fake import. The virtual function is also built under
	// virtualEmitMu; take that one first (same order as withVirtualBuilder).
	if funcName == string(VirtualFunctionName) {
		prog.virtualEmitMu.Lock()
		defer prog.virtualEmitMu.Unlock()
	}
	prog.builderMu.Lock()
	defer prog.builderMu.Unlock()
	return prog.getAndCreateFunctionBuilder(pkgName, funcName)
}

func (prog *Program) getAndCreateFunctionBuilder(pkgName string, funcName string) *FunctionBuilder {
	prog.SetPackageName(pkgName)
	fun := prog.GetAndCreateFunction(pkgName, funcName)
	builder := fun.builder
	if builder == nil {
		builder = NewBuilder(prog.GetCurrentEditor(), fun, nil)
	}
	return builder
}
func (prog *Program) SetPackageName(name string) {
	if prog == nil || prog.PkgName == name {
		return
	}
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
	if prog == nil || utils.IsNil(prog) {
		return
	}
	// only run once and not wait
	if prog.finished {
		return
	}
	prog.finished = true

	prog.LazyBuild()
	children := make([]*Program, 0, prog.UpStream.Len())
	prog.UpStream.ForEach(func(_ string, v *Program) bool {
		if v != nil {
			children = append(children, v)
		}
		return true
	})
	for _, child := range children {
		child.Finish()
	}
}

// SearchIndexAndOffsetByOffset binary-searches the sorted offset slice. The
// caller must hold prog.offsetMu (at least RLock).
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
	prog.offsetMu.RLock()
	defer prog.offsetMu.RUnlock()
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
	if p == nil {
		return false
	}
	if p.Cache != nil && p.Cache.sources != nil {
		return p.Cache.sources.HasVisitedURL(path)
	}
	return false
}

func (p *Program) GetEditor(url string) (*memedit.MemEditor, bool) {
	if p == nil {
		return nil, false
	}
	if url == "" {
		if editor := p.GetCurrentEditor(); editor != nil {
			return editor, true
		}
	}
	if p.Cache != nil && p.Cache.sources != nil {
		if editor, ok := p.Cache.sources.GetVisitedEditorByURL(url); ok {
			return editor, true
		}
		if editor, ok := p.Cache.sources.GetEditorByURL(url); ok {
			return editor, true
		}
	}
	if editor := p.GetCurrentEditor(); editor != nil && editor.GetUrl() == url {
		return editor, true
	}
	return nil, false
}

func (p *Program) GetEditorByHash(hash string) (*memedit.MemEditor, bool) {
	if p == nil || hash == "" {
		return nil, false
	}
	if editor := p.GetCurrentEditor(); editor != nil && editor.GetIrSourceHash() == hash {
		return editor, true
	}
	if p.Cache != nil && p.Cache.sources != nil {
		if editor, ok := p.Cache.sources.GetEditorByHash(hash); ok {
			return editor, true
		}
	}
	editor, err := ssadb.GetEditorByHash(hash)
	if err != nil || editor == nil {
		return nil, false
	}
	p.SetEditor(editor.GetUrl(), editor)
	return editor, true
}

func (p *Program) SetEditor(url string, me *memedit.MemEditor) {
	if p != nil && p.Cache != nil && p.Cache.sources != nil {
		p.Cache.sources.markVisitedEditor(url, me)
	}
}

func (p *Program) GetIncludeFiles() []string {
	if p != nil && p.Cache != nil && p.Cache.sources != nil {
		return p.Cache.sources.EditorURLs()
	}
	if editor := p.GetCurrentEditor(); editor != nil && editor.GetUrl() != "" {
		return []string{editor.GetUrl()}
	}
	return nil
}
func (p *Program) GetIncludeFileNum() int {
	if p != nil && p.Cache != nil && p.Cache.sources != nil {
		return len(p.Cache.sources.EditorURLs())
	}
	if editor := p.GetCurrentEditor(); editor != nil && editor.GetUrl() != "" {
		return 1
	}
	return 0
}

func (p *Program) PushEditor(e *memedit.MemEditor) {
	p.editorStackMu.Lock()
	p.editorStack.Push(e)
	p.editorStackMu.Unlock()
	if !p.PreHandler() {
		p.SetEditor(e.GetUrl(), e)
	}
}
func (p *Program) GetCurrentEditor() *memedit.MemEditor {
	p.editorStackMu.RLock()
	defer p.editorStackMu.RUnlock()
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
	p.editorStackMu.Lock()
	if p.editorStack == nil || p.editorStack.Len() <= 0 {
		p.editorStackMu.Unlock()
		return
	}
	e := p.editorStack.Pop()
	p.editorStackMu.Unlock()
	if save && e != nil {
		owner := p
		if app := p.GetApplication(); app != nil {
			owner = app
		}
		owner.fileListMu.Lock()
		if owner.FileList == nil {
			owner.FileList = make(map[string]string)
		}
		if owner.fileLineCounts == nil {
			owner.fileLineCounts = utils.NewSafeMapWithKey[string, int]()
		}
		previous, _ := owner.fileLineCounts.Get(e.GetUrl())
		owner.FileList[e.GetUrl()] = e.GetIrSourceHash()
		owner.LineCount += e.GetLineCount() - previous
		owner.fileLineCounts.Set(e.GetUrl(), e.GetLineCount())
		owner.fileListMu.Unlock()
	}
}

func (p *Program) SetExtraFile(path, hash string) {
	if p == nil || path == "" {
		return
	}
	owner := p
	if app := p.GetApplication(); app != nil {
		owner = app
	}
	owner.metaMu.Lock()
	if owner.ExtraFile == nil {
		owner.ExtraFile = make(map[string]string)
	}
	owner.ExtraFile[path] = hash
	owner.metaMu.Unlock()
}

func (p *Program) AppendSCAPackages(pkgs ...*dxtypes.Package) {
	if p == nil || len(pkgs) == 0 {
		return
	}
	owner := p
	if app := p.GetApplication(); app != nil {
		owner = app
	}
	owner.metaMu.Lock()
	owner.SCAPackages = append(owner.SCAPackages, pkgs...)
	owner.metaMu.Unlock()
}

func (p *Program) GetSCAPackageByName(name string) *dxtypes.Package {
	if p == nil {
		return nil
	}
	owner := p
	if app := p.GetApplication(); app != nil {
		owner = app
	}
	owner.metaMu.Lock()
	defer owner.metaMu.Unlock()
	for _, pkg := range owner.SCAPackages {
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

func (p *Program) templateOwner() *Program {
	if p == nil {
		return nil
	}
	if app := p.GetApplication(); app != nil {
		return app
	}
	return p
}

func (p *Program) GetTemplate(path string) tl.TemplateGeneratedInfo {
	owner := p.templateOwner()
	if owner == nil {
		return nil
	}
	owner.templateMu.Lock()
	info := owner.Template[path]
	owner.templateMu.Unlock()
	return info
}

func (p *Program) TryGetTemplate(path string) tl.TemplateGeneratedInfo {
	owner := p.templateOwner()
	if owner == nil {
		return nil
	}
	if t := owner.GetTemplate(path); t != nil {
		return t
	}
	fileName := filepath.Base(path)
	owner.templateMu.Lock()
	defer owner.templateMu.Unlock()
	var rets []tl.TemplateGeneratedInfo
	for tp, t := range owner.Template {
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
	owner := p.templateOwner()
	if owner == nil {
		return
	}
	owner.templateMu.Lock()
	if owner.Template == nil {
		owner.Template = make(map[string]tl.TemplateGeneratedInfo)
	}
	owner.Template[path] = info
	owner.templateMu.Unlock()
}

func (prog *Program) SetDiagnosticsRecorder(rec *diagnostics.Recorder) {
	if prog == nil {
		return
	}
	prog.diagnosticsRecorder = rec
	root := prog.Application
	if root == nil {
		root = prog
	}
	if prog != root {
		return
	}
	root.UpStream.ForEach(func(_ string, sub *Program) bool {
		if sub != nil {
			sub.diagnosticsRecorder = rec
		}
		return true
	})
}

func (prog *Program) DiagnosticsRecorder() *diagnostics.Recorder {
	if prog == nil {
		return nil
	}
	return prog.diagnosticsRecorder
}

func (prog *Program) diagnosticsRecorderForChild() *diagnostics.Recorder {
	if rec := prog.DiagnosticsRecorder(); rec != nil {
		return rec
	}
	if app := prog.Application; app != nil && app != prog {
		return app.DiagnosticsRecorder()
	}
	return nil
}

func (prog *Program) DiagnosticsTrack(name string, steps ...func() error) {
	_ = prog.DiagnosticsTrackErr(name, steps...)
}

func (prog *Program) DiagnosticsTrackErr(name string, steps ...func() error) error {
	if len(steps) == 0 {
		return nil
	}
	if prog == nil {
		return runDiagnosticSteps(steps)
	}
	if rec := prog.DiagnosticsRecorder(); rec != nil {
		return rec.Track(name, steps...)
	}
	return runDiagnosticSteps(steps)
}

func runDiagnosticSteps(steps []func() error) error {
	for _, step := range steps {
		if step != nil {
			if err := step(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *ProgramCache) diagnosticsTrack(name string, steps ...func() error) {
	_ = c.diagnosticsTrackErr(name, steps...)
}

func (c *ProgramCache) diagnosticsTrackErr(name string, steps ...func() error) error {
	if c == nil || len(steps) == 0 {
		return nil
	}
	if prog := c.program; prog != nil {
		return prog.DiagnosticsTrackErr(name, steps...)
	}
	return runDiagnosticSteps(steps)
}
