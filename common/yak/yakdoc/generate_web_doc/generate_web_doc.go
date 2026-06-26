package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// aiOverviewFallback 在未提供 overviews 目录或缺失 ai.md 时，作为 ai 库 MDX 的兜底描述。
const aiOverviewFallback = "AI 模块提供了与多种大语言模型集成的能力，支持 OpenAI、ChatGLM、Moonshot 等主流 AI 服务。通过统一的接口调用不同的 AI 服务，支持对话、函数调用、流式输出等功能。"

// loadOverviews 读取 overviews 目录下的所有 <lib>.md，返回 库名 -> 总览正文 的映射。
// 目录不存在或为空时返回空映射(模块总览为可选增强)。
// 关键词: 模块总览加载, overviews 目录
func loadOverviews(dir string) map[string]string {
	res := map[string]string{}
	if strings.TrimSpace(dir) == "" {
		return res
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warnf("read overviews dir %s failed: %v", dir, err)
		return res
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			log.Warnf("read overview %s failed: %v", e.Name(), err)
			continue
		}
		res[name] = strings.TrimSpace(string(content))
	}
	if len(res) > 0 {
		log.Infof("loaded %d module overview(s) from %s", len(res), dir)
	}
	return res
}

// collapseToSingleLine 把多行文本压成单行(用于 MDX frontmatter 的 description 字段，避免破坏 YAML)。
func collapseToSingleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// 本程序为 web 文档生成器的"薄壳"：负责从引擎取数(EngineToDocumentHelper)、写文件、跑
// 覆盖率与产物不变量校验。所有纯渲染逻辑都在无引擎依赖的 common/yak/yakdoc/webdoc 包中，
// 由该包的单元/边界/不变量测试在 essential-tests 里保证 Markdown 构建健壮。
// 关键词: web 文档生成器薄壳, webdoc 渲染, 产物不变量自检

// CheckDocCodeBlockMatched 校验所有导出注释里的 ``` 围栏成对，不成对则 panic(早失败)。
func CheckDocCodeBlockMatched() {
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	failCount := 0
	checkFunc := func(f *yakdoc.FuncDecl) {
		if len(f.Document) == 0 {
			return
		}
		if count := strings.Count(f.Document, "```"); count%2 != 0 {
			failCount++
			fmt.Printf("%s.%s code block not matched\n", f.LibName, f.MethodName)
		}
	}

	for _, lib := range helper.Libs {
		for _, f := range lib.Functions {
			checkFunc(f)
		}
	}
	for _, f := range helper.Functions {
		checkFunc(f)
	}
	for _, lib := range helper.StructMethods {
		for _, f := range lib.Functions {
			checkFunc(f)
		}
	}

	if failCount > 0 {
		panic("code block check not passed")
	}
}

// GenerateSingleFile 渲染并写出一个库的 .md，写后跑 CheckMarkdownInvariants 做产物自检。
// overview 为模块总览正文(可空)，注入到库标题之后。
func GenerateSingleFile(basepath string, lib *yakdoc.ScriptLib, overview string) {
	// 示例代码不做格式化(保持注释原样)，传 nil。
	md := webdoc.RenderLibMarkdown(lib, overview, nil)
	outPath := path.Join(basepath, lib.Name+".md")
	if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
		log.Errorf("create file error: %v", err)
		return
	}
	// 产物不变量自检：tag 期出文档时的二次兜底，违例打 error log(非阻断)。
	if err := webdoc.CheckMarkdownInvariants(md); err != nil {
		log.Errorf("markdown invariants check failed for lib %s: %v", lib.Name, err)
	}
}

// GenerateSingleFileMDX 渲染并写出一个库的 .mdx(含 Tabs)。MDX 含 JSX，健壮性由文档站构建保证。
func GenerateSingleFileMDX(basepath string, lib *yakdoc.ScriptLib, description string) {
	mdx := webdoc.RenderLibMDX(lib, description, nil)
	outPath := path.Join(basepath, lib.Name+".mdx")
	if err := os.WriteFile(outPath, []byte(mdx), 0o644); err != nil {
		log.Errorf("create file error: %v", err)
	}
}

func main() {
	// 关闭 GC 以规避 vendored ANTLR4 运行时（v4.0.0-20220911224424）的堆损坏 bug：
	// 该运行时偶发在 prediction-context 结构上写出野指针，GC 标记线程扫描堆时会触发
	// "fatal error: found bad pointer in Go heap" 导致生成器随机崩溃。本工具为短生命周期
	// 的一次性 CLI，关闭 GC 可稳定规避崩溃；根治需升级该 vendored ANTLR4 运行时。
	debug.SetGCPercent(-1)

	var (
		strict         bool
		coverageReport string
		overviewsDir   string
	)
	defaultOverviews := filepath.Join(yakdoc.GetProjectPath(), "common", "yak", "yakdoc", "generate_web_doc", "overviews")
	flag.BoolVar(&strict, "strict", false, "exit non-zero if any doc coverage gap is found (local use only, never enable in CI)")
	flag.StringVar(&coverageReport, "coverage-report", "", "write a markdown coverage baseline to this path (must be outside docs/api so it is not synced)")
	flag.StringVar(&overviewsDir, "overviews", defaultOverviews, "directory of per-library module overview markdown files (<lib>.md), injected after the H1")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		return
	}
	basepath := args[0]
	if !utils.IsDir(basepath) {
		if err := os.MkdirAll(basepath, 0o777); err != nil {
			log.Errorf("create dir error: %v", err)
			return
		}
	}

	CheckDocCodeBlockMatched()

	// 模块总览：按库读取 overviews/<lib>.md，渲染时注入到库标题之后。
	overviews := loadOverviews(overviewsDir)

	// 需要生成 MDX 的库(含 Tabs)。其描述优先取自 overviews/<lib>.md，缺失则用兜底文案。
	mdxLibs := map[string]bool{"ai": true}

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	for _, lib := range helper.Libs {
		if mdxLibs[lib.Name] {
			desc := overviews[lib.Name]
			if strings.TrimSpace(desc) == "" {
				desc = aiOverviewFallback
			}
			GenerateSingleFileMDX(basepath, lib, collapseToSingleLine(desc))
		} else {
			GenerateSingleFile(basepath, lib, overviews[lib.Name])
		}
	}

	// 文档覆盖率统计：非阻断，仅打印 warning 协助本地补全；CI 永不因此失败（除非显式 -strict）。
	report := webdoc.CollectDocCoverage(helper.Libs)
	report.LogSummary()
	if coverageReport != "" {
		if err := report.WriteMarkdown(coverageReport); err != nil {
			log.Errorf("write coverage report failed: %v", err)
		} else {
			log.Infof("coverage baseline written to %s", coverageReport)
		}
	}
	if strict && report.WithGap > 0 {
		log.Errorf("strict mode enabled and %d function(s) have documentation gaps", report.WithGap)
		os.Exit(1)
	}
}
