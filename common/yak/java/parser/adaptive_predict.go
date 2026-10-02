package javaparser

import "github.com/yaklang/antlr/v4"

// SetFastPrediction selects the original ATN predictor for differential tests.
// Recovering strategies, LL and exact ambiguity diagnostics retain the ATN.
func (p *JavaParser) SetFastPrediction(enabled bool) { p.disableFastPrediction = !enabled }

// Prefix prediction relies on the complete compilation-unit follow context.
// Partial public rule entries can accept a different prefix without reporting
// an error, even with whole-pass cancellation. Track the root at rule entry,
// rather than walking its parent chain for every decision in deeply nested ASTs.
// A new root resets this flag, including after SetTokenStream or a failed pass.
func (p *JavaParser) EnterRule(ctx antlr.ParserRuleContext, state, rule int) {
	if p.GetParserRuleContext() == nil {
		p.compilationUnitPrediction = rule == JavaParserRULE_compilationUnit
	}
	p.BaseParser.EnterRule(ctx, state, rule)
}

func (p *JavaParser) EnterRecursionRule(ctx antlr.ParserRuleContext, state, rule, precedence int) {
	if p.GetParserRuleContext() == nil {
		p.compilationUnitPrediction = false
	}
	p.BaseParser.EnterRecursionRule(ctx, state, rule, precedence)
}

// The runtime resets contexts and the error strategy but leaves its recognition
// error set after a cancellation panic. Reusing the Java parser must start with
// a clean pass rather than immediately cancelling an unrelated valid input.
func (p *JavaParser) SetTokenStream(input antlr.TokenStream) {
	p.BaseParser.SetTokenStream(input)
	p.SetError(nil)
	p.compilationUnitPrediction = false
}

func (p *JavaParser) SetInputStream(input antlr.TokenStream) { p.SetTokenStream(input) }

// AdaptivePredict resolves syntactically distinct SLL prefixes without exploring
// complete initializers, call arguments or lambda bodies. The generated rules
// still validate every token and construct every AST node. Precedence predicates,
// ambiguous prefixes, scans on other token streams and exceeded bounds use ATN.
func (p *JavaParser) AdaptivePredict(base *antlr.BaseParser, input antlr.TokenStream, decision int, ctx antlr.ParserRuleContext) int {
	interpreter := p.GetInterpreter()
	if p.disableFastPrediction || !p.compilationUnitPrediction || interpreter.GetPredictionMode() != antlr.PredictionModeSLL {
		return interpreter.AdaptivePredict(base, input, decision, ctx)
	}
	strategy, marked := p.GetErrorHandler().(interface{ BailsOnSyntaxError() bool })
	bail := marked && strategy.BailsOnSyntaxError()
	if bail {
		state := p.GetATN().DecisionToState[decision]
		if state.GetStateType() == antlr.ATNStateBlockStart {
			switch state.GetRuleIndex() {
			case JavaParserRULE_expression:
				if len(state.GetTransitions()) == 14 {
					if alt := expressionPrefix(input); alt != 0 {
						return alt
					}
					if alt := typeReferencePrefix(input); alt != 0 {
						return alt
					}
				}
			case JavaParserRULE_primary:
				if len(state.GetTransitions()) == 7 {
					if alt := primaryPrefix(input); alt != 0 {
						return alt
					}
				}
			case JavaParserRULE_statement:
				if len(state.GetTransitions()) == 19 {
					if alt := statementPrefix(input); alt != 0 {
						return alt
					}
				}
			case JavaParserRULE_blockStatement:
				if len(state.GetTransitions()) == 3 {
					if alt := p.blockStatementPrefix(input); alt != 0 {
						return alt
					}
				}
			case JavaParserRULE_memberDeclaration:
				if len(state.GetTransitions()) == 10 {
					switch input.LA(1) {
					case JavaParserCLASS:
						return 9
					case JavaParserINTERFACE:
						return 7
					}
					if alt := p.declarationPrefix(input); alt != 0 {
						return alt
					}
				}
			case JavaParserRULE_lambdaParameters:
				if len(state.GetTransitions()) == 4 {
					if isIdentifier(input.LA(1)) {
						return 1
					}
					if input.LA(1) == JavaParserLPAREN && input.LA(2) == JavaParserRPAREN {
						return 2
					}
				}
			case JavaParserRULE_classOrInterfaceType:
				if len(state.GetTransitions()) == 2 {
					if input.LA(1) == JavaParserLT {
						return 1
					}
					return 2
				}
			}
		} else if state.GetStateType() == antlr.ATNStateStarLoopEntry && len(state.GetTransitions()) == 2 && state.GetRuleIndex() == JavaParserRULE_classOrInterfaceType {
			if alt := qualifiedTypePrefix(input); alt != 0 {
				return alt
			}
		}
	}
	return interpreter.AdaptivePredict(base, input, decision, ctx)
}

func expressionPrefix(input antlr.TokenStream) int {
	token := input.LA(1)
	if isLiteral(token) {
		return 1
	}
	if isIdentifier(token) {
		switch input.LA(2) {
		case JavaParserLPAREN:
			return 2
		case JavaParserARROW:
			return 14
		case JavaParserINC, JavaParserDEC:
			return 6
		case JavaParserASSIGN:
			return 13
		case JavaParserADD_ASSIGN, JavaParserSUB_ASSIGN, JavaParserMUL_ASSIGN, JavaParserDIV_ASSIGN,
			JavaParserAND_ASSIGN, JavaParserOR_ASSIGN, JavaParserXOR_ASSIGN, JavaParserRSHIFT_ASSIGN,
			JavaParserURSHIFT_ASSIGN, JavaParserLSHIFT_ASSIGN, JavaParserMOD_ASSIGN:
			return 12
		case JavaParserDOT, JavaParserLBRACK, JavaParserLT, JavaParserCOLONCOLON:
			// Class literals, array constructor references and generic types
			// overlap ordinary member, array and relational expressions.
			return 0
		default:
			return 1
		}
	}
	switch token {
	case JavaParserTHIS, JavaParserSUPER:
		if input.LA(2) == JavaParserLPAREN {
			return 2
		}
		return 1
	case JavaParserNEW:
		return 11
	case JavaParserSWITCH:
		return 5
	case JavaParserADD, JavaParserSUB, JavaParserBANG, JavaParserTILDE:
		return 7
	case JavaParserLPAREN:
		// These tokens cannot begin a type or a formal lambda parameter.
		next := input.LA(2)
		if isLiteral(next) {
			return 1
		}
		switch next {
		case JavaParserLPAREN, JavaParserTHIS, JavaParserSUPER, JavaParserNEW,
			JavaParserSWITCH, JavaParserADD, JavaParserSUB, JavaParserBANG, JavaParserTILDE,
			JavaParserINC, JavaParserDEC:
			return 1
		}
	}
	return 0
}

func primaryPrefix(input antlr.TokenStream) int {
	token := input.LA(1)
	if isLiteral(token) {
		return 4
	}
	if isIdentifier(token) {
		switch input.LA(2) {
		case JavaParserLT:
			return nonTypePrimaryPrefix(input)
		case JavaParserDOT, JavaParserLBRACK:
			return 0
		default:
			return 5
		}
	}
	switch token {
	case JavaParserLPAREN:
		return 1
	case JavaParserTHIS:
		return 2
	case JavaParserSUPER:
		return 3
	case JavaParserLT:
		return 7
	case JavaParserVOID:
		return 6
	}
	if isPrimitive(token) {
		return 6
	}
	return 0
}

func statementPrefix(input antlr.TokenStream) int {
	switch input.LA(1) {
	case JavaParserLBRACE:
		return 1
	case JavaParserASSERT:
		return 2
	case JavaParserIF:
		return 3
	case JavaParserFOR:
		return 4
	case JavaParserWHILE:
		return 5
	case JavaParserDO:
		return 6
	case JavaParserTRY:
		switch input.LA(2) {
		case JavaParserLBRACE:
			return 7
		case JavaParserLPAREN:
			return 8
		}
	case JavaParserSYNCHRONIZED:
		return 10
	case JavaParserRETURN:
		return 11
	case JavaParserTHROW:
		return 12
	case JavaParserBREAK:
		return 13
	case JavaParserCONTINUE:
		return 14
	case JavaParserSEMI:
		return 16
	case JavaParserYIELD, JavaParserSWITCH:
		// yield is also an identifier; switch has three overlapping forms.
		return 0
	default:
		if isIdentifier(input.LA(1)) {
			if input.LA(2) == JavaParserCOLON {
				return 19
			}
			return 17
		}
		if expressionPrefix(input) != 0 {
			return 17
		}
	}
	return 0
}

func (p *JavaParser) blockStatementPrefix(input antlr.TokenStream) int {
	if alt := statementPrefix(input); alt != 0 {
		if !isIdentifier(input.LA(1)) && !isPrimitive(input.LA(1)) {
			return 3
		}
		if isIdentifier(input.LA(1)) {
			switch input.LA(2) {
			case JavaParserLPAREN, JavaParserASSIGN, JavaParserINC, JavaParserDEC,
				JavaParserCOLON, JavaParserARROW,
				JavaParserADD_ASSIGN, JavaParserSUB_ASSIGN, JavaParserMUL_ASSIGN, JavaParserDIV_ASSIGN,
				JavaParserAND_ASSIGN, JavaParserOR_ASSIGN, JavaParserXOR_ASSIGN, JavaParserRSHIFT_ASSIGN,
				JavaParserURSHIFT_ASSIGN, JavaParserLSHIFT_ASSIGN, JavaParserMOD_ASSIGN:
				return 3
			}
		}
	}
	if p.declarationPrefix(input) == 4 {
		return 1
	}
	return 0
}

func isIdentifier(token int) bool {
	switch token {
	case JavaParserIDENTIFIER, JavaParserENUM, JavaParserMODULE, JavaParserOPEN, JavaParserREQUIRES,
		JavaParserEXPORTS, JavaParserOPENS, JavaParserTO, JavaParserUSES, JavaParserPROVIDES,
		JavaParserWITH, JavaParserTRANSITIVE, JavaParserYIELD, JavaParserSEALED, JavaParserPERMITS,
		JavaParserRECORD, JavaParserVAR:
		return true
	}
	return false
}

func isPrimitive(token int) bool {
	switch token {
	case JavaParserBOOLEAN, JavaParserCHAR, JavaParserBYTE, JavaParserSHORT,
		JavaParserINT, JavaParserLONG, JavaParserFLOAT, JavaParserDOUBLE:
		return true
	}
	return false
}

func isLiteral(token int) bool {
	switch token {
	case JavaParserDECIMAL_LITERAL, JavaParserHEX_LITERAL, JavaParserOCT_LITERAL,
		JavaParserBINARY_LITERAL, JavaParserFLOAT_LITERAL, JavaParserHEX_FLOAT_LITERAL,
		JavaParserBOOL_LITERAL, JavaParserCHAR_LITERAL, JavaParserSTRING_LITERAL, JavaParserTEXT_BLOCK,
		JavaParserNULL_LITERAL:
		return true
	}
	return false
}
