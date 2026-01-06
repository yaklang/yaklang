package pythonparser

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// TabSize is the number of spaces that a tab character represents
const TabSize = 8

// PythonLexerBase provides base functionality for the Python lexer,
// handling indentation, dedentation, and line breaks.
type PythonLexerBase struct {
	*antlr.BaseLexer

	// The amount of opened braces, brackets and parenthesis.
	opened int

	// The stack that keeps track of the indentation level.
	indents []int

	// A circular buffer where extra tokens are pushed on (see the NEWLINE and WS lexer rules).
	firstTokensInd int
	lastTokenInd   int
	buffer         []antlr.Token
	lastToken      antlr.Token

	// Flag to indicate if the base has been initialized
	initialized bool

	// Flag to track if the last matched token was a line join (backslash continuation)
	wasLineJoin bool
}

// NewPythonLexerBase creates a new PythonLexerBase instance.
func NewPythonLexerBase(input antlr.CharStream) *PythonLexerBase {
	base := &PythonLexerBase{
		BaseLexer:   antlr.NewBaseLexer(input),
		indents:     make([]int, 0),
		buffer:      make([]antlr.Token, 32),
		initialized: true,
	}
	return base
}

// ensureInitialized ensures that the lexer base is properly initialized.
// This is needed because ANTLR generated code doesn't call our constructor.
func (p *PythonLexerBase) ensureInitialized() {
	if !p.initialized {
		p.indents = make([]int, 0)
		p.buffer = make([]antlr.Token, 32)
		p.initialized = true
	}
}

// SetInputStream sets the input stream and resets the lexer state.
func (p *PythonLexerBase) SetInputStream(input antlr.CharStream) {
	p.BaseLexer.SetInputStream(input)
	p.opened = 0
	p.indents = p.indents[:0]
	p.firstTokensInd = 0
	p.lastTokenInd = 0
	p.buffer = make([]antlr.Token, 32)
	p.lastToken = nil
}

// Emit emits a token. Regular tokens don't go to the buffer.
// Only synthetic tokens (LINE_BREAK, INDENT, DEDENT) should be in the buffer.
func (p *PythonLexerBase) Emit() antlr.Token {
	p.ensureInitialized()
	token := p.BaseLexer.Emit()
	p.lastToken = token
	return token
}

// NextToken returns the next token, handling the circular buffer and EOF dedents.
func (p *PythonLexerBase) NextToken() antlr.Token {
	p.ensureInitialized()

	// First, check if there are pending synthetic tokens in the buffer
	if p.buffer[p.firstTokensInd] != nil {
		result := p.buffer[p.firstTokensInd]
		p.buffer[p.firstTokensInd] = nil

		if p.firstTokensInd != p.lastTokenInd {
			p.firstTokensInd = p.incTokenInd(p.firstTokensInd)
		}

		return result
	}

	// Check if the end-of-file is ahead and there are still some DEDENTS expected.
	if p.GetInputStream().LA(1) == antlr.TokenEOF && len(p.indents) > 0 {
		// First emit an extra line break that serves as the end of the statement.
		p.emitToken(PythonLexerLINE_BREAK)

		// Now emit as much DEDENT tokens as needed.
		for len(p.indents) != 0 {
			p.emitToken(PythonLexerDEDENT)
			p.indents = p.indents[:len(p.indents)-1]
		}

		// Return from buffer (the LINE_BREAK we just emitted)
		if p.buffer[p.firstTokensInd] != nil {
			result := p.buffer[p.firstTokensInd]
			p.buffer[p.firstTokensInd] = nil

			if p.firstTokensInd != p.lastTokenInd {
				p.firstTokensInd = p.incTokenInd(p.firstTokensInd)
			}

			return result
		}
	}

	// Get the next regular token from the base lexer.
	// This may trigger HandleNewLine/HandleSpaces which add synthetic tokens to the buffer.
	next := p.BaseLexer.NextToken()

	// Track if this token is LINE_JOIN, so the next HandleSpaces call knows
	// not to treat leading whitespace as indentation
	if next.GetTokenType() == PythonLexerLINE_JOIN {
		p.wasLineJoin = true
	}

	// If synthetic tokens were added to the buffer, return from buffer first.
	// The regular token will be returned in subsequent calls.
	if p.buffer[p.firstTokensInd] != nil {
		// Save the regular token at the end of the buffer
		p.addToBuffer(next)

		result := p.buffer[p.firstTokensInd]
		p.buffer[p.firstTokensInd] = nil

		if p.firstTokensInd != p.lastTokenInd {
			p.firstTokensInd = p.incTokenInd(p.firstTokensInd)
		}

		return result
	}

	return next
}

// addToBuffer adds a token to the end of the circular buffer.
func (p *PythonLexerBase) addToBuffer(token antlr.Token) {
	if p.buffer[p.firstTokensInd] != nil {
		p.lastTokenInd = p.incTokenInd(p.lastTokenInd)

		if p.lastTokenInd == p.firstTokensInd {
			// Enlarge buffer
			newArray := make([]antlr.Token, len(p.buffer)*2)
			destInd := len(newArray) - (len(p.buffer) - p.firstTokensInd)

			copy(newArray[0:p.firstTokensInd], p.buffer[0:p.firstTokensInd])
			copy(newArray[destInd:destInd+len(p.buffer)-p.firstTokensInd], p.buffer[p.firstTokensInd:])

			p.firstTokensInd = destInd
			p.buffer = newArray
		}
	}

	p.buffer[p.lastTokenInd] = token
}

// HandleNewLine handles newline characters and processes indentation.
func (p *PythonLexerBase) HandleNewLine() {
	p.ensureInitialized()
	// Note: We don't emit NEWLINE here because the lexer rule already has -> channel(HIDDEN)
	// which will emit the NEWLINE token automatically.

	next := p.GetInputStream().LA(1)

	// Process whitespaces in HandleSpaces
	// If the next character is not space/tab and is actual content (not newline, form feed, or comment),
	// then process the new line with 0 indentation (no leading whitespace).
	if next != ' ' && next != '\t' && p.isNotNewLineOrComment(next) {
		p.processNewLine(0)
	}
}

// HandleSpaces handles whitespace characters and calculates indentation.
func (p *PythonLexerBase) HandleSpaces() {
	p.ensureInitialized()
	next := p.GetInputStream().LA(1)

	// Only process indentation if:
	// 1. We are at the start of a line (TokenStartColumn == 0)
	// 2. The next character is not a newline or comment
	// 3. We did NOT just have a line join (backslash continuation)
	atStartOfLine := p.TokenStartColumn == 0

	// Check if we just had a line join - if so, don't process indentation
	if p.wasLineJoin {
		p.wasLineJoin = false
		return
	}

	if atStartOfLine && p.isNotNewLineOrComment(next) {
		// Calculates the indentation of the provided spaces, taking the
		// following rules into account:
		//
		// "Tabs are replaced (from left to right) by one to eight spaces
		//  such that the total number of characters up to and including
		//  the replacement is a multiple of eight [...]"
		//
		//  -- https://docs.python.org/3.1/reference/lexical_analysis.html#indentation

		indent := 0
		text := p.GetText()

		for i := 0; i < len(text); i++ {
			if text[i] == '\t' {
				indent += TabSize - indent%TabSize
			} else {
				indent++
			}
		}

		p.processNewLine(indent)
	}
}

// IncIndentLevel increments the count of opened braces, brackets, and parentheses.
func (p *PythonLexerBase) IncIndentLevel() {
	p.opened++
}

// DecIndentLevel decrements the count of opened braces, brackets, and parentheses.
func (p *PythonLexerBase) DecIndentLevel() {
	if p.opened > 0 {
		p.opened--
	}
}

// isNotNewLineOrComment checks if the next character is not a newline or comment.
func (p *PythonLexerBase) isNotNewLineOrComment(next int) bool {
	return p.opened == 0 && next != '\r' && next != '\n' && next != '\f' && next != '#'
}

// processNewLine processes a newline and emits INDENT or DEDENT tokens as needed.
func (p *PythonLexerBase) processNewLine(indent int) {
	p.emitToken(PythonLexerLINE_BREAK)

	previous := 0
	if len(p.indents) > 0 {
		previous = p.indents[len(p.indents)-1]
	}

	if indent > previous {
		p.indents = append(p.indents, indent)
		p.emitToken(PythonLexerINDENT)
	} else {
		// Possibly emit more than 1 DEDENT token.
		for len(p.indents) != 0 && p.indents[len(p.indents)-1] > indent {
			p.emitToken(PythonLexerDEDENT)
			p.indents = p.indents[:len(p.indents)-1]
		}
	}
}

// incTokenInd increments the token index in the circular buffer.
func (p *PythonLexerBase) incTokenInd(ind int) int {
	return (ind + 1) % len(p.buffer)
}

// emitToken emits a token with the default channel.
func (p *PythonLexerBase) emitToken(tokenType int) {
	p.emitTokenWithChannel(tokenType, antlr.TokenDefaultChannel)
}

// emitTokenWithChannel emits a token with the specified channel.
func (p *PythonLexerBase) emitTokenWithChannel(tokenType int, channel int) {
	charIndex := p.GetCharPositionInLine()
	text := p.GetText()
	start := p.TokenStartCharIndex
	stop := start + len(text) - 1

	token := p.GetTokenFactory().Create(
		p.GetTokenSourceCharStreamPair(),
		tokenType,
		text,
		channel,
		start,
		stop,
		p.GetLine(),
		charIndex,
	)

	p.EmitToken(token)
}

// EmitToken adds a synthetic token to the buffer. This is called by processNewLine
// to emit LINE_BREAK, INDENT, and DEDENT tokens.
func (p *PythonLexerBase) EmitToken(token antlr.Token) {
	p.ensureInitialized()
	p.addToBuffer(token)
}
