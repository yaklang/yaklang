package parser

import "github.com/yaklang/antlr/v4"

const (
	statementPrefixTokens = 4096
	statementPrefixDepth  = 512
)

// Calls with a large closure can make statement's overlapping assignment and
// expression alternatives explore the whole closure through the ATN. Scan a
// bounded identifier/suffix prefix instead, then inspect its actual terminator.
// Buffered raw indices avoid quadratic repeated LA(k) traversal. Cache only the
// decision at this parser's current index, so the statement-list loop and its
// statement do not scan the same prefix twice.
func (p *YaklangParser) suffixStatementPrefix(input antlr.TokenStream) int {
	index := input.Index()
	stream, ok := input.(*antlr.CommonTokenStream)
	if !ok {
		return 0
	}
	if stream.Get(index).GetChannel() != antlr.TokenDefaultChannel {
		return 0
	}
	first, _ := stream.Get(index).(*antlr.CommonToken)
	if first != nil && p.prefixToken == first && p.prefixTokenIndex == index+1 {
		return p.prefixAlternative
	}
	p.prefixToken, p.prefixTokenIndex, p.prefixAlternative = first, index+1, 0
	scanner := statementScanner{stream: stream, index: index + 1, end: index + statementPrefixTokens}
	for {
		switch scanner.next() {
		case YaklangParserDot:
			scanner.index++
			if token := scanner.next(); token != YaklangParserIdentifier && token != YaklangParserIdentifierWithDollar {
				return 0
			}
			scanner.index++
		case YaklangParserLParen, YaklangParserLBracket:
			if !scanner.balanced() {
				return 0
			}
		default:
			switch scanner.next() {
			case YaklangParserAssignEq, YaklangParserColonAssignEq,
				YaklangParserPlusPlus, YaklangParserSubSub,
				YaklangParserPlusEq, YaklangParserMinusEq, YaklangParserMulEq,
				YaklangParserDivEq, YaklangParserModEq, YaklangParserAmpEq,
				YaklangParserBitAndEq, YaklangParserBitOrEq, YaklangParserLtLtEq,
				YaklangParserGtGtEq, YaklangParserBitAndNotEq:
				p.prefixAlternative = 3
			case YaklangParserSemiColon, YaklangParserLF, YaklangParserCOMMENT, YaklangParserLINE_COMMENT, antlr.TokenEOF:
				p.prefixAlternative = 4
			}
			return p.prefixAlternative
		}
	}
}

type statementScanner struct {
	stream     *antlr.CommonTokenStream
	index, end int
}

func (s *statementScanner) next() int {
	for s.index < s.end && s.stream.Sync(s.index) {
		token := s.stream.Get(s.index)
		if token.GetChannel() == antlr.TokenDefaultChannel || token.GetTokenType() == antlr.TokenEOF {
			return token.GetTokenType()
		}
		s.index++
	}
	return antlr.TokenInvalidType
}

func (s *statementScanner) balanced() bool {
	var stack [statementPrefixDepth]int
	depth := 0
	for {
		token := s.next()
		switch token {
		case YaklangParserLParen, YaklangParserLBracket, YaklangParserLBrace:
			if depth == len(stack) {
				return false
			}
			stack[depth] = token
			depth++
		case YaklangParserRParen, YaklangParserRBracket, YaklangParserRBrace:
			if depth == 0 {
				return false
			}
			want := YaklangParserLParen
			if token == YaklangParserRBracket {
				want = YaklangParserLBracket
			} else if token == YaklangParserRBrace {
				want = YaklangParserLBrace
			}
			if stack[depth-1] != want {
				return false
			}
			depth--
			if depth == 0 {
				s.index++
				return true
			}
		case antlr.TokenInvalidType, antlr.TokenEOF,
			YaklangParserTemplateSingleQuoteStringStartExpression,
			YaklangParserTemplateDoubleQuoteStringStartExpression,
			YaklangParserTemplateBackTickStringStartExpression:
			// Interpolation uses lexer modes and a distinct closing brace;
			// leave its nesting and every exceeded bound to the original ATN.
			return false
		}
		s.index++
	}
}
