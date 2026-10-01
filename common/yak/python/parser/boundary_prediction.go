package pythonparser

import "github.com/yaklang/antlr/v4"

const (
	boundaryPredictionTokens = 4096 // Includes hidden trivia, not just visible tokens.
	boundaryPredictionDepth  = 512
)

type boundaryScanner struct {
	stream     *antlr.CommonTokenStream
	index, end int
	brackets   [boundaryPredictionDepth]uint8
	depth      int
}

func newBoundaryScanner(input antlr.TokenStream) (boundaryScanner, bool) {
	stream, ok := input.(*antlr.CommonTokenStream)
	index := input.Index()
	return boundaryScanner{stream: stream, index: index, end: index + boundaryPredictionTokens}, ok && index >= 0
}

func (s *boundaryScanner) next() int {
	for s.index < s.end && s.stream.Sync(s.index) {
		token := s.stream.Get(s.index)
		s.index++
		if token.GetChannel() == antlr.TokenDefaultChannel || token.GetTokenType() == antlr.TokenEOF {
			return token.GetTokenType()
		}
	}
	return antlr.TokenInvalidType
}

func (s *boundaryScanner) bracket(token int) bool {
	switch token {
	case PythonParserOPEN_PAREN, PythonParserOPEN_BRACE, PythonParserOPEN_BRACKET:
		if s.depth == len(s.brackets) {
			return false
		}
		s.brackets[s.depth] = uint8(token)
		s.depth++
	case PythonParserCLOSE_PAREN, PythonParserCLOSE_BRACE, PythonParserCLOSE_BRACKET:
		if s.depth == 0 {
			return false
		}
		open := int(s.brackets[s.depth-1])
		if (token == PythonParserCLOSE_PAREN && open != PythonParserOPEN_PAREN) ||
			(token == PythonParserCLOSE_BRACE && open != PythonParserOPEN_BRACE) ||
			(token == PythonParserCLOSE_BRACKET && open != PythonParserOPEN_BRACKET) {
			return false
		}
		s.depth--
	}
	return true
}

// A simple statement followed by EOF takes root's first alternative, whereas
// further lines take file_input. Compound suites and soft match stay on ATN:
// synthetic INDENT/DEDENT and the extra single_input newline overlap there.
func rootPrefix(input antlr.TokenStream) int {
	token := input.LA(1)
	if token == antlr.TokenEOF {
		return 4
	}
	if token == PythonParserNAME && input.LA(2) != PythonParserASSIGN && input.LT(1).GetText() == "match" {
		return 0
	}
	if token != PythonParserNAME && token != PythonParserPRINT && token != PythonParserEXEC && token != PythonParserNONLOCAL && token != PythonParserLINE_BREAK && smallPrefix(token) == 0 {
		return 0
	}
	s, ok := newBoundaryScanner(input)
	if !ok {
		return 0
	}
	for {
		token = s.next()
		switch token {
		case antlr.TokenInvalidType, PythonParserINDENT, PythonParserDEDENT:
			return 0
		case antlr.TokenEOF:
			if s.depth == 0 {
				return 1
			}
			return 0
		case PythonParserLINE_BREAK:
			if s.depth != 0 {
				return 0
			}
			switch s.next() {
			case antlr.TokenEOF:
				return 1
			case antlr.TokenInvalidType:
				return 0
			default:
				return 2
			}
		}
		if !s.bracket(token) {
			return 0
		}
	}
}

// A top-level comma distinguishes tuple/star_expr from a single testlist.
// Commas inside calls, indexing and displays cannot terminate the test. Lambda
// parameter commas and Python 2 repr delimiters require full ATN prediction.
func tuplePrefix(input antlr.TokenStream) int {
	if input.LA(1) == PythonParserSTAR {
		return 1
	}
	s, ok := newBoundaryScanner(input)
	if !ok {
		return 0
	}
	for {
		token := s.next()
		if token == antlr.TokenInvalidType || token == PythonParserREVERSE_QUOTE {
			return 0
		}
		if s.depth == 0 {
			switch token {
			case PythonParserLAMBDA:
				return 0
			case PythonParserCOMMA:
				return 1
			case PythonParserASSIGN, PythonParserCOLON, PythonParserLINE_BREAK, PythonParserSEMI_COLON, antlr.TokenEOF,
				PythonParserADD_ASSIGN, PythonParserSUB_ASSIGN, PythonParserMULT_ASSIGN, PythonParserDIV_ASSIGN,
				PythonParserAT_ASSIGN, PythonParserMOD_ASSIGN, PythonParserAND_ASSIGN, PythonParserOR_ASSIGN, PythonParserXOR_ASSIGN,
				PythonParserLEFT_SHIFT_ASSIGN, PythonParserRIGHT_SHIFT_ASSIGN, PythonParserPOWER_ASSIGN, PythonParserIDIV_ASSIGN:
				return 2
			}
		}
		if token == antlr.TokenEOF || !s.bracket(token) {
			return 0
		}
	}
}
