package csharp2ssa

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
	// globalUsings is shared by every single-file builder created for one
	// project. It is populated before compile-unit execution and remains valid
	// across the per-batch Clearup calls made by the project compiler.
	globalUsings globalUsingState
	// fileScans is replaced for each project partition and then reused by
	// dependency extraction, avoiding a second full lexer pass over every file.
	fileScans     csharpFileScanState
	constructors  csharpConstructorRegistry
	declaredTypes csharpDeclaredTypeState
}

var _ ssa.Builder = (*SSABuilder)(nil)
var _ ssa.PreHandlerAnalyzer = (*SSABuilder)(nil)

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigSupportClass(true),
		ssa.WithLanguageConfigIsSupportClassStaticModifier(true),
		ssa.WithLanguageConfigAllowStaticMemberAccessByInstance(true),
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".cs")
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(csharpparser.GetCSharpLexerSerializedATN(), csharpparser.GetCSharpParserSerializedATN())
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return Frontend(src, cache)
}

func (s *SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	ast, ok := raw.(csharpparser.ICompilation_unitContext)
	if !ok {
		return utils.Errorf("invalid AST type: %T, expected csharpparser.ICompilation_unitContext", raw)
	}
	build := newSingleFileBuilder(b, &s.globalUsings, &s.constructors, &s.declaredTypes)
	build.VisitCompilationUnit(ast)
	return nil
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return fs
}

func (*SSABuilder) FilterFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".cs")
}

func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.CSHARP
}

func (*SSABuilder) UsesDeferredFileBuild() bool {
	return true
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".cs", ".aspx", ".ascx", ".ashx", ".asmx", ".config", ".cshtml":
		return true
	default:
		return strings.EqualFold(filepath.Base(path), "web.config")
	}
}

func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build(ast, editor, builder)
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, fb *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	prog := fb.GetProgram()
	if prog == nil {
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}
	path := editor.GetUrl()
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".cs":
		return prog.Build(ast, editor, fb)
	case ".aspx", ".ascx", ".ashx", ".asmx":
		cs, err := asp.ConvertToCSharp(editor.GetSourceCode(), path)
		if err != nil {
			log.Debugf("convert asp to csharp %s: %v", path, err)
			prog.ExtraFile[path] = editor.GetIrSourceHash()
			return nil
		}
		genAST, err := s.ParseAST(cs, s.GetAntlrCache())
		if err != nil {
			log.Debugf("parse generated csharp from %s: %v", path, err)
			prog.ExtraFile[path] = editor.GetIrSourceHash()
			return nil
		}
		defer ssa.ReleaseASTRoot(genAST)
		templateEditor := prog.CreateEditor([]byte(cs), path+".cs")
		return prog.Build(genAST, templateEditor, fb)
	default:
		prog.ExtraFile[path] = editor.GetIrSourceHash()
		return nil
	}
}

// singleFileBuilder walks one C# compilation unit and emits SSA.
//
// 文件级上下文（命名空间路径 / using 列表 / 别名）在 lazy builder 中需要恢复，
// 因此所有延迟构建都必须经过 lazyBuild，由它负责快照与还原。
type singleFileBuilder struct {
	*ssa.FunctionBuilder
	// globalUsings is project-wide and read dynamically by lazy builders. Local
	// using directives stay in the fields below so they cannot leak to another
	// compilation unit.
	globalUsings *globalUsingState
	// constructors is shared across compilation units so declarations and call
	// sites in different source files use the same overload set.
	constructors *csharpConstructorRegistry
	// declaredTypes is shared across compilation units. Field identity includes
	// its declaring blueprint, so partial classes and hidden same-name fields do
	// not collapse into one name-only slot.
	declaredTypes *csharpDeclaredTypeState
	// nonVirtualCallTargets records method-group values produced by `base.M`.
	// Local aliases retain the same SSA value, allowing a later `f()` invocation
	// to keep direct-base dispatch instead of becoming an ordinary virtual call.
	nonVirtualCallTargets map[int64]struct{}
	// constMap 记录类级常量（const），供标识符解析时兜底使用
	constMap map[string]ssa.Value
	// selfPkgPath 当前命名空间路径，例如 ["Demo", "Web"]
	selfPkgPath []string
	// usings 当前可见的 using 命名空间（文件级 + 命名空间级）
	usings []string
	// usingStatics 当前可见的 `using static Some.Type` 目标
	usingStatics []string
	// usingAlias using X = Some.Type; 的别名映射
	usingAlias map[string]string
	// usingAliasTypes preserves the alias target AST so generic arguments are
	// still available when a deferred method resolves the alias.
	usingAliasTypes map[string]csharpparser.INamespace_or_type_nameContext
	// activeFinalizers records the lexical finally clauses around the statement
	// currently being emitted. TryBuilder models the normal/error CFG edge into
	// finally, but a Return finishes its block immediately, so C# must emit the
	// active clauses on that abrupt edge before emitting the Return itself.
	activeFinalizers []*csharpFinally
	// returnSerial changes only when an EmitReturn succeeds. It lets an outer
	// return notice that a return inside an inlined finally superseded it.
	returnSerial map[*ssa.Function]uint64
	// anonymousSerial assigns lambda/anonymous/query helper names independently
	// for each lexical parent. Named local-function shells are predeclared before
	// statement emission and therefore must not perturb the $1/$2 sequence.
	anonymousSerial map[*ssa.Function]uint64
	// localFunctionShells lets a whole statement-list scope bind its local
	// functions before the first executable statement. The same function object
	// is later populated exactly once at the declaration (or after an abrupt
	// statement made that declaration unreachable).
	localFunctionShells map[*csharpparser.Local_function_declarationContext]*ssa.Function
	// patternConditionBindings carries prospective designation values alongside
	// a pattern boolean. Logical-and and enclosing statement builders bind them
	// only in their success scopes, so `is T x && use(x)` neither loses x nor
	// exposes it on the failure edge.
	patternConditionBindings map[int64][]patternBinding
	// declaredVariableTypes records the compile-time type on the lexical slot,
	// rather than only on its current SSA value. Every later assignment can then
	// retain `Base x` even when the new runtime value was constructed as Derived.
	declaredVariableTypes map[ssa.ScopeIF]map[string]ssa.Type
	// controlTargets mirrors the SSA loop/switch target stack while retaining
	// the lexical finally depth at which each target was entered.  A break or
	// continue only runs finally clauses that it actually exits (for example,
	// `while (...) { try { break; } finally { ... } }`), while a break in a loop
	// wholly contained by a try must leave that try's finally pending.
	controlTargets []csharpControlTarget
}

type csharpDeclaredMemberSlot struct {
	owner  *ssa.Blueprint
	name   string
	static bool
}

type csharpDeclaredValueKey struct {
	program *ssa.Program
	id      int64
}

type csharpDeclaredTypeState struct {
	sync.RWMutex
	members map[csharpDeclaredMemberSlot]ssa.Type
	writers map[csharpDeclaredValueKey]csharpDeclaredMemberSlot
}

// reset releases every project-owned object retained by the shared declared
// type indexes. A SSABuilder may be reused for more than one project, while the
// blueprint and value identities stored here are meaningful only within the
// project that created them.
func (s *csharpDeclaredTypeState) reset() {
	if s == nil {
		return
	}
	s.Lock()
	defer s.Unlock()
	s.members = nil
	s.writers = nil
}

type csharpFinally struct {
	function *ssa.Function
	body     func()
}

type csharpControlTarget struct {
	function       *ssa.Function
	finalizerDepth int
	canBreak       bool
	canContinue    bool
}

func newSingleFileBuilder(
	b *ssa.FunctionBuilder,
	globals *globalUsingState,
	constructors *csharpConstructorRegistry,
	declaredTypes *csharpDeclaredTypeState,
) *singleFileBuilder {
	return &singleFileBuilder{
		FunctionBuilder:          b,
		globalUsings:             globals,
		constructors:             constructors,
		declaredTypes:            declaredTypes,
		nonVirtualCallTargets:    make(map[int64]struct{}),
		constMap:                 make(map[string]ssa.Value),
		usingAlias:               make(map[string]string),
		usingAliasTypes:          make(map[string]csharpparser.INamespace_or_type_nameContext),
		returnSerial:             make(map[*ssa.Function]uint64),
		anonymousSerial:          make(map[*ssa.Function]uint64),
		localFunctionShells:      make(map[*csharpparser.Local_function_declarationContext]*ssa.Function),
		patternConditionBindings: make(map[int64][]patternBinding),
		declaredVariableTypes:    make(map[ssa.ScopeIF]map[string]ssa.Type),
	}
}

// fileScope 是命名空间/using 上下文的快照，用于 lazy builder 恢复。
type fileScope struct {
	pkgPath    []string
	usings     []string
	statics    []string
	alias      map[string]string
	aliasTypes map[string]csharpparser.INamespace_or_type_nameContext
}

func (b *singleFileBuilder) snapshotScope() fileScope {
	return fileScope{
		pkgPath:    append([]string(nil), b.selfPkgPath...),
		usings:     append([]string(nil), b.usings...),
		statics:    append([]string(nil), b.usingStatics...),
		alias:      cloneUsingAliases(b.usingAlias),
		aliasTypes: cloneUsingAliasTypes(b.usingAliasTypes),
	}
}

func (b *singleFileBuilder) restoreScope(s fileScope) func() {
	prevPkg := append([]string(nil), b.selfPkgPath...)
	prevUsings := append([]string(nil), b.usings...)
	prevStatics := append([]string(nil), b.usingStatics...)
	prevAlias := cloneUsingAliases(b.usingAlias)
	prevAliasTypes := cloneUsingAliasTypes(b.usingAliasTypes)
	b.selfPkgPath = append([]string(nil), s.pkgPath...)
	b.usings = append([]string(nil), s.usings...)
	b.usingStatics = append([]string(nil), s.statics...)
	b.usingAlias = cloneUsingAliases(s.alias)
	b.usingAliasTypes = cloneUsingAliasTypes(s.aliasTypes)
	return func() {
		b.selfPkgPath, b.usings, b.usingStatics, b.usingAlias, b.usingAliasTypes = prevPkg, prevUsings, prevStatics, prevAlias, prevAliasTypes
	}
}

func cloneUsingAliases(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for alias, target := range src {
		dst[alias] = target
	}
	return dst
}

func cloneUsingAliasTypes(src map[string]csharpparser.INamespace_or_type_nameContext) map[string]csharpparser.INamespace_or_type_nameContext {
	dst := make(map[string]csharpparser.INamespace_or_type_nameContext, len(src))
	for alias, target := range src {
		dst[alias] = target
	}
	return dst
}

// lazyBuild 注册一个延迟构建任务：在执行时恢复注册时的 FunctionBuilder
// 与文件级上下文。add 通常是 blueprint.AddLazyBuilder 或 function.AddLazyBuilder。
func (b *singleFileBuilder) lazyBuild(add func(func(), ...bool), fn func()) {
	if b == nil || add == nil || fn == nil {
		return
	}
	store := b.StoreFunctionBuilder()
	scope := b.snapshotScope()
	add(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		restore := b.restoreScope(scope)
		defer restore()
		fn()
	})
}

func newCSharpLexer(input antlr.CharStream) *csharpparser.CSharpLexer {
	lexer := csharpparser.NewCSharpLexer(input)
	lexer.InitCSharpLexer()
	return lexer
}

func Frontend(src string, caches ...*ssa.AntlrCache) (csharpparser.ICompilation_unitContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		newCSharpLexer,
		csharpparser.NewCSharpParser,
		nil,
		func(lexer *csharpparser.CSharpLexer, parser *csharpparser.CSharpParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
			parser.GetSymTable().PreScan(parser.GetTokenStream())
		},
		func(parser *csharpparser.CSharpParser) csharpparser.ICompilation_unitContext {
			prog := parser.Prog()
			if prog == nil {
				return nil
			}
			return prog.Compilation_unit()
		},
	)
}

func (b *singleFileBuilder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

func (b *singleFileBuilder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s)
}

func identText(id csharpparser.IIdentifierContext) string {
	if id == nil {
		return ""
	}
	return id.GetText()
}

func nilValue(b *singleFileBuilder, text string) ssa.Value {
	if b == nil {
		return nil
	}
	if text == "" {
		text = "undefined"
	}
	return b.EmitUndefined(text)
}
