package javaparser

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
	atn := antlr.NewATNDeserializer(nil).Deserialize(GetJavaParserSerializedATN())
	dfas := make([]*antlr.DFA, len(atn.DecisionToState))
	for i, state := range atn.DecisionToState {
		dfas[i] = antlr.NewDFA(state, i)
	}
	return &predictionAutomata{atn, dfas, antlr.NewPredictionContextCache()}
}

func (a *predictionAutomata) apply(p *JavaParser) { p.SetInterpreter(a.atn, a.dfas, a.cache) }

type predictionResult struct {
	tree ICompilationUnitContext
	err  error
	next int
}

type predictionLexer struct {
	*JavaLexer
	lexical *antlr4util.ErrorListener
}

func (l *predictionLexer) RemoveErrorListeners() {
	l.JavaLexer.RemoveErrorListeners()
	l.JavaLexer.AddErrorListener(l.lexical)
}

// Exercise the same two-stage parser, error recovery and cache detachment used
// by Java's SSA frontend. The independent LL oracle uses a separate automaton.
func parsePrediction(source string, fast bool, reference *predictionAutomata) predictionResult {
	var next int
	if reference == nil {
		lexical := antlr4util.NewErrorListener()
		tree, err := antlr4util.ParseASTWithSLLFirst(source, func(input antlr.CharStream) *predictionLexer { return &predictionLexer{NewJavaLexer(input), lexical} },
			func(input antlr.TokenStream) *JavaParser {
				p := NewJavaParser(input)
				p.SetFastPrediction(fast)
				return p
			}, nil, nil, func(p *JavaParser) ICompilationUnitContext {
				tree := p.CompilationUnit()
				next = p.GetTokenStream().LA(1)
				return tree
			})
		return predictionResult{tree, errors.Join(err, lexical.Error()), next}
	}
	errors := antlr4util.NewErrorListener()
	lexer := NewJavaLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errors)
	p := NewJavaParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.SetFastPrediction(fast)
	reference.apply(p)
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeLL)
	p.RemoveErrorListeners()
	p.AddErrorListener(errors)
	tree := p.CompilationUnit()
	next = p.GetTokenStream().LA(1)
	return predictionResult{tree, errors.Error(), next}
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
	if fmt.Sprint(original.err) != fmt.Sprint(optimized.err) || original.next != optimized.next {
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
		if ll.err != nil || ll.next != original.next || predictionSnapshot(ll.tree) != predictionSnapshot(original.tree) {
			t.Fatalf("SLL/LL oracle mismatch for %q: %v", source, ll.err)
		}
	}
}

func methodSource(body string) string { return "class C { Object m(){" + body + "} }" }

var predictionExpressions = []string{
	"a", "module", "record", "yield", "var", "enum", "1", "0x1", "01", "0b1", "1.2", "0x1p2", "true", "'a'", `"s"`, "null",
	"this", "super", "this.x", "super.x", "this()", "super()", "f()", "f(1,a,)", "f(x -> x)",
	"x -> x", "() -> x", "(x) -> x", "(x,y) -> x+y", "(T x) -> x", "(var x) -> x", "(final T x) -> x", "(@A T x) -> x",
	"a++", "a--", "++a", "--a", "++a.b", "--a[0]", "a.b++", "a[0]--",
	"+a", "-a", "!a", "~a", "(a)", "((a))", "(1)", "(this)", "(new T())", "(a,)",
	"(T)a", "(T)+a", "(T)-a", "(T)!a", "(T & U)a", "(@A T)a", "(T[])a", "(a)+b", "(a)*b",
	"new T()", "new T(a){ Object v; }", "new T[1][]", "new int[]{1,2,}", "new <T> C()",
	"a.b", "a[0]", "a.f(x -> x)", "a.<T>f(1)", "T.class", "a.b.T.class", "int.class", "void.class", "T[].class", "int[][].class",
	"T::m", "T::new", "T[]::new", "int[]::new", "a.b.T::new", "T::<U>m", "a::m", "this::m", "super::m",
	"a+b*c", "a<<b", "a>>b", "a>>>b", "a<b", "a>b", "a<=b", "a>=b", "a==b", "a!=b", "a&b^c|d", "a&&b||c", "T<X>>x",
	"a instanceof T", "a instanceof T t", "a?b:c", "a?b:c?d:e", "a=b=c", "a.b=c", "a[0]=c", "a+=b", "a.b+=c", "a[0]+=c",
	"switch(a){case 1 -> 2; default -> 3;}", "switch(a){case 1 -> {yield 2;} default -> 3;}",
}

var predictionStatements = []string{
	"{}", ";", "assert x;", "assert x:y;", "if(x) f();", "if(x) if(y) f(); else g();", "if(x){} else if(y){} else {}",
	"for(;;){}", "for(int i=0;i<10;i++) f(i);", "for(T x:xs) f(x);", "while(x) f();", "do {} while(x);",
	"try {} catch(E e){}", "try {} finally {}", "try(R r=f()){}", "try(r; s;){}", "synchronized(x){}",
	"return;", "return f(x -> x);", "throw new E();", "break;", "break label;", "continue;", "continue label;", "yield x;",
	"f();", "a=1;", "a+=f();", "a++;", "a[0]=1;", "label: while(x) break label;",
	"int a;", "int a[]=f();", "int[] a,b;", "T t=new T();", "T<X> x=f();", "T<X<Y>> x;", "a.b.T<X> x;",
	"T<? extends X> x;", "T<? super X[]> x;", "@A T t;", "final T t=f();", "var t=f();", "record=1;", "yield();", "var();", "enum();",
	"class L {}", "final class L {}", "interface L {}", "record R(T x){}",
	"switch(x){case 1: f(); break; default: g();}", "switch(x){case 1 -> f(); default -> g();}",
}

func TestPredictionGrammarEquivalence(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range predictionExpressions {
		t.Run(fmt.Sprintf("expression_%03d", i), func(t *testing.T) { checkPrediction(t, methodSource("return "+expr+";"), reference, true) })
	}
	for i, stmt := range predictionStatements {
		t.Run(fmt.Sprintf("statement_%03d", i), func(t *testing.T) { checkPrediction(t, methodSource(stmt), reference, true) })
	}
	for _, header := range []string{
		"int a;", "int a[],b=1;", "T<X<Y>> t;", "a.b.T<? extends X[]>[] t[];", "void m(){}", "T m(){}", "T<X> m(T t) throws E {}",
		"C(){}", "<T> C(T x){}", "<T> T m(T x){}", "class N {}", "interface N {}", "@interface A { int x(); }", "enum E { A, B; }", "record R(T x){}",
		"enum x;", "record x;", "@A T t;", "T @A [] t;", "public static T[] m(T... t)[] {}",
	} {
		t.Run(header, func(t *testing.T) { checkPrediction(t, "class C {"+header+"}", reference, true) })
	}
	for _, op := range []string{"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>="} {
		for _, lhs := range []string{"a", "a.b", "a[0]"} {
			checkPrediction(t, methodSource("return "+lhs+op+"f(x -> x);"), reference, true)
		}
	}
}

func TestPredictionAdversarialMatrix(t *testing.T) {
	reference := newPredictionAutomata()
	for i, expr := range predictionExpressions {
		for j, pattern := range []string{"return %s;", "f(%s);", "Object x=%s;", "if(%s) f();", "return (%s);", "return a?%s:b;"} {
			t.Run(fmt.Sprintf("%03d_%d", i, j), func(t *testing.T) { checkPrediction(t, methodSource(fmt.Sprintf(pattern, expr)), reference, true) })
		}
	}
	for _, trivia := range []string{" ", "\n", "\r\n", "/* gap */", "// gap\n"} {
		for _, source := range []string{"T<X<Y>> x=f(a);", "int[] x[]={1,2};", "f((x,y)->x+y);", "a+=f(x->x);", "return T::new;"} {
			lex := NewJavaLexer(antlr.NewInputStream(source))
			lex.RemoveErrorListeners()
			tokens := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
			tokens.Fill()
			var b strings.Builder
			for _, token := range tokens.GetAllTokens() {
				if token.GetTokenType() != antlr.TokenEOF {
					b.WriteString(token.GetText())
					b.WriteString(trivia)
				}
			}
			checkPrediction(t, methodSource(b.String()), reference, true)
		}
	}
}

func TestPredictionBoundaries(t *testing.T) {
	reference := newPredictionAutomata()
	for _, depth := range []int{1, 8, 32, 64, 128} {
		for i, expr := range []string{
			strings.Repeat("f(", depth) + "x" + strings.Repeat(")", depth),
			strings.Repeat("f(x -> ", depth) + "x" + strings.Repeat(")", depth),
			strings.Repeat("(T)", depth) + "x",
		} {
			t.Run(fmt.Sprintf("depth_%d_%d", depth, i), func(t *testing.T) { checkPrediction(t, methodSource("return "+expr+";"), reference, true) })
		}
	}
	for _, n := range []int{1, 64, 512, 513, 1024} {
		t.Run(fmt.Sprintf("generic_%d", n), func(t *testing.T) {
			checkPrediction(t, "class C {"+strings.Repeat("T<", n)+"X"+strings.Repeat(">", n)+" x;}", reference, true)
		})
	}
	for _, n := range []int{1, 100, 4096} {
		checkPrediction(t, methodSource("f("+strings.Repeat("x,", n)+"y);"), reference, true)
	}
}

func TestPredictionInvalidInputs(t *testing.T) {
	for _, body := range []string{"int x=;", "f(;", "return (x;", "return new ;", "return a+;", "T<> x;", "T<X x;", "int x[1];", "if(x", "try {} catch(", "return (x,)->;", "return \"unterminated;", "return 1; /* unterminated"} {
		for _, source := range []string{methodSource(body), methodSource("int before=1;" + body), "class C { void m(){" + body} {
			t.Run(fmt.Sprintf("%d_%s", len(source), body), func(t *testing.T) {
				checkPrediction(t, source, nil, false)
				if parsePrediction(source, false, nil).err == nil {
					t.Fatalf("invalid fixture unexpectedly accepted: %q", source)
				}
			})
		}
	}
}

func TestPredictionRealJavaCorpus(t *testing.T) {
	// WalkDir includes nested project fixtures unlike a single-level glob.
	var files []string
	err := filepath.WalkDir("../tests/code", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".java") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("missing corpus: %v", err)
	}
	reference := newPredictionAutomata()
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			checkPrediction(t, string(source), reference, false)
		})
	}
}

func TestPredictionConcurrentParsers(t *testing.T) {
	source := methodSource("T<X<Y>> x=f(a -> f(a)); return x;")
	want := predictionSnapshot(parsePrediction(source, false, nil).tree)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				got := parsePrediction(source, true, nil)
				if got.err != nil || predictionSnapshot(got.tree) != want {
					t.Errorf("concurrent parser changed AST: %v", got.err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestPredictionLLModes(t *testing.T) {
	for _, mode := range []int{antlr.PredictionModeLL, antlr.PredictionModeLLExactAmbigDetection} {
		for _, source := range []string{methodSource("return f((x,y)->x+y);"), "class C { T<X>.Y<Z>[] x; }", methodSource("return T[]::new;"), methodSource("if(x) if(y) f(); else g();")} {
			var snapshots []string
			for _, fast := range []bool{false, true} {
				lexer := NewJavaLexer(antlr.NewInputStream(source))
				lexer.RemoveErrorListeners()
				p := NewJavaParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
				p.RemoveErrorListeners()
				newPredictionAutomata().apply(p)
				p.SetFastPrediction(fast)
				p.GetInterpreter().SetPredictionMode(mode)
				snapshots = append(snapshots, predictionSnapshot(p.CompilationUnit()))
			}
			if snapshots[0] != snapshots[1] {
				t.Fatalf("fast prediction affected LL mode %d for %q", mode, source)
			}
		}
	}
}

func TestPredictionStandalonePrimaryDiagnostics(t *testing.T) {
	var diagnostics []string
	for _, fast := range []bool{false, true} {
		errors := antlr4util.NewErrorListener()
		lexer := NewJavaLexer(antlr.NewInputStream("?"))
		lexer.RemoveErrorListeners()
		lexer.AddErrorListener(errors)
		p := NewJavaParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
		p.RemoveErrorListeners()
		p.AddErrorListener(errors)
		p.SetFastPrediction(fast)
		p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
		p.Primary()
		if errors.Error() == nil {
			t.Fatal("standalone primary accepted invalid input")
		}
		diagnostics = append(diagnostics, errors.Error().Error())
	}
	if diagnostics[0] != diagnostics[1] {
		t.Fatal("standalone primary diagnostics changed")
	}
}
