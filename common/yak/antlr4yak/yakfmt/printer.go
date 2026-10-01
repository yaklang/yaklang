package yakfmt

import (
	"strings"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

type printer struct {
	continuationEnds map[int]int
	parenEnds        map[int]int
	commaAfterParen  map[int]bool
	widths           []int
	groups           []groupLayout
	breaks           []int
	source           string
	offsets          []int
	tokens           []antlr.Token
	marks            []layout
	out              strings.Builder
	indent           int
	pending          int // requested line breaks, emitted only before the next token
	prev             int
	prevText         string
	column           int
}

func isComment(typ int) bool {
	return typ == parser.YaklangLexerCOMMENT || typ == parser.YaklangLexerLINE_COMMENT
}
func (f *printer) newline(n int) {
	if n > f.pending {
		f.pending = n
	}
}
func (f *printer) emit(text string, space bool) {
	if f.pending > 0 && f.out.Len() > 0 {
		for n := f.pending; n > 0; n-- {
			f.out.WriteByte('\n')
		}
		f.column = 0
	}
	if f.out.Len() == 0 || f.pending > 0 {
		for i := 0; i < f.indent; i++ {
			f.out.WriteString("    ")
		}
		f.column = 4 * f.indent
	} else if space {
		f.out.WriteByte(' ')
		f.column++
	}
	f.pending = 0
	f.out.WriteString(text)
	if n := strings.LastIndexByte(text, '\n'); n >= 0 {
		f.column = len(text) - n - 1
	} else {
		f.column += len(text)
	}
}
func (f *printer) print() {
	f.prev = -1
	// Each case group remembers whether a clause body has begun.
	cases := make([]bool, 0, 8)
	for i := 0; i < len(f.tokens); i++ {
		t := f.tokens[i]
		if f.continuationEnds != nil {
			f.indent -= f.continuationEnds[i]
		}
		if f.parenEnds != nil {
			for n := f.parenEnds[i]; n > 0; n-- {
				f.indent--
				f.newline(1)
				f.emit(")", false)
				f.prev = i - 1
				f.prevText = ")"
			}
			if f.commaAfterParen[i] {
				f.out.WriteByte(',')
				f.column++
				f.prevText = ","
			}
		}
		m := f.marks[i]
		if m.flags&groupCandidate != 0 {
			index := -int(m.end) - 1
			g := f.groups[index]
			col := f.column
			if f.pending > 0 {
				col = 4 * f.indent
			}
			if col+f.widths[g.b]-f.widths[g.a] > lineWidth-8 {
				f.applyGroup(g, index, true)
				m = f.marks[i]
			}
		}
		typ := t.GetTokenType()
		text := f.text(i)
		f.indent += int(m.delta)
		if m.flags&statementEnd != 0 {
			f.newline(1)
		}
		if typ == antlr.TokenEOF {
			break
		}
		if typ == parser.YaklangLexerLF || m.flags&skipToken != 0 {
			continue
		}
		// Template characters can also contain a literal semicolon. Only
		// statement separator tokens (including lexer-inserted ones) are skipped.
		if typ == parser.YaklangLexerSemiColon && m.flags&headerSemi == 0 {
			continue
		}
		if m.flags&caseLabel != 0 {
			if len(cases) > 0 && cases[len(cases)-1] {
				f.indent--
				cases[len(cases)-1] = false
			}
			f.newline(1)
		}
		if m.flags&caseClose != 0 {
			if len(cases) > 0 {
				if cases[len(cases)-1] {
					f.indent--
				}
				cases = cases[:len(cases)-1]
			}
			f.newline(1)
		}
		if m.flags&(blockClose|listClose) != 0 {
			f.newline(1)
		}
		if m.flags&statementStart != 0 {
			f.newline(1)
		}
		comment := isComment(typ)
		if f.prev >= 0 {
			prev := f.tokens[f.prev]
			sameLine := t.GetLine() == prev.GetLine()+strings.Count(f.prevText, "\n")
			if comment && sameLine && m.flags&(blockClose|listClose) == 0 {
				f.pending = 0
			}
			if comment && !sameLine {
				f.newline(1)
			}
			if f.pending > 0 && t.GetLine() > prev.GetLine()+strings.Count(f.prevText, "\n")+1 && m.flags&(blockClose|caseClose|listClose|caseLabel) == 0 {
				f.newline(2)
			}
		}
		// Continuations after a block (else/catch/finally and expression suffixes).
		if text == "else" || text == "elif" || text == "catch" || text == "finally" {
			f.pending = 0
		}
		if m.flags&parenBefore != 0 && m.flags&wrapBefore == 0 && f.pending == 0 && f.prev >= 0 && f.column+len(text)+1 > lineWidth {
			f.emit("(", true)
			f.newline(1)
			f.indent++
			if f.parenEnds == nil {
				f.parenEnds = make(map[int]int)
			}
			f.parenEnds[int(m.wrapEnd)]++
		}
		if m.flags&wrapBefore != 0 && f.pending == 0 && f.prev >= 0 && f.column+len(text)+1 > lineWidth-8 {
			f.newline(1)
			f.indent++
			if f.continuationEnds == nil {
				f.continuationEnds = make(map[int]int)
			}
			f.continuationEnds[int(m.wrapEnd)]++
		}
		space := f.prev >= 0 && f.pending == 0
		if space {
			prevFlags := f.marks[f.prev].flags
			if m.flags&tightBefore != 0 || prevFlags&tightAfter != 0 || text == "," || text == ";" || text == ")" || text == "]" || text == "." || text == "~" || text == "..." || text == "++" || text == "--" || f.prevText == "." || (text == ":" && m.flags&spacedColon == 0) || f.prevText == "(" || f.prevText == "[" || (f.prevText == "{" && prevFlags&(blockOpen|caseOpen) == 0) || (text == "}" && m.flags&(blockClose|caseClose) == 0) {
				space = false
			}
			// Do not join two unary operators into a different lexer token.
			if prevFlags&unary != 0 && ((f.prevText == "+" && strings.HasPrefix(text, "+")) || (f.prevText == "-" && strings.HasPrefix(text, "-")) || (f.prevText == "&" && (strings.HasPrefix(text, "&") || strings.HasPrefix(text, "^")))) {
				space = true
			}
		}
		if m.flags&raw != 0 {
			end := int(m.end)
			text = f.source[f.byteOffset(t.GetStart()):f.byteOffset(f.tokens[end].GetStop()+1)]
			m.flags |= f.marks[end].flags & trailingComma
			f.emit(text, space)
			f.prev = i
			f.prevText = text
			i = end
		} else {
			f.emit(text, space)
			f.prev = i
			f.prevText = text
		}
		if m.flags&trailingComma != 0 {
			if f.parenEnds[i+1] > 0 {
				if f.commaAfterParen == nil {
					f.commaAfterParen = make(map[int]bool)
				}
				f.commaAfterParen[i+1] = true
			} else {
				f.out.WriteByte(',')
				f.column++
				f.prevText = ","
			}
		}
		if m.flags&blockOpen != 0 {
			f.indent++
			f.newline(1)
		}
		if m.flags&caseOpen != 0 {
			cases = append(cases, false)
			f.newline(1)
		}
		if m.flags&caseColon != 0 {
			f.indent++
			if len(cases) > 0 {
				cases[len(cases)-1] = true
			}
			f.newline(1)
		}
		if m.flags&listOpen != 0 {
			f.indent++
			f.newline(1)
		}
		if m.flags&listComma != 0 {
			f.newline(1)
		}
		if typ == parser.YaklangLexerLINE_COMMENT || (comment && strings.Contains(text, "\n")) {
			f.newline(1)
		}
	}
}
