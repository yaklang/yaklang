package antlr4yak

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"

	"github.com/yaklang/antlr/v4"
)

// 关键词: yaklang parser performance, lexer vs parser, AdaptivePredict, PredictionMode SLL
// 本测试用于定位大脚本（几百 K）编译慢的瓶颈：Lexer 还是 Parser。
// 通过分别计时 词法分析 / 语法分析(LL) / 语法分析(SLL) / 完整编译 来对比。
//
// 纯计时类测试仅用于人工基准测量，默认跳过，避免拖慢常规测试。设置 YAK_PARSER_BENCH=1 开启。
// SLL/LL 正确性守卫(TestPerf_SLLBailDiagnostic)始终运行。

func benchGate(t *testing.T) {
	t.Helper()
	if os.Getenv("YAK_PARSER_BENCH") == "" {
		t.Skip("perf/benchmark test skipped; set YAK_PARSER_BENCH=1 to run")
	}
}

// loadLargestCorePlugins 读取 coreplugin 下体积最大的若干 .yak 脚本
func loadLargestCorePlugins(t *testing.T, topN int) []struct {
	Name string
	Code string
} {
	t.Helper()
	root := "../../coreplugin/base-yak-plugin"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("cannot read coreplugin dir: %v", err)
	}
	type item struct {
		Name string
		Code string
		Size int64
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yak" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		items = append(items, item{Name: e.Name(), Code: string(content), Size: info.Size()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Size > items[j].Size })
	var result []struct {
		Name string
		Code string
	}
	for i := 0; i < len(items) && i < topN; i++ {
		result = append(result, struct {
			Name string
			Code string
		}{Name: items[i].Name, Code: items[i].Code})
	}
	return result
}

// lexOnly 只做词法分析，消费全部 token，返回耗时和 token 数
func lexOnly(code string) (time.Duration, int) {
	start := time.Now()
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokens := tokenStream.GetAllTokens()
	return time.Since(start), len(tokens)
}

// parseOnly 先词法分析（不计入），再只做语法分析，返回语法分析耗时
// predictionMode: antlr.PredictionModeLL(默认) 或 antlr.PredictionModeSLL
func parseOnly(code string, predictionMode int) time.Duration {
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokenStream.Seek(0)

	parser := yak.NewYaklangParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(predictionMode)

	start := time.Now()
	_ = parser.Program()
	return time.Since(start)
}

// fullCompile 完整编译（词法+语法+visitor 生成字节码），走真实的 Compiler() 路径（两阶段解析）
func fullCompile(code string) time.Duration {
	start := time.Now()
	_ = compiler(code)
	return time.Since(start)
}

// parseTreeString 用给定预测模式解析并返回解析树的字符串表示，用于对比 SLL / LL 是否产出一致
func parseTreeString(code string, predictionMode int) string {
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokenStream.Fill()
	tokenStream.Seek(0)

	parser := yak.NewYaklangParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(predictionMode)
	ast := parser.Program()
	return ast.ToStringTree(parser.GetRuleNames(), parser)
}

func TestPerf_LexerVsParser(t *testing.T) {
	benchGate(t)
	plugins := loadLargestCorePlugins(t, 6)
	if len(plugins) == 0 {
		t.Skip("no coreplugin scripts found")
	}

	fmt.Printf("\n%-40s %8s %8s | %10s | %12s %12s | %12s\n",
		"script", "bytes", "tokens", "lex", "parse(LL)", "parse(SLL)", "fullCompile")
	fmt.Println("--------------------------------------------------------------------------------------------------------------------")

	for _, p := range plugins {
		lexDur, tokenCount := lexOnly(p.Code)
		parseLL := parseOnly(p.Code, antlr.PredictionModeLL)
		parseSLL := parseOnly(p.Code, antlr.PredictionModeSLL)
		full := fullCompile(p.Code)

		name := p.Name
		if len(name) > 38 {
			name = name[:38]
		}
		fmt.Printf("%-40s %8d %8d | %10s | %12s %12s | %12s\n",
			name, len(p.Code), tokenCount,
			lexDur.Round(time.Microsecond),
			parseLL.Round(time.Microsecond),
			parseSLL.Round(time.Microsecond),
			full.Round(time.Microsecond),
		)
	}
	fmt.Println()
}

// sllBailParse 模拟两阶段解析的阶段一：SLL + BailErrorStrategy。
// 返回解析树字符串、是否 bail(被取消)、是否有监听器错误。
func sllBailParse(code string) (tree string, bailed bool, listenerErr error) {
	el := antlr4util.NewErrorListener()
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(el)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
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
		ast := parser.Program()
		tree = ast.ToStringTree(parser.GetRuleNames(), parser)
	}()
	return tree, bailed, el.Error()
}

// TestPerf_SLLBailDiagnostic 诊断：对每个 coreplugin，SLL(+Bail) 是否会 bail 回退到 LL。
// 若 SLL 未 bail 且无错误，则其解析树必须与 LL 完全一致(两阶段正确性的关键保证)。
func TestPerf_SLLBailDiagnostic(t *testing.T) {
	plugins := loadLargestCorePlugins(t, 100)
	if len(plugins) == 0 {
		t.Skip("no coreplugin scripts found")
	}
	var bailedList []string
	for _, p := range plugins {
		tree, bailed, lerr := sllBailParse(p.Code)
		if bailed || lerr != nil {
			bailedList = append(bailedList, p.Name)
			continue
		}
		// SLL 干净成功：必须与 LL 一致
		llTree := parseTreeString(p.Code, antlr.PredictionModeLL)
		if tree != llTree {
			t.Fatalf("DANGER: SLL succeeded (no error) but tree differs from LL: %s", p.Name)
		}
	}
	t.Logf("total=%d, SLL-bailed(fallback to LL)=%d: %v", len(plugins), len(bailedList), bailedList)
}

// TestPerf_SLLMinimalRepro 找出导致 SLL bail 的最小代码构造
func TestPerf_SLLMinimalRepro(t *testing.T) {
	benchGate(t)
	cases := []string{
		"a = 1\n",
		"a[0] = 1\n",
		"a.b = 1\n",
		"m[k] = true\n",
		"m[k] = v\n",
		"a, b = 1, 2\n",
		"a[0], b = 1, 2\n",
		"a := 1\n",
		"a[0] := 1\n",
		"f()\n",
		"a[0]\n",
		"a[i] = a[i] + 1\n",
		"x = a[0]\n",
	}
	for _, c := range cases {
		_, bailed, lerr := sllBailParse(c)
		status := "OK "
		if bailed || lerr != nil {
			status = "BAIL"
		}
		fmt.Printf("[MIN] %-4s %q\n", status, c)
	}
}

// TestPerf_EndToEndCompile 端到端测量真实 Compiler() 路径(受 YAK_ANTLR_SLL_FIRST 控制)
// 分别用两个进程(env=1 / env=0)运行即可得到修复前后对比
func TestPerf_EndToEndCompile(t *testing.T) {
	benchGate(t)
	plugins := loadLargestCorePlugins(t, 6)
	fmt.Printf("\nYAK_ANTLR_SLL_FIRST=%q\n", os.Getenv("YAK_ANTLR_SLL_FIRST"))
	fmt.Printf("%-40s %8s | %12s\n", "script", "bytes", "compile")
	fmt.Println("--------------------------------------------------------------------")
	for _, p := range plugins {
		// 预热一次(warm DFA)，再计时，模拟进程内稳定态
		_ = compiler(p.Code)
		start := time.Now()
		_ = compiler(p.Code)
		dur := time.Since(start)
		name := p.Name
		if len(name) > 38 {
			name = name[:38]
		}
		fmt.Printf("%-40s %8d | %12s\n", name, len(p.Code), dur.Round(time.Microsecond))
	}
}

// TestPerf_EndToEndAggregate 端到端聚合测量全部 coreplugin 的编译总耗时(受 YAK_ANTLR_SLL_FIRST 控制)
// 用两个进程(env=1 / env=0)对比两阶段解析对真实插件集合的净收益
func TestPerf_EndToEndAggregate(t *testing.T) {
	benchGate(t)
	plugins := loadLargestCorePlugins(t, 1000)
	if len(plugins) == 0 {
		t.Skip("no coreplugin scripts found")
	}
	// 预热所有脚本(warm DFA)
	for _, p := range plugins {
		_ = compiler(p.Code)
	}
	start := time.Now()
	for _, p := range plugins {
		_ = compiler(p.Code)
	}
	total := time.Since(start)
	var bytes int
	for _, p := range plugins {
		bytes += len(p.Code)
	}
	fmt.Printf("\n[AGG] SLL_FIRST=%q scripts=%d total_bytes=%d total_compile=%s\n",
		os.Getenv("YAK_ANTLR_SLL_FIRST"), len(plugins), bytes, total.Round(time.Millisecond))
}

// TestPerf_ScalingSynthetic 通过重复拼接表达式，观察 parser 是否随规模超线性增长
func TestPerf_ScalingSynthetic(t *testing.T) {
	benchGate(t)
	// 构造大量深度嵌套 / 长链式表达式，最容易触发 AdaptivePredict 回溯
	genExprHeavy := func(lines int) string {
		buf := make([]byte, 0, lines*64)
		buf = append(buf, []byte("a = 0\n")...)
		for i := 0; i < lines; i++ {
			// 混合二元运算、成员调用、三元表达式，制造预测压力
			line := "a = a + 1 * 2 - 3 / 4 % 5 && a > 1 || a < 10 ? a.b.c(1,2,3) : a[0]\n"
			buf = append(buf, []byte(line)...)
		}
		return string(buf)
	}

	fmt.Printf("\n%-8s %10s | %10s | %12s %12s | %12s\n",
		"lines", "tokens", "lex", "parse(LL)", "parse(SLL)", "fullCompile")
	fmt.Println("-----------------------------------------------------------------------------------")
	for _, lines := range []int{200, 400, 800, 1600, 3200} {
		code := genExprHeavy(lines)
		lexDur, tokenCount := lexOnly(code)
		parseLL := parseOnly(code, antlr.PredictionModeLL)
		parseSLL := parseOnly(code, antlr.PredictionModeSLL)
		full := fullCompile(code)
		fmt.Printf("%-8d %10d | %10s | %12s %12s | %12s\n",
			lines, tokenCount,
			lexDur.Round(time.Microsecond),
			parseLL.Round(time.Microsecond),
			parseSLL.Round(time.Microsecond),
			full.Round(time.Microsecond),
		)
	}
	fmt.Println()
}
