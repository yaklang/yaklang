package scala2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	scalaparser "github.com/yaklang/yaklang/common/yak/scala/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func Frontend(src string, caches ...*ssa.AntlrCache) (scalaparser.ICompilationUnitContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		scalaparser.NewScalaLexer,
		scalaparser.NewScalaParser,
		nil,
		func(lexer *scalaparser.ScalaLexer, parser *scalaparser.ScalaParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *scalaparser.ScalaParser) scalaparser.ICompilationUnitContext {
			return parser.CompilationUnit()
		},
	)
}
