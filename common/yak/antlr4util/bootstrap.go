package antlr4util

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type ErrorBaseInterface interface {
	RemoveErrorListeners()
	AddErrorListener(listener *ErrorListener)
}

func GetASTParser[T antlr.Lexer, Y antlr.Parser](
	code string,
	lexer func(source antlr.CharStream) T,
	parser func(stream antlr.TokenStream) Y) Y {
	el := NewErrorListener()
	lexerIns := lexer(antlr.NewInputStream(code))
	lexerIns.RemoveErrorListeners()
	lexerIns.AddErrorListener(el)
	ts := antlr.NewCommonTokenStream(lexerIns, antlr.TokenDefaultChannel)
	parserIns := parser(ts)
	parserIns.RemoveErrorListeners()
	parserIns.AddErrorListener(el)
	parserIns.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	return parserIns
}
