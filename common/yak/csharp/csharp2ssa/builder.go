package csharp2ssa

import (
	"path/filepath"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
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
	return filepath.Ext(path) == ".cs"
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(csharpparser.GetCSharpLexerSerializedATN(), csharpparser.GetCSharpParserSerializedATN())
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return Frontend(src, cache)
}

func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	ast, ok := raw.(csharpparser.ICompilation_unitContext)
	if !ok {
		return utils.Errorf("invalid AST type: %T, expected csharpparser.ICompilation_unitContext", raw)
	}
	build := &singleFileBuilder{
		FunctionBuilder: b,
		constMap:        make(map[string]ssa.Value),
	}
	build.VisitCompilationUnit(ast)
	return nil
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return fs
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".cs"
}

func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.CSHARP
}

func (*SSABuilder) UsesDeferredFileBuild() bool {
	return true
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".cs", ".aspx", ".ascx", ".ashx", ".asmx", ".config", ".cshtml":
		return true
	default:
		return filepath.Base(path) == "web.config"
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
	if filepath.Ext(path) == ".cs" {
		return prog.Build(ast, editor, fb)
	}
	prog.ExtraFile[path] = editor.GetIrSourceHash()
	return nil
}

type singleFileBuilder struct {
	*ssa.FunctionBuilder
	constMap    map[string]ssa.Value
	selfPkgPath []string
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
