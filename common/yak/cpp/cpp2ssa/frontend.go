package cpp2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Frontend parses one C++ translation unit and returns its parse tree.
//
// The grammar is the single CPP14 grammar; C++17/20/23 spellings that its lexer
// and parser accept are handled, and everything else degrades into a parse tree
// with error nodes instead of a hard failure.
func Frontend(src string, caches ...*ssa.AntlrCache) (cppparser.ITranslationUnitContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		cppparser.NewCPP14Lexer,
		cppparser.NewCPP14Parser,
		nil,
		func(lexer *cppparser.CPP14Lexer, parser *cppparser.CPP14Parser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *cppparser.CPP14Parser) cppparser.ITranslationUnitContext {
			return parser.TranslationUnit()
		},
	)
}
