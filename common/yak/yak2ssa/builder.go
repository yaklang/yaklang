package yak2ssa

import (
	"path/filepath"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

// var Builder = &singleFileBuilder{}
func Biulder() *SSABuilder {
	return &SSABuilder{}
}

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigShouldBuild(func(filename string) bool {
			return true
		}),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(yak.GetLexerSerializedATN(), yak.GetParserSerializedATN())
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".yak"
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return FrontEnd(src, cache)
}

func (*SSABuilder) BuildFromAST(ast ssa.FrontAST, b *ssa.FunctionBuilder) error {
	b.SupportClosure = true
	astBuilder := &astbuilder{
		FunctionBuilder: b,
	}
	astBuilder.build(ast)
	return nil
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem) fi.FileSystem {
	return fs
}

func (*SSABuilder) FilterFile(path string) bool {
	a := filepath.Ext(path)
	_ = a
	return filepath.Ext(path) == ".yak"
}
func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	return filepath.Ext(path) == ".yak" || filepath.Ext(path) == ".yaklang"
}

func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.Yak
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func FrontEnd(src string, cache *ssa.AntlrCache) (yak.IProgramContext, error) {
	return antlr4util.ParseASTWithSLLFirst(
		src,
		yak.NewYaklangLexer,
		yak.NewYaklangParser,
		func(lexer *yak.YaklangLexer, parser *yak.YaklangParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *yak.YaklangParser) yak.IProgramContext {
			return parser.Program()
		},
	)
}
