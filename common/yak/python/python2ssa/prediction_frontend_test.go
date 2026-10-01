package python2ssa

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
)

func TestFrontendPredictionRejectsInvalidSource(t *testing.T) {
	for _, source := range []string{"x=$1\n", "x=1$\n", "if x:\n    y=$1\n", "x=; y=$2\n", "x='unterminated\n", "x=\x00\n", "x=\"\"\"unfinished", "if x:\n    f(\n", "match x:\n    case:\n        pass\n", "x=1\ny=\n"} {
		t.Run(fmt.Sprintf("%q", source), func(t *testing.T) {
			builder := CreateBuilder().(*SSABuilder)
			defer builder.Clearup()
			for _, cached := range []bool{false, true} {
				var err error
				if cached {
					_, err = FrontendWithCache(source, builder.GetAntlrCache())
				} else {
					_, err = Frontend(source)
				}
				if err == nil {
					t.Fatal("invalid source accepted")
				}
				if _, err = FrontendWithCache("def f(x:T):\n    return x\n", builder.GetAntlrCache()); err != nil {
					t.Fatalf("state leaked: %v", err)
				}
			}
		})
	}
	// Errors during the first SLL pass AND Fill before LL must survive.
	for _, source := range []string{"x=$1\n", "x=; y=$2\n", "if x:\n    y=$1\n"} {
		_, err := Frontend(source)
		if err == nil || !strings.Contains(err.Error(), "token recognition error") {
			t.Fatalf("lost lexical diagnostic: %v", err)
		}
	}
}

func TestFrontendPredictionLLOnly(t *testing.T) {
	if os.Getenv("YAK_PYTHON_TEST_LL_CHILD") == "1" {
		TestFrontendPredictionRejectsInvalidSource(t)
		return
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestFrontendPredictionLLOnly$", "-test.timeout=1m")
	cmd.Env = append(os.Environ(), "YAK_ANTLR_SLL_FIRST=0", "YAK_PYTHON_TEST_LL_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("LL-only regression: %v\n%s", err, output)
	}
}

func frontendSnapshot(tree antlr.Tree) string {
	var out strings.Builder
	stack := []antlr.Tree{tree}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch n := node.(type) {
		case antlr.ParserRuleContext:
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d\n", n, n.GetRuleIndex(), n.GetChildCount(), n.GetStart().GetTokenIndex(), n.GetStop().GetTokenIndex())
		case antlr.TerminalNode:
			v := n.GetSymbol()
			fmt.Fprintf(&out, "%T:%d:%d:%d:%d:%d:%d:%q\n", n, v.GetTokenType(), v.GetTokenIndex(), v.GetStart(), v.GetStop(), v.GetLine(), v.GetColumn(), v.GetText())
		}
		children := node.GetChildren()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return out.String()
}

func TestFrontendPredictionASTEquivalence(t *testing.T) {
	for _, source := range []string{
		"x=f(lambda x:f(x))[1:2:3]\n", "if x:\n    if y:\n        f()\n    else:\n        g()\n",
		"match=type(x)\nmatch x:\n    case 1 | 2 if y:\n        pass\n", "type Alias=list[int]\n",
		"@a.b(x)\nasync def f[T](x:T=1,/,*args,**kw)->T:\n    return await g(x)\n", "print >>f,x,\n",
		"x=[y for x in xs if x for y in x if y]\n", "class C:\n    def f(self):\n        return [1,2]\n",
	} {
		t.Run(source, func(t *testing.T) {
			want, err := antlr4util.ParseASTWithSLLFirst(source, pythonparser.NewPythonLexer, func(input antlr.TokenStream) *pythonparser.PythonParser {
				p := pythonparser.NewPythonParser(input)
				p.SetFastPrediction(false)
				return p
			}, nil, nil, func(p *pythonparser.PythonParser) pythonparser.IRootContext { return p.Root() })
			if err != nil {
				t.Fatal(err)
			}
			builder := CreateBuilder().(*SSABuilder)
			defer builder.Clearup()
			for _, cached := range []bool{false, true} {
				var got pythonparser.IRootContext
				if cached {
					got, err = FrontendWithCache(source, builder.GetAntlrCache())
				} else {
					got, err = Frontend(source)
				}
				if err != nil {
					t.Fatal(err)
				}
				if frontendSnapshot(got) != frontendSnapshot(want) {
					t.Fatal("frontend changed exact AST")
				}
			}
		})
	}
}

func TestFrontendPredictionConcurrentCaches(t *testing.T) {
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			builder := CreateBuilder().(*SSABuilder)
			defer builder.Clearup()
			for i := 0; i < 10; i++ {
				if _, err := FrontendWithCache("async def f[T](x:T):\n    return await g(lambda:x)\n", builder.GetAntlrCache()); err != nil {
					t.Errorf("concurrent frontend: %v", err)
					return
				}
				if _, err := FrontendWithCache("x=$1\n", builder.GetAntlrCache()); err == nil {
					t.Error("invalid source accepted")
					return
				}
			}
		}()
	}
	wg.Wait()
}
