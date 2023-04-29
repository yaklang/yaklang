package yakast

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "yaklang/common/yak/antlr4yak/parser"
	"testing"
)

func testYakParsing(i string) {
	inputStream := antlr.NewInputStream(i)
	lex := yak.NewYaklangLexer(inputStream)

	//for _, t := range lex.GetAllTokens() {
	//	println(t.GetText())
	//}

	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(tokenStream)
	vt := NewYakCompiler()
	vt.AntlrTokenStream = tokenStream
	vt.VisitProgram(p.Program().(*yak.ProgramContext))
	println(vt.formatted.String())
	vt.ShowOpcodes()
}

func TestYakAST(t *testing.T) {
	testYakParsing(`

   1
11
asdfa
asdf
asdf
asd
f;123;123  
  a=12
123
sadfas


sd
fa
sdf
asd
expression(123) 
a+1=123
`)
}

func TestYakAST_Cond(t *testing.T) {
	testYakParsing(`

`)
}
