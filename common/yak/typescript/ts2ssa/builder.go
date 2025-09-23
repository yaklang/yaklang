package ts2ssa

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

var Builder ssa.Builder = &SSABuilder{}

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(initHandler),
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

func initHandler(fb *ssa.FunctionBuilder) {
	container := fb.EmitEmptyContainer()

	prog := fb.GetProgram()
	if prog.GlobalVariablesBlueprint != nil {
		prog.GlobalVariablesBlueprint.InitializeWithContainer(container)
	}
}

type builder struct {
	*ssa.FunctionBuilder
	sourceFile *ast.SourceFile

	useStrict         bool
	contextLabelStack []string

	currentImportModule               string
	unresolvedCurrentImportModulePath string
	namedValueExports                 map[string]string // exportedName -> realName (exportedName may not be the same as realName in case of export alias)
	namedTypeExports                  map[string]string
	cjsExport                         string                       // export equal + require syntax only support one export per ts file
	reExports                         map[string]map[string]string // re-exported name -> (path -> exportName)

}

func Frontend(src string) (*ast.SourceFile, error) {
	tsast := parser.ParseSourceFile("", "", src, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	var err error
	if len(tsast.Diagnostics()) != 0 {
		err = utils.Errorf("parse AST FrontEnd error: %v", tsast.Diagnostics()[0].Message())
	}
	return tsast, err
}

type SSABuilder struct {
	*ssa.PreHandlerBase
}

func (*SSABuilder) FilterFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".ts" || ext == ".tsx" || ext == ".js"
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".ts" || ext == ".tsx" || ext == ".js"
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.TS
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return nil
}

func (*SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
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
		namedValueExports: make(map[string]string),
		namedTypeExports:  make(map[string]string),
		reExports:         make(map[string]map[string]string),
	}
	build.VisitSourceFile(jsAST)
	return nil
}
