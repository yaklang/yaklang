package parser

import (
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
)

func TestPredictionScanBounds(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"within_token_bound", "f(" + strings.Repeat("1,", 2045) + "0)", 4},
		{"past_token_bound", "f(" + strings.Repeat("1,", 2046) + "0)", 0},
		{"within_depth_bound", "f(" + strings.Repeat("(", 511) + "1" + strings.Repeat(")", 512), 4},
		{"past_depth_bound", "f(" + strings.Repeat("(", 512) + "1" + strings.Repeat(")", 513), 0},
		{"mismatched_delimiters", "f([)]", 0},
		{"unfinished_closure", "f(func(){", 0},
		{"template_interpolation", "f(f\"${f(1)}\")", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := antlr.NewCommonTokenStream(NewYaklangLexer(antlr.NewInputStream(tc.source)), antlr.TokenDefaultChannel)
			p := NewYaklangParser(stream)
			stream.LT(1)
			index := stream.Index()
			if got := p.suffixStatementPrefix(stream); got != tc.want || stream.Index() != index {
				t.Fatalf("bounded prediction consumed input or chose an unsafe alternative: got=%d want=%d", got, tc.want)
			}
		})
	}
}

type hiddenCommentLexer struct{ *YaklangLexer }
type hiddenToken struct{ antlr.Token }

func (*hiddenToken) GetChannel() int { return antlr.TokenHiddenChannel }
func (l *hiddenCommentLexer) NextToken() antlr.Token {
	token := l.YaklangLexer.NextToken()
	if token.GetTokenType() == YaklangParserCOMMENT {
		return &hiddenToken{token}
	}
	return token
}

func TestPredictionTokenStreamReuse(t *testing.T) {
	stream := antlr.NewCommonTokenStream(NewYaklangLexer(antlr.NewInputStream("f()[0]=1")), antlr.TokenDefaultChannel)
	p := NewYaklangParser(stream)
	for _, source := range []string{"f()[0]=1", "f()[0]\n", "f()[0]+=1", "f()[0]\n"} {
		stream.SetTokenSource(NewYaklangLexer(antlr.NewInputStream(source)))
		stream.LT(1)
		index := stream.Index()
		want := 4
		if source[len(source)-1] == '1' {
			want = 3
		}
		for i := 0; i < 2; i++ {
			if got := p.suffixStatementPrefix(stream); got != want || stream.Index() != index {
				t.Fatalf("cache reused a different source or moved input: source=%q got=%d want=%d", source, got, want)
			}
		}
	}
}

func TestPredictionHiddenCommentBoundaries(t *testing.T) {
	source := "f /* member */ .value(/* before */func(){return 1},2) /* assign */ [0]=1"
	lexer := &hiddenCommentLexer{NewYaklangLexer(antlr.NewInputStream(source))}
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := NewYaklangParser(stream)
	stream.LT(1)
	index := stream.Index()
	if got := p.suffixStatementPrefix(stream); got != 3 || stream.Index() != index {
		t.Fatal("hidden comments changed the prefix or consumed input", got)
	}
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
	program := p.Program().(*ProgramContext)
	if program.StatementList().(*StatementListContext).Statement(0).(*StatementContext).AssignExpressionStmt() == nil {
		t.Fatal("hidden-channel input lost its assignment")
	}
}
