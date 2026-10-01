package antlr4yak

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakfmt"
)

func TestFormatterAdversarialExecution(t *testing.T) {
	cases := []struct{ name, source string }{
		{"multiline_lhs", "a\n,b=1,2;assert a==1 && b==2"},
		{"commented_lhs", "a/* before comma */,b=1,2;assert a==1 && b==2"},
		{"assignment_trailing_comma", "a=1,;b=2;assert a==1 && b==2"},
		{"return_trailing_comma", "f=func(){return 1,;2};assert f()==1"},
		{"empty_switch", "switch{};switch true{};a=1;assert a==1"},
		{"empty_switch_evaluation", "a=0;f=func(){a++;return a};switch f(){};assert a==1"},
	}
	for _, sep := range []string{"\n", "\r\n"} {
		for _, label := range []string{"TAG", "较长的标签"} {
			payload := label + "_suffix" + sep + "原文 ; // ${x}"
			cases = append(cases, struct{ name, source string }{
				fmt.Sprintf("quoted_heredoc_%q_%s", sep, label),
				"value=<<<'" + label + "'" + sep + payload + sep + label + "\nassert value==" + fmt.Sprintf("%q", payload),
			})
		}
	}
	for _, quote := range []string{"'", "\"", "`"} {
		cases = append(cases, struct{ name, source string }{
			"nested_interpolation_" + quote,
			"f=func(x){return x[1]};value=f" + quote + "raw ${func(){return f({1:2})}()} tail" + quote + ";assert value==\"raw 2 tail\"",
		})
	}
	cases = append(cases, struct{ name, source string }{
		"nested_template_frame",
		`f=func(x){return x[1]};value=f"${func(){return f'${f({1:2})}'}()}";assert value=="2"`,
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			formatted, err := yakfmt.Format(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			for i, source := range []string{tc.source, strings.TrimSuffix(formatted, "\n")} {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					if err := New().SafeEval(context.Background(), source); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}
