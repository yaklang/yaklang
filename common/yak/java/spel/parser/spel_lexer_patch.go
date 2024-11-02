package spelparser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type SpelLexerBase struct {
	*antlr.BaseLexer

	indent int
}
