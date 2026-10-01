package yakfmt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
)

// Identifier/suffix prefixes overlap assignment and expression statements.
// Combine each suffix with every assignment terminator instead of testing only
// ordinary calls: delimiters and lexer modes can move the disambiguating token.
func TestFormatPredictionStatementAdversarial(t *testing.T) {
	prefixes := []string{
		"a", "a.member", "a.$key", "a[0]", "a[(1+2)*3]",
		"a[1:3]", "a[:]", "a[1:3:2]", "a()", "a()~", "a(1,2)",
		"a(func(){return 1})", "a(1).b()[0].$c",
		"a[{'k':[1,2]}['k'][0]]", "a(\"()[]{}; // 原文\")",
		"a(`raw ) ] } ; //\n原文`)", "a(f\"raw ; {} [] ()\")",
		"a(f\"${f(1)}\")", "a(f'${f([1,2])}')", "a(f`raw ${f({'k':1})}`)",
		"a(<<<TAG\n原文 ;{}[]()\nTAG\n)",
		"a(<<<TAG\r\n原文 ;{}[]()\r\nTAG\n)",
		"a(/* [ ( { */func(){\n// } ) ] 原文\nreturn 1},2)",
		"a[func(){return {'k':[1,2]}}()['k'][0]]",
	}
	assignments := []string{"=1", ":=1", "+=1", "-=1", "*=1", "/=1", "%=1", "&=1", "|=1", "^=1", "<<=1", ">>=1", "&^=1", "++", "--"}
	terminators := []string{"", ";", "\n", "\r\n", "/* 原文 ) ] } ; */", "// 原文 ) ] } ;\n", "# 原文 ) ] } ;\r\n"}
	for i, prefix := range prefixes {
		for j, assignment := range assignments {
			t.Run(fmt.Sprintf("suffix_%02d/assign_%02d", i, j), func(t *testing.T) {
				checkPrediction(t, prefix+assignment)
			})
		}
		for j, terminator := range terminators {
			t.Run(fmt.Sprintf("suffix_%02d/eos_%02d", i, j), func(t *testing.T) {
				checkPrediction(t, prefix+terminator)
			})
		}
		for j, operator := range []string{"=", ":="} {
			t.Run(fmt.Sprintf("suffix_%02d/multiple_lhs_%02d", i, j), func(t *testing.T) {
				checkPrediction(t, prefix+",\n b.$member[0] "+operator+" f(1),\n func(){return [2,3]}")
			})
		}
	}

	// Unknown suffixes must still be decided by the original ATN. The same
	// identifier is also legal as contextual select and as a loop variable.
	fallbacks := []string{
		"a+f(1)", "a-f(1)", "a*f(1)", "a/f(1)", "a%f(1)",
		"a<<1", "a>>1", "a&1", "a&^1", "a|1", "a^1",
		"a>f(1)", "a<1", "a>=1", "a<=1", "a!=1", "a<>1", "a==1",
		"a in [1,2]", "a not in [1,2]", "a&&f(1)", "a||f(1)", "a?f(1):g(2)", "a<-f(1)",
		"a=>a+1", "(a,b)=>a+b", "a(1,2,)", "a([1,2]...)", "a(1)~.value[0]",
		"select", "select=f;select(1);select.member=1", "select\n{}", "select\r\n{}",
		"select /* separate statements */ {}", "select{default:a().member=1}",
		"select{default:a();default:b()}", // duplicate defaults are a semantic check
		"select{case a[0]=<-ch:a();case ch<-a(1):a[0]++;default:a()}",
		"for a in xs{a(1)}", "for a[0],b.$key in xs{a()}", "for a:=range xs{a()}",
		"for a=range xs{a()}", "for a=0;a<10;a++{a()}", "for (a=0;a<10;a++){a()}",
		"for a{a()}", "for{a()}",
		"a(func(x int,y string) (int,error){return x,nil})",
		"a(func f(x []byte) map[string]int{return {'k':1}})",
		"a(func{a=1;return a})", "a(func(){if a{a()}elif b{b()}else{c()}})",
		"a(func(){try{a()}catch err{b()}finally{c()}})",
		"a(func(){switch a{case 1:a();case 2:a[0]++;default:a.$key=1}})",
		"a(func(){select{case ch<-a(1):a();default:b()}})",
		"a(\n// delimiter-shaped comment )]}\nfunc(){return 1},\r\n2\n)",
		"a(f\"${f(f'${f(1)}')}\")",
		"a(f\"${func(){return f'${f(1)}'}()}\")",
	}
	for i, source := range fallbacks {
		t.Run(fmt.Sprintf("fallback_%02d", i), func(t *testing.T) { checkPrediction(t, source) })
	}
	for i, source := range []string{"a()", "a[0]=f(1)", "a.$key++", "a(func(){return 1})", "a(f\"${f(1)}\")"} {
		for j, wrap := range []func(string) string{
			func(s string) string { return "if a{" + s + "}else{b()}" },
			func(s string) string { return "switch a{case 1:" + s + ";default:b()}" },
			func(s string) string { return "select{case ch<-1:" + s + ";default:b()}" },
			func(s string) string { return "for a in xs{" + s + "}" },
			func(s string) string { return "try{" + s + "}catch e{b()}finally{c()}" },
		} {
			t.Run(fmt.Sprintf("context_%02d_%02d", i, j), func(t *testing.T) { checkPrediction(t, wrap(source)) })
		}
	}
	for _, depth := range []int{1, 8, 32} {
		source := "a(" + strings.Repeat("[", depth) + "{'k':1}" + strings.Repeat("]", depth) + ").member[0]=1"
		t.Run(fmt.Sprintf("mixed_depth_%d", depth), func(t *testing.T) { checkPrediction(t, source) })
	}
	for i, prefix := range []string{"a", "a[0]", "a().member", "a(func(){return 1})"} {
		for j, trivia := range []string{"\n", "\r\n", "/* before comma */", "// before comma\n", "# before comma\r\n"} {
			t.Run(fmt.Sprintf("trivia_before_comma_%d_%d", i, j), func(t *testing.T) {
				source := prefix + trivia + ",b=1,2"
				checkPrediction(t, source)
				assertRoundTrip(t, source, nil)
			})
			t.Run(fmt.Sprintf("trivia_after_comma_%d_%d", i, j), func(t *testing.T) {
				source := prefix + "," + trivia + "b=1,2"
				checkPrediction(t, source)
				assertRoundTrip(t, source, nil)
			})
		}
	}
	for i, quote := range []string{"'", "\"", "`"} {
		for j, expression := range []string{
			"f({'k':[1,2]}['k'][0])",
			"func(){return {'k':f(1)}}()",
			"func(){if a{return f(1)}else{return f(2)}}()",
			"f(func(){return [f(1),f(2)]})",
			"f(f'${f(1)}')",
			"f(f\"${f({'k':1})}\")",
			"func(){return f'${f(1)}'}()",
		} {
			t.Run(fmt.Sprintf("interpolation_nested_code_%d_%d", i, j), func(t *testing.T) {
				source := "a(f" + quote + "raw )]} ; ${" + expression + "} {[( raw" + quote + ")[0]=1"
				checkPrediction(t, source)
				assertRoundTrip(t, source, nil)
			})
		}
	}
	// Large bounds are also checked directly in parser tests without repeatedly
	// constructing enormous LL trees under the formatter's race timeout.
	for _, count := range []int{2045, 2046} {
		source := "a(" + strings.Repeat("1,", count) + "0)[0]=1"
		t.Run(fmt.Sprintf("token_boundary_%d", count), func(t *testing.T) { checkPrediction(t, source) })
	}
	for _, count := range []int{4090, 4092} {
		source := "a()\n" + strings.Repeat("/* before comma */", count) + ",b=1,2"
		t.Run(fmt.Sprintf("trivia_token_boundary_%d", count), func(t *testing.T) { checkPrediction(t, source) })
	}
}

func TestFormatPredictionStatementAdversarialInvalid(t *testing.T) {
	prefixes := []string{"a", "a.member", "a.$key", "a[0]", "a(1)", "a(func(){return 1})", "a(1).b()[0].$c", "a(f\"${f(1)}\")"}
	tails := []string{"=", ":=", "+=", "++ +", ",b=", ".", "[", "(", "[)]", "(]}", ")", ". $", "=1, @", "=\"unterminated"}
	for i, prefix := range prefixes {
		for j, tail := range tails {
			t.Run(fmt.Sprintf("suffix_%02d/mutation_%02d", i, j), func(t *testing.T) {
				checkStatementPredictionError(t, prefix+tail)
			})
		}
	}
	for i, source := range []string{
		"a(/* unclosed", "a(<<<TAG\nno terminator)", "a(f\"${1+}\")", "a(f'${f(]')", "a(f`raw ${f(1)}`",
		"a(func(){b=;},@)", "a(func(){b=;},\r\n @)", "a(func(){if{}},\"unterminated)",
		"a(func(){b=;},f\"${1+}\")", "a(func(){return ]},<<<TAG\nraw\nTAG\n)",
		"select{case:a()}", "select{case ch<-:a()}", "select{default:a(]}",
		"for a in{a()}", "for a:=range{a()}", "for a=0;a<;a++{a()}", "for (a=0;a<1;a++{a()}",
		"a(f\"${func(){return {'k':1}}()}\") )", "a(f'${f({)}')",
		"a()\n@", "a(func(){x=;})\n@", "a(func(){if{}})/* before error */@",
	} {
		t.Run(fmt.Sprintf("fallback_mutation_%02d", i), func(t *testing.T) { checkStatementPredictionError(t, source) })
	}
	for i, prefix := range []string{"a", "a[0]", "a(func(){return 1})"} {
		for j, trivia := range []string{"\n", "\r\n", "/* before comma */", "// before comma\n", "# before comma\r\n"} {
			for k, tail := range []string{"=", "=1,@", "=func(){return ]}", "=f\"${1+}\""} {
				t.Run(fmt.Sprintf("invalid_trivia_lhs_%d_%d_%d", i, j, k), func(t *testing.T) {
					checkStatementPredictionError(t, prefix+trivia+",b"+tail)
				})
			}
		}
	}
}

func checkStatementPredictionError(t *testing.T, source string) {
	t.Helper()
	if _, _, err := parsePrediction(source, false, antlr.PredictionModeLL); err == nil {
		t.Fatalf("invalid fixture accepted by original LL: %q", source)
	}
	_, _, original := parsePrediction(source, false, antlr.PredictionModeSLL)
	if original == nil {
		t.Fatalf("invalid fixture accepted by original SLL-first parser: %q", source)
	}
	_, _, optimized := parsePrediction(source, true, antlr.PredictionModeSLL)
	if optimized == nil || optimized.Error() != original.Error() {
		t.Fatalf("optimized first error changed for %q: got %v, want %v", source, optimized, original)
	}
	if result, err := Format(source); result != "" || err == nil || err.Error() != original.Error() {
		t.Fatalf("formatter first error or empty output changed for %q: got %q, %v, want %v", source, result, err, original)
	}
	if result, err := Format("a[0]=1"); err != nil || result != "a[0] = 1\n" {
		t.Fatal("invalid prediction contaminated a later source", result, err)
	}
}
