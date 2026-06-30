package spelparser

import "github.com/yaklang/antlr/v4"

type SpelLexerBase struct {
	*antlr.BaseLexer

	indent int
}
