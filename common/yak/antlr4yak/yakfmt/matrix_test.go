package yakfmt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

// This deterministic matrix is intentionally independent of the layout code.
// Every expression is embedded in every grammar position, with several trivia
// shapes. It provides thousands of reproducible round-trip checks in seconds.
func TestFormatFastMatrix(t *testing.T) {
	expressions := []string{
		"1", "0xABC", ".123", "'x'", `"你好"`, "`raw`", "true", "nil", "undefined", "()", "(x)",
		"- -x", "+ +x", "& ^x", "!x", "<-ch", "a + b * c", "(a + b) * c", "a << b & c", "a && b || c", "a ? b : c ? d : e", "x not in y",
		"f()", "f(x,y...)", "f(x)~", "a.b.$key", "a[1:2:3]", "a[:x]", "a[x:]", "a[x::y]",
		"[]", "[1,2]", "[\n1,2\n]", "{}", "{1:2,3:4}", "map[string]any{\"x\":1}", "[]any{1,2}", "make(chan int,1)", "int(x)",
		"func(){}", "func(x){return x+1}", "func(x int) (int,error){return x,nil}", "fn{1}", "x=>x+1", "(x,y)=>{return x+y}",
		"f'hello ${x+1}'", "f\"hello ${f(x)}\"", "f`hello ${x?1:2}`",
	}
	contexts := []string{
		"x=%s", "return %s", "func(){return %s}", "if %s{x=1}else{x=2}", "for %s{break}", "switch %s{case 1:break;default:}",
		"select{case ch<-%s:break;default:}", "select{case x:= <-ch:f(%s);default:}", "var x=%s;assert x==x", "fn{%s}",
		"f(%s,2)", "x=[%s,2]", "x={\"key\":%s}", "x=[]any{%s,2}", "x=map[string]any{\"key\":%s}", "x=(%s)",
	}
	checked := 0
	for ei, e := range expressions {
		for ci, c := range contexts {
			for wi := 0; wi < 3; wi++ {
				source := fmt.Sprintf(c, e)
				if wi == 1 {
					source = strings.ReplaceAll(source, " ", "\t ")
				}
				if wi == 2 {
					source = "\n\n// matrix\n" + strings.ReplaceAll(source, " ", "   ") + " // tail\n"
				}
				t.Run(fmt.Sprintf("e%02d/c%02d/w%d", ei, ci, wi), func(t *testing.T) { assertRoundTrip(t, source, nil) })
				checked++
			}
		}
	}
	t.Logf("checked %d expression/context/trivia combinations", checked)
}

func assertWidth(t *testing.T, source string) {
	t.Helper()
	tree, ts, err := parseTest(source)
	if err != nil {
		t.Fatal(err)
	}
	_ = tree
	ts.Fill()
	exempt := make(map[int]bool)
	for _, tok := range ts.GetAllTokens() {
		typ := tok.GetTokenType()
		// Payloads, comments, and indivisible identifiers cannot be split by a
		// whitespace formatter. Their bytes remain part of the semantic check.
		text := tok.GetText()
		if isComment(typ) || typ == parser.YaklangLexerStringLiteral || typ == parser.YaklangLexerCharacterLiteral || strings.Contains(text, "\n") || len(text) > lineWidth-8 {
			for line := tok.GetLine(); line <= tok.GetLine()+strings.Count(text, "\n"); line++ {
				exempt[line] = true
			}
		}
	}
	for i, line := range strings.Split(source, "\n") {
		if len(line) > lineWidth && !exempt[i+1] && len(line)-len(strings.TrimLeft(line, " ")) < lineWidth-8 {
			t.Fatalf("line %d is %d columns:\n%s", i+1, len(line), line)
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if !exempt[i+1] && strings.Contains(indent, "\t") {
			t.Fatalf("indentation contains tab: %q", line)
		}
	}
}
func TestFormatExtreme(t *testing.T) {
	for _, count := range []int{0, 1, 2, 10, 100, 1000} {
		args := make([]string, count)
		params := make([]string, count)
		types := make([]string, count)
		for i := range args {
			args[i] = fmt.Sprintf("parameter_%04d", i)
			params[i] = args[i] + " map[string][]int"
			types[i] = "map[string][]int"
		}
		sources := []string{"f(" + strings.Join(args, ",") + ")", "func(" + strings.Join(params, ",") + ") {}", "x=[" + strings.Join(args, ",") + "]"}
		if count > 0 {
			sources = append(sources, "func() ("+strings.Join(types, ",")+") {}")
		}
		for i, source := range sources {
			t.Run(fmt.Sprintf("parameters_%d/%d", count, i), func(t *testing.T) {
				assertRoundTrip(t, source, nil)
				got, err := Format(source)
				if err != nil {
					t.Fatal(err)
				}
				assertWidth(t, got)
			})
		}
	}
	for _, depth := range []int{1, 8, 16, 64, 128} {
		source := "x=" + strings.Repeat("func(){return ", depth) + "1" + strings.Repeat("}", depth)
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) { assertRoundTrip(t, source, nil) })
	}
	for _, size := range []int{32, 64, 128, 4096, 65536} {
		sources := []string{"x=\"" + strings.Repeat("x", size) + "\"", "x=\"" + strings.Repeat("x", size) + "\"+\"suffix\"", "x={\"" + strings.Repeat("k", size) + "\":\"" + strings.Repeat("v", size) + "\"}"}
		for i, source := range sources {
			t.Run(fmt.Sprintf("literal_%d/%d", size, i), func(t *testing.T) { assertRoundTrip(t, source, nil) })
		}
	}
	sources := []string{
		"x=1" + strings.Repeat(" + 1", 2000),
		"x=f(" + strings.Repeat("f(1,2),", 1000) + "0)",
		"select{" + strings.Repeat("case ch<-1: x=1;", 1000) + "default: x=0}",
		"switch x{" + strings.Repeat("case 1: x=1;", 1000) + "default: x=0}",
		"func(){" + strings.Repeat("x=1;", 1000) + "}",
		"x=" + strings.Repeat("long_variable_name_", 3) + "(" + strings.Repeat("p,", 50) + "q)",
		"x={\"" + strings.Repeat("k", 64) + "\":\"" + strings.Repeat("v", 64) + "\"}",
		"x=" + strings.Repeat("a,", 64) + "b", // expression list, not a slice
	}
	for i, source := range sources {
		t.Run(fmt.Sprintf("scaling_%d", i), func(t *testing.T) { assertRoundTrip(t, source, nil); got, _ := Format(source); assertWidth(t, got) })
	}
}
func TestFormatRecoveryAndLiteralIsolation(t *testing.T) {
	for _, source := range []string{"a=[1", "func(a", "if true", "select{case <-ch", "a=\"unterminated", "a=" + strings.Repeat("(", 100), "dump(1)))", "a=make(\nchan int,1\n)", "f(\n)", "func(\n){}"} {
		if got, err := Format(source); err == nil || got != "" {
			t.Fatalf("invalid source escaped: %q %v", got, err)
		}
		if got, err := Format("a=1"); err != nil || got != "a = 1\n" {
			t.Fatal("failed parse contaminated next call", got, err)
		}
	}
	payload := "\n" + strings.Repeat("  ${fake} } // literal\r\n", 1000)
	for _, source := range []string{"x=`" + payload + "`", "x=<<<TAG" + payload + "\nTAG", "x=[`" + payload + "`]"} {
		assertRoundTrip(t, source, nil)
	}
	// Existing parentheses must still preserve precedence when continuations
	// require extra transparent grouping elsewhere.
	assertRoundTrip(t, "x=(1+2)*3;y=1+2*3", nil)
}
