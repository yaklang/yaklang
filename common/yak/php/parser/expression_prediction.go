package phpparser

// Namespace-relative names start with namespace\, unlike a declaration.
func (p *PHPParser) IsNamespaceDeclarationAhead() bool {
	return p.GetTokenStream().LA(2) != PHPParserNamespaceSeparator
}

// Constructor types overlap expression ->member. Even a named constructor
// can enter that alternative in SLL when its arguments contain member access.
// Resolve just this type in LL so these prefixes do not retry an entire file.
func (p *PHPParser) IsDynamicTypeRefAhead() bool {
	switch p.GetTokenStream().LA(1) {
	case PHPParserLabel, PHPParserStatic:
		return p.GetTokenStream().LA(2) == PHPParserOpenRoundBracket
	case PHPParserVarName, PHPParserDollar, PHPParserOpenRoundBracket:
		return true
	}
	return false
}

// IsCallResultAssignmentAhead identifies a bounded subset of the overlapping
// call/assignment prefixes. It selects LL for that expression only: unrecognized
// or long prefixes keep SLL plus the normal whole-parse LL recovery path.
// Unlike a predicate on common expression alternatives, this action does not
// add semantic contexts to ANTLR's recursive prediction configuration sets.
func (p *PHPParser) IsCallResultAssignmentAhead() bool {
	stream := p.GetTokenStream()
	const limit = 128
	i := 1
	if stream.LA(i) == PHPParserNamespaceSeparator {
		i++
	}
	if stream.LA(i) != PHPParserLabel && stream.LA(i) != PHPParserVarName {
		return false
	}
	i++
	for i < limit && stream.LA(i) == PHPParserNamespaceSeparator && stream.LA(i+1) == PHPParserLabel {
		i += 2
	}
	if stream.LA(i) == PHPParserDoubleColon && stream.LA(i+1) == PHPParserLabel {
		i += 2
	}
	called, accessed := false, false
	var stack [limit]int
	for i < limit {
		switch stream.LA(i) {
		case PHPParserOpenRoundBracket, PHPParserOpenSquareBracket:
			open := stream.LA(i)
			if open == PHPParserOpenRoundBracket {
				called = true
			} else if called {
				accessed = true
			}
			stack[0] = open
			depth := 1
			i++
			for i < limit && depth > 0 {
				token := stream.LA(i)
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
					if stack[depth-1] != want {
						return false
					}
					depth--
				case -1:
					return false
				}
				i++
			}
			if depth != 0 {
				return false
			}
		case PHPParserObjectOperator:
			if !called || !isSimpleMemberIdentifier(stream.LA(i+1)) && stream.LA(i+1) != PHPParserVarName {
				return false
			}
			accessed = true
			i += 2
		case PHPParserEq, PHPParserPlusEqual, PHPParserMinusEqual, PHPParserMulEqual,
			PHPParserPowEqual, PHPParserDivEqual, PHPParserConcaequal, PHPParserModEqual,
			PHPParserAndEqual, PHPParserOrEqual, PHPParserXorEqual,
			PHPParserShiftLeftEqual, PHPParserShiftRightEqual, PHPParserNullCoalescingEqual:
			return called && accessed
		default:
			return false
		}
	}
	return false
}

// Resolve only the overlapping static-variable expression, keeping the same
// original LL choice while avoiding a second parse of its enclosing file.
func (p *PHPParser) IsDynamicStaticVariableAhead() bool {
	input := p.GetTokenStream()
	end, ok := 2, true
	switch input.LA(1) {
	case PHPParserVarName:
		end, ok = variablePrefixEnd(input)
	case PHPParserLabel:
		for steps := 0; steps < prefixTokenLimit && input.LA(end) == PHPParserNamespaceSeparator && input.LA(end+1) == PHPParserLabel; steps++ {
			end += 2
		}
	case PHPParserStatic, PHPParserParent_:
	case PHPParserNamespaceSeparator:
		if input.LA(2) != PHPParserLabel {
			return false
		}
		end = 3
		for steps := 0; steps < prefixTokenLimit && input.LA(end) == PHPParserNamespaceSeparator && input.LA(end+1) == PHPParserLabel; steps++ {
			end += 2
		}
	case PHPParserOpenRoundBracket:
		end, ok = skipBalancedPrefix(input, 1)
	default:
		return false
	}
	return ok && input.LA(end) == PHPParserDoubleColon && (input.LA(end+1) == PHPParserVarName || input.LA(end+1) == PHPParserDollar)
}
