package minirehs

import (
	"reflect"
	"regexp/syntax"
	"testing"
)

func TestExtractRequiredLiteralFactors(t *testing.T) {
	cases := []struct {
		expr string
		want [][]string
	}{
		{"foo.*bar", [][]string{{"foo"}, {"bar"}}},
		{"(foo|bar).*baz", [][]string{{"bar", "foo"}, {"baz"}}},
		// Simplify 把该表达式化为 fo + o? + ba + [rz]，故两个最小必要字面量
		// 是 fo 与 ba；二者都比原始分支名更适合作为通用 trigger。
		{"foo?(bar|baz)", [][]string{{"fo"}, {"ba"}}},
		{"a*foo", [][]string{{"foo"}}},
		{"(?i)Foo.*BAR", [][]string{{"foo"}, {"bar"}}},
	}
	for _, tc := range cases {
		re, err := syntax.Parse(tc.expr, syntax.Perl)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.expr, err)
		}
		if got := extractRequiredLiteralFactors(re.Simplify(), 2); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: factors=%v want=%v", tc.expr, got, tc.want)
		}
	}
}

func TestRequiredLiteralFactorsAreNecessary(t *testing.T) {
	exprs := []string{"foo.*bar", "(foo|bar).*baz", "(?i)Token.*Cookie"}
	inputs := [][]byte{
		[]byte("foo___bar"), []byte("bar___baz"), []byte("TOKEN x COOKIE"),
		[]byte("foo only"), []byte("baz only"), []byte("unrelated"),
	}
	for _, expr := range exprs {
		re, parsed, err := compileAndParse(expr)
		if err != nil {
			t.Fatalf("compile %q: %v", expr, err)
		}
		factors := extractRequiredLiteralFactors(parsed, 2)
		for _, input := range inputs {
			if !re.Match(input) {
				continue
			}
			for _, factor := range factors {
				found := false
				for _, lit := range factor {
					if containsASCIIFold(input, []byte(lit)) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%q matched %q without necessary factor %v", expr, input, factor)
				}
			}
		}
	}
}

func containsASCIIFold(data, lit []byte) bool {
	if len(lit) == 0 || len(data) < len(lit) {
		return false
	}
	for i := 0; i+len(lit) <= len(data); i++ {
		ok := true
		for j := range lit {
			c := data[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != lit[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
