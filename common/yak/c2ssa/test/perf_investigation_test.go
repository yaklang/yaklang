package test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	cparser "github.com/yaklang/yaklang/common/yak/antlr4c/parser"
	"github.com/yaklang/yaklang/common/yak/c2ssa"
)

// 关键词: c parser performance, lexer vs parser, AdaptivePredict, PredictionMode SLL
// 纯计时类测试默认跳过，设置 C_PARSER_BENCH=1 开启。
// SLL/LL 正确性守卫(TestC_SLLBailDiagnostic)始终运行。

func cBenchGate(t *testing.T) {
	t.Helper()
	if os.Getenv("C_PARSER_BENCH") == "" {
		t.Skip("perf/benchmark test skipped; set C_PARSER_BENCH=1 to run")
	}
}

func wrapCMain(body string) string {
	return "int main() {\n" + body + "\nreturn 0;\n}\n"
}

func loadCFixtures(t *testing.T) []struct {
	Name string
	Code string
} {
	t.Helper()
	var result []struct {
		Name string
		Code string
	}
	err := fs.WalkDir(codeFs, "code", func(codePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(codePath, ".c") {
			return nil
		}
		raw, err := codeFs.ReadFile(codePath)
		if err != nil {
			return err
		}
		result = append(result, struct {
			Name string
			Code string
		}{Name: codePath, Code: string(raw)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk c fixtures: %v", err)
	}
	sort.Slice(result, func(i, j int) bool {
		return len(result[i].Code) > len(result[j].Code)
	})
	return result
}

func cLexOnly(code string) (time.Duration, int) {
	start := time.Now()
	lexer := cparser.NewCLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	return time.Since(start), len(tokenStream.GetAllTokens())
}

func cParseOnly(code string, predictionMode int) time.Duration {
	lexer := cparser.NewCLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokenStream.Seek(0)

	parser := cparser.NewCParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(predictionMode)

	start := time.Now()
	_ = parser.CompilationUnit()
	return time.Since(start)
}

func cParseTreeString(code string, predictionMode int) string {
	lexer := cparser.NewCLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokenStream.Seek(0)

	parser := cparser.NewCParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(predictionMode)
	ast := parser.CompilationUnit()
	return ast.ToStringTree(parser.GetRuleNames(), parser)
}

func cSLLBailParse(code string) (tree string, bailed bool, listenerErr error) {
	el := antlr4util.NewErrorListener()
	lexer := cparser.NewCLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(el)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := cparser.NewCParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(el)
	parser.SetErrorHandler(antlr.NewBailErrorStrategy())
	parser.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)

	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(*antlr.ParseCancellationException); ok {
					bailed = true
					return
				}
				panic(r)
			}
		}()
		ast := parser.CompilationUnit()
		tree = ast.ToStringTree(parser.GetRuleNames(), parser)
	}()
	return tree, bailed, el.Error()
}

func cFrontend(code string) time.Duration {
	start := time.Now()
	_, _ = c2ssa.Frontend(code, nil)
	return time.Since(start)
}

// TestC_SLLMinimalRepro 找出导致 SLL bail 的最小 C 构造。
func TestC_SLLMinimalRepro(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{"assign", wrapCMain("a = 1;")},
		{"index_assign", wrapCMain("a[0] = 1;")},
		{"member_assign", wrapCMain("a.b = 1;")},
		{"multi_assign", wrapCMain("a = b = 1;")},
		{"inc", wrapCMain("i++;")},
		{"dec", wrapCMain("i--;")},
		{"call", wrapCMain("f();")},
		{"index_expr", wrapCMain("a[0];")},
		{"compound", wrapCMain("a[i] = a[i] + 1;")},
		{"sizeof_expr", wrapCMain("sizeof(a);")},
		{"cast_assign", wrapCMain("(int*)p = q;")},
		{"for_init", wrapCMain("for (i = 0; i < 10; i++) {}")},
	}
	for _, c := range cases {
		_, bailed, lerr := cSLLBailParse(c.code)
		status := "OK "
		if bailed || lerr != nil {
			status = "BAIL"
		}
		t.Logf("[MIN] %-4s %-16s", status, c.name)
	}
}

// TestC_SLLBailDiagnostic 对每个 c fixture，SLL(+Bail) 成功时解析树必须与 LL 完全一致。
func TestC_SLLBailDiagnostic(t *testing.T) {
	fixtures := loadCFixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no c fixtures found")
	}
	var bailedList []string
	for _, f := range fixtures {
		tree, bailed, lerr := cSLLBailParse(f.Code)
		if bailed || lerr != nil {
			bailedList = append(bailedList, f.Name)
			continue
		}
		llTree := cParseTreeString(f.Code, antlr.PredictionModeLL)
		if tree != llTree {
			t.Fatalf("DANGER: SLL succeeded (no error) but tree differs from LL: %s", f.Name)
		}
	}
	t.Logf("total=%d, SLL-bailed(fallback to LL)=%d: %v", len(fixtures), len(bailedList), bailedList)
	if len(bailedList) > 12 {
		t.Fatalf("SLL bail regression: got %d bailed fixtures, baseline is 12", len(bailedList))
	}
}

func TestC_LexerVsParser(t *testing.T) {
	cBenchGate(t)
	fixtures := loadCFixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no c fixtures found")
	}
	topN := 6
	if len(fixtures) < topN {
		topN = len(fixtures)
	}

	fmt.Printf("\n%-40s %8s %8s | %10s | %12s %12s | %12s\n",
		"fixture", "bytes", "tokens", "lex", "parse(LL)", "parse(SLL)", "frontend")
	fmt.Println("--------------------------------------------------------------------------------------------------------------------")

	for _, f := range fixtures[:topN] {
		lexDur, tokenCount := cLexOnly(f.Code)
		parseLL := cParseOnly(f.Code, antlr.PredictionModeLL)
		parseSLL := cParseOnly(f.Code, antlr.PredictionModeSLL)
		frontend := cFrontend(f.Code)

		name := filepath.Base(f.Name)
		if len(name) > 38 {
			name = name[:38]
		}
		fmt.Printf("%-40s %8d %8d | %10s | %12s %12s | %12s\n",
			name, len(f.Code), tokenCount,
			lexDur.Round(time.Microsecond),
			parseLL.Round(time.Microsecond),
			parseSLL.Round(time.Microsecond),
			frontend.Round(time.Microsecond),
		)
	}
	fmt.Println()
}
