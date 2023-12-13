package syntaxflow

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"testing"
)

func check(c string) {
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(c))
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	astParser.Flow()
}

func TestSyntaxFlow_Basic(t *testing.T) {
	check(`>> fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
}
