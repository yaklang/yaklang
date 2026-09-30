package phpparser

import "github.com/yaklang/antlr/v4"

// These are the primary alternatives of expression in PHPParser.g4, after
// ANTLR separates its left-recursive operator alternatives. The typed-tree
// differential tests compare each fast choice against the original predictor.
const (
	primaryCall          = 5
	primaryName          = 10
	primaryVariable      = 12
	primaryConstant      = 14
	primaryString        = 15
	primaryParenthesis   = 20
	primaryCast          = 26
	primaryPostfix       = 30
	primaryReference     = 34
	primaryAssignment    = 35
	primaryCount         = 36
	prefixTokenLimit     = 128
	prefixScanTokenLimit = 16384
)

// SetFastPrediction allows diagnostics and differential tests to use ANTLR's
// original predictor. It does not change the grammar or the error strategy.
func (p *PHPParser) SetFastPrediction(enabled bool) {
	p.disableFastPrediction = !enabled
}

// AdaptivePredict keeps the generated parser's construction, precedence checks
// and error handling. It only bypasses ATN exploration for known prefixes;
// uncertain decisions, long prefixes and exact ambiguity diagnostics use ANTLR.
// Decisions are identified by their rule and topology, not generated indices.
func (p *PHPParser) AdaptivePredict(base *antlr.BaseParser, input antlr.TokenStream, decision int, ctx antlr.ParserRuleContext) int {
	interpreter := p.GetInterpreter()
	if !p.disableFastPrediction && interpreter.GetPredictionMode() != antlr.PredictionModeLLExactAmbigDetection && !reservesSuffix(ctx) {
		state := p.GetATN().DecisionToState[decision]
		if alt := p.predictPrefix(input, state, ctx); alt != 0 {
			return alt
		}
	}
	return interpreter.AdaptivePredict(base, input, decision, ctx)
}

// These rules require a suffix after their leading expression/call. Its greedy
// member or index loop must let full-context prediction decide where to stop.
// This includes new $object->className() and factory()[key] = value; nested
// arguments conservatively use the same original predictor as their owner.
func reservesSuffix(ctx antlr.ParserRuleContext) bool {
	for ctx != nil {
		switch ctx.GetRuleIndex() {
		case PHPParserRULE_indirectTypeRef, PHPParserRULE_functionCallAssignable, PHPParserRULE_assignableChainOrigin:
			return true
		}
		parent, ok := ctx.GetParent().(antlr.ParserRuleContext)
		if !ok {
			break
		}
		ctx = parent
	}
	return false
}

func (p *PHPParser) predictPrefix(input antlr.TokenStream, state antlr.DecisionState, ctx antlr.ParserRuleContext) int {
	token, next := input.LA(1), input.LA(2)
	kind, alternatives := state.GetStateType(), len(state.GetTransitions())
	switch state.GetRuleIndex() {
	case PHPParserRULE_phpBlock:
		if kind == antlr.ATNStatePlusLoopBack && isPHPTopStatementStart(token) {
			return 1
		}
		if kind == antlr.ATNStateBlockStart && alternatives == 7 {
			if isPHPStatementStart(token) {
				return 7 // statement follows the declaration alternatives
			}
			if alt := declarationPrefix(input, token, next); alt != 0 {
				return alt
			}
		}
	case PHPParserRULE_namespaceDeclaration:
		if kind == antlr.ATNStateStarLoopEntry && isPHPTopStatementStart(token) {
			return 1
		}
	case PHPParserRULE_namespaceStatement:
		if alternatives == 6 {
			if isPHPStatementStart(token) {
				return 6
			}
			if alt := declarationPrefix(input, token, next); alt >= 3 {
				return alt - 1 // declarations have the same order without namespace
			} else if alt == 1 {
				return 1
			}
		}
	case PHPParserRULE_innerStatementList:
		if kind == antlr.ATNStateStarLoopEntry {
			if isPHPStatementStart(token) || isPHPClassStart(token) || token == PHPParserFunction_ {
				return 2 // the grammar's nongreedy loop continues
			}
			if token == PHPParserCloseCurlyBracket || next != PHPParserNamespaceSeparator && next != PHPParserDoubleColon && isPHPBodyDelimiter(token) {
				return 1 // the enclosing rule must still match its delimiter
			}
		}
	case PHPParserRULE_innerStatement:
		if alternatives == 3 {
			if isPHPStatementStart(token) {
				return 1
			}
			switch declarationPrefix(input, token, next) {
			case 3:
				return 2
			case 4:
				return 3
			case 7:
				return 1
			}
		}
	case PHPParserRULE_statement:
		if alternatives == 23 {
			return predictStatement(token, next)
		}
	case PHPParserRULE_ifStatement:
		if kind == antlr.ATNStateStarLoopEntry {
			if token == PHPParserElseIf && next != PHPParserNamespaceSeparator && next != PHPParserDoubleColon {
				return 1
			}
			return 2
		}
		if kind == antlr.ATNStateBlockStart && alternatives == 2 {
			// The two body styles and the optional else both have two arms.
			// Their FIRST sets distinguish them without a generated decision ID.
			first := p.GetATN().NextTokens(state, nil)
			if predictionSetContains(first, PHPParserIf) {
				if token == PHPParserIf && next == PHPParserOpenRoundBracket {
					if end, ok := skipBalancedPrefix(input, 2); ok {
						if input.LA(end) == PHPParserColon {
							return 2
						}
						return 1
					}
				}
			} else if predictionSetContains(first, PHPParserElse) {
				if token == PHPParserElse && next != PHPParserNamespaceSeparator && next != PHPParserDoubleColon {
					return 1
				}
				return 2
			}
		}
	case PHPParserRULE_expression:
		if kind == antlr.ATNStateBlockStart && alternatives == primaryCount {
			return predictPrimary(input, token, next)
		}
	case PHPParserRULE_functionCallName:
		if alternatives == 5 {
			if isBareFunctionNameToken(token) && next == PHPParserOpenRoundBracket {
				return 1
			}
			if token == PHPParserLabel && next == PHPParserDoubleColon && input.LA(3) == PHPParserLabel && input.LA(4) == PHPParserOpenRoundBracket {
				return 2
			}
		}
	case PHPParserRULE_flexiVariable:
		switch kind {
		case antlr.ATNStateStarLoopEntry:
			precedence := 0
			switch token {
			case PHPParserOpenSquareBracket:
				precedence = 3
			case PHPParserOpenCurlyBracket:
				precedence = 2
			case PHPParserObjectOperator:
				precedence = 1
			}
			if precedence != 0 && p.Precpred(ctx, precedence) {
				return 1
			}
			return 2
		case antlr.ATNStateStarBlockStart:
			switch token {
			case PHPParserOpenSquareBracket:
				return 1
			case PHPParserOpenCurlyBracket:
				return 2
			case PHPParserObjectOperator:
				return 3
			}
		case antlr.ATNStateBlockStart:
			if alternatives == 2 {
				return optionalPrediction(token == PHPParserOpenRoundBracket)
			}
		}
	case PHPParserRULE_variable:
		if kind == antlr.ATNStateStarLoopEntry {
			return optionalPrediction(token == PHPParserOpenSquareBracket || token == PHPParserOpenCurlyBracket)
		}
	case PHPParserRULE_actualArguments:
		switch kind {
		case antlr.ATNStatePlusLoopBack:
			return optionalPrediction(token == PHPParserOpenRoundBracket)
		case antlr.ATNStateStarLoopEntry:
			return optionalPrediction(token == PHPParserOpenSquareBracket || token == PHPParserOpenCurlyBracket)
		}
	case PHPParserRULE_arrayCreation:
		if kind == antlr.ATNStateBlockStart && alternatives == 2 {
			return optionalPrediction(token == PHPParserEllipsis)
		}
	case PHPParserRULE_arrayItemList:
		switch kind {
		case antlr.ATNStateStarLoopEntry:
			return optionalPrediction(token == PHPParserComma && next != PHPParserCloseSquareBracket && next != PHPParserCloseRoundBracket)
		case antlr.ATNStateBlockStart:
			if alternatives == 2 {
				return optionalPrediction(token == PHPParserComma)
			}
		}
	}
	return 0
}

func optionalPrediction(present bool) int {
	if present {
		return 1
	}
	return 2
}

func predictionSetContains(set *antlr.IntervalSet, token int) bool {
	for _, interval := range set.GetIntervals() {
		if token >= interval.Start && token < interval.Stop {
			return true
		}
	}
	return false
}

func predictPrimary(input antlr.TokenStream, token, next int) int {
	switch token {
	case PHPParserVarName:
		if end, ok := variablePrefixEnd(input); ok {
			tail := input.LA(end)
			if isAssignmentToken(tail) {
				if tail == PHPParserEq && input.LA(end+1) == PHPParserAmpersand {
					return primaryReference
				}
				return primaryAssignment
			}
			if tail == PHPParserInc || tail == PHPParserDec {
				return primaryPostfix
			}
			if isPrimaryBoundary(tail) {
				return primaryVariable
			}
		}
	case PHPParserDecimal, PHPParserHex, PHPParserOctal, PHPParserBinary, PHPParserReal:
		return primaryConstant
	case PHPParserSingleQuoteString:
		if next != PHPParserDoubleColon {
			return primaryString
		}
	case PHPParserOpenRoundBracket:
		if isCastToken(next) {
			if input.LA(3) == PHPParserCloseRoundBracket && isCastOperandStart(input.LA(4)) {
				return primaryCast
			}
		} else if end, ok := skipBalancedPrefix(input, 1); ok && isPrimaryBoundary(input.LA(end)) {
			return primaryParenthesis
		}
	case PHPParserLabel, PHPParserIsSet, PHPParserEmpty, PHPParserEval, PHPParserExit,
		PHPParserDie, PHPParserDefine, PHPParserDefined:
		if end, ok := callPrefixEnd(input); ok && isPrimaryBoundary(input.LA(end)) {
			return primaryCall
		}
		if token == PHPParserLabel && next != PHPParserNamespaceSeparator && next != PHPParserDoubleColon && next != PHPParserOpenRoundBracket {
			return primaryName
		}
	}
	return 0
}

func isBareFunctionNameToken(token int) bool {
	switch token {
	case PHPParserLabel, PHPParserIsSet, PHPParserEmpty, PHPParserEval, PHPParserExit,
		PHPParserDie, PHPParserDefine, PHPParserDefined:
		return true
	}
	return false
}

func isCastToken(token int) bool {
	switch token {
	case PHPParserBoolType, PHPParserInt8Cast, PHPParserInt16Cast, PHPParserIntType,
		PHPParserInt64Type, PHPParserUintCast, PHPParserDoubleCast, PHPParserDoubleType,
		PHPParserFloatCast, PHPParserStringType, PHPParserBinaryCast, PHPParserUnicodeCast,
		PHPParserArray, PHPParserObjectType, PHPParserResource, PHPParserUnset:
		return true
	}
	return false
}

func isCastOperandStart(token int) bool {
	switch token {
	case PHPParserVarName, PHPParserDollar, PHPParserLabel, PHPParserSingleQuoteString,
		PHPParserDoubleQuote, PHPParserDecimal, PHPParserHex, PHPParserOctal,
		PHPParserBinary, PHPParserReal, PHPParserNew, PHPParserClone:
		return true
	}
	return false
}

func isPrimaryBoundary(token int) bool {
	switch token {
	case PHPParserNamespaceSeparator, PHPParserDoubleColon, PHPParserOpenRoundBracket,
		PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket, PHPParserObjectOperator,
		PHPParserNullsafeObjectOperator, PHPParserInc, PHPParserDec:
		return false
	}
	return !isAssignmentToken(token)
}

func isAssignmentToken(token int) bool {
	switch token {
	case PHPParserEq, PHPParserPlusEqual, PHPParserMinusEqual, PHPParserMulEqual,
		PHPParserPowEqual, PHPParserDivEqual, PHPParserConcaequal, PHPParserModEqual,
		PHPParserAndEqual, PHPParserOrEqual, PHPParserXorEqual, PHPParserShiftLeftEqual,
		PHPParserShiftRightEqual, PHPParserNullCoalescingEqual:
		return true
	}
	return false
}

// Inspect balanced arguments without consuming or seeking the stream. Reading
// CommonTokenStream by raw index is linear; repeated LA(k) walks the prefix again
// for each k and becomes quadratic on large closure arguments. Hidden tokens do
// not participate in balancing. Both scanned tokens and nesting are bounded;
// reaching a bound delegates to ANTLR rather than imposing a syntax limit.
func skipBalancedPrefix(input antlr.TokenStream, offset int) (int, bool) {
	if stream, ok := input.(*antlr.CommonTokenStream); ok {
		start := stream.LT(offset)
		var stack [prefixTokenLimit]int
		depth := 0
		index := start.GetTokenIndex()
		for scanned := 0; scanned < prefixScanTokenLimit; scanned++ {
			if !stream.Sync(index) {
				return offset, false
			}
			token := stream.Get(index)
			index++
			kind := token.GetTokenType()
			if kind == antlr.TokenEOF {
				return offset, false
			}
			if token.GetChannel() != start.GetChannel() {
				continue
			}
			switch kind {
			case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket, PHPParserCurlyOpen:
				if depth == len(stack) {
					return offset, false
				}
				if kind == PHPParserCurlyOpen {
					kind = PHPParserOpenCurlyBracket
				}
				stack[depth] = kind
				depth++
			case PHPParserCloseRoundBracket, PHPParserCloseSquareBracket, PHPParserCloseCurlyBracket:
				want := PHPParserOpenRoundBracket
				if kind == PHPParserCloseSquareBracket {
					want = PHPParserOpenSquareBracket
				}
				if kind == PHPParserCloseCurlyBracket {
					want = PHPParserOpenCurlyBracket
				}
				if depth == 0 || stack[depth-1] != want {
					return offset, false
				}
				depth--
				if depth == 0 {
					return offset + 1, true
				}
			}
			offset++
		}
		return offset, false
	}

	var stack [prefixTokenLimit]int
	depth := 0
	for offset < prefixTokenLimit {
		token := input.LA(offset)
		switch token {
		case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket, PHPParserCurlyOpen:
			if token == PHPParserCurlyOpen {
				token = PHPParserOpenCurlyBracket
			}
			stack[depth] = token
			depth++
		case PHPParserCloseRoundBracket, PHPParserCloseSquareBracket, PHPParserCloseCurlyBracket:
			want := PHPParserOpenRoundBracket
			if token == PHPParserCloseSquareBracket {
				want = PHPParserOpenSquareBracket
			}
			if token == PHPParserCloseCurlyBracket {
				want = PHPParserOpenCurlyBracket
			}
			if depth == 0 || stack[depth-1] != want {
				return offset, false
			}
			depth--
			if depth == 0 {
				return offset + 1, true
			}
		case antlr.TokenEOF:
			return offset, false
		}
		offset++
	}
	return offset, false
}

func variablePrefixEnd(input antlr.TokenStream) (int, bool) {
	offset := 2
	for steps := 0; steps < prefixTokenLimit; steps++ {
		switch input.LA(offset) {
		case PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket:
			var ok bool
			offset, ok = skipBalancedPrefix(input, offset)
			if !ok {
				return offset, false
			}
		case PHPParserObjectOperator:
			// Dynamic and keyword member names keep the original prediction.
			if input.LA(offset+1) != PHPParserLabel {
				return offset, false
			}
			offset += 2
			if input.LA(offset) == PHPParserOpenRoundBracket {
				var ok bool
				offset, ok = skipBalancedPrefix(input, offset)
				if !ok {
					return offset, false
				}
			}
		default:
			return offset, true
		}
	}
	return offset, false
}

func callPrefixEnd(input antlr.TokenStream) (int, bool) {
	offset := 2
	if input.LA(offset) == PHPParserDoubleColon && input.LA(offset+1) == PHPParserLabel {
		offset += 2
	}
	if input.LA(offset) != PHPParserOpenRoundBracket {
		return offset, false
	}
	for steps := 0; steps < prefixTokenLimit; steps++ {
		switch input.LA(offset) {
		case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket:
			var ok bool
			offset, ok = skipBalancedPrefix(input, offset)
			if !ok {
				return offset, false
			}
		case PHPParserObjectOperator:
			if input.LA(offset+1) != PHPParserLabel {
				return offset, false
			}
			offset += 2
		default:
			return offset, true
		}
	}
	return offset, false
}

func isPHPBodyDelimiter(token int) bool {
	switch token {
	case PHPParserElse, PHPParserElseIf, PHPParserEndIf, PHPParserEndWhile,
		PHPParserEndFor, PHPParserEndForeach, PHPParserEndSwitch, PHPParserEndDeclare,
		PHPParserCase, PHPParserDefault:
		return true
	}
	return false
}

func isPHPStatementStart(token int) bool {
	switch token {
	case PHPParserVarName, PHPParserDollar, PHPParserLabel, PHPParserOpenCurlyBracket,
		PHPParserIf, PHPParserWhile, PHPParserDo, PHPParserFor, PHPParserSwitch,
		PHPParserBreak, PHPParserContinue, PHPParserReturn, PHPParserGlobal,
		PHPParserStatic, PHPParserEcho, PHPParserUnset, PHPParserForeach, PHPParserTry,
		PHPParserThrow, PHPParserGoto, PHPParserDeclare, PHPParserSemiColon,
		PHPParserNew, PHPParserClone, PHPParserOpenRoundBracket, PHPParserInc,
		PHPParserDec, PHPParserPlus, PHPParserMinus, PHPParserBang, PHPParserTilde,
		PHPParserSuppressWarnings, PHPParserSingleQuoteString, PHPParserDoubleQuote,
		PHPParserDecimal, PHPParserHex, PHPParserOctal, PHPParserBinary, PHPParserReal:
		return true
	}
	return false
}

func isPHPClassStart(token int) bool {
	switch token {
	case PHPParserClass, PHPParserInterface, PHPParserTrait, PHPParserAbstract,
		PHPParserFinal, PHPParserReadonly, PHPParserPrivate, PHPParserPartial:
		return true
	}
	return false
}

func isPHPTopStatementStart(token int) bool {
	return isPHPStatementStart(token) || isPHPClassStart(token) ||
		token == PHPParserFunction_ || token == PHPParserUse || token == PHPParserConst ||
		token == PHPParserEnum_ || token == PHPParserAttributeStart
}

// Return phpBlock's declaration alternative. Namespace declarations and
// attributes stay with ANTLR; reference closures must remain statements.
func declarationPrefix(input antlr.TokenStream, token, next int) int {
	if next == PHPParserNamespaceSeparator || next == PHPParserDoubleColon {
		return 0
	}
	switch token {
	case PHPParserUse:
		return 1
	case PHPParserFunction_:
		if next == PHPParserOpenRoundBracket || next == PHPParserAmpersand && input.LA(3) == PHPParserOpenRoundBracket {
			return 7
		}
		return 3
	case PHPParserClass, PHPParserInterface, PHPParserTrait:
		return 4
	case PHPParserConst:
		return 5
	case PHPParserEnum_:
		if next == PHPParserLabel {
			return 6
		}
	}
	return 0
}

func predictStatement(token, next int) int {
	if next == PHPParserNamespaceSeparator || next == PHPParserDoubleColon {
		return 0 // keyword namespace segments and static receivers are expressions
	}
	switch token {
	case PHPParserOpenCurlyBracket:
		return 2
	case PHPParserIf:
		return 3
	case PHPParserWhile:
		return 4
	case PHPParserDo:
		return 5
	case PHPParserFor:
		return 6
	case PHPParserSwitch:
		return 7
	case PHPParserBreak:
		return 8
	case PHPParserContinue:
		return 9
	case PHPParserReturn:
		return 10
	case PHPParserGlobal:
		return 12
	case PHPParserStatic:
		if next == PHPParserVarName {
			return 13
		}
	case PHPParserEcho:
		return 14
	case PHPParserForeach:
		return 17
	case PHPParserTry:
		return 18
	case PHPParserGoto:
		return 20
	case PHPParserDeclare:
		return 21
	case PHPParserSemiColon:
		return 22
	case PHPParserLabel:
		if next == PHPParserColon {
			return 1
		}
		return 15
	default:
		if isPHPStatementStart(token) {
			// In this grammar expressionStatement precedes throwStatement and
			// unsetStatement. Preserve that choice and its existing SSA visitor.
			return 15
		}
	}
	return 0
}
