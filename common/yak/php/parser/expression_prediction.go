package phpparser

// Namespace-relative names start with namespace\, unlike a declaration.
func (p *PHPParser) IsNamespaceDeclarationAhead() bool {
	return p.GetTokenStream().LA(2) != PHPParserNamespaceSeparator
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
			if !called || stream.LA(i+1) != PHPParserLabel && stream.LA(i+1) != PHPParserVarName {
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
