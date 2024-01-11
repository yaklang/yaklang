package JS

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// JavaScriptLexerBase state
type JavaScriptLexerBase struct {
	*antlr.BaseLexer

	scopeStrictModes []bool
	stackLength      int
	stackIx          int

	lastToken        antlr.Token
	useStrictDefault bool
	useStrictCurrent bool
	templateDepth    int

	waitForCloseBrace antlr.Token
	BeforeWords       [2]antlr.Token
	keyWordsMap       map[int]bool
	handlePlusMinus   antlr.Token
}

func (l *JavaScriptLexerBase) IsStartOfFile() bool {
	return l.lastToken == nil
}

func (l *JavaScriptLexerBase) pushStrictModeScope(v bool) {
	if l.stackIx == l.stackLength {
		l.scopeStrictModes = append(l.scopeStrictModes, v)
		l.stackLength++
	} else {
		l.scopeStrictModes[l.stackIx] = v
	}
	l.stackIx++
}

func (l *JavaScriptLexerBase) popStrictModeScope() bool {
	l.stackIx--
	v := l.scopeStrictModes[l.stackIx]
	l.scopeStrictModes[l.stackIx] = false
	return v
}

// IsStrictMode is self explanatory.
func (l *JavaScriptLexerBase) IsStrictMode() bool {
	return l.useStrictCurrent
}

// NextToken from the character stream.
func (l *JavaScriptLexerBase) NextToken() antlr.Token {
	if l.waitForCloseBrace != nil {
		token := l.waitForCloseBrace
		l.waitForCloseBrace = nil
		return token
	}

	if l.handlePlusMinus != nil {
		token := l.handlePlusMinus
		l.handlePlusMinus = nil
		return token
	}

	if l.keyWordsMap == nil {
		m := make(map[int]bool, 6)
		m[JavaScriptLexerReturn] = true
		m[JavaScriptLexerBreak] = true
		m[JavaScriptLexerContinue] = true
		m[JavaScriptLexerThrow] = true
		m[JavaScriptLexerYield] = true
		m[JavaScriptLexerAsync] = true
		l.keyWordsMap = m
	}

	next := l.BaseLexer.NextToken() // Get next token

	if next.GetTokenType() > 0 {
		l.BeforeWords[0] = l.BeforeWords[1]
		l.BeforeWords[1] = next
	}

	if next.GetChannel() == antlr.TokenDefaultChannel {
		// Keep track of the last token on default channel
		l.lastToken = next
	}

	// 预读一位处理++
	if next.GetTokenType() == JavaScriptLexerLineTerminator {
		Next := l.BaseLexer.NextToken() // Get next next token
		// 是++则加一个;
		if Next != nil && (Next.GetTokenType() == JavaScriptLexerPlusPlus || Next.GetTokenType() == JavaScriptLexerMinusMinus) {
			semit := l.GetTokenFactory().Create(
				l.GetTokenSourceCharStreamPair(), JavaScriptLexerSemiColon, ";", antlr.TokenDefaultChannel,
				next.GetStart(), next.GetStop(),
				next.GetLine(), next.GetColumn(),
			)
			// 存预读位
			l.handlePlusMinus = Next
			next = semit
			l.lastToken = next
			return semit
		}
		l.handlePlusMinus = Next
	}

	// 判断是否是},对其进行加;优化
	if next.GetTokenType() == JavaScriptLexerCloseBrace {
		semit := l.GetTokenFactory().Create(
			l.GetTokenSourceCharStreamPair(), JavaScriptLexerSemiColon, ";", next.GetChannel(),
			next.GetStart(), next.GetStop()-1,
			next.GetLine(), next.GetColumn(),
		)
		l.waitForCloseBrace = next
		next = semit
	}

	return next
}

// ProcessOpenBrace is called when a { is encountered during
// lexing, we push a new scope everytime.
func (l *JavaScriptLexerBase) ProcessOpenBrace() {
	l.useStrictCurrent = l.useStrictDefault
	if l.stackIx > 0 && l.scopeStrictModes[l.stackIx-1] {
		l.useStrictCurrent = true
	}
	l.pushStrictModeScope(l.useStrictCurrent)
}

// ProcessCloseBrace is called when a } is encountered during
// lexing, we pop a scope unless we're inside global scope.
func (l *JavaScriptLexerBase) ProcessCloseBrace() {
	l.useStrictCurrent = l.useStrictDefault
	if l.stackIx > 0 {
		l.useStrictCurrent = l.popStrictModeScope()
	}
}

// ProcessStringLiteral is called when lexing a string literal.
func (l *JavaScriptLexerBase) ProcessStringLiteral() {
	if l.lastToken == nil || l.lastToken.GetTokenType() == JavaScriptLexerOpenBrace {
		if l.GetText() == `"use strict"` || l.GetText() == "'use strict'" {
			if l.stackIx > 0 {
				l.popStrictModeScope()
			}
			l.useStrictCurrent = true
			l.pushStrictModeScope(l.useStrictCurrent)
		}
	}
}

// IsRegexPossible returns true if the lexer can match a
// regex literal.
func (l *JavaScriptLexerBase) IsRegexPossible() bool {
	if l.lastToken == nil {
		return true
	}
	switch l.lastToken.GetTokenType() {
	case JavaScriptLexerIdentifier, JavaScriptLexerNullLiteral,
		JavaScriptLexerBooleanLiteral, JavaScriptLexerThis,
		JavaScriptLexerCloseBracket, JavaScriptLexerCloseParen,
		JavaScriptLexerOctalIntegerLiteral, JavaScriptLexerDecimalLiteral,
		JavaScriptLexerHexIntegerLiteral, JavaScriptLexerStringLiteral,
		JavaScriptLexerPlusPlus, JavaScriptLexerMinusMinus:
		return false
	default:
		return true
	}
}

func (l *JavaScriptLexerBase) IncreaseTemplateDepth() {
	l.templateDepth++
}

func (l *JavaScriptLexerBase) DecreaseTemplateDepth() {
	l.templateDepth--
}

func (l *JavaScriptLexerBase) IsInTemplateString() bool {
	return l.templateDepth > 0
}

func (l *JavaScriptLexerBase) Reset() {
	l.scopeStrictModes = nil
	l.stackLength = 0
	l.stackIx = 0
	l.lastToken = nil
	l.useStrictDefault = false
	l.useStrictCurrent = false
	l.templateDepth = 0
	// l.BaseLexer.Reset()
}

func (l *JavaScriptLexerBase) afterKeyword() bool {
	var token, dot int
	if l.BeforeWords[1] == nil {
		token = 0
	} else {
		token = l.BeforeWords[1].GetTokenType()
	}

	if l.BeforeWords[0] == nil {
		dot = 0
	} else {
		dot = l.BeforeWords[0].GetTokenType()
	}

	_, ok := l.keyWordsMap[token]
	return ok && (dot != JavaScriptLexerDot)
}
