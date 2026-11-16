package bash2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

	"mvdan.cc/sh/v3/syntax"
)

// SSABuilder compiles bash into SSA. Unlike the other SSA frontends it does not
// use an ANTLR grammar: bash quoting, heredocs and expansions are represented
// directly by mvdan/sh's typed AST, so there is no generated parser or ANTLR
// cache to manage.
type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.BASH
}

func (*SSABuilder) FilterFile(path string) bool {
	return isBashSourceFile(path)
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	return isBashSourceFile(path)
}

func (*SSABuilder) FilterParseAST(path string) bool {
	return isBashSourceFile(path)
}

func isBashSourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sh", ".bash":
		return true
	default:
		return false
	}
}

// GetAntlrCache is nil by design: the AST comes from mvdan/sh, not ANTLR.
func (*SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return nil
}

func (*SSABuilder) ParseAST(src string, _ *ssa.AntlrCache) (ssa.FrontAST, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return nil, utils.Wrap(err, "bash parse failed")
	}
	return file, nil
}

func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, builder *ssa.FunctionBuilder) error {
	file, ok := raw.(*syntax.File)
	if !ok {
		return utils.Errorf("invalid bash AST type: %T", raw)
	}
	newASTBuilder(builder).buildFile(file)
	return nil
}

func (*SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return fs
}
