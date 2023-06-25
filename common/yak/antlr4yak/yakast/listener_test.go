package yakast_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func testYakParserWalKing(i string) {
	// include
	file, err := os.CreateTemp("", "test*.yak")
	if err != nil {
		panic(err)
	}
	file.WriteString(i)
	defer os.Remove(file.Name())

	inputStream, _ := antlr.NewFileStream(file.Name())
	lex := yak.NewYaklangLexer(inputStream)

	// for _, t := range lex.GetAllTokens() {
	// fmt.Println(t.GetText())
	// }

	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)

	p := yak.NewYaklangParser(tokenStream)
	p.BuildParseTrees = true
	tree := p.Program()
	listener := visitors.NewParseTreeListener()
	listener.SetEnter(func(ctx antlr.ParserRuleContext) {})
	listener.SetExit(func(ctx antlr.ParserRuleContext) {
		fmt.Printf("rule->%d, text->[%s]\n", ctx.GetRuleIndex(), ctx.GetText())
	})
	antlr.ParseTreeWalkerDefault.Walk(listener, tree)

}

func TestYakParserASTWalking(t *testing.T) {
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
		testYakParserWalKing(expr)
	})

}

func testYakLexerWalKing(i string) {
	// include
	file, err := os.CreateTemp("", "test*.yak")
	if err != nil {
		panic(err)
	}
	file.WriteString(i)
	defer os.Remove(file.Name())

	inputStream, _ := antlr.NewFileStream(file.Name())
	lex := yak.NewYaklangLexer(inputStream)

	for _, t := range lex.GetAllTokens() {
		fmt.Printf("token->[%s], type->%d\n", t.GetText(), t.GetTokenType())
	}
}

func TestYakTokenWalking(t *testing.T) {
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
		testYakLexerWalKing(expr)
	})

	t.Run("non sense txt", func(t *testing.T) {
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
		testYakLexerWalKing(expr)
	})

}
