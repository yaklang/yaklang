package kotlin2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	kotlinparser "github.com/yaklang/yaklang/common/yak/kotlin/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func Frontend(src string, caches ...*ssa.AntlrCache) (kotlinparser.IKotlinFileContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		kotlinparser.NewKotlinLexer,
		kotlinparser.NewKotlinParser,
		nil,
		func(lexer *kotlinparser.KotlinLexer, parser *kotlinparser.KotlinParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *kotlinparser.KotlinParser) kotlinparser.IKotlinFileContext {
			return parser.KotlinFile()
		},
	)
}
