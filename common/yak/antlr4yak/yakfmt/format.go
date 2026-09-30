// Package yakfmt formats Yak source without compiling or resolving symbols.
package yakfmt

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

// Version identifies the canonical formatting style.
const Version = "0.2.0"

const lineWidth = 100

// Format returns canonical source with four-space indentation and a final LF.
// Invalid source returns an empty result and the first syntax error. Literal and
// comment contents are preserved, including CRLF inside strings and heredocs.
func Format(source string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*syntaxError); ok {
				result, err = "", e
			} else {
				panic(r)
			}
		}
	}()
	if strings.TrimSpace(source) == "" {
		return "", nil
	}
	errors := &errorListener{DefaultErrorListener: antlr.NewDefaultErrorListener()}
	lexer := parser.NewYaklangLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errors)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewYaklangParser(stream)
	p.RemoveErrorListeners()
	p.SetErrorHandler(&bailErrorStrategy{antlr.NewDefaultErrorStrategy()})
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
	tree, ok := trySLL(p)
	if !ok {
		tree = retryLL(tree, stream, errors)
	}
	return FormatTree(source, tree, stream), nil
}

// The runtime's BailErrorStrategy sets an error and continues recovering. A
// formatter must actually stop the SLL pass; LL reports the first error without
// recovery, avoiding both partial output and error storms on malformed input.
type bailErrorStrategy struct{ *antlr.DefaultErrorStrategy }

func (b *bailErrorStrategy) Recover(antlr.Parser, antlr.RecognitionException) {
	panic(antlr.NewParseCancellationException())
}
func (b *bailErrorStrategy) RecoverInline(p antlr.Parser) antlr.Token { b.Recover(p, nil); return nil }
func (b *bailErrorStrategy) Sync(antlr.Parser)                        {}

type syntaxError struct {
	line, column int
	message      string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("line %d:%d %s", e.line, e.column, e.message)
}

type errorListener struct{ *antlr.DefaultErrorListener }

func (l *errorListener) SyntaxError(_ antlr.Recognizer, _ interface{}, line, column int, msg string, _ antlr.RecognitionException) {
	panic(&syntaxError{line, column, msg})
}
func trySLL(p *parser.YaklangParser) (tree parser.IProgramContext, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, cancel := r.(*antlr.ParseCancellationException); cancel {
				// Only completed top-level statements can be reused. Their
				// boundaries are closed in this grammar; never reuse an
				// incomplete function/block/expression after SLL failure.
				for ctx := p.GetParserRuleContext(); ctx != nil; {
					if root, isProgram := ctx.(*parser.ProgramContext); isProgram {
						tree = root
						break
					}
					ctx, _ = ctx.GetParent().(antlr.ParserRuleContext)
				}
				ok = false
			} else {
				panic(r)
			}
		}
	}()
	return p.Program(), true
}

// Whole-program LL replay recomputes full-context predictions for every valid
// prefix statement when an editor buffer has a malformed tail. Retry from the
// last completed top-level statement boundary instead. No enclosing rule spans
// that boundary in Yak's program: ws* statementList EOF grammar.
func retryLL(prefix parser.IProgramContext, stream *antlr.CommonTokenStream, errors *errorListener) parser.IProgramContext {
	var statements *parser.StatementListContext
	completed, start := 0, 0
	if prefix != nil && prefix.StatementList() != nil {
		statements = prefix.StatementList().(*parser.StatementListContext)
		for _, child := range statements.GetChildren() {
			stmt, ok := child.(*parser.StatementContext)
			if !ok || stmt.GetStop() == nil {
				break
			}
			completed++
			start = stmt.GetStop().GetTokenIndex() + 1
		}
	}
	stream.Seek(start)
	p := parser.NewYaklangParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(errors)
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeLL)
	suffix := p.Program()
	if completed == 0 {
		return suffix
	}
	for statements.GetChildCount() > completed {
		statements.RemoveLastChild()
	}
	for _, child := range suffix.GetChildren() {
		switch c := child.(type) {
		case *parser.WsContext:
			c.SetParent(statements)
			statements.AddChild(c)
		case *parser.StatementListContext:
			for _, stmt := range c.GetChildren() {
				stmt.SetParent(statements)
				statements.AddChild(stmt.(antlr.RuleContext))
			}
		case antlr.TerminalNode:
			prefix.AddTokenNode(c.GetSymbol())
		}
	}
	statements.SetStop(suffix.StatementList().GetStop())
	prefix.SetStop(suffix.GetStop())
	return prefix
}

// FormatTree reuses an already validated program and its tokens. Layout visits
// each child once; it never calls GetText on a subtree or generated AllX getters
// (both can turn large left-recursive expressions/lists into quadratic work).
func FormatTree(source string, tree parser.IProgramContext, stream *antlr.CommonTokenStream) string {
	stream.Fill()
	f := &printer{source: source, tokens: stream.GetAllTokens()}
	f.marks = make([]layout, len(f.tokens)+1)
	// ANTLR offsets count runes. ASCII needs no offset map or token text copies.
	if !isASCII(source) {
		f.offsets = make([]int, 0, utf8.RuneCountInString(source)+1)
		for offset := range source {
			f.offsets = append(f.offsets, offset)
		}
		f.offsets = append(f.offsets, len(source))
	}
	f.annotate(tree)
	f.out.Grow(len(source) + len(f.tokens))
	f.print()

	if f.out.Len() > 0 {
		f.out.WriteByte('\n')
	}
	return f.out.String()
}
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
