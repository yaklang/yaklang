package test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/yaklang/antlr/v4"
	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// 关键词: go parser performance, lexer vs parser, AdaptivePredict, PredictionMode SLL
// 纯计时类测试默认跳过，设置 GO_PARSER_BENCH=1 开启。
// SLL/LL 正确性守卫(TestGo_SLLBailDiagnostic)始终运行。

func goBenchGate(t *testing.T) {
	t.Helper()
	if os.Getenv("GO_PARSER_BENCH") == "" {
		t.Skip("perf/benchmark test skipped; set GO_PARSER_BENCH=1 to run")
	}
}

func wrapGoMain(body string) string {
	return "package main\n\nfunc main() {\n" + body + "}\n"
}

func loadGoFixtures(t *testing.T) []struct {
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
		if d.IsDir() || !isGoSyntaxASTFixture(codePath) {
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
		t.Fatalf("walk go fixtures: %v", err)
	}
	sort.Slice(result, func(i, j int) bool {
		return len(result[i].Code) > len(result[j].Code)
	})
	return result
}

func goLexOnly(code string) (time.Duration, int) {
	start := time.Now()
	lexer := gol.NewGoLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	return time.Since(start), len(tokenStream.GetAllTokens())
}

func goParseOnly(code string, predictionMode int) time.Duration {
	lexer := gol.NewGoLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokenStream.Seek(0)

	parser := gol.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(predictionMode)

	start := time.Now()
	_ = parser.SourceFile()
	return time.Since(start)
}

func goParseTreeString(code string, predictionMode int) string {
	lexer := gol.NewGoLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokenStream.Seek(0)

	parser := gol.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(predictionMode)
	ast := parser.SourceFile()
	return ast.ToStringTree(parser.GetRuleNames(), parser)
}

func goSLLBailParse(code string) (tree string, bailed bool, listenerErr error) {
	el := antlr4util.NewErrorListener()
	lexer := gol.NewGoLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(el)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := gol.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(el)
	parser.SetErrorHandler(antlr4util.NewBailErrorStrategy())
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
		ast := parser.SourceFile()
		tree = ast.ToStringTree(parser.GetRuleNames(), parser)
	}()
	return tree, bailed, el.Error()
}

func goFrontend(code string, cache *ssa.AntlrCache) time.Duration {
	start := time.Now()
	_, _ = go2ssa.Frontend(code, cache)
	return time.Since(start)
}

// TestGo_SLLMinimalRepro 找出导致 SLL bail 的最小 Go 构造。
func TestGo_SLLMinimalRepro(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{"assign", wrapGoMain("a = 1\n")},
		{"index_assign", wrapGoMain("a[0] = 1\n")},
		{"member_assign", wrapGoMain("a.b = 1\n")},
		{"map_assign", wrapGoMain("m[k] = true\n")},
		{"multi_assign", wrapGoMain("a, b = 1, 2\n")},
		{"index_multi_assign", wrapGoMain("a[0], b = 1, 2\n")},
		{"short_var", wrapGoMain("a := 1\n")},
		{"inc", wrapGoMain("i++\n")},
		{"dec", wrapGoMain("i--\n")},
		{"call", wrapGoMain("f()\n")},
		{"index_expr", wrapGoMain("a[0]\n")},
		{"compound", wrapGoMain("a[i] = a[i] + 1\n")},
		{"go", "package main\n\nfunc main() { go f() }\nfunc f() {}\n"},
		{"defer", "package main\n\nfunc main() { defer f() }\nfunc f() {}\n"},
		{"range", wrapGoMain("for k, v := range m { _ = k; _ = v }\n")},
		{"select_recv", "package main\n\nfunc main() { select { case x = <-ch: _ = x } }\n"},
	}
	for _, c := range cases {
		_, bailed, lerr := goSLLBailParse(c.code)
		status := "OK "
		if bailed || lerr != nil {
			status = "BAIL"
		}
		t.Logf("[MIN] %-4s %-16s", status, c.name)
	}
}

// TestGo_SLLBailDiagnostic 对每个 go fixture，SLL(+Bail) 成功时解析树必须与 LL 完全一致。
func TestGo_SLLBailDiagnostic(t *testing.T) {
	fixtures := loadGoFixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no go fixtures found")
	}
	var bailedList []string
	for _, f := range fixtures {
		tree, bailed, lerr := goSLLBailParse(f.Code)
		if bailed || lerr != nil {
			bailedList = append(bailedList, f.Name)
			continue
		}
		llTree := goParseTreeString(f.Code, antlr.PredictionModeLL)
		if tree != llTree {
			t.Fatalf("DANGER: SLL succeeded (no error) but tree differs from LL: %s", f.Name)
		}
	}
	t.Logf("total=%d, SLL-bailed(fallback to LL)=%d: %v", len(fixtures), len(bailedList), bailedList)
	if len(bailedList) > 12 {
		t.Fatalf("SLL bail regression: got %d bailed fixtures, baseline is 12", len(bailedList))
	}
}

func TestGo_LexerVsParser(t *testing.T) {
	goBenchGate(t)
	fixtures := loadGoFixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no go fixtures found")
	}
	topN := 6
	if len(fixtures) < topN {
		topN = len(fixtures)
	}
	cache := goTestAntlrCache

	fmt.Printf("\n%-40s %8s %8s | %10s | %12s %12s | %12s\n",
		"fixture", "bytes", "tokens", "lex", "parse(LL)", "parse(SLL)", "frontend")
	fmt.Println("--------------------------------------------------------------------------------------------------------------------")

	for _, f := range fixtures[:topN] {
		lexDur, tokenCount := goLexOnly(f.Code)
		parseLL := goParseOnly(f.Code, antlr.PredictionModeLL)
		parseSLL := goParseOnly(f.Code, antlr.PredictionModeSLL)
		frontend := goFrontend(f.Code, cache)

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
