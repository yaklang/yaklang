package js2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/parser"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
}

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true), // 设置处理语言闭包的副作用的策略
		ssa.WithLanguageConfigSupportClass(true),
		ssa.WithLanguageConfigIsSupportClassStaticModifier(true),
		ssa.WithLanguageBuilder(builder),
		ssa.WithLanguageConfigTryBuildValue(true),
	)
	return builder
}

type builder struct {
	*ssa.FunctionBuilder
	sourceFile *ast.SourceFile

	useStrict         bool
	contextLabelStack []string
}

var Builder ssa.Builder = &SSABuilder{}

func (*SSABuilder) ParseAST(src string) (ssa.FrontAST, error) {
	return Frontend(src)
}

func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	jsAST, ok := raw.(*ast.SourceFile)
	if !ok {
		return utils.Errorf("invalid AST type: expected *ast.SourceFile, got %T", raw)
	}
	b.SupportClosure = true
	build := &builder{
		FunctionBuilder:   b,
		sourceFile:        jsAST,
		useStrict:         false,
		contextLabelStack: make([]string, 0),
	}
	build.VisitSourceFile(jsAST)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".js"
}

func (*SSABuilder) GetLanguage() consts.Language {
	//TODO implement me
	return consts.JS
}

func Frontend(src string) (*ast.SourceFile, error) {
	jsast := parser.ParseSourceFile("", "", src, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	var err error
	if len(jsast.Diagnostics()) != 0 {
		err = utils.Errorf("parse AST FrontEnd error: %v", jsast.Diagnostics()[0].Message())
	}
	return jsast, err
}
