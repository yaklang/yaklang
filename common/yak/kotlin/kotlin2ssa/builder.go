package kotlin2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	kotlinparser "github.com/yaklang/yaklang/common/yak/kotlin/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{PreHandlerBase: ssa.NewPreHandlerBase()}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) FilterFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".kt" || ext == ".kts"
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	return s.FilterFile(path)
}

func (s *SSABuilder) FilterPreHandlerFile(path string) bool {
	return s.FilterFile(path)
}

func (s *SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.KOTLIN
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return fs
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(kotlinparser.GetKotlinLexerSerializedATN(), kotlinparser.GetKotlinParserSerializedATN())
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return Frontend(src, cache)
}

func (s *SSABuilder) BuildFromAST(raw ssa.FrontAST, builder *ssa.FunctionBuilder) error {
	ast, ok := raw.(kotlinparser.IKotlinFileContext)
	if !ok || ast == nil {
		return utils.Errorf("invalid Kotlin AST type: %T", raw)
	}
	b := &singleFileBuilder{FunctionBuilder: builder}
	b.VisitKotlinFile(ast)
	return nil
}

type singleFileBuilder struct {
	*ssa.FunctionBuilder
	imports []string
}
