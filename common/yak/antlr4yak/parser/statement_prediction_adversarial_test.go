package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
)

func TestPredictionStatementScannerAdversarial(t *testing.T) {
	cases := []struct {
		name, source string
		want         int
	}{
		{"member", "a.member=1", 3},
		{"dollar_member", "a.$key[0]++", 3},
		{"call_result", "a(1).b()[0].$key&^=1", 3},
		{"slice", "a[1:3:2]+=1", 3},
		{"multiple_lhs", "a(1),b[0]=1,2", 0},
		{"lf_before_comma", "a\n,b=1,2", 0},
		{"crlf_before_comma", "a[0]\r\n,b=1,2", 0},
		{"comment_before_comma", "a()/* before comma */,b=1,2", 0},
		{"line_comment_before_comma", "a()// before comma\n,b=1,2", 0},
		{"trivia_before_next_statement", "a()\n/* comment */b()", 4},
		{"trivia_token_limit_inside", "a()\n" + strings.Repeat("/* comment */", 4090) + "b()", 4},
		{"trivia_token_limit_outside", "a()\n" + strings.Repeat("/* comment */", 4092) + "b()", 0},
		{"wavy_suffix", "a(1)~", 0},
		{"binary_tail", "a(1)+b", 0},
		{"channel_tail", "a[0]<-b", 0},
		{"reserved_member", "a.default", 0},
		{"numeric_member", "a.1", 0},
		{"mismatched_paren", "a([)]", 0},
		{"mismatched_brace", "a({])", 0},
		{"mismatched_slice", "a()[0)", 0},
		{"unfinished_member", "a.", 0},
		{"unfinished_call", "a(", 0},
		{"unfinished_closure", "a(func(){return 1", 0},
		{"raw_punctuation", "a(`raw ) ] } ; //\n原文`)[0]=1", 3},
		{"string_punctuation", "a(\"literal ()[]{}; // 原文\")", 4},
		{"comment_punctuation", "a(/* )]} */func(){return 1})", 4},
		{"template_raw", "a(f\"raw ) ] } ;\")[0]=1", 3},
		{"template_single_interpolation", "a(f'${f(1)}')[0]=1", 0},
		{"template_double_interpolation", "a(f\"${f(1)}\")[0]=1", 0},
		{"template_backtick_interpolation", "a(f`raw ${f(1)}`)[0]=1", 0},
		{"heredoc_lf", "a(<<<TAG\n原文 ;{}[]()\nTAG\n)[0]=1", 3},
		{"heredoc_crlf", "a(<<<TAG\r\n原文 ;{}[]()\r\nTAG\n)[0]=1", 3},
		{"token_limit_inside", "a(" + strings.Repeat("1,", 2045) + "0)", 4},
		{"token_limit_outside", "a(" + strings.Repeat("1,", 2046) + "0)", 0},
		{"suffix_limit_inside", "a(" + strings.Repeat("1,", 2043) + "0).b[0]=1", 3},
		{"suffix_limit_outside", "a(" + strings.Repeat("1,", 2044) + "0).b[0]=1", 0},
		{"depth_limit_inside", "a(" + strings.Repeat("(", 511) + "1" + strings.Repeat(")", 512), 4},
		{"depth_limit_outside", "a(" + strings.Repeat("(", 512) + "1" + strings.Repeat(")", 513), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := antlr.NewCommonTokenStream(NewYaklangLexer(antlr.NewInputStream(tc.source)), antlr.TokenDefaultChannel)
			p := NewYaklangParser(stream)
			stream.LT(1)
			index := stream.Index()
			for attempt := 0; attempt < 2; attempt++ {
				if got := p.suffixStatementPrefix(stream); got != tc.want || stream.Index() != index {
					t.Fatalf("unsafe scan or stale cache: source=%q attempt=%d got=%d want=%d index=%d original=%d", tc.source, attempt, got, tc.want, stream.Index(), index)
				}
			}
		})
	}
}

type predictionWrappedStream struct{ *antlr.CommonTokenStream }
type predictionHiddenLexer struct{ *YaklangLexer }

func (l *predictionHiddenLexer) NextToken() antlr.Token {
	token := l.YaklangLexer.NextToken()
	if token.GetTokenType() != antlr.TokenEOF {
		return &hiddenToken{token}
	}
	return token
}

func TestPredictionStatementStreamFallbacks(t *testing.T) {
	for _, source := range []string{"a(1)[0]=1", "a.member(1)\n", "a.$key[0]+=1", "a(func(){return 1})", "select{default:a()}"} {
		t.Run(source, func(t *testing.T) {
			base := antlr.NewCommonTokenStream(NewYaklangLexer(antlr.NewInputStream(source)), antlr.TokenDefaultChannel)
			wrapped := &predictionWrappedStream{base}
			p := NewYaklangParser(wrapped)
			wrapped.LT(1)
			index := wrapped.Index()
			if got := p.suffixStatementPrefix(wrapped); got != 0 || wrapped.Index() != index {
				t.Fatal("non-CommonTokenStream did not fall back without consuming input", got)
			}
			p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
			optimized := p.Program()
			original := NewYaklangParser(antlr.NewCommonTokenStream(NewYaklangLexer(antlr.NewInputStream(source)), antlr.TokenDefaultChannel))
			original.SetFastPrediction(false)
			original.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
			want := original.Program()
			if p.HasError() || original.HasError() || optimized.ToStringTree(p.RuleNames, p) != want.ToStringTree(original.RuleNames, original) {
				t.Fatal("wrapped-stream fallback changed parser structure", p.GetError(), original.GetError())
			}
		})
	}
	for i, source := range []string{"a(1)[0]=1", "a(1)\n", "a.$key[0]&^=1"} {
		t.Run(fmt.Sprintf("hidden_channel_%d", i), func(t *testing.T) {
			stream := antlr.NewCommonTokenStream(&predictionHiddenLexer{NewYaklangLexer(antlr.NewInputStream(source))}, antlr.TokenHiddenChannel)
			p := NewYaklangParser(stream)
			stream.LT(1)
			index := stream.Index()
			if got := p.suffixStatementPrefix(stream); got != 0 || stream.Index() != index {
				t.Fatal("alternate selected channel did not fall back without consuming input", got)
			}
		})
	}
	for _, tc := range []struct{ comments, want int }{{4090, 4}, {4092, 0}} {
		t.Run(fmt.Sprintf("hidden_token_bound_%d", tc.comments), func(t *testing.T) {
			source := "a(" + strings.Repeat("/* hidden */", tc.comments) + "1)"
			stream := antlr.NewCommonTokenStream(&hiddenCommentLexer{NewYaklangLexer(antlr.NewInputStream(source))}, antlr.TokenDefaultChannel)
			p := NewYaklangParser(stream)
			stream.LT(1)
			index := stream.Index()
			if got := p.suffixStatementPrefix(stream); got != tc.want || stream.Index() != index {
				t.Fatalf("hidden trivia bypassed the raw token bound: got=%d want=%d index=%d original=%d", got, tc.want, stream.Index(), index)
			}
		})
	}
}

func TestPredictionStatementSeekAndSourceReuse(t *testing.T) {
	stream := antlr.NewCommonTokenStream(NewYaklangLexer(antlr.NewInputStream("a()[0]=1;a()[0]\n")), antlr.TokenDefaultChannel)
	p := NewYaklangParser(stream)
	stream.Fill()
	second := -1
	for _, token := range stream.GetAllTokens() {
		if token.GetTokenType() == YaklangParserIdentifier && token.GetTokenIndex() > 0 {
			second = token.GetTokenIndex()
			break
		}
	}
	if second < 0 {
		t.Fatal("missing second identifier in seek fixture")
	}
	for _, tc := range []struct{ index, want int }{{0, 3}, {second, 4}, {0, 3}, {second, 4}} {
		stream.Seek(tc.index)
		if got := p.suffixStatementPrefix(stream); got != tc.want || stream.Index() != tc.index {
			t.Fatalf("seek changed a prefix prediction or consumed input: index=%d got=%d want=%d", tc.index, got, tc.want)
		}
	}
	for _, source := range []string{"a()[0]\n", "a()[0]+=1", "a(f\"${f(1)}\")[0]=1", "a()[0]=1", "a()~", "a()[0]\n"} {
		stream.SetTokenSource(NewYaklangLexer(antlr.NewInputStream(source)))
		stream.LT(1)
		want := 4
		if strings.Contains(source, "${") || strings.HasSuffix(source, "~") {
			want = 0
		} else if strings.HasSuffix(source, "=1") {
			want = 3
		}
		if got := p.suffixStatementPrefix(stream); got != want || stream.Index() != 0 {
			t.Fatalf("reused source retained an old prefix: source=%q got=%d want=%d", source, got, want)
		}
	}
}
