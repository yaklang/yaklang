package spel

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	spelparser "github.com/yaklang/yaklang/common/yak/java/spel/parser"
)

func GetAST(code string) (*spelparser.SpelParser, error) {
	var parser any = antlr4util.GetASTParser(
		code,
		spelparser.NewSpelLexer,
		spelparser.NewSpelParser,
	)

	switch ret := parser.(type) {
	case *spelparser.SpelParser:
		return ret, nil
	}

	return nil, utils.Errorf("cannot fetch *spelparser.SpelParser, got: %T", parser)
}
