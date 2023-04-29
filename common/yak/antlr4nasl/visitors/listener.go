package visitors

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type ErrorListener struct {
	handler func(msg string)
	antlr.DefaultErrorListener
}

func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	if el.handler != nil {
		el.handler(msg)
	}
}
func NewErrorListener(handle func(msg string)) *ErrorListener {
	return &ErrorListener{
		handler: handle,
	}
}

type ParseTreeListener struct {
	EnterEveryRuleHandle func(ctx antlr.ParserRuleContext)
	ExitEveryRuleHandle  func(ctx antlr.ParserRuleContext)
}

func NewParseTreeListener() *ParseTreeListener {
	return &ParseTreeListener{}
}
func (p *ParseTreeListener) SetEnter(h func(ctx antlr.ParserRuleContext)) {
	p.EnterEveryRuleHandle = h
}
func (p *ParseTreeListener) SetExit(h func(ctx antlr.ParserRuleContext)) {
	p.ExitEveryRuleHandle = h
}
func (p *ParseTreeListener) VisitTerminal(node antlr.TerminalNode) {

}

func (p *ParseTreeListener) VisitErrorNode(node antlr.ErrorNode) {

}

func (p *ParseTreeListener) EnterEveryRule(ctx antlr.ParserRuleContext) {
	p.EnterEveryRuleHandle(ctx)
}

func (p *ParseTreeListener) ExitEveryRule(ctx antlr.ParserRuleContext) {
	p.ExitEveryRuleHandle(ctx)
}
