package lua2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	luaparser "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SSABuilder builds SSA for Lua source files.
type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) FilterFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".lua")
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".lua")
}

func (s *SSABuilder) FilterPreHandlerFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".lua")
}

func (s *SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.LUA
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return fs
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return nil
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return Frontend(src, cache)
}

func (s *SSABuilder) BuildFromAST(raw ssa.FrontAST, builder *ssa.FunctionBuilder) error {
	ast, ok := raw.(luaparser.IChunkContext)
	if !ok || ast == nil {
		return utils.Errorf("invalid Lua AST type: %T", raw)
	}
	b := &singleFileBuilder{FunctionBuilder: builder}
	b.VisitChunk(ast)
	return nil
}

type singleFileBuilder struct {
	*ssa.FunctionBuilder
	functionDepth int
}
