package lua2ssa

import (
	luaparser "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Frontend parses Lua source into the existing ANTLR4 Lua chunk context.
func Frontend(src string, caches ...*ssa.AntlrCache) (luaparser.IChunkContext, error) {
	return antlr4util.ParseASTWithSLLFirst(
		src,
		luaparser.NewLuaLexer,
		luaparser.NewLuaParser,
		nil,
		nil,
		func(parser *luaparser.LuaParser) luaparser.IChunkContext {
			return parser.Chunk()
		},
	)
}
