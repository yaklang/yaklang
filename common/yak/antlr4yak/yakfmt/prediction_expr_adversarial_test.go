package yakfmt

import (
	"fmt"
	"testing"

	"github.com/yaklang/antlr/v4"
)

// These fixtures are intentionally small. Each one gets a fresh original LL
// automaton through checkPrediction, so combining many grammar alternatives
// does not hide a prediction mismatch behind a previously warmed DFA.
func TestFormatExpressionPredictionAdversarial(t *testing.T) {
	primaries := []struct {
		name string
		expr string
	}{
		{"identifier", "value"},
		{"identifier_arrow", "value=>value+1"},
		{"paren_arrow", "(x,y)=>x+y"},
		{"typed_arrow", "(x int,y []string)=>x"},
		{"empty_arrow", "()=>1"},
		{"arrow_block", "x=>{return x}"},
		{"arrow_nested", "x=>y=>x+y"},
		{"paren", "(value)"},
		{"empty_paren", "()"},
		{"integer", "42"},
		{"integer_hex", "0Xff"},
		{"integer_octal", "0o17"},
		{"integer_binary", "0B1010"},
		{"float", "12.34"},
		{"float_leading_dot", ".125"},
		{"character", "'x'"},
		{"character_escape", "'\\n'"},
		{"string", `"[]{}(); // ${x}"`},
		{"string_prefix", `b"bytes"`},
		{"string_single", "'two chars'"},
		{"string_hex", "0h4142"},
		{"string_raw", "`a\nb ; /* */ ${x}`"},
		{"heredoc_lf", "<<<TAG\n[]{}(); // ${x}\nTAG\n"},
		{"heredoc_crlf", "<<<TAG\r\n[]{}(); // ${x}\r\nTAG\n"},
		{"heredoc_quoted_lf", "<<<'TAG'\n[]{}(); // ${x}\nTAG\n"},
		{"heredoc_quoted_crlf", "<<<'TAG'\r\n[]{}(); // ${x}\r\nTAG\n"},
		{"heredoc_quoted_unicode_lf", "<<<'标签'\nTAG_2\n原文 ; // ${x}\n标签\n"},
		{"heredoc_quoted_unicode_crlf", "<<<'标签'\r\nTAG_2\r\n原文 ; // ${x}\r\n标签\n"},
		{"true", "true"},
		{"false", "false"},
		{"nil", "nil"},
		{"undefined", "undefined"},
		{"template_double", `f"[]{}(); ${x+f(1,2)}"`},
		{"template_single", "f'punctuation ; ${x[0]+1}'"},
		{"template_raw", "f`a\nb ${x.value(1)} []{}()`"},
		{"map_empty", "{}"},
		{"map", `{"a":1,"b":f(2),}`},
		{"slice_empty", "[]"},
		{"slice_empty_comment", "[/* comment */]"},
		{"slice", "[1,x,func(){return 2},]"},
		{"slice_typed_empty", "[]int{}"},
		{"slice_typed", "[]int{1,2,}"},
		{"map_typed", `map[string]int{"a":1,}`},
		{"type_int", "int"},
		{"type_any", "any"},
		{"type_var", "var"},
		{"type_slice", "[][]byte"},
		{"type_map", "map[string][]int"},
		{"type_channel", "chan []int"},
		{"type_interface", "interface{}"},
		{"convert_int", "int(1)"},
		{"convert_empty", "int()"},
		{"convert_var", "var(value)"},
		{"convert_any", "any(value)"},
		{"convert_slice", "[]int(value)"},
		{"convert_map", "map[string]int(value)"},
		{"convert_channel", "chan int(value)"},
		{"convert_interface", "interface{}(nil)"},
		{"panic", `panic("test")`},
		{"recover", "recover()"},
		{"make_slice", "make([]int,1,2)"},
		{"make_map", "make(map[string][]int)"},
		{"make_channel", "make(chan int,1)"},
	}
	contexts := []struct {
		name string
		form string
	}{
		{"assignment", "result=%s"},
		{"argument", "consume(%s)"},
		{"return", "func(){return %s}"},
	}
	for _, primary := range primaries {
		for _, context := range contexts {
			t.Run(primary.name+"/"+context.name, func(t *testing.T) {
				checkPrediction(t, fmt.Sprintf(context.form, primary.expr))
			})
		}
	}
	for _, spelling := range []string{"func", "fn", "def", "function"} {
		for i, form := range []string{
			"%s(){return 1}",
			"%s named(a,b...){return a,b}",
			"%s(a string,b []int) (int,error){return 1,nil}",
			"%s() interface{}{return nil}",
			"%s{value=1;return value}",
		} {
			expression := fmt.Sprintf(form, spelling)
			for j, context := range contexts[:2] {
				t.Run(fmt.Sprintf("function_%s_%d_%d", spelling, i, j), func(t *testing.T) {
					checkPrediction(t, fmt.Sprintf(context.form, expression))
				})
			}
		}
	}
	for _, operator := range []string{"!", "-", "+", "^", "&", "*", "<-"} {
		for i, operand := range []string{"value", "(a+b*c)", "f(1)[0].value", "- +value"} {
			t.Run(fmt.Sprintf("unary_%s_%d", operator, i), func(t *testing.T) {
				checkPrediction(t, "result="+operator+" "+operand)
			})
		}
	}
	// Include every binary operator, the unusual bitwise precedence, receive vs
	// send, both membership forms, and right-nested conditional expressions.
	for i, expression := range []string{
		"a<<b+c*d", "a>>b+c", "a&b+c", "a&^b+c", "a|b+c", "a^b+c",
		"a*b+c", "a/b-c", "a%b+c", "a+b*c", "a-b/c",
		"a>b&&c", "a<b||c", "a<=b&&c", "a>=b||c", "a!=b&&c", "a<>b||c", "a==b&&c",
		"a in [b,c]", "a not in [b,c]", "a&&b||c", "a||b&&c",
		"a?b:c?d:e", "a?b?c:d:e", "channel<-a+b", "<-channel+a",
	} {
		for j, context := range contexts[:2] {
			t.Run(fmt.Sprintf("precedence_%d_%d", i, j), func(t *testing.T) {
				checkPrediction(t, fmt.Sprintf(context.form, expression))
			})
		}
	}
	for i, expression := range []string{
		"value.field", "value.$field", "value[0]", "value[:end]", "value[start:]",
		"value[start:end]", "value[start:end:limit]", "value[::]", "value()",
		"value(1,2,)~", "value(others...)", "value().field[0](1).$field",
		"(x=>x+1)(2)", "(func(x){return x})(1)", "[func(){return 1}][0]()",
	} {
		for j, context := range contexts[:2] {
			t.Run(fmt.Sprintf("suffix_%d_%d", i, j), func(t *testing.T) {
				checkPrediction(t, fmt.Sprintf(context.form, expression))
			})
		}
	}
	for i, source := range []string{
		"a=1+\n2*3", "a=1<<\n2+3", "a=true&&\nfalse||true",
		"a=x?\ny+1:\nz+2", "a=int(\n/* before */value\n)",
		"a=make(\n[]int,\n1,\n2,)", "a=func(x,\ny,\n){return x+y}",
		"a=(x,\ny,\n)=>x+y", "a=[\n1,\n/* middle */2,\n]",
		"a={\n\"key\":1,\n// pair\n\"value\":2,\n}",
		"f(\n// argument\nfunc(){\nreturn 1\n},\n2,\n)",
		"a=f\"${int(1)+f(2)} raw ; () [] {}\";b=f'${recover()}'",
		"a=fn(){return f(\n1,\n2\n)};a()",
	} {
		t.Run(fmt.Sprintf("trivia_%d", i), func(t *testing.T) { checkPrediction(t, source) })
	}
	// Start expressions at a statement boundary as well as in assignments and
	// arguments. In particular, a leading Identifier followed by => must not be
	// mistaken for the ordinary Identifier primary or suffix-statement path.
	for i, source := range []string{
		"value", "value=>value+1", "(x,y)=>x+y", "()=>1", "value=>{return value}",
		"panic(\"test\")", "recover()", "make([]int,1)", "-value", "<-channel",
		"func{value=1}", "fn(value){return value}", "[func(){return 1}][0]()",
	} {
		t.Run(fmt.Sprintf("standalone_%d", i), func(t *testing.T) { checkPrediction(t, source) })
	}
}

func TestFormatKeywordPredictionAdversarial(t *testing.T) {
	// Keywords select statement alternatives before their bodies are parsed.
	// Test bodies and terminators independently, including statement-list exit
	// at }, case and default, optional clauses, and neighboring fallback rules.
	statements := []string{
		"try{}catch{}", "try{a=1}catch err{a=2}", "try{}catch{}finally{a=3}",
		"if a{}", "if a{b=1}else{b=2}", "if a{}elif b{}else if c{}else{}",
		"if a=1;a>0{a++}", "if var a=1;a>0{a--}", "if f();a{f()}",
		"switch{}", "switch true{}", "switch true{default:return}", "switch value{case 1:break;default:return}",
		"switch{case a,:assert true}",
		"switch{case a,b:fallthrough;case c:continue;default:assert true}",
		"break", "break // comment", "continue", "continue /* comment */", "fallthrough",
		"return", "return a,b", "return func(){return 1}", "return a,\nb,",
		"include \"name.yak\"", "include `name.yak`", "include 'name.yak'",
		"defer recover()", "defer panic(\"value\")", "defer f(1,2)~", "defer func{a=1}",
		"go f()", "go func(){a=1}", "go func{a=1}",
		"assert true", "assert a,b,c", "assert f(1),\"message\"",
		"for a{if b{break}}", "for v in values{continue}", "for i=0;i<1;i++{f(i)}",
		"var a,b", "var a,b=1,2", "{var a=1;a++}",
		"select{case value=<-channel:return value;default:break}",
	}
	for i, statement := range statements {
		for j, source := range []string{
			statement + "\nnext=1",
			"func(){" + statement + "\nnext=1}",
		} {
			t.Run(fmt.Sprintf("keyword_%d_%d", i, j), func(t *testing.T) { checkPrediction(t, source) })
		}
	}
}

func TestFormatExpressionPredictionAdversarialHeredocLabels(t *testing.T) {
	// Lexer DFAs are shared across instances. Alternate label lengths, quoting
	// and newline modes: an action located before a closing quote must not reuse
	// a cached offset from a differently sized label and truncate the next one.
	for i, label := range []string{"X", "LONG_TAG_123", "标签", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "Y", "标签甲"} {
		for _, quoted := range []bool{false, true} {
			for _, crlf := range []bool{false, true} {
				header, newline := label, "\n"
				if quoted {
					header = "'" + label + "'"
				}
				if crlf {
					newline = "\r\n"
				}
				source := "result=<<<" + header + newline + "raw ; // ${x}" + newline + label + "\n"
				t.Run(fmt.Sprintf("label_%d/quoted=%v/crlf=%v", i, quoted, crlf), func(t *testing.T) {
					checkPrediction(t, source)
				})
			}
		}
	}
}

func TestFormatExpressionPredictionAdversarialInvalid(t *testing.T) {
	invalid := []string{
		"a=x=>", "a=(x,y)=>", "a=()=>", "a=x=>{return (}", "a=(x,,y)=>1",
		"a=func", "a=fn name", "a=def()", "a=function(x,,y){}", "a=func(x) (int,){}",
		"a=func(x) []{}", "a=func{a=;}", "a=func(x) int", "a=func(x) (int,error){return ]}",
		"a=[1,,2]", "a=[]int{1,,2}", "a=map[string]int{\"a\":}", "a=[]int(",
		"a=chan int(", "a=interface{}(", "a=int(,)", "a=make()", "a=make([])",
		"a=make([]int,,1)", "a=make(map[string]int,)", "a=panic()", "a=recover(1)",
		"a=+", "a=-", "a=!", "a=^", "a=&", "a=*", "a=<-", "a=+*",
		"a=x+", "a=x<<", "a=x in", "a=x not in", "a=x?y:", "a=x?y", "a=x&&",
		"a=value.", "a=value.$", "a=value[", "a=value[1:2:3:4]", "a=value(1,,2)",
		"a=f\"${}\"", "a=f'${x+}'", "a=f`raw ${x[}`", "a=f\"${x}\" @",
		"a=<<<TAG\nunterminated", "a=\"unterminated", "a='\\q' @",
		"try{}", "try{}catch", "try{}catch err", "try{}catch{}finally", "if{}", "if a{}elif{}",
		"if var a=;a{}", "switch{case:}", "switch{default}", "switch{case a,,b:}",
		"break(1)", "continue(1)", "fallthrough(1)", "return (", "include 1", "include 'x'",
		"defer", "defer recover(1)", "defer panic()", "go", "go (", "assert", "assert a,",
	}
	for i, malformed := range invalid {
		for j, source := range []string{malformed, "prefix=f(1)\n" + malformed} {
			t.Run(fmt.Sprintf("invalid_%d_%d", i, j), func(t *testing.T) {
				_, _, originalLL := parsePrediction(source, false, antlr.PredictionModeLL)
				if originalLL == nil {
					t.Fatalf("invalid fixture accepted by original LL: %q", source)
				}
				_, _, want := parsePrediction(source, false, antlr.PredictionModeSLL)
				if want == nil {
					t.Fatalf("invalid fixture accepted by original SLL-first parser: %q", source)
				}
				_, _, got := parsePrediction(source, true, antlr.PredictionModeSLL)
				if got == nil || got.Error() != want.Error() {
					t.Fatalf("fast prediction changed first error for %q: got %v, want %v", source, got, want)
				}
				if output, err := Format(source); output != "" || err == nil || err.Error() != want.Error() {
					t.Fatalf("formatter changed first error for %q: output %q, got %v, want %v", source, output, err, want)
				}
				if output, err := Format("a=1"); err != nil || output != "a = 1\n" {
					t.Fatal("malformed input contaminated the next formatter", output, err)
				}
			})
		}
	}
}
