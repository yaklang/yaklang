package cpp2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Frontend parses one C++ translation unit and returns its parse tree.
//
// The grammar is the single C++ grammar; C++17/20/23 spellings that its lexer
// and parser accept are handled, and everything else degrades into a parse tree
// with error nodes instead of a hard failure.
func Frontend(src string, caches ...*ssa.AntlrCache) (cppparser.ITranslationUnitContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	return antlr4util.ParseASTWithSLLFirst(
		src,
		cppparser.NewCPPLexer,
		cppparser.NewCPPParser,
		nil,
		func(lexer *cppparser.CPPLexer, parser *cppparser.CPPParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *cppparser.CPPParser) cppparser.ITranslationUnitContext {
			return parser.TranslationUnit()
		},
	)
}
