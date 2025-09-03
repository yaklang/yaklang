package js2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"

	"github.com/yaklang/yaklang/common/utils"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type SSABuild struct {
	*ssa.PreHandlerBase
}

// var Builder ssa.Builder = &SSABuild{}

// func (s *SSABuild) Create() ssa.Builder {
// 	return &SSABuild{
// 		PreHandlerInit: ssa.NewPreHandlerInit().WithLanguageConfigOpts(
// 			ssa.WithLanguageConfigBind(true),
// 			ssa.WithLanguageBuilder(s)),
// 	}
// }

func (*SSABuild) Build(src string, force bool, builder *ssa.FunctionBuilder) error {
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}
	builder.SupportClosure = true
	astBuilder := &astbuilder{
		FunctionBuilder: builder,
		lmap:            make(map[string]struct{}),
		cmap:            make(map[string]struct{}),
	}
	// log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
	astBuilder.build(ast)
	return nil
}

func (*SSABuild) FilterFile(path string) bool {
	return filepath.Ext(path) == ".js"
}

func (*SSABuild) GetLanguage() consts.Language {
	return consts.JS
}

type astbuilder struct {
	*ssa.FunctionBuilder
	lmap map[string]struct{}
	cmap map[string]struct{}
}

func Frontend(src string, must bool) (*JS.ProgramContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program().(*JS.ProgramContext)
	return ast, errListener.Error()
}

func (b *astbuilder) AddToCmap(key string) {
	b.cmap[key] = struct{}{}
}

func (b *astbuilder) GetFromCmap(key string) bool {
	if _, ok := b.cmap[key]; ok {
		return true
	} else {
		return false
	}
}

func (b *astbuilder) AddToLmap(key string) {
	b.lmap[key] = struct{}{}
}

func (b *astbuilder) GetFromLmap(key string) bool {
	if _, ok := b.lmap[key]; ok {
		return true
	} else {
		return false
	}
}
