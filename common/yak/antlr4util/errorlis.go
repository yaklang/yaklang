package antlr4util

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// error listener for lexer and parser
type ErrorListener struct {
	err []string
	*antlr.DefaultErrorListener
}

func (el *ErrorListener) GetErrors() []string {
	return el.err
}

func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	el.err = append(el.err, msg)
}

func NewErrorListener() *ErrorListener {
	return &ErrorListener{
		err:                  []string{},
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}

// error listener for lexer and parser
type LegacyErrorListener struct {
	err []string
	*antlr.DefaultErrorListener
}

func (el *LegacyErrorListener) GetErrors() []string {
	return el.err
}

func (el *LegacyErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	el.err = append(el.err, msg)
}

func NewLegacyErrorListener() *LegacyErrorListener {
	return &LegacyErrorListener{
		err:                  []string{},
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}
