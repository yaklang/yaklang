package pythonparser

import (
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

// SetFastPrediction permits differential testing against the original ATN.
// Recovering strategies, LL and exact ambiguity diagnostics retain the ATN.
func (p *PythonParser) SetFastPrediction(enabled bool) { p.disableFastPrediction = !enabled }

// AdaptivePredict avoids exploring complete expressions and suites when their
// first tokens already distinguish the alternatives. Most lookahead is at most
// two visible tokens; root/tuple boundaries use a bounded scanner. Neither path
// seeks/consumes or has a source-dependent cache. Rule, state kind and
// alternative-count guards keep unrelated decisions on the ATN.
func (p *PythonParser) AdaptivePredict(base *antlr.BaseParser, input antlr.TokenStream, decision int, ctx antlr.ParserRuleContext) int {
	interpreter := p.GetInterpreter()
	if p.disableFastPrediction || interpreter.GetPredictionMode() != antlr.PredictionModeSLL {
		return interpreter.AdaptivePredict(base, input, decision, ctx)
	}
	_, bail := p.GetErrorHandler().(*antlr4util.BailErrorStrategy)
	if !bail {
		_, bail = p.GetErrorHandler().(*antlr.BailErrorStrategy)
	}
	if bail {
		state := p.GetATN().DecisionToState[decision]
		if state.GetStateType() == antlr.ATNStateBlockStart {
			switch state.GetRuleIndex() {
			case PythonParserRULE_root:
				if len(state.GetTransitions()) == 4 {
					if alt := rootPrefix(input); alt != 0 {
						return alt
					}
				}
			case PythonParserRULE_testlist_star_expr:
				if len(state.GetTransitions()) == 2 {
					if alt := tuplePrefix(input); alt != 0 {
						return alt
					}
				}
			case PythonParserRULE_stmt:
				if len(state.GetTransitions()) == 2 {
					if compoundPrefix(input) != 0 {
						return 2
					}
					if smallPrefix(input.LA(1)) != 0 {
						return 1
					}
				}
			case PythonParserRULE_compound_stmt:
				if len(state.GetTransitions()) == 7 {
					if alt := compoundPrefix(input); alt != 0 {
						return alt
					}
				}
			case PythonParserRULE_small_stmt:
				if len(state.GetTransitions()) == 16 {
					if alt := smallPrefix(input.LA(1)); alt != 0 {
						return alt
					}
				}
			case PythonParserRULE_atom:
				if len(state.GetTransitions()) == 11 {
					if alt := atomPrefix(input.LA(1)); alt != 0 {
						return alt
					}
				}
			case PythonParserRULE_expr:
				if len(state.GetTransitions()) == 2 {
					switch input.LA(1) {
					case PythonParserADD, PythonParserNOT_OP:
						return 2
					case PythonParserAWAIT:
						return 1
					default:
						if atomPrefix(input.LA(1)) != 0 {
							return 1
						}
					}
				}
			case PythonParserRULE_test:
				// Both optional branches have two alternatives. IF also starts a
				// comprehension filter, so a present IF retains ATN prediction.
				if len(state.GetTransitions()) == 2 {
					if decision == 95 && input.LA(1) != PythonParserCOLON_ASSIGN {
						return 2
					}
					if decision == 96 && input.LA(1) != PythonParserIF {
						return 2
					}
				}
			}
		} else if state.GetStateType() == antlr.ATNStateStarLoopEntry && state.GetRuleIndex() == PythonParserRULE_expr && decision == 117 && len(state.GetTransitions()) == 2 {
			// Only the non-precedence trailer loop; arithmetic recursion and
			// every precedence predicate stay with the original simulator.
			switch input.LA(1) {
			case PythonParserDOT, PythonParserOPEN_PAREN, PythonParserOPEN_BRACKET:
				return 1
			default:
				return 2
			}
		}
	}
	return interpreter.AdaptivePredict(base, input, decision, ctx)
}

func compoundPrefix(input antlr.TokenStream) int {
	switch input.LA(1) {
	case PythonParserIF:
		return 1
	case PythonParserWHILE:
		return 2
	case PythonParserFOR:
		return 3
	case PythonParserTRY:
		return 4
	case PythonParserWITH:
		return 5
	case PythonParserCLASS, PythonParserDEF, PythonParserAT:
		return 7
	case PythonParserASYNC:
		switch input.LA(2) {
		case PythonParserFOR:
			return 3
		case PythonParserWITH:
			return 5
		case PythonParserDEF:
			return 7
		}
	}
	// NAME includes match; its semantic predicate and ordinary expressions
	// must be considered together by the ATN.
	return 0
}

func smallPrefix(token int) int {
	switch token {
	case PythonParserDEL:
		return 4
	case PythonParserPASS:
		return 5
	case PythonParserBREAK:
		return 6
	case PythonParserCONTINUE:
		return 7
	case PythonParserRETURN:
		return 8
	case PythonParserRAISE:
		return 9
	case PythonParserYIELD:
		return 10
	case PythonParserIMPORT:
		return 11
	case PythonParserFROM:
		return 12
	case PythonParserGLOBAL:
		return 13
	case PythonParserASSERT:
		return 15
	case PythonParserNAME, PythonParserPRINT, PythonParserEXEC, PythonParserNONLOCAL:
		// type aliases and Python 2/3 predicates overlap these prefixes.
		return 0
	case PythonParserLAMBDA, PythonParserNOT, PythonParserAWAIT,
		PythonParserADD, PythonParserMINUS, PythonParserNOT_OP, PythonParserSTAR:
		return 2
	default:
		if atomPrefix(token) != 0 {
			return 2
		}
	}
	return 0
}

func atomPrefix(token int) int {
	switch token {
	case PythonParserOPEN_PAREN:
		return 1
	case PythonParserOPEN_BRACKET:
		return 2
	case PythonParserOPEN_BRACE:
		return 3
	case PythonParserREVERSE_QUOTE:
		return 4
	case PythonParserELLIPSIS:
		return 5
	case PythonParserNAME, PythonParserTRUE, PythonParserFALSE, PythonParserPRINT, PythonParserEXEC:
		// name precedes the overlapping PRINT and EXEC alternatives.
		return 6
	case PythonParserDECIMAL_INTEGER, PythonParserOCT_INTEGER, PythonParserHEX_INTEGER,
		PythonParserBIN_INTEGER, PythonParserIMAG_NUMBER, PythonParserFLOAT_NUMBER:
		return 9
	case PythonParserNONE:
		return 10
	case PythonParserSTRING:
		return 11
	}
	// MINUS number overlaps unary minus in expr; do not resolve it here.
	return 0
}
