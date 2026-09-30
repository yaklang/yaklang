package yakfmt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

// Together these exercise every parser rule, each statement and expression
// alternative, every operator, and all lexer literal modes. Syntax-only examples
// deliberately include free identifiers and include paths that need not exist.
var grammarSamples = []string{
	`a=f"; ${x} ;";a=f';';a=f` + "`" + `;` + "`" + `;a=f"{}[](),:?+-*/<>=!&|%^~"`,
	`x=panic(1);x=recover();x={func(){return 1}:2,[1,2]:3};func(long_parameter_name_long_parameter_name_long_parameter_name_long_parameter_name_ map[string][]int){}`,
	`a="\\\"\n\t";a='\\';a=f"escaped \$ and ${x}";a=b'bytes'`,
	`// line
/* block */
# hash
var a,b;// middle
any c,d=1,2;a,b:=3,4;a.b=1;a.$key=2;a[0]=3;{a++};try{}catch{};try{}catch e{}finally{}
include "does-not-exist.yak";defer recover();defer panic(1);defer f();go f();go fn{};assert true,1,"ok";return;return 1,2`,
	`for{};for true{};for i=0;i<1;i++{};for(i=0;i<1;i+=1){};for ; ;{};for f();true;f(){};for range a{};for in a{};for v in a{};for k,v:=range a{};for k,v=range a{continue;break}`,
	`if true{};if x=1;x{}elif y{}elif z{}else{};if var x=1;x{}else if y{};if f();x{};switch{case true:};switch x{case 1,2:a++;fallthrough;case 3:default:};select{};select{case <-ch:case ch<-1:case v= <-ch:case v,ok:= <-ch:default:}`,
	`a=!x;a=-x;a=+x;a=^x;a=&x;a=*x;a= <-x;a= & ^x;a=x<<y;a=x>>y;a=x&y;a=x&^y;a=x|y;a=x^y;a=x*y;a=x/y;a=x%y;a=x+y;a=x-y;a=x>y;a=x<y;a=x>=y;a=x<=y;a=x!=y;a=x<>y;a=x==y;a=x in y;a=x not in y;a=x&&y;a=x||y;a=x?y:z;a=ch<-x`,
	`a+=1;a-=1;a*=1;a/=1;a%=1;a&=1;a|=1;a^=1;a<<=1;a>>=1;a&^=1;a++;a--;a=(x);a=();a=int(1);a=string();a=f()~;a=f(x,y...);a=m.$key;a=m.key;a=m[0];a=m[:];a=m[1:];a=m[:2];a=m[1:2];a=m[::];a=m[1::3];a=m[:2:3];a=m[1:2:3]`,
	`func named(){};fn(x...){};def(x,y,){};function(x){};func(x string,y map[int][]byte) []int {};func(x error) error{};func() (int,error){};()=>1;(x,y)=>{};x=>x;fn{};func{};a=make([]int);a=make(chan int,1,2);a=make(map[string]int,1)`,
	`a=0;a=12;a=077;a=0o77;a=0O77;a=0xAB;a=0Xab;a=0b10;a=1.23;a=.123;a='x';a='xx';a="汉字";a=b"bytes";a=r'raw';a=0h1234;a=undefined;a=nil;a=true;a=false;a=[];a=[1,2,];a=[]int{};a=[]int{1,2,};a={};a={1:2,3:4,};a=map[string]int{"x":1};a=map[string]chan []byte{};a=interface{};a=var;a=any`,
	"a=`raw\n  text`;a=f'hello ${a+1}';a=f\"hello ${f(1)}\";a=f`hello\n${a?1:2}`;a=f\"${f'${x}'}\"",
	"a=<<<TAG\nraw\nTAG\na=<<<TAG\r\nraw\r\nTAG\n",
	"f(\n1, // one\n2 /* two */\n)\nx=[\n1, // one\n2\n]\ny={\n\"x\":1, // one\n\"y\":2\n}\nfunc(a, // first\nb /* second */) {\nreturn a,\nb\n}\n",
	"x=[<<<TAG\nraw\nTAG\n]\nf(<<<TAG\nraw\nTAG\n)",
	`f(func(){return 1},func(){return 2});x={"a":func(){return 3}};select{case ch<-func(){return 1}:switch x{case 1:break};default:select{default:break}}`,
}

func parseTest(source string) (tree parser.IProgramContext, stream *antlr.CommonTokenStream, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*syntaxError); ok {
				err = e
			} else {
				panic(r)
			}
		}
	}()
	lexer := parser.NewYaklangLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(&errorListener{antlr.NewDefaultErrorListener()})
	stream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewYaklangParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(&errorListener{antlr.NewDefaultErrorListener()})
	tree = p.Program()
	return
}

// The normalized parse-tree structure catches changes in statement boundaries,
// precedence, operator arity, types, and literal payloads. Only layout rules,
// comments and optional trailing commas/semicolons are omitted. Comments are
// checked independently by their exact token text and order.
func fingerprint(tree antlr.Tree, rules map[int]bool) string {
	var out strings.Builder
	var walk func(antlr.Tree)
	walk = func(node antlr.Tree) {
		if c, ok := node.(antlr.ParserRuleContext); ok {
			rule := c.GetRuleIndex()
			// Parentheses inserted at positions where grammar forbids a bare
			// LF are transparent; binary/unary tree shape still proves precedence.
			if rule == parser.YaklangParserRULE_expression {
				children := c.GetChildren()
				if len(children) == 1 {
					if p, ok := children[0].(*parser.ParenExpressionContext); ok && p.Expression() != nil {
						walk(p.Expression())
						return
					}
				}
			}
			if rules != nil {
				rules[rule] = true
			}
			switch rule {
			case parser.YaklangParserRULE_ws, parser.YaklangParserRULE_eos, parser.YaklangParserRULE_empty:
				return
			}
			if rule == parser.YaklangParserRULE_statement {
				for _, ch := range c.GetChildren() {
					if r, ok := ch.(antlr.ParserRuleContext); ok && (r.GetRuleIndex() == parser.YaklangParserRULE_empty || r.GetRuleIndex() == parser.YaklangParserRULE_lineCommentStmt) {
						return
					}
				}
			}
			if rule == parser.YaklangParserRULE_statementList {
				// Empty bodies may gain/lose an ASI-generated empty statement.
				for _, ch := range c.GetChildren() {
					walk(ch)
				}
				return
			}
			fmt.Fprintf(&out, "(%d", rule)
			for _, ch := range c.GetChildren() {
				walk(ch)
			}
			out.WriteByte(')')
		} else if n, ok := node.(antlr.TerminalNode); ok {
			token := n.GetSymbol()
			typ := token.GetTokenType()
			if typ == antlr.TokenEOF || typ == parser.YaklangLexerLF || typ == parser.YaklangLexerSemiColon || typ == parser.YaklangLexerComma || isComment(typ) {
				return
			}
			fmt.Fprintf(&out, "[%d:%q]", typ, token.GetText())
		}
	}
	walk(tree)
	return out.String()
}
func comments(stream *antlr.CommonTokenStream) []string {
	stream.Fill()
	var result []string
	for _, t := range stream.GetAllTokens() {
		if isComment(t.GetTokenType()) {
			result = append(result, t.GetText())
		}
	}
	return result
}
func assertRoundTrip(t *testing.T, source string, rules map[int]bool) {
	t.Helper()
	tree, ts, err := parseTest(source)
	if err != nil {
		t.Fatal("invalid fixture:", err)
	}
	// Collect rule coverage separately, including layout rules omitted by the
	// semantic fingerprint and descendants of empty/comment statements.
	if rules != nil {
		stack := []antlr.Tree{tree}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if c, ok := n.(antlr.ParserRuleContext); ok {
				rules[c.GetRuleIndex()] = true
			}
			stack = append(stack, n.GetChildren()...)
		}
	}
	got, err := Format(source)
	if err != nil {
		t.Fatal(err)
	}
	formatted, fts, err := parseTest(got)
	if err != nil {
		t.Fatalf("output invalid: %v\n%s", err, excerpt(got))
	}
	if a, b := fingerprint(tree, nil), fingerprint(formatted, nil); a != b {
		t.Fatalf("changed syntax structure\nsource:\n%s\nformatted:\n%s\nbefore: %s\nafter: %s", excerpt(source), excerpt(got), excerpt(a), excerpt(b))
	}
	if fmt.Sprint(comments(ts)) != fmt.Sprint(comments(fts)) {
		t.Fatalf("changed comments:\n%s", excerpt(got))
	}
	again, err := Format(got)
	if err != nil || again != got {
		t.Fatalf("not idempotent: %v\nfirst:\n%s\nsecond:\n%s", err, excerpt(got), excerpt(again))
	}
}
func TestFormatGrammarCoverage(t *testing.T) {
	rules := make(map[int]bool)
	for i, source := range grammarSamples {
		t.Run(fmt.Sprint(i), func(t *testing.T) { assertRoundTrip(t, source, rules) })
	}
	parser.YaklangParserInit()
	for i, name := range parser.YaklangParserParserStaticData.RuleNames {
		if !rules[i] {
			t.Errorf("grammar rule not covered: %s", name)
		}
	}
	t.Logf("covered %d parser rules", len(rules))
}
func TestFormatRepositoryCorpus(t *testing.T) {
	paths, err := filepath.Glob("../../yaktest/mustpass/files/*.yak")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 100 {
		t.Fatalf("missing repository corpus: %d files", len(paths))
	}
	valid, invalid := 0, 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		if _, err := Format(source); err != nil {
			invalid++
			continue
		}
		valid++
		t.Run(filepath.Base(path), func(t *testing.T) { assertRoundTrip(t, source, nil) })
	}
	if valid < 100 {
		t.Fatalf("only %d valid scripts", valid)
	}
	t.Logf("round-tripped %d repository scripts, %d pre-existing syntax-invalid fixtures", valid, invalid)
}
func FuzzFormat(f *testing.F) {
	for _, s := range grammarSamples {
		f.Add(s)
	}
	for _, s := range []string{"", "dump(123)))", "@", "select\n{}", "a=", "x= & ^y", "x=[" + strings.Repeat("1,", 40) + "2]", "x=1" + strings.Repeat("+2", 100)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 16*1024 {
			return
		}
		got, err := Format(source)
		if err != nil {
			return
		}
		if got == "" {
			return
		}
		assertRoundTrip(t, source, nil)
	})
}

func TestFormatFiles(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.input.yak")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 5 {
		t.Fatal("missing style fixtures")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(strings.TrimSuffix(path, ".input.yak") + ".golden.yak")
			if err != nil {
				t.Fatal(err)
			}
			got, err := Format(string(source))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(expected) {
				t.Fatalf("golden changed:\n%s", got)
			}
			assertRoundTrip(t, string(source), nil)
			assertWidth(t, got)
		})
	}
}

func excerpt(source string) string {
	if len(source) > 4096 {
		return source[:3584] + "\n... truncated ...\n" + source[len(source)-512:]
	}
	return source
}

// Rule coverage alone cannot prove that alternatives sharing one rule were
// exercised. Track the direct grammar shape of statements and expressions too.
func TestFormatGrammarAlternatives(t *testing.T) {
	statements, expressions := map[string]bool{}, map[string]bool{}
	operators := map[string]map[string]bool{}
	for _, source := range grammarSamples {
		tree, _, err := parseTest(source)
		if err != nil {
			t.Fatal(err)
		}
		stack := []antlr.Tree{tree}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stack = append(stack, node.GetChildren()...)
			c, ok := node.(antlr.ParserRuleContext)
			if !ok {
				continue
			}
			rule := c.GetRuleIndex()
			names := parser.YaklangParserParserStaticData.RuleNames
			if rule == parser.YaklangParserRULE_statement {
				for _, child := range c.GetChildren() {
					if r, ok := child.(antlr.ParserRuleContext); ok {
						statements[names[r.GetRuleIndex()]] = true
						break
					}
				}
			}
			switch rule {
			case parser.YaklangParserRULE_unaryOperator, parser.YaklangParserRULE_bitBinaryOperator, parser.YaklangParserRULE_additiveBinaryOperator, parser.YaklangParserRULE_multiplicativeBinaryOperator, parser.YaklangParserRULE_comparisonBinaryOperator, parser.YaklangParserRULE_inplaceAssignOperator:
				name := names[rule]
				if operators[name] == nil {
					operators[name] = map[string]bool{}
				}
				operators[name][c.GetText()] = true
			}
			if rule != parser.YaklangParserRULE_expression {
				continue
			}
			directRules, directTokens := map[int]bool{}, map[int]bool{}
			for _, child := range c.GetChildren() {
				if r, ok := child.(antlr.ParserRuleContext); ok {
					directRules[r.GetRuleIndex()] = true
				} else if n, ok := child.(antlr.TerminalNode); ok {
					directTokens[n.GetSymbol().GetTokenType()] = true
				}
			}
			for _, r := range []int{parser.YaklangParserRULE_typeLiteral, parser.YaklangParserRULE_literal, parser.YaklangParserRULE_anonymousFunctionDecl, parser.YaklangParserRULE_panicStmt, parser.YaklangParserRULE_recoverStmt, parser.YaklangParserRULE_memberCall, parser.YaklangParserRULE_sliceCall, parser.YaklangParserRULE_functionCall, parser.YaklangParserRULE_parenExpression, parser.YaklangParserRULE_instanceCode, parser.YaklangParserRULE_makeExpression, parser.YaklangParserRULE_unaryOperator, parser.YaklangParserRULE_bitBinaryOperator, parser.YaklangParserRULE_multiplicativeBinaryOperator, parser.YaklangParserRULE_additiveBinaryOperator, parser.YaklangParserRULE_comparisonBinaryOperator} {
				if directRules[r] {
					expressions[names[r]] = true
				}
			}
			for token, name := range map[int]string{parser.YaklangLexerIdentifier: "identifier", parser.YaklangLexerLogicAnd: "and", parser.YaklangLexerLogicOr: "or", parser.YaklangLexerQuestion: "ternary", parser.YaklangLexerChanIn: "send"} {
				if directTokens[token] {
					expressions[name] = true
				}
			}
			if directTokens[parser.YaklangLexerIn] {
				if directTokens[parser.YaklangLexerNotLiteral] {
					expressions["not_in"] = true
				} else {
					expressions["in"] = true
				}
			}
		}
	}
	for _, name := range []string{"lineCommentStmt", "declareVariableExpressionStmt", "assignExpressionStmt", "expressionStmt", "block", "tryStmt", "empty", "selectStmt", "ifStmt", "switchStmt", "forRangeStmt", "forStmt", "breakStmt", "returnStmt", "continueStmt", "fallthroughStmt", "includeStmt", "deferStmt", "goStmt", "assertStmt"} {
		if !statements[name] {
			t.Errorf("statement alternative missing: %s", name)
		}
	}
	for _, name := range []string{"typeLiteral", "literal", "anonymousFunctionDecl", "panicStmt", "recoverStmt", "identifier", "memberCall", "sliceCall", "functionCall", "parenExpression", "instanceCode", "makeExpression", "unaryOperator", "bitBinaryOperator", "multiplicativeBinaryOperator", "additiveBinaryOperator", "comparisonBinaryOperator", "in", "not_in", "and", "or", "ternary", "send"} {
		if !expressions[name] {
			t.Errorf("expression alternative missing: %s", name)
		}
	}
	for rule, expected := range map[string][]string{
		"unaryOperator": {"!", "-", "+", "^", "&", "*", "<-"}, "bitBinaryOperator": {"<<", ">>", "&", "&^", "|", "^"}, "additiveBinaryOperator": {"+", "-"}, "multiplicativeBinaryOperator": {"*", "/", "%"}, "comparisonBinaryOperator": {">", "<", "<=", ">=", "!=", "<>", "=="}, "inplaceAssignOperator": {"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", "&^="},
	} {
		for _, op := range expected {
			if !operators[rule][op] {
				t.Errorf("operator branch missing: %s %s", rule, op)
			}
		}
	}
	t.Logf("covered %d statement shapes, %d expression shapes, and every operator alternative", len(statements), len(expressions))
}
