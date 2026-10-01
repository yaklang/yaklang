package parser

import "github.com/yaklang/antlr/v4"

// SetFastPrediction allows differential tests to use the original ATN predictor.
// LL recovery and exact ambiguity diagnostics always use the original predictor.
func (p *YaklangParser) SetFastPrediction(enabled bool) {
	p.disableFastPrediction = !enabled
}

// AdaptivePredict avoids exploring an entire statement when its first one or
// two tokens identify a single alternative, or a bounded delimiter scan finds
// its terminator. It does not interpret bodies, change the ATN, construct nodes,
// or bypass expression precedence predicates.
// Unknown prefixes and all LL predictions use the original implementation.
func (p *YaklangParser) AdaptivePredict(base *antlr.BaseParser, input antlr.TokenStream, decision int, ctx antlr.ParserRuleContext) int {
	interpreter := p.GetInterpreter()
	if !p.disableFastPrediction && interpreter.GetPredictionMode() == antlr.PredictionModeSLL {
		state := p.GetATN().DecisionToState[decision]
		switch state.GetRuleIndex() {
		case YaklangParserRULE_statement:
			if state.GetStateType() == antlr.ATNStateBlockStart && len(state.GetTransitions()) == 20 {
				if alt := p.statementPrefix(input); alt != 0 {
					return alt
				}
			}
		case YaklangParserRULE_statementList:
			if state.GetStateType() == antlr.ATNStatePlusLoopBack && len(state.GetTransitions()) == 2 {
				if p.statementPrefix(input) != 0 {
					return 1
				}
				switch input.LA(1) {
				case antlr.TokenEOF, YaklangParserRBrace, YaklangParserCase, YaklangParserDefault:
					return 2
				}
			}
		case YaklangParserRULE_expression:
			if state.GetStateType() == antlr.ATNStateBlockStart && len(state.GetTransitions()) == 10 {
				switch input.LA(1) {
				case YaklangParserIdentifier:
					if input.LA(2) != YaklangParserEqGt {
						return 6
					}
				case YaklangParserIntegerLiteral, YaklangParserFloatLiteral,
					YaklangParserCharacterLiteral, YaklangParserStringLiteral,
					YaklangParserStartNowDoc, YaklangParserTrue, YaklangParserFalse,
					YaklangParserNilLiteral, YaklangParserUndefinedLiteral,
					YaklangParserTemplateSingleQuoteStringStart,
					YaklangParserTemplateDoubleQuoteStringStart,
					YaklangParserTemplateBackTickStringStart, YaklangParserLBrace:
					return 2
				case YaklangParserLBracket:
					if input.LA(2) != YaklangParserRBracket {
						return 2
					}
				case YaklangParserFunc:
					switch input.LA(2) {
					case YaklangParserLBrace:
						return 8
					case YaklangParserLParen, YaklangParserIdentifier:
						return 3
					}
				case YaklangParserPanic:
					return 4
				case YaklangParserRecover:
					return 5
				case YaklangParserMake:
					return 9
				case YaklangParserNot, YaklangParserSub, YaklangParserPlus,
					YaklangParserXor, YaklangParserAmp, YaklangParserMul, YaklangParserChanIn:
					return 10
				}
			}
		}
	}
	return interpreter.AdaptivePredict(base, input, decision, ctx)
}

// Alternatives follow statement in YaklangParser.g4. Contextual select, for,
// brace/map literals, comments, declarations and unknown expression prefixes
// remain with the ATN because they overlap other grammar alternatives.
func (p *YaklangParser) statementPrefix(input antlr.TokenStream) int {
	switch input.LA(1) {
	case YaklangParserIdentifier:
		switch input.LA(2) {
		case YaklangParserAssignEq, YaklangParserColonAssignEq,
			YaklangParserPlusPlus, YaklangParserSubSub,
			YaklangParserPlusEq, YaklangParserMinusEq, YaklangParserMulEq,
			YaklangParserDivEq, YaklangParserModEq, YaklangParserAmpEq,
			YaklangParserBitAndEq, YaklangParserBitOrEq, YaklangParserLtLtEq,
			YaklangParserGtGtEq, YaklangParserBitAndNotEq:
			return 3
		}
		return p.suffixStatementPrefix(input)
	case YaklangParserTry:
		return 6
	case YaklangParserIf:
		return 9
	case YaklangParserSwitch:
		return 10
	case YaklangParserBreak:
		return 13
	case YaklangParserReturn:
		return 14
	case YaklangParserContinue:
		return 15
	case YaklangParserFallthrough:
		return 16
	case YaklangParserInclude:
		return 17
	case YaklangParserDefer:
		return 18
	case YaklangParserGo:
		return 19
	case YaklangParserAssert:
		return 20
	}
	return 0
}
