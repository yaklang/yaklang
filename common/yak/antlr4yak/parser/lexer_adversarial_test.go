package parser

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/yaklang/antlr/v4"
)

func lexerSnapshot(l *YaklangLexer) []string {
	var tokens []string
	for {
		token := l.NextToken()
		tokens = append(tokens, fmt.Sprintf("%d:%d:%d:%d:%d:%q", token.GetTokenType(), token.GetStart(), token.GetStop(), token.GetLine(), token.GetColumn(), token.GetText()))
		if token.GetTokenType() == antlr.TokenEOF {
			return tokens
		}
	}
}

func TestLexerInputStateReuse(t *testing.T) {
	for _, source := range []string{
		`f"${`, `f"${func(){`, "<<<'TAG'\nunfinished", "a=1",
	} {
		t.Run(source, func(t *testing.T) {
			l := NewYaklangLexer(antlr.NewInputStream(source))
			lexerSnapshot(l)
			for _, next := range []string{
				`value=f"${{1:2}[1]}";{value=1}`, "value=<<<'标签'\r\nraw\r\n标签\n", "value=1",
			} {
				l.SetInputStream(antlr.NewInputStream(next))
				want := lexerSnapshot(NewYaklangLexer(antlr.NewInputStream(next)))
				if got := lexerSnapshot(l); !reflect.DeepEqual(got, want) {
					t.Fatalf("previous input contaminated lexer: got %v, want %v", got, want)
				}
				l.Reset()
				if got := lexerSnapshot(l); !reflect.DeepEqual(got, want) {
					t.Fatalf("reset did not reproduce a fresh lexer: got %v, want %v", got, want)
				}
			}
		})
	}
}
