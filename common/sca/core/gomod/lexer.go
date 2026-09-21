// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.
// Derived from golang.org/x/mod v0.19.0 modfile/read.go. Modified for SCA:
// lexer only, context and token budgets; no editor, formatter or file IO.
package gomod

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Position struct{ Line, LineRune, Byte int }
type input struct {
	complete, remaining, tokenStart []byte
	token                           token
	pos                             Position
	ctx                             context.Context
	limits                          Limits
}
type parseFailure struct{ err error }

func (in *input) Error(s string) {
	panic(parseFailure{fmt.Errorf("go.mod:%d:%d: %s", in.pos.Line, in.pos.LineRune, s)})
}
func (in *input) eof() bool {
	return len(in.remaining) == 0
}

// peekRune returns the next rune in the input without consuming it.
func (in *input) peekRune() int {
	if len(in.remaining) == 0 {
		return 0
	}
	r, _ := utf8.DecodeRune(in.remaining)
	return int(r)
}

// peekPrefix reports whether the remaining input begins with the given prefix.
func (in *input) peekPrefix(prefix string) bool {
	// This is like bytes.HasPrefix(in.remaining, []byte(prefix))
	// but without the allocation of the []byte copy of prefix.
	for i := 0; i < len(prefix); i++ {
		if i >= len(in.remaining) || in.remaining[i] != prefix[i] {
			return false
		}
	}
	return true
}

// readRune consumes and returns the next rune in the input.
func (in *input) readRune() int {
	if len(in.remaining) == 0 {
		in.Error("internal lexer error: readRune at EOF")
	}
	r, size := utf8.DecodeRune(in.remaining)
	in.remaining = in.remaining[size:]
	if r == '\n' {
		in.pos.Line++
		in.pos.LineRune = 1
	} else {
		in.pos.LineRune++
	}
	in.pos.Byte += size
	if in.pos.Byte&1023 < size {
		if err := in.ctx.Err(); err != nil {
			panic(parseFailure{err})
		}
	}
	if len(in.tokenStart)-len(in.remaining) > in.limits.MaxTokenBytes {
		panic(parseFailure{ErrLimit})
	}
	return int(r)
}

type token struct {
	kind   tokenKind
	pos    Position
	endPos Position
	text   string
}

type tokenKind int

const (
	_EOF tokenKind = -(iota + 1)
	_EOLCOMMENT
	_IDENT
	_STRING
	_COMMENT

	// newlines and punctuation tokens are allowed as ASCII codes.
)

func (k tokenKind) isComment() bool {
	return k == _COMMENT || k == _EOLCOMMENT
}

// isEOL returns whether a token terminates a line.
func (k tokenKind) isEOL() bool {
	return k == _EOF || k == _EOLCOMMENT || k == '\n'
}

// startToken marks the beginning of the next input token.
// It must be followed by a call to endToken, once the token's text has
// been consumed using readRune.
func (in *input) startToken() {
	in.tokenStart = in.remaining
	in.token.text = ""
	in.token.pos = in.pos
}

// endToken marks the end of an input token.
// It records the actual token string in tok.text.
// A single trailing newline (LF or CRLF) will be removed from comment tokens.
func (in *input) endToken(kind tokenKind) {
	in.token.kind = kind
	text := string(in.tokenStart[:len(in.tokenStart)-len(in.remaining)])
	if kind.isComment() {
		if strings.HasSuffix(text, "\r\n") {
			text = text[:len(text)-2]
		} else {
			text = strings.TrimSuffix(text, "\n")
		}
	}
	in.token.text = text
	in.token.endPos = in.pos
}

// peek returns the kind of the next token returned by lex.
func (in *input) peek() tokenKind {
	return in.token.kind
}

// lex is called from the parser to obtain the next input token.
func (in *input) lex() token {
	tok := in.token
	in.readToken()
	return tok
}

// readToken lexes the next token from the text and stores it in in.token.
func (in *input) readToken() {
	// Skip past spaces, stopping at non-space or EOF.
	for !in.eof() {
		c := in.peekRune()
		if c == ' ' || c == '\t' || c == '\r' {
			in.readRune()
			continue
		}

		// Comment runs to end of line.
		if in.peekPrefix("//") {
			in.startToken()

			// Is this comment the only thing on its line?
			// Find the last \n before this // and see if it's all
			// spaces from there to here.
			i := bytes.LastIndex(in.complete[:in.pos.Byte], []byte("\n"))
			suffix := len(bytes.TrimSpace(in.complete[i+1:in.pos.Byte])) > 0
			in.readRune()
			in.readRune()

			// Consume comment.
			for len(in.remaining) > 0 && in.readRune() != '\n' {
			}

			// If we are at top level (not in a statement), hand the comment to
			// the parser as a _COMMENT token. The grammar is written
			// to handle top-level comments itself.
			if !suffix {
				in.endToken(_COMMENT)
				return
			}

			// Otherwise, save comment for later attachment to syntax tree.
			in.endToken(_EOLCOMMENT)
			// SCA consumes suffix comments directly; no syntax-tree attachment.
			return
		}

		if in.peekPrefix("/*") {
			in.Error("mod files must use // comments (not /* */ comments)")
		}

		// Found non-space non-comment.
		break
	}

	// Found the beginning of the next token.
	in.startToken()

	// End of file.
	if in.eof() {
		in.endToken(_EOF)
		return
	}

	// Punctuation tokens.
	switch c := in.peekRune(); c {
	case '\n', '(', ')', '[', ']', '{', '}', ',':
		in.readRune()
		in.endToken(tokenKind(c))
		return

	case '"', '`': // quoted string
		quote := c
		in.readRune()
		for {
			if in.eof() {
				in.pos = in.token.pos
				in.Error("unexpected EOF in string")
			}
			if in.peekRune() == '\n' {
				in.Error("unexpected newline in string")
			}
			c := in.readRune()
			if c == quote {
				break
			}
			if c == '\\' && quote != '`' {
				if in.eof() {
					in.pos = in.token.pos
					in.Error("unexpected EOF in string")
				}
				in.readRune()
			}
		}
		in.endToken(_STRING)
		return
	}

	// Checked all punctuation. Must be identifier token.
	if c := in.peekRune(); !isIdent(c) {
		in.Error(fmt.Sprintf("unexpected input character %#q", c))
	}

	// Scan over identifier.
	for isIdent(in.peekRune()) {
		if in.peekPrefix("//") {
			break
		}
		if in.peekPrefix("/*") {
			in.Error("mod files must use // comments (not /* */ comments)")
		}
		in.readRune()
	}
	in.endToken(_IDENT)
}

// isIdent reports whether c is an identifier rune.
// We treat most printable runes as identifier runes, except for a handful of
// ASCII punctuation characters.
func isIdent(c int) bool {
	switch r := rune(c); r {
	case ' ', '(', ')', '[', ']', '{', '}', ',':
		return false
	default:
		return !unicode.IsSpace(r) && unicode.IsPrint(r)
	}
}
