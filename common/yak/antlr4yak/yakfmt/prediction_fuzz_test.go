package yakfmt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

type predictionCase struct {
	name, source string
	valid        bool
}

// Make the predictor's overlapping prefixes occur in different containing
// rules. These are syntax fixtures; free names, control flow outside loops and
// arbitrary left expressions do not ask the compiler to accept their semantics.
func predictionGrammarCases() []predictionCase {
	var cases []predictionCase
	add := func(name, source string) {
		cases = append(cases, predictionCase{name: name, source: source, valid: true})
	}
	statements := []string{
		"// line\nvalue=1", "/* block */", "# hash\nvalue=1",
		"var value,other", "var value,other=1,2", "any value=1",
		"value=1", "value:=1", "value,other=1,2", "value[0]=1",
		"value.field=1", "value.$field=1", "f().field=1", "f()[0]++",
		"value+=1", "f().field&^=1", "value++", "value--", "f(1)",
		"{value=1}", "try{}catch err{}finally{}", ";\n",
		"select{case <-ch: value=1;case ch<-value:break;default:value=2}",
		"select(1)", "select=value;select.field=1",
		"if value{}elif other{}else{}", "if value=1;value{}else if other{}",
		"if var value=1;value{}", "if f();value{}",
		"switch value{case 1,2:value++;fallthrough;default:break}",
		"for range value{}", "for key,value:=range other{}", "for value in other{}",
		"for i=0;i<3;i++{}", "for(f();true;f()){}", "for{}",
		"break", "return value,other", "continue", "fallthrough",
		`include "nonexistent.yak"`, "defer panic(1)", "defer recover()",
		"defer f()", "go f()", "go fn{}", `assert true,1,"ok"`,
	}
	for i, statement := range statements {
		add(fmt.Sprintf("statement_%02d/program", i), "prefix=1\n"+statement+"\nsuffix=2\n")
		add(fmt.Sprintf("statement_%02d/closure", i), "call(func(){\n"+statement+"\n})\n")
	}
	expressions := []string{
		"value", "int(1)", "string()", "var", "any", "interface{}",
		"chan []int", "[]int", "map[string][]int", "0", "0xAB", "0o77",
		"0b10", "0h1234", "1.25", ".125", "'x'", "'xx'", `"汉字"`,
		`b"bytes"`, "r'raw'", "`raw\n [] {} ()`", "undefined", "nil", "true", "false",
		`f"prefix ${value+1} suffix"`, "f'${f(1)}'", "f`raw ${value?1:2}`",
		`f"${f'${value}'}"`, `f"${func(){return 1}()}"`,
		`f"${{"key":1}["key"]}"`, "f'${func(x){return x}(1)}'",
		"f`raw\n${fn{return 1}}`", `f"${f(func(){return {"key":[1,2]}})}"`,
		`f"${f"${func(){return 1}()}"}"`, "<<<TAG\nraw [] {} ()\nTAG\n",
		"<<<'TAG'\nraw [] {} ()\nTAG\n", "<<<'TAG'\r\nraw [] {} ()\r\nTAG\n",
		"<<<'标签'\n原文 ; // ${x}\n标签\n", "<<<'标签'\r\n原文 ; // ${x}\r\n标签\n",
		"<<<TAG\r\nraw [] {} ()\r\nTAG\n", "{}", `{value:1,"x":2,}`,
		"[]", "[value,1,]", "[]int{}", "[]int{1,2,}", `map[string]int{"x":1}`,
		"func(){return 1}", "func named(x int,y error) (int,error){return x,y}",
		"(x,y)=>x+y", "x=>x?1:2", "()=>{}", "panic(value)", "recover()",
		"(value)", "()", "fn{return 1}", "make(chan int,1)", "make(map[string]int)",
		"!value", "-value", "+value", "^value", "&value", "*value", "<-ch",
		"value.field", "value.$field", "value[0]", "value[1:2:3]", "value[::]",
		"f(1,value...,)", "f(func(){return 1},2)~", "value?f(1):f(2)",
	}
	for i, expression := range expressions {
		add(fmt.Sprintf("expression_%02d/assignment", i), "result="+expression+"\n")
		add(fmt.Sprintf("expression_%02d/argument", i), "call("+expression+")\n")
	}
	for i, operator := range []string{
		"<<", ">>", "&", "&^", "|", "^", "*", "/", "%", "+", "-",
		">", "<", ">=", "<=", "!=", "<>", "==", "in", "not in", "&&", "||", "<-",
	} {
		add(fmt.Sprintf("binary_%02d", i), "call(func(){return left "+operator+" right})\n")
	}
	for i, operator := range []string{"=", ":=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", "&^="} {
		add(fmt.Sprintf("suffix_assignment_%02d", i), "obj.field()[key].$slot "+operator+" func(){return 1}\n")
	}
	for i, source := range grammarSamples {
		add(fmt.Sprintf("all_rules_%02d", i), source)
	}
	for i, source := range []string{
		"A=0,;0#0", "a=1,;2", "return 1,;2", "a,b=1,2,;f()",
		"f(func(){return 1,;2})", "a=1,;b=2", "a=1,;/* boundary */2",
	} {
		add(fmt.Sprintf("trailing_comma_boundary_%02d", i), source)
	}
	for i, source := range []string{
		"a\n,b=1,2", "a\r\n,b=1,2", "a/* before comma */,b=1,2",
		"a// before comma\n,b=1,2", "a# before comma\n,b=1,2",
		"a/* first */\n// second\r\n,b,c=1,2,3", "a.field\n,b[0]=1,2",
		"f(func(){return 1})/* after call */\n,b[0]:=1,2",
	} {
		add(fmt.Sprintf("lhs_trivia_boundary_%02d", i), source+"\n")
	}
	// Long fallback fixtures stay in the deterministic suite. Runtime mutation
	// uses smaller sources so a slow original-ATN oracle cannot stall a fuzz worker.
	add("scan_token_limit", "f("+strings.Repeat("1,", 4096)+"0)\n")
	add("scan_depth_limit", "f("+strings.Repeat("[", 513)+"1"+strings.Repeat("]", 513)+")\n")
	add("prefix_limit_then_assignment", "f("+strings.Repeat("1,", 4096)+"0).value=1\n")
	return cases
}

// Mutate whole lexer token intervals, using rune offsets rather than byte
// offsets. Synthetic ASI tokens do not own source text and must not be edited.
// Bounded sampling includes the first and last token as well as interior tokens
// of each source, so both accepted prefixes and failing tails are exercised.
func predictionMutationCases() []predictionCase {
	bases := []string{
		"f(func(){if x {return f(1)}else{return 2}})[0].field=1\n",
		"select{case value,ok:= <-ch: f(value);default: return 0}\n",
		"for key,value:=range values{if value{continue};f(key)}\n",
		"if var value=f(1);value{go f(value)}else{defer recover()}\n",
		"try{f()}catch err{panic(err)}finally{assert true,1}\n",
		"value=func(x int,y map[string][]int) (int,error){return x,nil}\n",
		"value=(x,y)=>x+y;f(value(1,2)...,)\n",
		`value=f"prefix ${f(1,2)} []; ${f'${x}'}"` + "\n",
		"f(<<<TAG\r\nraw []; ${x}\r\nTAG\n)\n",
		"obj.$key[1:2:3](func(){return 1}).field&^=2\n",
		"value=[]map[string]int{{\"x\":1},{\"y\":2}}\n",
		"switch f(){case 1,2:f();fallthrough;default:return 0}\n",
		`value=f"${func(){return {"key":[1,2]}}()}"` + "\n",
		"f(<<<'标签'\r\n原文 ; // ${x}\r\n标签\n)\n",
		"a// before comma\n,b[0]=f(1),2\n",
		"obj.field(func(){return 1})/* before comma */\r\n,b:=1,2\n",
	}
	replacements := []string{"return", "default", "func", "=", "=>", "]", "(", ":", `f"${x}"`, "'", "`", "@"}
	insertions := []string{"\n", "\r\n", "/* [] {} ; */", "// ; }\n", "@", "'", "`", "${", "\x00", "case ", "default ", "func "}
	var cases []predictionCase
	for baseIndex, source := range bases {
		lexer := parser.NewYaklangLexer(antlr.NewInputStream(source))
		lexer.RemoveErrorListeners()
		stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		stream.Fill()
		runes := []rune(source)
		var tokens []antlr.Token
		for _, token := range stream.GetAllTokens() {
			start, stop := token.GetStart(), token.GetStop()
			if token.GetTokenType() == antlr.TokenEOF || start < 0 || stop < start || stop >= len(runes) {
				continue
			}
			if string(runes[start:stop+1]) == token.GetText() {
				tokens = append(tokens, token)
			}
		}
		for sample := 0; sample < 6 && sample < len(tokens); sample++ {
			index := sample
			if len(tokens) > 6 {
				index = sample * (len(tokens) - 1) / 5
			}
			token := tokens[index]
			start, end := token.GetStart(), token.GetStop()+1
			prefix, suffix := string(runes[:start]), string(runes[end:])
			name := fmt.Sprintf("mutation_%02d/token_%02d", baseIndex, index)
			cases = append(cases,
				predictionCase{name: name + "/delete", source: prefix + suffix},
				predictionCase{name: name + "/replace", source: prefix + replacements[(sample+baseIndex)%len(replacements)] + suffix},
				predictionCase{name: name + "/insert", source: prefix + insertions[(2*sample+baseIndex)%len(insertions)] + string(runes[start:])},
			)
		}
	}
	for i, source := range []string{
		"", " \t\r\n", "a=", "a++ +", "select\n{}", "select{case:}",
		"f(func(){a=;},@)", "f(func(){a=;},\n @)", "f(func(){if{}},@)",
		"f(func(){a=;},\"unterminated)", "f(func(){a=1},\n @)", "a=1\n\"unterminated",
		`f(f"${func(){return [}}")`, "f(<<<TAG\nunterminated\n)",
		"if value{case 1:}", "switch{default:default:}", "func(x int,) (int,error {return 1}",
	} {
		cases = append(cases, predictionCase{name: fmt.Sprintf("boundary_%02d", i), source: source})
	}
	return cases
}

// The comparison uses production's real SLL bail strategy on both sides. LL
// recovery has different messages from an initial full LL parse, so invalid
// input is compared to the original SLL-first path, not to a different mode.
// Valid input has an additional fresh full-context LL reference, preventing a
// shared DFA from hiding a common acceptance or AST error.
func assertPredictionDifferential(t *testing.T, source string) bool {
	t.Helper()
	if strings.TrimSpace(source) == "" {
		if output, err := Format(source); output != "" || err != nil {
			t.Fatalf("empty-format API changed: %q, %v", output, err)
		}
		return true
	}
	reference, referenceStream, referenceErr := parsePrediction(source, false, antlr.PredictionModeSLL)
	optimized, optimizedStream, optimizedErr := parsePrediction(source, true, antlr.PredictionModeSLL)
	if (referenceErr == nil) != (optimizedErr == nil) {
		t.Fatalf("changed accept/reject for %q: original=%v, fast=%v", excerpt(source), referenceErr, optimizedErr)
	}
	if referenceErr != nil {
		if optimizedErr.Error() != referenceErr.Error() {
			t.Fatalf("changed first diagnostic for %q: original=%v, fast=%v", excerpt(source), referenceErr, optimizedErr)
		}
		if output, err := Format(source); output != "" || err == nil || err.Error() != referenceErr.Error() {
			t.Fatalf("invalid input changed formatter error for %q: output=%q, err=%v, want=%v", excerpt(source), excerpt(output), err, referenceErr)
		}
		// Every rejected input is followed by a different valid prefix and mode.
		// Failed lookahead, lexer modes and parser-instance caches must not leak.
		if output, err := Format(`next=f"原文 ${f(1,2)}"`); err != nil || output != "next = f\"原文 ${f(1, 2)}\"\n" {
			t.Fatalf("rejected parse contaminated later formatter: %q, %v", output, err)
		}
		return false
	}
	wantAST := predictionSnapshot(reference)
	if got := predictionSnapshot(optimized); got != wantAST {
		t.Fatalf("changed exact AST for %q", excerpt(source))
	}
	wantLayout := FormatTree(source, reference, referenceStream)
	if got := FormatTree(source, optimized, optimizedStream); got != wantLayout {
		t.Fatalf("changed predicted layout for %q: original=%q, fast=%q", excerpt(source), excerpt(wantLayout), excerpt(got))
	}
	full, fullStream, err := parsePrediction(source, false, antlr.PredictionModeLL)
	if err != nil || predictionSnapshot(full) != wantAST {
		t.Fatalf("SLL-first differs from fresh LL AST for %q: %v", excerpt(source), err)
	}
	if got := FormatTree(source, full, fullStream); got != wantLayout {
		t.Fatalf("SLL-first differs from fresh LL layout for %q", excerpt(source))
	}
	output, err := Format(source)
	if err != nil || output != wantLayout {
		t.Fatalf("formatter differs from original predictor for %q: output=%q, err=%v", excerpt(source), excerpt(output), err)
	}
	formatted, formattedStream, err := parsePrediction(output, false, antlr.PredictionModeSLL)
	if err != nil {
		t.Fatalf("formatted source no longer parses for %q: %v", excerpt(source), err)
	}
	if fingerprint(reference, nil) != fingerprint(formatted, nil) || fmt.Sprint(comments(referenceStream)) != fmt.Sprint(comments(formattedStream)) {
		t.Fatalf("formatting changed syntax or trivia for %q: %q", excerpt(source), excerpt(output))
	}
	if second, err := Format(output); err != nil || second != output {
		t.Fatalf("formatting not idempotent for %q: %v", excerpt(source), err)
	}
	return true
}

func TestFormatPredictionAdversarialMutations(t *testing.T) {
	cases := append(predictionGrammarCases(), predictionMutationCases()...)
	valid, invalid := 0, 0
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			accepted := assertPredictionDifferential(t, fixture.source)
			if fixture.valid && !accepted {
				t.Fatalf("grammar-generated valid fixture was rejected: %q", excerpt(fixture.source))
			}
			if accepted {
				valid++
			} else {
				invalid++
			}
		})
	}
	if valid < 250 || invalid < 100 {
		t.Fatalf("insufficient accepted/rejected adversarial coverage: valid=%d invalid=%d", valid, invalid)
	}
	t.Logf("differential-checked %d cases: %d accepted, %d rejected", len(cases), valid, invalid)
}

func boundedPredictionFuzzSource(source string) bool {
	if len(source) > 4*1024 {
		return false
	}
	// This conservative byte scan also counts delimiters inside literals. That
	// is intentional: the reference parser, not optimized throughput, sets the
	// mutation budget. Full literal/depth limits are covered deterministically.
	depth, delimiters := 0, 0
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '(', '[', '{':
			depth++
			delimiters++
			if depth > 48 || delimiters > 512 {
				return false
			}
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return true
}

func FuzzFormatPrediction(f *testing.F) {
	for _, fixture := range append(predictionGrammarCases(), predictionMutationCases()...) {
		if boundedPredictionFuzzSource(fixture.source) {
			f.Add(fixture.source)
		}
	}
	f.Fuzz(func(t *testing.T, source string) {
		if !boundedPredictionFuzzSource(source) {
			return
		}
		assertPredictionDifferential(t, source)
	})
}
