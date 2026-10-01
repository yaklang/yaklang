package pythonparser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

type predictionAutomata struct {
	atn   *antlr.ATN
	dfas  []*antlr.DFA
	cache *antlr.PredictionContextCache
}

func newPredictionAutomata() *predictionAutomata {
	atn := antlr.NewATNDeserializer(nil).Deserialize(GetPythonParserSerializedATN())
	dfas := make([]*antlr.DFA, len(atn.DecisionToState))
	for i, state := range atn.DecisionToState {
		dfas[i] = antlr.NewDFA(state, i)
	}
	return &predictionAutomata{atn, dfas, antlr.NewPredictionContextCache()}
}

func (a *predictionAutomata) apply(p *PythonParser) { p.SetInterpreter(a.atn, a.dfas, a.cache) }

type predictionResult struct {
	tree    IRootContext
	err     error
	next    int
	version PythonVersion
}

type predictionLexer struct {
	*PythonLexer
	lexical *antlr4util.ErrorListener
}

func (l *predictionLexer) RemoveErrorListeners() {
	l.PythonLexer.RemoveErrorListeners()
	l.PythonLexer.AddErrorListener(l.lexical)
}

// Exercise the same two-stage parser, error recovery and cache detachment used
// by Python's SSA frontend. The independent LL oracle uses a separate automaton.
func parsePrediction(source string, fast bool, reference *predictionAutomata) predictionResult {
	var next int
	var version PythonVersion
	if reference == nil {
		lexical := antlr4util.NewErrorListener()
		tree, err := antlr4util.ParseASTWithSLLFirst(source, func(input antlr.CharStream) *predictionLexer { return &predictionLexer{NewPythonLexer(input), lexical} },
			func(input antlr.TokenStream) *PythonParser {
				p := NewPythonParser(input)
				p.SetFastPrediction(fast)
				return p
			}, nil, nil, func(p *PythonParser) IRootContext {
				tree := p.Root()
				next = p.GetTokenStream().LA(1)
				version = p.Version
				return tree
			})
		return predictionResult{tree, errors.Join(err, lexical.Error()), next, version}
	}
	errors := antlr4util.NewErrorListener()
	lexer := NewPythonLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errors)
	p := NewPythonParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.SetFastPrediction(fast)
	reference.apply(p)
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeLL)
	p.RemoveErrorListeners()
	p.AddErrorListener(errors)
	tree := p.Root()
	next = p.GetTokenStream().LA(1)
	return predictionResult{tree, errors.Error(), next, p.Version}
}

func predictionSnapshot(tree antlr.Tree) string {
	var out strings.Builder
	if tree == nil {
		return "nil"
	}
	stack := []antlr.Tree{tree}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch n := node.(type) {
		case antlr.ParserRuleContext:
			start, stop := -1, -1
			if n.GetStart() != nil {
				start = n.GetStart().GetTokenIndex()
			}
			if n.GetStop() != nil {
				stop = n.GetStop().GetTokenIndex()
			}
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d\n", n, n.GetRuleIndex(), n.GetChildCount(), start, stop)
		case antlr.TerminalNode:
			t := n.GetSymbol()
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d:%d:%d:%d:%q\n", n, t.GetTokenType(), t.GetTokenIndex(), t.GetStart(), t.GetStop(), t.GetLine(), t.GetColumn(), t.GetChannel(), t.GetText())
		}
		children := node.GetChildren()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return out.String()
}

func checkPrediction(t *testing.T, source string, reference *predictionAutomata, valid bool) {
	t.Helper()
	original := parsePrediction(source, false, nil)
	optimized := parsePrediction(source, true, nil)
	if fmt.Sprint(original.err) != fmt.Sprint(optimized.err) || original.next != optimized.next || original.version != optimized.version {
		t.Fatalf("changed diagnostics/remaining input for %q\noriginal: %v (%d)\noptimized: %v (%d)", source, original.err, original.next, optimized.err, optimized.next)
	}
	if got, want := predictionSnapshot(optimized.tree), predictionSnapshot(original.tree); got != want {
		t.Fatalf("changed exact SLL-first AST for %q\noriginal: %.1800s\noptimized: %.1800s", source, want, got)
	}
	if valid && (original.err != nil || original.next != antlr.TokenEOF) {
		t.Fatalf("invalid valid fixture: %q: %v (%d)", source, original.err, original.next)
	}
	if reference != nil && original.err == nil {
		ll := parsePrediction(source, false, reference)
		if ll.err != nil || ll.next != original.next || ll.version != original.version || predictionSnapshot(ll.tree) != predictionSnapshot(original.tree) {
			t.Fatalf("SLL/LL oracle mismatch for %q: %v", source, ll.err)
		}
	}
}

var predictionExpressions = []string{
	"x", "True", "False", "None", "...", "0", "0xFF", "0o77", "0b101", "1_000", "1.25e-3", "3j",
	"'x'", "'a' 'b'", "r'\\n'", "b'abc'", "u'雪'", "f'{x}'", "'''a\nb'''", "-1", "--1", "+1", "~x", "-2**3", "(-2)**3", "2**-3",
	"a+b*c", "a**b**c", "a<<b+1", "a&b^c|d", "not a and b or c", "a<b<=c", "a not in b", "a is not b",
	"()", "(x)", "(x,)", "(x,y)", "[]", "[x,*ys]", "{}", "{x,y}", "{x:y,**z}", "{**x}",
	"(x for x in xs)", "[x for x in xs if x]", "{x for x in xs}", "{x:y for x in xs if x}",
	"[y for x in xs for y in x if y]", "[x if x else y for x in xs]", "[x for x in xs if x if y]",
	"f()", "f(x,y=1,*xs,**kw)", "f(x for x in xs)", "f(lambda x:x)", "lambda:x", "lambda x,y=1,*args,**kw:x+y",
	"lambda x,/,y=1:x", "x if y else z", "x if y else a if b else c", "(x:=f())", "await f(x)",
	"a.b(c)[x].d", "a[...,...]", "a[...:x]", "a[x:y:z]", "a[::]", "a[:,x,y:]", "(yield x)", "(yield from xs)",
	"print", "exec", "obj.print", "obj.exec", "type(x)", "match(x)", "case", "`x,y`",
}

var predictionStatements = []string{
	"x=1\n", "a=b=c\n", "x,y,*z=xs\n", "x:T=f()\n", "x+=y\n", "x**=y\n", "pass\n", "break\n", "continue\n", "return\n", "return x,y\n",
	"raise\n", "raise E(x) from e\n", "yield\n", "yield from xs\n", "del x,y[0]\n", "assert x,y\n", "global x,y\n", "nonlocal x,y\n",
	"import a.b as c,d\n", "from ..a import b as c,d\n", "from a import (b,c,)\n", "from a import *\n", "type Alias = list[int]\n",
	"match=1\ncase=2\ntype=3\n", "print x,y\n", "print >>f,x,\n", "exec x in g,l\n", "try:\n    f()\nexcept E, e:\n    pass\n",
	"if x:\n    pass\nelif y:\n    f()\nelse:\n    g()\n", "while x:\n    continue\nelse:\n    break\n",
	"for x,*ys in xs:\n    f(x)\nelse:\n    pass\n", "async for x in xs:\n    pass\n", "with f() as x,g() as y:\n    pass\n",
	"with (f() as x,g() as y,):\n    pass\n", "async with f() as x:\n    pass\n",
	"try:\n    f()\nexcept (A,B) as e:\n    g()\nelse:\n    h()\nfinally:\n    i()\n", "try:\n    pass\nfinally:\n    pass\n",
	"@a.b(x)\n@c\nasync def f(x:T=1,/,y=2,*args,**kw)->T:\n    return x\n", "def f[T](x:T)->T:\n    return x\n",
	"class C[T](Base,metaclass=M):\n    def f(self):\n        pass\n", "match x:\n    case 1 | 2 if y:\n        pass\n    case head,*tail:\n        pass\n",
	"if x: f(); g()\n", "x=(1+\\\n    2)\n", "if x:\n\tpass\n", "if x:\n    # comment\n    pass", "x=1;pass;\n",
}

func TestPredictionGrammarEquivalence(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range predictionExpressions {
		t.Run(fmt.Sprintf("expr_%03d", i), func(t *testing.T) { checkPrediction(t, "x="+expr+"\n", reference, true) })
	}
	for i, stmt := range predictionStatements {
		t.Run(fmt.Sprintf("stmt_%03d", i), func(t *testing.T) { checkPrediction(t, stmt, reference, true) })
	}
	for _, source := range []string{"", "\n", "#comment", "x", "x,y", "if x:\n    pass\n\n"} {
		checkPrediction(t, source, reference, true)
	}
}

func TestPredictionAdversarialMatrix(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range predictionExpressions {
		for j, pattern := range []string{"x=%s\n", "f(%s)\n", "return %s\n", "if %s:\n    pass\n", "x=(%s)\n", "x=[%s, y]\n", "x={y:%s}\n", "x=lambda:%s\n"} {
			t.Run(fmt.Sprintf("%03d_%d", i, j), func(t *testing.T) { checkPrediction(t, fmt.Sprintf(pattern, expr), reference, true) })
		}
	}
	for _, name := range []string{"match", "case", "type", "print", "exec", "True", "False", "正常"} {
		for _, pattern := range []string{"%s(x)\n", "x=%s\n", "x=obj.%s\n", "def f(%s=1):\n    pass\n"} {
			checkPrediction(t, fmt.Sprintf(pattern, name), reference, true)
		}
	}
	for _, op := range []string{"+=", "-=", "*=", "/=", "//=", "%=", "**=", "&=", "|=", "^=", "<<=", ">>="} {
		for _, lhs := range []string{"x", "x.y", "x[0]"} {
			checkPrediction(t, lhs+op+"f(lambda x:x)\n", reference, true)
		}
	}
	for _, stmt := range predictionStatements {
		checkPrediction(t, strings.ReplaceAll(stmt, "\n", "\r\n"), reference, true)
	}
}

func TestPredictionBoundaries(t *testing.T) {
	reference := newPredictionAutomata()
	for _, depth := range []int{1, 8, 32, 64, 128} {
		for i, expr := range []string{strings.Repeat("f(", depth) + "x" + strings.Repeat(")", depth), strings.Repeat("f(lambda x:", depth) + "x" + strings.Repeat(")", depth), strings.Repeat("[", depth) + "x" + strings.Repeat("]", depth), strings.Repeat("not ", depth) + "x"} {
			t.Run(fmt.Sprintf("depth_%d_%d", depth, i), func(t *testing.T) { checkPrediction(t, "x="+expr+"\n", reference, true) })
		}
	}
	for _, n := range []int{1, 64, 512, 1024} {
		checkPrediction(t, "f("+strings.Repeat("x,", n)+"x)\n", reference, true)
		checkPrediction(t, "x="+strings.Repeat("a+", n)+"a\n", reference, true)
	}
	for _, gap := range []string{" ", "\t", "\n#comment\n", "\r\n", "\n" + strings.Repeat("# comment\n", 1000)} {
		checkPrediction(t, "x=f("+gap+"lambda x:"+gap+"x"+gap+")\n", reference, true)
	}
}

func TestPredictionInvalidInputs(t *testing.T) {
	for _, source := range []string{"x=$1\n", "x=1$\n", "if x:\n    y=$1\n", "x='unterminated\n", "x=\x00\n", "x=\"\"\"unterminated", "f(\n", "x=1+\n", "if:\n    pass\n", "if x:\n", "x=[1,\n", "x=lambda x\n", "type T =\n", "match x:\n    case:\n        pass\n", "async def f(:\n", "x[1:2:3:4]\n", "print x\nnonlocal y\n"} {
		t.Run(fmt.Sprintf("%q", source), func(t *testing.T) {
			checkPrediction(t, source, nil, false)
			if parsePrediction(source, false, nil).err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestPredictionRealPythonCorpus(t *testing.T) {
	var files []string
	err := filepath.WalkDir("../test/syntax", func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(file, ".py") {
			files = append(files, file)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("missing Python corpus: %v", err)
	}
	reference := newPredictionAutomata()
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			checkPrediction(t, string(source), reference, false)
		})
	}
}

func TestPredictionConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reference := newPredictionAutomata()
			for i := 0; i < 8; i++ {
				for _, source := range []string{"print x\n", "x:T=1\n", "match x:\n    case 1:\n        pass\n", "x=$1\n", "x=f(lambda x:x)\n"} {
					checkPrediction(t, source, reference, false)
				}
			}
		}()
	}
	wg.Wait()
}

func TestPredictionGrammarRuleCoverage(t *testing.T) {
	seen := make(map[int]bool)
	var sources []string
	for _, expr := range predictionExpressions {
		sources = append(sources, "x="+expr+"\n")
	}
	sources = append(sources, predictionStatements...)
	files, err := filepath.Glob("../test/syntax/g4/*.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, string(source))
	}
	for _, source := range sources {
		result := parsePrediction(source, true, nil)
		if result.err != nil {
			continue
		}
		stack := []antlr.Tree{result.tree}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if rule, ok := node.(antlr.ParserRuleContext); ok {
				seen[rule.GetRuleIndex()] = true
			}
			stack = append(stack, node.GetChildren()...)
		}
	}
	// eval_input is shadowed by earlier root alternatives for ordinary
	// expressions, but is also a public entry rule. Cover it directly.
	p := NewPythonParser(boundaryTokens("(x:=1),lambda:2\n"))
	p.RemoveErrorListeners()
	eval := p.Eval_input()
	if p.GetTokenStream().LA(1) != antlr.TokenEOF {
		t.Fatal("eval_input did not consume fixture")
	}
	seen[eval.GetRuleIndex()] = true
	for i, name := range p.RuleNames {
		if !seen[i] {
			t.Errorf("grammar rule has no accepted adversarial instance: %s", name)
		}
	}
}
