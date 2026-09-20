package sfvm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
)

func parseMemoryTestHereDoc(input string) *sf.HereDocContext {
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(input))
	parser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	return parser.HereDoc().(*sf.HereDocContext)
}

func TestHereDocTextPreservesContent(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		for _, body := range []string{"", " a\t中🙂 <>& ", strings.Repeat(" rule 中🙂\t"+newline, 200)} {
			doc := parseMemoryTestHereDoc("<<<END" + newline + body + newline + "END" + newline)
			var expected string
			if lf := doc.LfHereDoc(); lf != nil && lf.LfText() != nil {
				expected = lf.LfText().GetText()
			} else if crlf := doc.CrlfHereDoc(); crlf != nil && crlf.CrlfText() != nil {
				expected = crlf.CrlfText().GetText()
			}
			require.Equal(t, expected, NewSyntaxFlowVisitor().VisitHereDoc(doc))
			require.Equal(t, expected, (&RuleFormat{}).VisitHereDoc(doc))
			if body != "" {
				require.Contains(t, expected, body)
			}
		}
	}
}

func BenchmarkHereDocText(b *testing.B) {
	doc := parseMemoryTestHereDoc("<<<END\n" + strings.Repeat("description ", 700) + "\nEND\n")
	ctx := doc.LfHereDoc().LfText()
	b.Run("concatenation", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ctx.GetText()
		}
	})
	b.Run("builder", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = hereDocText(ctx)
		}
	})
}
