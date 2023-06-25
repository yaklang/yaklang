package yakast

import (
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
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

func testYakParsingWithSource(i string) {
	inputStream := antlr.NewInputStream(i)
	lex := yak.NewYaklangLexer(inputStream)

	// for _, t := range lex.GetAllTokens() {
	// 	println(t.GetText())
	// }

	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)

	p := yak.NewYaklangParser(tokenStream)
	vt := NewYakCompiler()
	vt.AntlrTokenStream = tokenStream
	vt.VisitProgram(p.Program().(*yak.ProgramContext))
	println(vt.GetFormattedCode())
	vt.ShowOpcodesWithSource(i)
}

func TestSimpleYakAST(t *testing.T) {
	t.Run("simple func", func(t *testing.T) {
		expr := `
		func sum(a) {
			result = 0
			for i = range a {
				result = result +  i
			}
			return result
		}
		
		a=sum(2)
		dump(a)
		assert a == 3
			`
		testYakParsingWithSource(expr)
	})

	t.Run("nonsense txt", func(t *testing.T) {
		expr := `
		
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
			`
		testYakParsingWithSource(expr)
	})
}
