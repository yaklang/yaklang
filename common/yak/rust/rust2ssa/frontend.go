package rust2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	rustparser "github.com/yaklang/yaklang/common/yak/rust/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func Frontend(src string, caches ...*ssa.AntlrCache) (rustparser.ICrateContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		rustparser.NewRustLexer,
		rustparser.NewRustParser,
		nil,
		func(lexer *rustparser.RustLexer, parser *rustparser.RustParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *rustparser.RustParser) rustparser.ICrateContext {
			return parser.Crate()
		},
	)
}
