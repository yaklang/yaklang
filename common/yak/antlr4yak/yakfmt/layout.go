package yakfmt

import (
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

type flags uint32

const (
	tightBefore flags = 1 << iota
	tightAfter
	unary
	statementStart
	statementEnd
	headerSemi
	blockOpen
	blockClose
	caseOpen
	caseClose
	caseLabel
	caseColon
	listOpen
	listClose
	listComma
	raw
	trailingComma
	wrapBefore
	groupCandidate
	parenBefore
	spacedColon
)

type layout struct {
	flags   flags
	end     int32 // verbatim span end or encoded compact group index
	wrapEnd int32 // end of a continuation expression
	delta   int16
}

type groupLayout struct {
	a, b                int
	context             antlr.ParserRuleContext
	breakable, trailing bool
}

func (f *printer) byteOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	if f.offsets != nil {
		if offset >= len(f.offsets) {
			return len(f.source)
		}
		return f.offsets[offset]
	}
	if offset > len(f.source) {
		return len(f.source)
	}
	return offset
}
func (f *printer) text(i int) string {
	t := f.tokens[i]
	if t.GetTokenType() == antlr.TokenEOF {
		return ""
	}
	if t.GetStop() < t.GetStart() {
		return ";"
	} // lexer-inserted semicolon
	return f.source[f.byteOffset(t.GetStart()):f.byteOffset(t.GetStop()+1)]
}
func bounds(c antlr.ParserRuleContext) (int, int) {
	return c.GetStart().GetTokenIndex(), c.GetStop().GetTokenIndex()
}
func (f *printer) delimiters(c antlr.ParserRuleContext, open, close string) (int, int) {
	a, b := bounds(c)
	// Only direct children: nested function/literal delimiters are not ours.
	for _, child := range c.GetChildren() {
		if t, ok := child.(antlr.TerminalNode); ok {
			i := t.GetSymbol().GetTokenIndex()
			switch f.text(i) {
			case open:
				a = i
			case close:
				b = i
			}
		}
	}
	return a, b
}
func (f *printer) group(c antlr.ParserRuleContext, open, close string, breakable, trailing bool) {
	a, b := f.delimiters(c, open, close)
	if f.text(a) != open || f.text(b) != close {
		return
	}
	f.groups = append(f.groups, groupLayout{a, b, c, breakable, trailing})
}
func (f *printer) applyGroup(g groupLayout, groupIndex int, force bool) {
	c, breakable, trailing := g.context, g.breakable, g.trailing
	a, b := g.a, g.b
	f.marks[a].flags |= tightAfter
	f.marks[b].flags |= tightBefore
	if !breakable || f.emptyGroup(a, b) {
		return
	}
	multiline := force || f.breaks[b]-f.breaks[a+1] > 0 || f.tokens[b].GetLine() > f.tokens[a].GetLine() || f.widths[b]-f.widths[a] > lineWidth
	if !multiline {
		f.marks[a].flags |= groupCandidate
		f.marks[a].end = int32(-groupIndex - 1)
		return
	}
	f.marks[a].flags &^= groupCandidate
	f.marks[a].flags |= listOpen
	// makeExpression has no ws* before its final ')'. Its last argument
	// must share the closing line; calls, lists and result types allow an LF.
	if c.GetRuleIndex() != parser.YaklangParserRULE_makeExpression {
		f.marks[b].flags |= listClose
	}
	f.marks[b].delta--
	// A make call and result annotation also have direct commas.
	for _, child := range c.GetChildren() {
		if t, ok := child.(antlr.TerminalNode); ok && t.GetSymbol().GetTokenType() == parser.YaklangLexerComma {
			f.marks[t.GetSymbol().GetTokenIndex()].flags |= listComma
		}
	}
	// The commas belonging to this group are direct children of its list rule.
	for _, child := range c.GetChildren() {
		if rule, ok := child.(antlr.ParserRuleContext); ok {
			switch rule.GetRuleIndex() {
			case parser.YaklangParserRULE_ordinaryArguments, parser.YaklangParserRULE_functionParamDecl,
				parser.YaklangParserRULE_expressionListMultiline, parser.YaklangParserRULE_mapPairs:
				for _, entry := range rule.GetChildren() {
					if token, ok := entry.(antlr.TerminalNode); ok && token.GetSymbol().GetTokenType() == parser.YaklangLexerComma {
						f.marks[token.GetSymbol().GetTokenIndex()].flags |= listComma
					}
				}
			}
		}
	}
	if trailing { // insert a trailing comma before any final comment
		last := b - 1
		for last > a && (f.text(last) == ";" || f.tokens[last].GetTokenType() == parser.YaklangLexerLF || isComment(f.tokens[last].GetTokenType())) {
			last--
		}
		if last > a && f.text(last) != "," {
			f.marks[last].flags |= trailingComma
		}
	}
}
func (f *printer) annotate(tree antlr.ParserRuleContext) {
	stack := []antlr.ParserRuleContext{tree}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c := node
		a, b := bounds(c)
		if a < 0 || b < a || b >= len(f.tokens) {
			continue
		}
		skip := false
		switch c.GetRuleIndex() {
		case parser.YaklangParserRULE_statement:
			// Empty statements and comment statements are handled by their tokens.
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok && r.GetRuleIndex() != parser.YaklangParserRULE_empty && r.GetRuleIndex() != parser.YaklangParserRULE_lineCommentStmt && r.GetRuleIndex() != parser.YaklangParserRULE_eos {
					f.marks[a].flags |= statementStart
					f.marks[b+1].flags |= statementEnd
					break
				}
			}
		case parser.YaklangParserRULE_block:
			a, b = f.delimiters(c, "{", "}")
			if f.emptyGroup(a, b) {
				f.marks[a].flags |= tightAfter
				f.marks[b].flags |= tightBefore
			} else {
				f.marks[a].flags |= blockOpen
				f.marks[b].flags |= blockClose
				f.marks[b].delta--
			}
		case parser.YaklangParserRULE_switchStmt, parser.YaklangParserRULE_selectStmt:
			a, b = f.delimiters(c, "{", "}")
			if f.emptyGroup(a, b) {
				f.marks[a].flags |= tightAfter
				f.marks[b].flags |= tightBefore
			} else {
				f.marks[a].flags |= caseOpen
				f.marks[b].flags |= caseClose
			}
			// Switch labels are direct terminals; select labels live in clauses.
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok {
					idx := t.GetSymbol().GetTokenIndex()
					switch f.text(idx) {
					case "case", "default":
						f.marks[idx].flags |= caseLabel
					case ":":
						f.marks[idx].flags |= caseColon
					}
				}
			}
		case parser.YaklangParserRULE_selectClause:
			f.marks[a].flags |= caseLabel
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok && f.text(t.GetSymbol().GetTokenIndex()) == ":" {
					f.marks[t.GetSymbol().GetTokenIndex()].flags |= caseColon
				}
			}
		case parser.YaklangParserRULE_forStmtCond, parser.YaklangParserRULE_ifStmt:
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok && f.text(t.GetSymbol().GetTokenIndex()) == ";" {
					f.marks[t.GetSymbol().GetTokenIndex()].flags |= headerSemi
				}
			}
		case parser.YaklangParserRULE_unaryOperator:
			f.marks[a].flags |= unary | tightAfter
		case parser.YaklangParserRULE_sliceCall:
			f.marks[a].flags |= tightBefore
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok && f.text(t.GetSymbol().GetTokenIndex()) == ":" {
					f.marks[t.GetSymbol().GetTokenIndex()].flags |= tightBefore | tightAfter
				}
			}
			f.group(c, "[", "]", false, false)
		case parser.YaklangParserRULE_sliceTypeLiteral, parser.YaklangParserRULE_mapTypeLiteral:
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok && f.text(t.GetSymbol().GetTokenIndex()) == "[" && c.GetRuleIndex() == parser.YaklangParserRULE_mapTypeLiteral {
					f.marks[t.GetSymbol().GetTokenIndex()].flags |= tightBefore
				}
			}
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok {
					start, _ := bounds(r)
					f.marks[start].flags |= tightBefore
				}
			}
		case parser.YaklangParserRULE_functionCall:
			f.marks[a].flags |= tightBefore
			f.group(c, "(", ")", true, true)
		case parser.YaklangParserRULE_anonymousFunctionDecl:
			f.group(c, "(", ")", true, true)
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok && f.text(t.GetSymbol().GetTokenIndex()) == "(" && f.text(a) != "(" {
					f.marks[t.GetSymbol().GetTokenIndex()].flags |= tightBefore
				}
			}
		case parser.YaklangParserRULE_functionResultType:
			f.group(c, "(", ")", true, false)
		case parser.YaklangParserRULE_parenExpression:
			f.group(c, "(", ")", true, false)
		case parser.YaklangParserRULE_makeExpression, parser.YaklangParserRULE_panicStmt, parser.YaklangParserRULE_recoverStmt:
			open, _ := f.delimiters(c, "(", ")")
			f.marks[open].flags |= tightBefore
			f.group(c, "(", ")", true, false)
		case parser.YaklangParserRULE_expression:
			// Type conversions, ternary colons and other operators.
			for _, child := range c.GetChildren() {
				if t, ok := child.(antlr.TerminalNode); ok {
					idx := t.GetSymbol().GetTokenIndex()
					switch f.text(idx) {
					case "(":
						f.marks[idx].flags |= tightBefore | tightAfter
					case ")":
						f.marks[idx].flags |= tightBefore
					case ":":
						f.marks[idx].flags |= spacedColon
					}
				}
			}
		case parser.YaklangParserRULE_sliceLiteral:
			f.group(c, "[", "]", true, true)
		case parser.YaklangParserRULE_mapLiteral, parser.YaklangParserRULE_mapTypedLiteral, parser.YaklangParserRULE_sliceTypedLiteral:
			if f.text(a) != "{" && c.GetRuleIndex() == parser.YaklangParserRULE_mapLiteral {
				break
			}
			open, _ := f.delimiters(c, "{", "}")
			if open != a {
				f.marks[open].flags |= tightBefore
			}
			f.group(c, "{", "}", true, true)
		case parser.YaklangParserRULE_stringLiteral:
			f.marks[a].flags |= raw
			f.marks[a].end = int32(b)
			skip = true
		case parser.YaklangParserRULE_templateSingleQuoteStringAtom, parser.YaklangParserRULE_templateDoubleQuoteStringAtom, parser.YaklangParserRULE_templateBackTickStringAtom:
			typ := f.tokens[a].GetTokenType()
			if typ == parser.YaklangLexerTemplateSingleQuoteStringCharacter || typ == parser.YaklangLexerTemplateDoubleQuoteStringCharacter || typ == parser.YaklangLexerTemplateBackTickStringCharacter {
				f.marks[a].flags |= raw | tightBefore | tightAfter
				f.marks[a].end = int32(b)
				skip = true
			} else {
				f.marks[a].flags |= tightBefore | tightAfter
				f.marks[b].flags |= tightBefore | tightAfter
			}
		case parser.YaklangParserRULE_templateSingleQuoteStringLiteral, parser.YaklangParserRULE_templateDoubleQuoteStringLiteral, parser.YaklangParserRULE_templateBackTickStringLiteral:
			f.marks[a].flags |= tightAfter
			f.marks[b].flags |= tightBefore
		}
		switch c.GetRuleIndex() {
		case parser.YaklangParserRULE_expression:
			var previous antlr.Tree
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok {
					if r.GetRuleIndex() == parser.YaklangParserRULE_ws {
						continue
					}
					if r.GetRuleIndex() == parser.YaklangParserRULE_expression && canWrapAfter(previous) {
						f.allowWrap(r)
					}
				}
				previous = child
			}
		case parser.YaklangParserRULE_functionParam:
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok && r.GetRuleIndex() == parser.YaklangParserRULE_funcTypeRef {
					f.allowWrap(r)
				}
			}
		case parser.YaklangParserRULE_mapPair:
			expressionIndex := 0
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok && r.GetRuleIndex() == parser.YaklangParserRULE_expression {
					expressionIndex++
					if expressionIndex == 2 {
						f.allowParen(r)
					}
				}
			}
		case parser.YaklangParserRULE_expressionList, parser.YaklangParserRULE_leftExpressionList, parser.YaklangParserRULE_ordinaryArguments, parser.YaklangParserRULE_expressionListMultiline:
			first := true
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok && (r.GetRuleIndex() == parser.YaklangParserRULE_expression || r.GetRuleIndex() == parser.YaklangParserRULE_leftExpression) {
					if !first || c.GetRuleIndex() == parser.YaklangParserRULE_ordinaryArguments || c.GetRuleIndex() == parser.YaklangParserRULE_expressionListMultiline {
						f.allowWrap(r)
					} else if c.GetRuleIndex() == parser.YaklangParserRULE_expressionList {
						f.allowParen(r)
					}
					first = false
				}
			}
		}
		if !skip {
			children := node.GetChildren()
			for i := len(children) - 1; i >= 0; i-- {
				if child, ok := children[i].(antlr.ParserRuleContext); ok {
					stack = append(stack, child)
				}
			}
		}
	}
	f.breaks = make([]int, len(f.tokens)+1)
	f.widths = make([]int, len(f.tokens)+1)
	for i, m := range f.marks[:len(f.tokens)] {
		f.breaks[i+1] = f.breaks[i]
		f.widths[i+1] = f.widths[i]
		// A stable conservative width, independent of input trivia. Measuring
		// the raw source span would wrap compact lists only on the SECOND run
		// once spaces had expanded them. One separator per token bounds width.
		text := f.text(i)
		if typ := f.tokens[i].GetTokenType(); typ != parser.YaklangLexerLF && typ != antlr.TokenEOF && text != ";" {
			f.widths[i+1] += len(text) + 1
		}
		if m.flags&(blockOpen|blockClose|caseOpen|caseClose) != 0 {
			f.breaks[i+1]++
		}
	}
	for i := len(f.groups) - 1; i >= 0; i-- {
		g := f.groups[i]
		f.applyGroup(g, i, false)
	}

}
func canWrapAfter(node antlr.Tree) bool {
	if r, ok := node.(antlr.ParserRuleContext); ok {
		switch r.GetRuleIndex() {
		case parser.YaklangParserRULE_bitBinaryOperator, parser.YaklangParserRULE_multiplicativeBinaryOperator, parser.YaklangParserRULE_additiveBinaryOperator, parser.YaklangParserRULE_comparisonBinaryOperator:
			return true
		}
	}
	if t, ok := node.(antlr.TerminalNode); ok {
		switch t.GetSymbol().GetTokenType() {
		case parser.YaklangLexerLogicAnd, parser.YaklangLexerLogicOr, parser.YaklangLexerQuestion, parser.YaklangLexerColon:
			return true
		}
	}
	return false
}
func (f *printer) allowWrap(c antlr.ParserRuleContext) {
	a, b := bounds(c)
	if f.marks[a].flags&statementStart != 0 {
		return
	}
	f.marks[a].flags |= wrapBefore
	// Multiple rules may begin at the same token; retain the outer scope.
	if end := int32(b + 1); end > f.marks[a].wrapEnd {
		f.marks[a].wrapEnd = end
	}
}

// Assignment/return and map-value positions do not accept a bare LF. A
// parenthesized continuation is legal there and preserves the expression tree.
func (f *printer) allowParen(c antlr.ParserRuleContext) {
	a, b := bounds(c)
	if f.text(a) == "(" {
		return
	}
	f.marks[a].flags |= parenBefore
	if end := int32(b + 1); end > f.marks[a].wrapEnd {
		f.marks[a].wrapEnd = end
	}
}
func (f *printer) emptyGroup(a, b int) bool {
	for i := a + 1; i < b; i++ {
		if f.tokens[i].GetTokenType() != parser.YaklangLexerLF && f.text(i) != ";" {
			return false
		}
	}
	return true
}
