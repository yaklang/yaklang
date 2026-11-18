package scalaparser

import "github.com/yaklang/antlr/v4"

// ScalaLexerBase injects INDENT/DEDENT for indentation-sensitive Scala 3.
// Ported from grammars-v4 scala/scala3/Java/Scala3LexerBase.java.
type scalaRegion int

const (
	scalaRegionTopLevel scalaRegion = iota
	scalaRegionIndented
	scalaRegionInBraces
	scalaRegionInParens
)

type ScalaLexerBase struct {
	*antlr.BaseLexer

	regionStack              []scalaRegion
	indentLengthStack        []int
	pendingTokens            []antlr.Token
	previousPendingTokenType int
	lastNonHiddenType        int
	curToken                 antlr.Token
	ffgToken                 antlr.Token
}

func (l *ScalaLexerBase) initScalaState() {
	l.regionStack = nil
	l.indentLengthStack = nil
	l.pendingTokens = nil
	l.previousPendingTokenType = 0
	l.lastNonHiddenType = 0
	l.curToken = nil
	l.ffgToken = nil
}

func (l *ScalaLexerBase) Reset() {
	l.initScalaState()
	l.BaseLexer.Reset()
}

func (l *ScalaLexerBase) NextToken() antlr.Token {
	l.checkNextToken()
	token := l.pendingTokens[0]
	l.pendingTokens = l.pendingTokens[1:]
	return token
}

func (l *ScalaLexerBase) checkNextToken() {
	if l.previousPendingTokenType == antlr.TokenEOF {
		return
	}
	if len(l.regionStack) == 0 {
		l.regionStack = append(l.regionStack, scalaRegionTopLevel)
		l.indentLengthStack = append(l.indentLengthStack, 0)
		l.setCurrentAndFollowingTokens()
		l.handleStartOfInput()
	} else {
		l.setCurrentAndFollowingTokens()
	}

	tokenType := l.curToken.GetTokenType()
	switch tokenType {
	case ScalaLexerNEWLINE:
		l.handleNewlineToken()
	case ScalaLexerLPAREN, ScalaLexerLBRACKET:
		l.regionStack = append(l.regionStack, scalaRegionInParens)
		l.addPendingToken(l.curToken)
	case ScalaLexerCOMMA:
		for len(l.regionStack) > 0 && l.peekRegion() == scalaRegionIndented {
			l.popIndent()
			l.createAndAddPendingToken(ScalaLexerDEDENT, antlr.TokenDefaultChannel, "<DEDENT>", l.curToken)
		}
		l.addPendingToken(l.curToken)
	case ScalaLexerRPAREN, ScalaLexerRBRACKET:
		for len(l.regionStack) > 0 && l.peekRegion() == scalaRegionIndented {
			l.popIndent()
			l.createAndAddPendingToken(ScalaLexerDEDENT, antlr.TokenDefaultChannel, "<DEDENT>", l.curToken)
		}
		if len(l.regionStack) > 0 && l.peekRegion() == scalaRegionInParens {
			l.regionStack = l.regionStack[:len(l.regionStack)-1]
		}
		l.addPendingToken(l.curToken)
	case ScalaLexerLBRACE:
		l.regionStack = append(l.regionStack, scalaRegionInBraces)
		l.addPendingToken(l.curToken)
	case ScalaLexerRBRACE:
		if len(l.regionStack) > 0 && l.peekRegion() == scalaRegionInBraces {
			l.regionStack = l.regionStack[:len(l.regionStack)-1]
		}
		l.addPendingToken(l.curToken)
	case antlr.TokenEOF:
		l.handleEofToken()
	default:
		l.addPendingToken(l.curToken)
	}
}

func (l *ScalaLexerBase) setCurrentAndFollowingTokens() {
	if l.ffgToken != nil {
		l.curToken = l.ffgToken
	} else {
		l.curToken = l.BaseLexer.NextToken()
	}
	if l.curToken.GetTokenType() == antlr.TokenEOF {
		l.ffgToken = l.curToken
	} else {
		l.ffgToken = l.BaseLexer.NextToken()
	}
}

func (l *ScalaLexerBase) handleStartOfInput() {
	for l.curToken.GetTokenType() == ScalaLexerNEWLINE {
		l.hideAndAddPendingToken(l.curToken)
		l.setCurrentAndFollowingTokens()
	}
}

func (l *ScalaLexerBase) handleNewlineToken() {
	nlToken := cloneScalaToken(l.curToken)
	hasLeadingWS := l.ffgToken.GetTokenType() == ScalaLexerWS
	if hasLeadingWS {
		l.setCurrentAndFollowingTokens()
	}
	if l.ffgToken.GetTokenType() == ScalaLexerNEWLINE {
		l.hideAndAddPendingToken(nlToken)
		if hasLeadingWS {
			l.hideAndAddPendingToken(l.curToken)
		}
		return
	}

	top := l.peekRegion()
	newIndent := 0
	if l.ffgToken.GetTokenType() == antlr.TokenEOF {
		newIndent = 0
	} else if hasLeadingWS {
		newIndent = scalaIndentLength(l.curToken.GetText())
	}
	isDot := l.ffgToken.GetTokenType() == ScalaLexerDOT
	curIndent := 0
	if len(l.indentLengthStack) > 0 {
		curIndent = l.indentLengthStack[len(l.indentLengthStack)-1]
	}
	rparenOpensIndent := l.lastNonHiddenType == ScalaLexerRPAREN &&
		top != scalaRegionInParens && top != scalaRegionInBraces &&
		l.ffgToken.GetTokenType() != ScalaLexerEXTENDS &&
		l.ffgToken.GetTokenType() != ScalaLexerWITH
	willIndent := newIndent > curIndent && !isDot &&
		(scalaCanStartIndent(l.lastNonHiddenType) || rparenOpensIndent)

	surfaceNewline := false
	if newIndent < curIndent || willIndent {
		surfaceNewline = false
	} else if top == scalaRegionInParens {
		surfaceNewline = false
	} else if top == scalaRegionInBraces {
		surfaceNewline = !isDot &&
			scalaCanEndStat(l.lastNonHiddenType) &&
			scalaCanStartStat(l.ffgToken.GetTokenType()) &&
			!scalaIsStatContinuation(l.ffgToken.GetTokenType())
	} else if newIndent > curIndent {
		surfaceNewline = false
	} else {
		surfaceNewline = !isDot &&
			l.ffgToken.GetTokenType() != antlr.TokenEOF &&
			l.ffgToken.GetTokenType() != ScalaLexerRBRACE &&
			scalaCanEndStat(l.lastNonHiddenType) &&
			scalaCanStartStat(l.ffgToken.GetTokenType()) &&
			!scalaIsStatContinuation(l.ffgToken.GetTokenType())
	}

	if surfaceNewline {
		l.addPendingToken(nlToken)
	} else {
		l.hideAndAddPendingToken(nlToken)
	}
	if hasLeadingWS {
		l.hideAndAddPendingToken(l.curToken)
	}

	if newIndent < curIndent {
		l.insertDedentTokens(newIndent, l.ffgToken)
		newTop := l.peekRegion()
		if newTop != scalaRegionInParens && !isDot &&
			l.ffgToken.GetTokenType() != antlr.TokenEOF &&
			l.ffgToken.GetTokenType() != ScalaLexerRBRACE &&
			scalaCanEndStat(l.lastNonHiddenType) &&
			scalaCanStartStat(l.ffgToken.GetTokenType()) &&
			!scalaIsStatContinuation(l.ffgToken.GetTokenType()) {
			l.addPendingToken(cloneScalaToken(nlToken))
		}
	} else if willIndent {
		l.indentLengthStack = append(l.indentLengthStack, newIndent)
		l.regionStack = append(l.regionStack, scalaRegionIndented)
		l.createAndAddPendingToken(ScalaLexerINDENT, antlr.TokenDefaultChannel, "<INDENT>", l.ffgToken)
	}
}

func (l *ScalaLexerBase) insertDedentTokens(newIndent int, anchor antlr.Token) {
	for len(l.regionStack) > 0 && l.peekRegion() == scalaRegionIndented {
		if l.indentLengthStack[len(l.indentLengthStack)-1] <= newIndent {
			break
		}
		l.popIndent()
		l.createAndAddPendingToken(ScalaLexerDEDENT, antlr.TokenDefaultChannel, "<DEDENT>", anchor)
	}
}

func (l *ScalaLexerBase) handleEofToken() {
	for len(l.regionStack) > 0 && l.peekRegion() == scalaRegionIndented {
		l.popIndent()
		l.createAndAddPendingToken(ScalaLexerDEDENT, antlr.TokenDefaultChannel, "<DEDENT>", l.curToken)
	}
	l.addPendingToken(l.curToken)
}

func (l *ScalaLexerBase) peekRegion() scalaRegion {
	if len(l.regionStack) == 0 {
		return scalaRegionTopLevel
	}
	return l.regionStack[len(l.regionStack)-1]
}

func (l *ScalaLexerBase) popIndent() {
	if len(l.indentLengthStack) > 0 {
		l.indentLengthStack = l.indentLengthStack[:len(l.indentLengthStack)-1]
	}
	if len(l.regionStack) > 0 {
		l.regionStack = l.regionStack[:len(l.regionStack)-1]
	}
}

func (l *ScalaLexerBase) addPendingToken(token antlr.Token) {
	l.previousPendingTokenType = token.GetTokenType()
	if token.GetChannel() == antlr.TokenDefaultChannel {
		l.lastNonHiddenType = token.GetTokenType()
	}
	l.pendingTokens = append(l.pendingTokens, token)
}

func (l *ScalaLexerBase) hideAndAddPendingToken(token antlr.Token) {
	l.addPendingToken(cloneScalaTokenWithChannel(token, antlr.TokenHiddenChannel))
}

func (l *ScalaLexerBase) createAndAddPendingToken(tokenType, channel int, text string, anchor antlr.Token) {
	token := antlr.NewCommonToken(anchor.GetSource(), tokenType, channel, anchor.GetStart(), anchor.GetStart()-1)
	token.SetText(text)
	l.addPendingToken(token)
}

func cloneScalaToken(token antlr.Token) *antlr.CommonToken {
	return cloneScalaTokenWithChannel(token, token.GetChannel())
}

func cloneScalaTokenWithChannel(token antlr.Token, channel int) *antlr.CommonToken {
	clone := antlr.NewCommonToken(token.GetSource(), token.GetTokenType(), channel, token.GetStart(), token.GetStop())
	clone.SetText(token.GetText())
	return clone
}

func scalaIndentLength(text string) int {
	length := 0
	for _, c := range text {
		switch c {
		case ' ':
			length++
		case '\t':
			length = (length/8 + 1) * 8
		case '\f':
			length = 0
		}
	}
	return length
}

func scalaCanEndStat(tokenType int) bool {
	switch tokenType {
	case ScalaLexerId, ScalaLexerVarid, ScalaLexerBacktickId, ScalaLexerOp,
		ScalaLexerIntegerLiteral, ScalaLexerFloatingPointLiteral, ScalaLexerBooleanLiteral,
		ScalaLexerCharacterLiteral, ScalaLexerStringLiteral, ScalaLexerInterpolatedStringLiteral,
		ScalaLexerSymbolLiteral, ScalaLexerNullLiteral, ScalaLexerQuoteId, ScalaLexerUSCORE,
		ScalaLexerTHIS, ScalaLexerSUPER, ScalaLexerRETURN, ScalaLexerTYPE, ScalaLexerGIVEN,
		ScalaLexerRPAREN, ScalaLexerRBRACE, ScalaLexerRBRACKET, ScalaLexerDEDENT,
		ScalaLexerNEWLINE, ScalaLexerIF, ScalaLexerWHILE, ScalaLexerFOR, ScalaLexerMATCH,
		ScalaLexerTRY, ScalaLexerVAL, ScalaLexerNEW, ScalaLexerEXTENSION:
		return true
	default:
		return false
	}
}

func scalaIsStatContinuation(tokenType int) bool {
	switch tokenType {
	case ScalaLexerTHEN, ScalaLexerELSE, ScalaLexerDO, ScalaLexerCATCH,
		ScalaLexerFINALLY, ScalaLexerYIELD, ScalaLexerMATCH:
		return true
	default:
		return false
	}
}

func scalaCanStartStat(tokenType int) bool {
	switch tokenType {
	case ScalaLexerId, ScalaLexerVarid, ScalaLexerBacktickId, ScalaLexerOp,
		ScalaLexerIntegerLiteral, ScalaLexerFloatingPointLiteral, ScalaLexerBooleanLiteral,
		ScalaLexerCharacterLiteral, ScalaLexerStringLiteral, ScalaLexerInterpolatedStringLiteral,
		ScalaLexerSymbolLiteral, ScalaLexerNullLiteral, ScalaLexerQuoteId, ScalaLexerUSCORE,
		ScalaLexerTHIS, ScalaLexerSUPER, ScalaLexerNEW, ScalaLexerRETURN, ScalaLexerTHROW,
		ScalaLexerIF, ScalaLexerWHILE, ScalaLexerFOR, ScalaLexerTRY, ScalaLexerLBRACE,
		ScalaLexerLPAREN, ScalaLexerLBRACKET, ScalaLexerQUOTE, ScalaLexerINDENT, ScalaLexerAT,
		ScalaLexerCASE, ScalaLexerEND, ScalaLexerDEF, ScalaLexerVAL, ScalaLexerVAR, ScalaLexerTYPE,
		ScalaLexerGIVEN, ScalaLexerABSTRACT, ScalaLexerFINAL, ScalaLexerPRIVATE, ScalaLexerPROTECTED,
		ScalaLexerOVERRIDE, ScalaLexerSEALED, ScalaLexerCLASS, ScalaLexerTRAIT, ScalaLexerOBJECT,
		ScalaLexerENUM, ScalaLexerIMPORT, ScalaLexerEXPORT, ScalaLexerPACKAGE, ScalaLexerINLINE,
		ScalaLexerLAZY, ScalaLexerIMPLICIT, ScalaLexerEXTENSION, ScalaLexerOPEN, ScalaLexerINFIX,
		ScalaLexerTRANSPARENT, ScalaLexerOPAQUE, ScalaLexerAS, ScalaLexerDERIVES, ScalaLexerUSING:
		return true
	default:
		return false
	}
}

func scalaCanStartIndent(tokenType int) bool {
	switch tokenType {
	case ScalaLexerTHEN, ScalaLexerELSE, ScalaLexerDO, ScalaLexerCATCH, ScalaLexerFINALLY,
		ScalaLexerYIELD, ScalaLexerMATCH, ScalaLexerCOLON, ScalaLexerWITH, ScalaLexerASSIGN,
		ScalaLexerARROW, ScalaLexerCTXARROW, ScalaLexerLARROW, ScalaLexerWHILE, ScalaLexerTRY,
		ScalaLexerFOR, ScalaLexerIF, ScalaLexerTHROW, ScalaLexerRETURN:
		return true
	default:
		return false
	}
}
