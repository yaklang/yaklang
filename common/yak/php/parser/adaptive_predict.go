package phpparser

import "github.com/yaklang/antlr/v4"

// These are the primary alternatives of expression in PHPParser.g4, after
// ANTLR separates its left-recursive operator alternatives. The typed-tree
// differential tests compare each fast choice against the original predictor.
const (
	primaryCall           = 5
	primaryDynamicStatic  = 8
	primaryName           = 10
	primaryVariable       = 12
	primaryArray          = 13
	primaryConstant       = 14
	primaryString         = 15
	primaryParenthesis    = 20
	primaryListAssignment = 24
	primaryCast           = 26
	primaryPostfix        = 30
	primaryReference      = 34
	primaryAssignment     = 35
	primaryCount          = 36
	operatorCount         = 23
	prefixTokenLimit      = 128
	prefixScanTokenLimit  = 16384
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
		if token == PHPParserNamespace && namespaceDeclarationPrefix(input) {
			if kind == antlr.ATNStatePlusLoopBack {
				return 1
			}
			if kind == antlr.ATNStateBlockStart && alternatives == 7 {
				return 2
			}
		}
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
		if kind == antlr.ATNStateStarLoopEntry || kind == antlr.ATNStateStarBlockStart && alternatives == operatorCount {
			// Match the generated recursion predicate before choosing an arm or
			// continuing the loop. The generated parser still constructs the
			// same typed nodes and checks the predicate again.
			alt, precedence := expressionOperator(token, next)
			if alt != 0 {
				if kind == antlr.ATNStateStarLoopEntry {
					return optionalPrediction(p.Precpred(ctx, precedence))
				}
				if p.Precpred(ctx, precedence) {
					return alt
				}
			} else if kind == antlr.ATNStateStarLoopEntry && isExpressionEnd(token) {
				return 2
			}
		}
		if kind == antlr.ATNStateBlockStart && alternatives == primaryCount {
			return predictPrimary(input, token, next)
		}
	case PHPParserRULE_tryCatchFinally:
		switch kind {
		case antlr.ATNStatePlusLoopBack, antlr.ATNStateStarLoopEntry:
			if token == PHPParserCatch && catchPrefixComplete(input) {
				return 1
			}
			if token != PHPParserCatch {
				return 2
			}
		case antlr.ATNStateBlockStart:
			if alternatives == 2 {
				first := p.GetATN().NextTokens(state, nil)
				if predictionSetContains(first, PHPParserCatch) {
					if token == PHPParserCatch && catchPrefixComplete(input) {
						return 1
					}
					if token == PHPParserFinally && next == PHPParserOpenCurlyBracket && finallyPrefixComplete(input) {
						return 2
					}
				} else if predictionSetContains(first, PHPParserFinally) {
					if token == PHPParserFinally && next == PHPParserOpenCurlyBracket && finallyPrefixComplete(input) {
						return 1
					}
					if token != PHPParserFinally {
						return 2
					}
				}
			}
		}
	case PHPParserRULE_newExpr:
		// The two new alternatives overlap on anonymousClass. A complete
		// literal class with no ambiguous suffix prefers the first arm.
		if kind == antlr.ATNStateBlockStart && alternatives == 2 && token == PHPParserNew && next == PHPParserClass && anonymousClassPrefixComplete(input) {
			return 1
		}
	case PHPParserRULE_foreachStatement:
		if kind == antlr.ATNStateBlockStart && alternatives == 6 && token == PHPParserOpenRoundBracket {
			return predictForeach(input)
		}
	case PHPParserRULE_dynamicStaticClassExpr:
		if kind == antlr.ATNStateBlockStart && alternatives == 2 && token == PHPParserOpenRoundBracket {
			if end, ok := skipBalancedPrefix(input, 1); ok && input.LA(end) == PHPParserDoubleColon && input.LA(end+1) == PHPParserLabel {
				return 1
			}
		}
	case PHPParserRULE_dynamicStaticReceiver:
		if kind == antlr.ATNStateBlockStart && alternatives == 2 && token == PHPParserOpenRoundBracket {
			if end, ok := skipBalancedPrefix(input, 1); ok && input.LA(end) == PHPParserDoubleColon {
				return 2
			}
		}
	case PHPParserRULE_dynamicStaticReceiverBase:
		if kind == antlr.ATNStateBlockStart && alternatives == 3 && token == PHPParserOpenRoundBracket {
			if end, ok := skipBalancedPrefix(input, 1); ok && input.LA(end) == PHPParserDoubleColon {
				return 2
			}
		}
	case PHPParserRULE_functionCallName:
		if alternatives == 5 {
			if isBareFunctionNameToken(token) && next == PHPParserOpenRoundBracket {
				return 1
			}
			if (token == PHPParserLabel || token == PHPParserAssert) && next == PHPParserDoubleColon && input.LA(3) == PHPParserLabel && input.LA(4) == PHPParserOpenRoundBracket {
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
				// Dynamic/keyword members can overlap the enclosing
				// expression's legacy index arm, including an empty ->{}.
				if !isSimpleMemberIdentifier(next) {
					return 0
				}
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
				if !isSimpleMemberIdentifier(next) {
					return 0
				}
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
	case PHPParserRULE_constantInitializer:
		if kind == antlr.ATNStateBlockStart && alternatives == 5 {
			return predictConstantArray(input, token, next)
		}
	case PHPParserRULE_arrayCreation:
		if kind == antlr.ATNStateBlockStart && alternatives == 2 {
			return optionalPrediction(token == PHPParserEllipsis)
		}
	case PHPParserRULE_arrayItem:
		if kind == antlr.ATNStateBlockStart && alternatives == 3 {
			if token == PHPParserEllipsis {
				return 1
			}
			// The reference arm requires an ampersand after its optional key.
			// Only bypass it for a complete known prefix at an item boundary,
			// or a key whose value does not start with an ampersand.
			end, ok := 0, false
			switch token {
			case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket:
				end, ok = skipBalancedPrefix(input, 1)
			case PHPParserVarName:
				end, ok = variablePrefixEnd(input)
			case PHPParserLabel, PHPParserAssert, PHPParserIsSet, PHPParserEmpty, PHPParserEval, PHPParserExit, PHPParserDie, PHPParserDefine, PHPParserDefined:
				end, ok = callPrefixEnd(input)
			case PHPParserSingleQuoteString, PHPParserDecimal, PHPParserHex, PHPParserOctal, PHPParserBinary, PHPParserReal:
				end, ok = 2, true
			}
			if ok {
				switch input.LA(end) {
				case PHPParserComma, PHPParserCloseSquareBracket, PHPParserCloseRoundBracket:
					return 2
				case PHPParserDoubleArrow:
					if input.LA(end+1) != PHPParserAmpersand {
						return 2
					}
				}
			}
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
	case PHPParserArray, PHPParserList:
		// These keywords also occur in qualifiedNamespaceName. Positional
		// arguments prefer functionCall; top-level keyed items require the
		// array arm. An arrow lambda also uses =>, so it retains ATN prediction.
		if next == PHPParserOpenRoundBracket {
			if end, flags, ok := inspectBalancedPrefix(input, 2); ok {
				if token == PHPParserList && input.LA(end) == PHPParserEq {
					return primaryListAssignment
				}
				if flags&balancedArrowLambda != 0 || !isPrimaryBoundary(input.LA(end)) {
					return 0
				}
				if flags&balancedArrayKey != 0 {
					return primaryArray
				}
				return primaryCall
			}
		}
	case PHPParserOpenSquareBracket:
		if end, ok := skipBalancedPrefix(input, 1); ok && isPrimaryBoundary(input.LA(end)) {
			return primaryArray
		}
	case PHPParserVarName:
		// Immediate arguments, including a named static method, prefer the
		// functionCall arm. Assignment and unresolved static tails delegate.
		if next == PHPParserOpenRoundBracket || next == PHPParserDoubleColon && input.LA(3) == PHPParserLabel {
			if end, ok := callPrefixEnd(input); ok && isPrimaryBoundary(input.LA(end)) {
				return primaryCall
			}
		}
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
		} else if end, ok := skipBalancedPrefix(input, 1); ok {
			// A grouped receiver followed by :: cannot be a functionCallName
			// or staticClass. Recognize complete simple keys only; dynamic keys
			// and assignment tails still need the original full-context choice.
			if input.LA(end) == PHPParserDoubleColon && (input.LA(end+1) == PHPParserLabel || input.LA(end+1) == PHPParserVarName) {
				if tail, ok := memberSuffixEnd(input, end+2); ok && isPrimaryBoundary(input.LA(tail)) {
					return primaryDynamicStatic
				}
			}
			callable := input.LA(end) == PHPParserOpenRoundBracket
			if tail, ok := memberSuffixEnd(input, end); ok && isPrimaryBoundary(input.LA(tail)) {
				if callable {
					return primaryCall
				}
				return primaryParenthesis
			}
		}
	case PHPParserLabel, PHPParserAssert, PHPParserIsSet, PHPParserEmpty, PHPParserEval, PHPParserExit,
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
	case PHPParserLabel, PHPParserAssert, PHPParserIsSet, PHPParserEmpty, PHPParserEval, PHPParserExit,
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

// Classify only tokens immediately inside the balanced prefix. Nested array
// keys or lambda arrows must not select the enclosing keyword's alternative.
type balancedPrefixFlags uint8

const (
	balancedArrayKey balancedPrefixFlags = 1 << iota
	balancedArrowLambda
	balancedColon
)

func balancedTopFlag(token int) balancedPrefixFlags {
	switch token {
	case PHPParserDoubleArrow:
		return balancedArrayKey
	case PHPParserLambdaFn:
		return balancedArrowLambda
	case PHPParserColon:
		return balancedColon
	}
	return 0
}

// Inspect balanced arguments without consuming or seeking the stream. Reading
// CommonTokenStream by raw index is linear; repeated LA(k) walks the prefix again
// for each k and becomes quadratic on large closure arguments. Hidden tokens do
// not participate in balancing. Both scanned tokens and nesting are bounded;
// reaching a bound delegates to ANTLR rather than imposing a syntax limit.
func skipBalancedPrefix(input antlr.TokenStream, offset int) (int, bool) {
	end, _, ok := inspectBalancedPrefix(input, offset)
	return end, ok
}

func inspectBalancedPrefix(input antlr.TokenStream, offset int) (int, balancedPrefixFlags, bool) {
	if stream, ok := input.(*antlr.CommonTokenStream); ok {
		start := stream.LT(offset)
		var stack [prefixTokenLimit]int
		depth := 0
		var flags balancedPrefixFlags
		index := start.GetTokenIndex()
		for scanned := 0; scanned < prefixScanTokenLimit; scanned++ {
			if !stream.Sync(index) {
				return offset, 0, false
			}
			token := stream.Get(index)
			index++
			kind := token.GetTokenType()
			if kind == antlr.TokenEOF {
				return offset, 0, false
			}
			if token.GetChannel() != start.GetChannel() {
				continue
			}
			switch kind {
			case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket, PHPParserCurlyOpen:
				if depth == len(stack) {
					return offset, 0, false
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
					return offset, 0, false
				}
				depth--
				if depth == 0 {
					return offset + 1, flags, true
				}
			}
			if depth == 1 {
				flags |= balancedTopFlag(kind)
			}
			offset++
		}
		return offset, 0, false
	}

	var stack [prefixTokenLimit]int
	depth := 0
	var flags balancedPrefixFlags
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
				return offset, 0, false
			}
			depth--
			if depth == 0 {
				return offset + 1, flags, true
			}
		case antlr.TokenEOF:
			return offset, 0, false
		}
		if depth == 1 {
			flags |= balancedTopFlag(token)
		}
		offset++
	}
	return offset, 0, false
}

func variablePrefixEnd(input antlr.TokenStream) (int, bool) {
	return variablePrefixEndAt(input, 1)
}

func variablePrefixEndAt(input antlr.TokenStream, start int) (int, bool) {
	offset := start + 1
	for steps := 0; steps < prefixTokenLimit; steps++ {
		switch input.LA(offset) {
		case PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket:
			var ok bool
			offset, ok = skipBalancedPrefix(input, offset)
			if !ok {
				return offset, false
			}
		case PHPParserObjectOperator:
			// Dynamic and other keyword members keep the original prediction.
			if !isSimpleMemberIdentifier(input.LA(offset + 1)) {
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
	return callPrefixEndAt(input, 1)
}
func callPrefixEndAt(input antlr.TokenStream, start int) (int, bool) {
	offset := start + 1
	if input.LA(offset) == PHPParserDoubleColon && input.LA(offset+1) == PHPParserLabel {
		offset += 2
	}
	if input.LA(offset) != PHPParserOpenRoundBracket {
		return offset, false
	}
	return memberSuffixEnd(input, offset)
}
func memberSuffixEnd(input antlr.TokenStream, offset int) (int, bool) {
	for steps := 0; steps < prefixTokenLimit; steps++ {
		switch input.LA(offset) {
		case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket, PHPParserOpenCurlyBracket:
			var ok bool
			offset, ok = skipBalancedPrefix(input, offset)
			if !ok {
				return offset, false
			}
		case PHPParserObjectOperator, PHPParserNullsafeObjectOperator:
			if !isSimpleMemberIdentifier(input.LA(offset + 1)) {
				return offset, false
			}
			offset += 2
		default:
			return offset, true
		}
	}
	return offset, false
}

// The six header alternatives overlap on both the source and the binding.
// Choose only complete variable bindings; list and destructuring bindings keep
// their original prediction. A flexiVariable source prefers chain (arm 2),
// while a call or grouped/array expression uses arm 3.
func predictForeach(input antlr.TokenStream) int {
	end, ok, chain := 0, false, false
	switch input.LA(2) {
	case PHPParserVarName:
		end, ok = variablePrefixEndAt(input, 2)
		chain = true
	case PHPParserLabel, PHPParserArray:
		end, ok = callPrefixEndAt(input, 2)
	case PHPParserOpenSquareBracket, PHPParserOpenRoundBracket:
		end, ok = skipBalancedPrefix(input, 2)
	}
	if !ok || input.LA(end) != PHPParserAs {
		return 0
	}
	offset := end + 1
	if input.LA(offset) == PHPParserAmpersand {
		offset++
	}
	if input.LA(offset) != PHPParserVarName {
		return 0
	}
	offset, ok = variablePrefixEndAt(input, offset)
	if !ok {
		return 0
	}
	if input.LA(offset) == PHPParserDoubleArrow {
		offset++
		if input.LA(offset) == PHPParserAmpersand {
			offset++
		}
		if input.LA(offset) != PHPParserVarName {
			return 0
		}
		offset, ok = variablePrefixEndAt(input, offset)
		if !ok {
			return 0
		}
	}
	if input.LA(offset) != PHPParserCloseRoundBracket {
		return 0
	}
	if chain {
		return 2
	}
	return 3
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

// These are the operator alternatives and precedence predicates generated
// from expression in PHPParser.g4. The operator-pair differential test checks
// every binary binding, including associativity, against the original ATN.
func expressionOperator(token, next int) (alt, precedence int) {
	switch token {
	case PHPParserPow:
		return 1, 24
	case PHPParserInstanceOf:
		return 2, 23
	case PHPParserAsterisk, PHPParserDivide, PHPParserPercent:
		return 3, 22
	case PHPParserPlus, PHPParserMinus, PHPParserDot:
		return 4, 21
	case PHPParserShiftLeft, PHPParserShiftRight:
		return 5, 20
	case PHPParserLess, PHPParserIsSmallerOrEqual, PHPParserGreater, PHPParserIsGreaterOrEqual:
		return 6, 19
	case PHPParserIsIdentical, PHPParserIsNoidentical, PHPParserIsEqual, PHPParserIsNotEq:
		return 7, 18
	case PHPParserAmpersand:
		return 8, 17
	case PHPParserCaret:
		return 9, 16
	case PHPParserPipe:
		return 10, 15
	case PHPParserBooleanAnd:
		return 11, 14
	case PHPParserBooleanOr:
		return 12, 13
	case PHPParserQuestionMark:
		return 13, 12
	case PHPParserNullCoalescing:
		return 14, 11
	case PHPParserSpaceship:
		return 15, 10
	case PHPParserLogicalAnd:
		return 16, 4
	case PHPParserLogicalXor:
		return 17, 3
	case PHPParserLogicalOr:
		return 18, 2
	case PHPParserObjectOperator:
		// The legacy ->{...} form overlaps memberCallKey; leave it to ANTLR.
		if !isSimpleMemberIdentifier(next) {
			return 0, 0
		}
		return 19, 51
	case PHPParserNullsafeObjectOperator:
		return 20, 50
	case PHPParserOpenSquareBracket:
		return 21, 49
	case PHPParserOpenCurlyBracket:
		return 22, 48
	case PHPParserOpenRoundBracket:
		return 23, 29
	}
	return 0, 0
}

func isInitializerBoundary(token int) bool {
	switch token {
	case PHPParserComma, PHPParserSemiColon, PHPParserCloseRoundBracket, PHPParserCloseSquareBracket:
		return true
	}
	return false
}

func predictConstantArray(input antlr.TokenStream, token, next int) int {
	offset, alt := 0, 0
	switch token {
	case PHPParserOpenSquareBracket:
		offset, alt = 1, 3
	case PHPParserArray:
		if next == PHPParserOpenRoundBracket {
			offset, alt = 2, 2
		}
	}
	if offset == 0 {
		return 0
	}
	// A bare placeholder or extra leading spread only belongs to expression.
	if input.LA(offset+1) == PHPParserEllipsis {
		switch input.LA(offset + 2) {
		case PHPParserCloseRoundBracket, PHPParserCloseSquareBracket, PHPParserEllipsis:
			return 0
		}
	}
	if end, flags, ok := inspectBalancedPrefix(input, offset); ok && isInitializerBoundary(input.LA(end)) {
		if alt == 2 && flags&balancedColon != 0 {
			return 0
		}
		return alt
	}
	return 0
}

// None of these delimiters can start a recursive expression operator.
func isExpressionEnd(token int) bool {
	switch token {
	case antlr.TokenEOF, PHPParserCloseRoundBracket, PHPParserCloseSquareBracket,
		PHPParserCloseCurlyBracket, PHPParserSemiColon, PHPParserComma,
		PHPParserColon, PHPParserDoubleArrow, PHPParserAs:
		return true
	}
	return false
}

func namespaceDeclarationPrefix(input antlr.TokenStream) bool {
	if input.LA(2) == PHPParserOpenCurlyBracket {
		return true
	}
	offset := 2
	for steps := 0; steps < prefixTokenLimit; steps++ {
		if input.LA(offset) != PHPParserLabel {
			return false
		}
		offset++
		switch input.LA(offset) {
		case PHPParserSemiColon, PHPParserOpenCurlyBracket:
			return true
		case PHPParserNamespaceSeparator:
			offset++
		default:
			return false
		}
	}
	return false
}

func finallyPrefixComplete(input antlr.TokenStream) bool {
	_, ok := skipBalancedPrefix(input, 2)
	return ok
}
func catchPrefixComplete(input antlr.TokenStream) bool {
	if input.LA(2) != PHPParserOpenRoundBracket {
		return false
	}
	offset := 3
	for steps := 0; steps < prefixTokenLimit; steps++ {
		if input.LA(offset) == PHPParserNamespaceSeparator {
			offset++
		}
		if input.LA(offset) != PHPParserLabel {
			return false
		}
		offset++
		for input.LA(offset) == PHPParserNamespaceSeparator {
			offset++
			if offset >= prefixTokenLimit || input.LA(offset) != PHPParserLabel {
				return false
			}
			offset++
		}
		if input.LA(offset) != PHPParserPipe {
			break
		}
		offset++
	}
	if input.LA(offset) == PHPParserVarName {
		offset++
	}
	if input.LA(offset) != PHPParserCloseRoundBracket || input.LA(offset+1) != PHPParserOpenCurlyBracket {
		return false
	}
	_, ok := skipBalancedPrefix(input, offset+1)
	return ok
}

// The class() member is common in attribute-builder chains, and Class is an
// unambiguous identifier token here. Other keyword and dynamic keys delegate.
func isSimpleMemberIdentifier(token int) bool {
	return token == PHPParserLabel || token == PHPParserClass
}

func simpleNamespaceNameEnd(input antlr.TokenStream, offset int) (int, bool) {
	if offset >= prefixTokenLimit {
		return offset, false
	}
	if input.LA(offset) == PHPParserNamespaceSeparator {
		offset++
	}
	// The entire header shares one visible-token budget, including comma
	// separated interfaces. Repeated LA(k) must never scan a growing long head.
	for offset < prefixTokenLimit {
		if input.LA(offset) != PHPParserLabel {
			return offset, false
		}
		offset++
		if offset >= prefixTokenLimit {
			return offset, false
		}
		if input.LA(offset) != PHPParserNamespaceSeparator {
			return offset, true
		}
		offset++
	}
	return offset, false
}

func anonymousClassPrefixComplete(input antlr.TokenStream) bool {
	offset := 3 // New Class
	var ok bool
	// Class is also a qualified-name token. Without an inheritance clause,
	// new class($arg) {$value} can instead be a named constructor followed
	// by a legacy index. A balanced body alone does not distinguish them.
	hasBase := false
	if input.LA(offset) == PHPParserOpenRoundBracket {
		offset, ok = skipBalancedPrefix(input, offset)
		if !ok {
			return false
		}
	}
	if offset >= prefixTokenLimit {
		return false
	}
	if input.LA(offset) == PHPParserExtends {
		hasBase = true
		offset, ok = simpleNamespaceNameEnd(input, offset+1)
		if !ok {
			return false
		}
	}
	if input.LA(offset) == PHPParserImplements {
		hasBase = true
		offset++
		for steps := 0; steps < prefixTokenLimit; steps++ {
			offset, ok = simpleNamespaceNameEnd(input, offset)
			if !ok {
				return false
			}
			if input.LA(offset) != PHPParserComma {
				break
			}
			offset++
		}
	}
	if !hasBase || input.LA(offset) != PHPParserOpenCurlyBracket {
		return false
	}
	end, ok := skipBalancedPrefix(input, offset)
	return ok && isPrimaryBoundary(input.LA(end))
}
