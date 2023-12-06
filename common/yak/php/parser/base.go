package phpparser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type PHPLexerBase struct {
	*antlr.BaseLexer

	Interpreter     *antlr.LexerATNSimulator
	RuleNames       []string
	LiteralNames    []string
	SymbolicNames   []string
	GrammarFileName string

	// inline
	_scriptTag bool
	_styleTag  bool
}

func (p *PHPLexerBase) PushModeOnHtmlClose() {

}

func (p *PHPLexerBase) PopModeOnCurlyBracketClose() {

}

func (p *PHPLexerBase) SetInsideString() {

}

func (p *PHPLexerBase) IsNewLineOrStart(i int) bool {
	return false
}

func (p *PHPLexerBase) HasAspTags() bool {
	return false
}

func (p *PHPLexerBase) HasPhpScriptTag() bool {
	return false
}

func (p *PHPLexerBase) ShouldPushHereDocMode(i int) bool {
	return false
}

func (p *PHPLexerBase) IsCurlyDollar(i int) bool {
	return false
}
