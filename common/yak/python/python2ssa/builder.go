package python2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ========================================== For SSAAPI ==========================================

// SSABuilder implements the ssa.Builder interface for Python code.
// It provides methods to parse Python source code and build SSA representation.
// Similar to java2ssa.SSABuilder, this builder handles Python-specific parsing and SSA construction.
type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

// CreateBuilder creates a new SSABuilder instance for Python.
// This function is used by the SSA API to create language-specific builders.
// Returns a builder configured for Python language features.
func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigSupportClass(true),
		ssa.WithLanguageConfigIsSupportClassStaticModifier(true), // Python class attributes are readable via Class.attr
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

// FilterParseAST determines if a file should be parsed based on its extension.
// Returns true if the file has a .py extension.
func (s *SSABuilder) FilterParseAST(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".py"
}

// GetAntlrCache returns the AntlrCache for Python parser and lexer.
// This cache improves parsing performance by reusing ATN data.
func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(pythonparser.GetPythonLexerSerializedATN(), pythonparser.GetPythonParserSerializedATN())
}

// ParseAST parses Python source code and returns the AST.
// It uses the provided cache if available to improve performance.
func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return FrontendWithCache(src, cache)
}

// BuildFromAST builds SSA representation from the parsed AST.
// This is the main entry point for converting Python AST to SSA.
func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	ast, ok := raw.(pythonparser.IRootContext)
	if !ok {
		return utils.Errorf("invalid AST type: %T, expected pythonparser.IRootContext", raw)
	}
	build := &singleFileBuilder{
		FunctionBuilder: b,
		constMap:        make(map[string]ssa.Value),
		globalNames:     make(map[string]bool),
	}
	build.VisitRoot(ast)
	return nil
}

// WrapWithPreprocessedFS wraps the filesystem with preprocessing if needed.
// For Python, this is a no-op currently as Python doesn't need template preprocessing like Java.
func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem) fi.FileSystem {
	// Python doesn't need special filesystem preprocessing like Java's template files (JSP, Freemarker, etc.)
	return fs
}

// FilterFile determines if a file should be processed based on its extension.
// Returns true if the file has a .py extension.
func (*SSABuilder) FilterFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".py"
}

// GetLanguage returns the language identifier for Python.
func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.PYTHON
}

// ========================================== PreHandlerAnalyzer Implementation ==========================================

var _ ssa.PreHandlerAnalyzer = &SSABuilder{}

// FilterPreHandlerFile determines if a file should be preprocessed.
// For Python, we use a whitelist approach to only process Python-related files.
// This avoids conflicts with other language builders (e.g., Java's JSP files).
func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	// Whitelist: only process files that Python projects typically contain
	whiteList := []string{".py", ".pyi", ".pyw", ".yaml", ".yml", ".txt", ".toml", ".cfg", ".ini", ".json"}
	for _, ext := range whiteList {
		if extension == ext {
			return true
		}
	}
	return false
}

// PreHandlerFile handles preprocessing of a single file.
// For Python, we don't need to call Build here as the framework calls it automatically.
// This is used for any pre-processing before the main build.
func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	// Empty - the framework calls Build automatically
}

// PreHandlerProject handles preprocessing at the project level.
// For Python, this handles project-level configuration and dependencies.
func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, fb *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	prog := fb.GetProgram()
	if prog == nil {
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	path := editor.GetUrl()
	ext := filepath.Ext(path)

	// Save extra file information
	saveExtraFile := func(path string) {
		if prog.GetProgramName() == "" {
			prog.ExtraFile[path] = editor.GetIrSourceHash()
		} else {
			prog.ExtraFile[path] = editor.GetIrSourceHash()
		}
	}

	switch ext {
	case ".py":
		// Python files are built by PreHandlerFile, not here
		// This prevents double compilation
		return nil
	case ".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc":
		// Skip binary/media files
		return nil
	case ".yaml", ".yml":
		// Handle YAML configuration files (e.g., requirements.yaml, setup.yaml)
		saveExtraFile(path)
		if err := prog.ParseProjectConfig([]byte(editor.GetSourceCode()), path, ssa.PROJECT_CONFIG_YAML); err != nil {
			return err
		}
	case ".txt":
		// Handle requirements.txt
		if filepath.Base(path) == "requirements.txt" {
			saveExtraFile(path)
			// TODO: Parse requirements.txt for dependency information
		} else {
			saveExtraFile(path)
		}
	default:
		saveExtraFile(path)
	}
	return nil
}

// ========================================== Build Front End ==========================================

// singleFileBuilder handles the conversion of a single Python file's AST to SSA.
// It maintains state during the conversion process, including constants and function builders.
type singleFileBuilder struct {
	*ssa.FunctionBuilder
	constMap               map[string]ssa.Value
	globalNames            map[string]bool
	staticLoopControls     []*staticLoopControl
	wildcardImportPackages []string
	tryControls            []*tryControl
}

type staticLoopControlState uint8

const (
	staticLoopControlNone staticLoopControlState = iota
	staticLoopControlBreak
	staticLoopControlContinue
)

type staticLoopControl struct {
	state staticLoopControlState
}

type tryControl struct {
	raised         bool
	lastRaised     ssa.Value
	lastRaisedType string
}

func (b *singleFileBuilder) pushStaticLoopControl() *staticLoopControl {
	control := &staticLoopControl{}
	b.staticLoopControls = append(b.staticLoopControls, control)
	return control
}

func (b *singleFileBuilder) popStaticLoopControl() {
	if len(b.staticLoopControls) == 0 {
		return
	}
	b.staticLoopControls = b.staticLoopControls[:len(b.staticLoopControls)-1]
}

func (b *singleFileBuilder) currentStaticLoopControl() *staticLoopControl {
	if len(b.staticLoopControls) == 0 {
		return nil
	}
	return b.staticLoopControls[len(b.staticLoopControls)-1]
}

func (b *singleFileBuilder) hasPendingStaticLoopControl() bool {
	control := b.currentStaticLoopControl()
	return control != nil && control.state != staticLoopControlNone
}

func (b *singleFileBuilder) shouldStopStatementWalk() bool {
	return b.IsBlockFinish() || b.hasPendingStaticLoopControl()
}

func (b *singleFileBuilder) addWildcardImportPackage(pkg string) {
	if pkg == "" {
		return
	}
	for _, existing := range b.wildcardImportPackages {
		if existing == pkg {
			return
		}
	}
	b.wildcardImportPackages = append(b.wildcardImportPackages, pkg)
}

func (b *singleFileBuilder) newDynamicPlaceholder(name string) ssa.Value {
	if name == "" {
		name = "dynamic"
	}
	value := b.EmitUndefined(name)
	if value == nil {
		return nil
	}
	value.Kind = ssa.UndefinedValueValid
	if value.GetType() == nil ||
		value.GetType().GetTypeKind() == ssa.NullTypeKind ||
		value.GetType().GetTypeKind() == ssa.UndefinedTypeKind {
		value.SetType(ssa.CreateAnyType())
	}
	return value
}

func (b *singleFileBuilder) ensureDynamicValueType(value ssa.Value) ssa.Value {
	if value == nil {
		return nil
	}
	if value.GetType() == nil ||
		value.GetType().GetTypeKind() == ssa.NullTypeKind ||
		value.GetType().GetTypeKind() == ssa.UndefinedTypeKind {
		value.SetType(ssa.CreateAnyType())
	}
	return value
}

func (b *singleFileBuilder) ensureDynamicObjectType(value ssa.Value) ssa.Value {
	if value == nil {
		return nil
	}
	if value.GetType() == nil {
		return b.ensureDynamicValueType(value)
	}

	switch value.GetType().GetTypeKind() {
	case ssa.AnyTypeKind,
		ssa.ObjectTypeKind,
		ssa.StructTypeKind,
		ssa.MapTypeKind,
		ssa.SliceTypeKind,
		ssa.TupleTypeKind,
		ssa.BytesTypeKind,
		ssa.StringTypeKind,
		ssa.ClassBluePrintTypeKind:
		return value
	default:
		value.SetType(ssa.CreateAnyType())
		return value
	}
}

func (b *singleFileBuilder) shouldUseDynamicMemberFallback(value ssa.Value) bool {
	if value == nil {
		return true
	}
	switch value.GetName() {
	case "self", "cls":
		return true
	}
	switch value.GetOpcode() {
	case ssa.SSAOpcodeParameter,
		ssa.SSAOpcodeFreeValue,
		ssa.SSAOpcodeParameterMember,
		ssa.SSAOpcodeUndefined,
		ssa.SSAOpcodeConstInst:
		return true
	default:
		return false
	}
}

func (b *singleFileBuilder) hasBlueprintMemberOrMethod(blueprint *ssa.Blueprint, name string) bool {
	if blueprint == nil || name == "" {
		return false
	}
	return !utils.IsNil(blueprint.GetNormalMethod(name)) ||
		!utils.IsNil(blueprint.GetStaticMethod(name)) ||
		!utils.IsNil(blueprint.GetNormalMember(name)) ||
		!utils.IsNil(blueprint.GetStaticMember(name))
}

func (b *singleFileBuilder) ensureBlueprintConstructorSlot(blueprint *ssa.Blueprint) {
	if blueprint == nil || blueprint.Name == "" {
		return
	}
	if !utils.IsNil(blueprint.GetNormalMember(blueprint.Name)) || !utils.IsNil(blueprint.GetStaticMember(blueprint.Name)) {
		return
	}
	constructorSlot := b.newDynamicPlaceholder(blueprint.Name)
	blueprint.RegisterNormalMember(blueprint.Name, constructorSlot, false)
	blueprint.RegisterStaticMember(blueprint.Name, constructorSlot, false)
}

func (b *singleFileBuilder) normalizePythonCallArgument(value ssa.Value) ssa.Value {
	if value == nil {
		return nil
	}
	if value.GetType() == nil {
		return b.ensureDynamicValueType(value)
	}

	switch value.GetType().GetTypeKind() {
	case ssa.BooleanTypeKind,
		ssa.NumberTypeKind,
		ssa.StringTypeKind,
		ssa.BytesTypeKind,
		ssa.SliceTypeKind,
		ssa.MapTypeKind,
		ssa.TupleTypeKind:
		return value
	case ssa.ClassBluePrintTypeKind,
		ssa.FunctionTypeKind:
		value.SetType(ssa.CreateAnyType())
		return value
	}

	switch value.GetOpcode() {
	case ssa.SSAOpcodePhi,
		ssa.SSAOpcodeParameter,
		ssa.SSAOpcodeFreeValue,
		ssa.SSAOpcodeParameterMember:
		value.SetType(ssa.CreateAnyType())
	}
	return value
}

func (b *singleFileBuilder) ensureBlueprintMember(obj ssa.Value, name string) {
	if obj == nil || name == "" || obj.GetType() == nil {
		return
	}
	blueprint, ok := ssa.ToClassBluePrintType(obj.GetType())
	if !ok || blueprint == nil {
		return
	}
	if !utils.IsNil(blueprint.GetNormalMember(name)) || !utils.IsNil(blueprint.GetStaticMember(name)) {
		return
	}
	blueprint.RegisterNormalMember(name, b.newDynamicPlaceholder(name), false)
}

func (b *singleFileBuilder) ensureBlueprintCallableMember(obj ssa.Value, name string) {
	if obj == nil || name == "" || obj.GetType() == nil {
		return
	}
	blueprint, ok := ssa.ToClassBluePrintType(obj.GetType())
	if !ok || blueprint == nil {
		return
	}
	if !utils.IsNil(blueprint.GetNormalMethod(name)) ||
		!utils.IsNil(blueprint.GetStaticMethod(name)) ||
		!utils.IsNil(blueprint.GetNormalMember(name)) ||
		!utils.IsNil(blueprint.GetStaticMember(name)) {
		return
	}
	placeholder := b.NewFunc(blueprint.Name + "_" + name)
	placeholder.SetMethodName(name)
	placeholder.SetType(ssa.NewFunctionTypeDefine(name, nil, []ssa.Type{ssa.CreateAnyType()}, false))
	blueprint.RegisterNormalMethod(name, placeholder, false)
}

func (b *singleFileBuilder) syncBlueprintContainerMember(blueprint *ssa.Blueprint, name string, value ssa.Value) {
	if b == nil || blueprint == nil || name == "" || value == nil {
		return
	}
	container := blueprint.Container()
	if container == nil {
		return
	}
	member := b.CreateMemberCallVariable(container, b.EmitConstInst(name))
	if member == nil {
		return
	}
	b.AssignVariable(member, value)
}

func (b *singleFileBuilder) bindImportedPlaceholder(bindingName, sourceName string) ssa.Value {
	value := b.newDynamicPlaceholder(sourceName)
	if value == nil || bindingName == "" {
		return value
	}
	b.AssignVariable(b.createVar(bindingName), value)
	return value
}

func (b *singleFileBuilder) resolveWildcardImportName(name string) ssa.Value {
	if name == "" {
		return nil
	}

	prog := b.GetProgram()
	if prog == nil {
		return nil
	}

	for _, pkg := range b.wildcardImportPackages {
		if prog.GetCurrentEditor() == nil {
			return b.bindImportedPlaceholder(name, joinImportPath(pkg, name))
		}
		lib, err := prog.GetOrCreateLibrary(pkg)
		if err != nil || lib == nil {
			return b.bindImportedPlaceholder(name, joinImportPath(pkg, name))
		}

		value := lib.GetExportValue(name)
		if value == nil {
			libBuilder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(ssa.VirtualFunctionName))
			if libBuilder == nil {
				return b.bindImportedPlaceholder(name, joinImportPath(pkg, name))
			}
			value = b.newDynamicPlaceholder(joinImportPath(pkg, name))
			lib.SetExportValue(name, value)
		}

		if err := prog.ImportValueFromLib(lib, name); err != nil {
			return b.bindImportedPlaceholder(name, joinImportPath(pkg, name))
		}
		if imported, ok := prog.ReadImportValue(name); ok {
			return imported
		}
		if value != nil {
			return value
		}
	}

	return nil
}

func (b *singleFileBuilder) pushTryControl() *tryControl {
	control := &tryControl{}
	b.tryControls = append(b.tryControls, control)
	return control
}

func (b *singleFileBuilder) popTryControl() {
	if len(b.tryControls) == 0 {
		return
	}
	b.tryControls = b.tryControls[:len(b.tryControls)-1]
}

func (b *singleFileBuilder) currentTryControl() *tryControl {
	if len(b.tryControls) == 0 {
		return nil
	}
	return b.tryControls[len(b.tryControls)-1]
}

// SwitchFunctionBuilder saves the current FunctionBuilder state and switches to the stored one.
// Returns a function that restores the previous state when called.
// This is used in Blueprint lazy builders to ensure proper function context.
func (b *singleFileBuilder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

// LoadBuilder restores the FunctionBuilder from a stored state.
func (b *singleFileBuilder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s.Store)
}

// FrontendWithCache parses Python source code and returns the root AST node.
// It supports optional AntlrCache for improved parsing performance.
// Similar to java2ssa.Frontend, but for Python syntax.
func FrontendWithCache(src string, caches ...*ssa.AntlrCache) (pythonparser.IRootContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		pythonparser.NewPythonLexer,
		pythonparser.NewPythonParser,
		nil,
		func(lexer *pythonparser.PythonLexer, parser *pythonparser.PythonParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *pythonparser.PythonParser) pythonparser.IRootContext {
			return parser.Root()
		},
	)
}

// Frontend parses Python source code and returns the root AST node.
// This is a convenience function that calls the Frontend function in builder.go.
// For better performance with caching, use the Frontend function in builder.go directly.
func Frontend(src string) (pythonparser.IRootContext, error) {
	return FrontendWithCache(src)
}
