package syntaxflow

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"testing"
)

func check(c string) {
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(c))
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	result := sfvm.NewSyntaxFlowVisitor[string, any]()
	result.VisitFlow(astParser.Flow())

	fmt.Println()
	fmt.Println(c)
	result.Show()
}

func TestSyntaxFlow_Basic(t *testing.T) {
	check(`$abc >> fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
}

func TestSyntaxFlow_Fetch(t *testing.T) {
	check(`fetch => [param] => {header: 2.(dict); path: 1.(str); } => $request`)
}
