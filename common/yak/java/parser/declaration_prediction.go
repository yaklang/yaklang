package javaparser

import "github.com/yaklang/antlr/v4"

const (
	declarationPrefixTokens = 4096
	declarationPrefixDepth  = 512
)

// declarationPrefix recognizes an annotation-free type, its declared name and
// the distinguishing delimiter. It never scans an initializer or method body.
// Return values follow memberDeclaration: method=2, field=4, constructor=5.
// Contextual record/enum declarations, annotations and generic methods remain
// with ATN. All scanner state is local to this invocation.
func (p *JavaParser) declarationPrefix(input antlr.TokenStream) int {
	stream, ok := input.(*antlr.CommonTokenStream)
	if !ok {
		return 0
	}
	index := input.Index()
	if index < 0 || !stream.Sync(index) {
		return 0
	}
	if stream.Get(index).GetChannel() != antlr.TokenDefaultChannel {
		return 0
	}
	s := declarationScanner{stream: stream, index: index, end: index + declarationPrefixTokens}
	token := s.next()
	if token == JavaParserRECORD || token == JavaParserENUM {
		return 0
	}
	primitive, void := isPrimitive(token), token == JavaParserVOID
	if !primitive && !void && !isIdentifier(token) {
		return 0
	}
	s.index++
	if !primitive && !void {
		if s.next() == JavaParserLPAREN {
			return 5
		}
		for {
			if s.next() == JavaParserLT && !s.typeArguments() {
				return 0
			}
			if s.next() != JavaParserDOT {
				break
			}
			s.index++
			if !isIdentifier(s.next()) {
				return 0
			}
			s.index++
		}
	}
	if !void && !s.arrayDimensions() {
		return 0
	}
	if !isIdentifier(s.next()) {
		return 0
	}
	s.index++
	if s.next() == JavaParserLPAREN {
		return 2
	}
	if void || !s.arrayDimensions() {
		return 0
	}
	switch s.next() {
	case JavaParserASSIGN, JavaParserSEMI, JavaParserCOMMA:
		return 4
	}
	return 0
}

type declarationScanner struct {
	stream     *antlr.CommonTokenStream
	index, end int
}

// The qualified-type loop asks whether identifier typeArguments? is followed
// by a dot. Repeated ATN exploration of nested arguments otherwise revisits
// the same generic suffix at each nesting level. Scan only that bounded suffix.
func qualifiedTypePrefix(input antlr.TokenStream) int {
	stream, ok := input.(*antlr.CommonTokenStream)
	if !ok {
		return 0
	}
	index := input.Index()
	if index < 0 || !stream.Sync(index) || stream.Get(index).GetChannel() != antlr.TokenDefaultChannel {
		return 0
	}
	s := declarationScanner{stream: stream, index: index, end: index + declarationPrefixTokens}
	if !isIdentifier(s.next()) {
		return 0
	}
	s.index++
	if s.next() == JavaParserLT && !s.typeArguments() {
		return 0
	}
	switch s.next() {
	case JavaParserDOT:
		return 1
	case antlr.TokenInvalidType:
		return 0
	default:
		return 2
	}
}

func (s *declarationScanner) next() int {
	for s.index < s.end && s.stream.Sync(s.index) {
		token := s.stream.Get(s.index)
		if token.GetChannel() == antlr.TokenDefaultChannel || token.GetTokenType() == antlr.TokenEOF {
			return token.GetTokenType()
		}
		s.index++
	}
	return antlr.TokenInvalidType
}

func (s *declarationScanner) arrayDimensions() bool {
	for s.next() == JavaParserLBRACK {
		s.index++
		if s.next() != JavaParserRBRACK {
			return false
		}
		s.index++
	}
	return s.next() != antlr.TokenInvalidType
}

func (s *declarationScanner) typeArguments() bool {
	depth := 0
	for {
		token := s.next()
		switch token {
		case JavaParserLT:
			if depth == declarationPrefixDepth {
				return false
			}
			depth++
		case JavaParserGT:
			depth--
			if depth == 0 {
				s.index++
				return true
			}
		case JavaParserDOT, JavaParserCOMMA, JavaParserQUESTION, JavaParserEXTENDS, JavaParserSUPER,
			JavaParserLBRACK, JavaParserRBRACK:
		default:
			if !isIdentifier(token) && !isPrimitive(token) {
				return false
			}
		}
		s.index++
	}
}
