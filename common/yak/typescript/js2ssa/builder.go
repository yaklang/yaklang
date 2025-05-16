package js2ssa

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/parser"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
	"path/filepath"
)

type SSABuilder struct {
	*ssa.PreHandlerInit
}

type builder struct {
	*ssa.FunctionBuilder
	sourceFile *ast.SourceFile

	// 存储跳转标签
	labels map[string]*ssa.LabelBuilder
}

var Builder ssa.Builder = &SSABuilder{}

func (*SSABuilder) Build(src string, force bool, b *ssa.FunctionBuilder) error {
	jsAST, err := Frontend(src, force)
	if err != nil {
		return err
	}
	b.SupportClosure = true
	build := &builder{
		FunctionBuilder: b,
		sourceFile:      jsAST,
	}
	b.SetEditor(memedit.NewMemEditor(src))
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

func Frontend(src string, force bool) (*ast.SourceFile, error) {
	jsast := parser.ParseSourceFile("", "", src, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	if force || len(jsast.Diagnostics()) == 0 {
		return jsast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error: %v", jsast.Diagnostics()[0].Message())
}
