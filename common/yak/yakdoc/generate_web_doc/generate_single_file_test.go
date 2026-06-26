package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
)

// 本文件聚焦"薄壳"生成器的端到端行为：GenerateSingleFile 写出的 .md 必须符合新结构、
// 示例为 14 反引号块、且通过 webdoc.CheckMarkdownInvariants。纯渲染函数的细粒度测试在
// common/yak/yakdoc/webdoc 包内。
// 关键词: 文档生成端到端测试, 新结构, 示例 14 反引号, 产物不变量

// makeLib 构造一个内存 ScriptLib 夹具，便于直接测试 GenerateSingleFile 的输出。
func makeLib(name string, funcs ...*yakdoc.FuncDecl) *yakdoc.ScriptLib {
	m := make(map[string]*yakdoc.FuncDecl, len(funcs))
	for _, f := range funcs {
		m[f.MethodName] = f
	}
	return &yakdoc.ScriptLib{
		Name:      name,
		Functions: m,
		Instances: map[string]*yakdoc.LibInstance{},
	}
}

func genAndRead(t *testing.T, lib *yakdoc.ScriptLib) string {
	t.Helper()
	dir := t.TempDir()
	GenerateSingleFile(dir, lib, "")
	raw, err := os.ReadFile(filepath.Join(dir, lib.Name+".md"))
	if err != nil {
		t.Fatalf("read generated file failed: %v", err)
	}
	out := string(raw)
	// 产物必须通过不变量校验（断锚/破表/裸URL/裸<等都会被拦截）
	if err := webdoc.CheckMarkdownInvariants(out); err != nil {
		t.Fatalf("generated markdown failed invariants: %v\n%s", err, out)
	}
	return out
}

// 关键词: 二次转义回归, 代码块漏表回归, 签名代码块, 参数返回值解释列
func TestGenerateSingleFile_NoDoubleEscapeAndRichTable(t *testing.T) {
	doc := "Foo 演示函数，处理 \"input\" 与 <tag> & more。\n" +
		"\n" +
		"参数:\n" +
		"- a: 输入字符串\n" +
		"- b: 输出通道\n" +
		"\n" +
		"返回值:\n" +
		"- n: 处理数量\n" +
		"- err: 错误信息\n" +
		"\n" +
		"example：\n" +
		"```yak\n" +
		"Foo(\"x\")\n" +
		"```\n"

	fn := &yakdoc.FuncDecl{
		LibName:    "demo",
		MethodName: "Foo",
		Document:   doc,
		Decl:       "Foo(a string, b chan<- int) (n int, err error)",
		Params: []*yakdoc.Field{
			{Name: "a", Type: "string"},
			{Name: "b", Type: "chan<- int"},
		},
		Results: []*yakdoc.Field{
			{Name: "n", Type: "int"},
			{Name: "err", Type: "error"},
		},
	}

	out := genAndRead(t, makeLib("demo", fn))

	// 索引摘要：引号应只转义一次（&#34;），绝不出现二次转义
	if !strings.Contains(out, "&#34;") {
		t.Errorf("expected single-escaped quote (&#34;) in summary, got:\n%s", out)
	}
	for _, bad := range []string{"&amp;#34;", "&amp;lt;", "&amp;gt;", "&amp;amp;"} {
		if strings.Contains(out, bad) {
			t.Errorf("found double-escaped sequence %q in output, double escaping regression:\n%s", bad, out)
		}
	}

	// 代码块不应漏进函数索引表格行
	indexLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[demo.Foo]") {
			indexLine = line
			break
		}
	}
	if indexLine == "" {
		t.Fatalf("index table row for demo.Foo not found:\n%s", out)
	}
	if strings.Contains(indexLine, "```") || strings.Contains(indexLine, "Foo(\"x\")") {
		t.Errorf("code/example leaked into index table row: %q", indexLine)
	}

	// 签名应放进 go 代码块且原样保留（chan<- int 不被 HTML 转义）
	if !strings.Contains(out, "```go\nFoo(a string, b chan<- int) (n int, err error)\n```") {
		t.Errorf("expected signature in go code block, got:\n%s", out)
	}
	if strings.Contains(out, "chan&lt;-") {
		t.Errorf("signature/type was HTML-escaped (chan&lt;-), regression:\n%s", out)
	}
	if !strings.Contains(out, "`chan<- int`") {
		t.Errorf("expected raw param type `chan<- int`, got:\n%s", out)
	}

	// 参数/返回值解释列应被填充
	for _, want := range []string{"输入字符串", "输出通道", "处理数量", "错误信息"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected explanation %q filled into table, got:\n%s", want, out)
		}
	}

	// 新结构标志
	for _, want := range []string{"# demo {#library-demo}", "## 函数索引", "## 函数详情", "### Foo {#foo}", "**参数**", "**返回值**"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected new-structure marker %q, got:\n%s", want, out)
		}
	}
}

// 空文档应给描述加占位，避免标题下空白
func TestGenerateSingleFile_EmptyDocPlaceholder(t *testing.T) {
	fn := &yakdoc.FuncDecl{
		LibName:    "demo",
		MethodName: "Bar",
		Document:   "",
		Decl:       "Bar() error",
		Results:    []*yakdoc.Field{{Name: "r1", Type: "error"}},
	}
	out := genAndRead(t, makeLib("demo", fn))
	if !strings.Contains(out, "暂无描述") {
		t.Errorf("expected placeholder for empty document, got:\n%s", out)
	}
}

// fence14 是 MANUAL_EXAMPLE_SPEC §2 的 14 反引号围栏，测试里独立定义一份避免依赖实现常量。
const fence14 = "``````````````"

// extractFence14Blocks 复刻 verify-manual-examples.py 抽取 14 反引号 yak 块的逻辑。
func extractFence14Blocks(s string) []string {
	var blocks []string
	var cur []string
	inBlock := false
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if !inBlock {
			if trimmed == fence14+"yak" {
				inBlock = true
				cur = nil
			}
			continue
		}
		if trimmed == fence14 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			inBlock = false
			continue
		}
		cur = append(cur, trimmed)
	}
	return blocks
}

// GenerateSingleFile 产出的 .md 示例为 14 反引号块且原样保留代码
func TestGenerateSingleFile_ExampleFence14(t *testing.T) {
	doc := "Demo func\n" +
		"example:\n" +
		"\tassert codec.EncodeBase64(\"a\") == \"YQ==\"\n"
	fn := &yakdoc.FuncDecl{
		LibName:    "demo",
		MethodName: "Enc",
		Document:   doc,
		Decl:       "Enc(s string) string",
		Params:     []*yakdoc.Field{{Name: "s", Type: "string"}},
		Results:    []*yakdoc.Field{{Name: "r1", Type: "string"}},
	}
	out := genAndRead(t, makeLib("demo", fn))
	blocks := extractFence14Blocks(out)
	found := false
	for _, b := range blocks {
		if strings.Contains(b, "assert codec.EncodeBase64(\"a\") == \"YQ==\"") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 14-backtick yak block with raw example in generated md, got:\n%s", out)
	}
}

// collectDocCoverage（迁移至 webdoc.CollectDocCoverage）对夹具返回正确缺口集合
func TestCollectDocCoverage(t *testing.T) {
	full := &yakdoc.FuncDecl{
		LibName:    "demo",
		MethodName: "Full",
		Document: "完整描述\n" +
			"参数:\n- a: 输入\n" +
			"返回值:\n- r: 结果\n" +
			"example:\n\tassert true\n",
		Decl:    "Full(a string) bool",
		Params:  []*yakdoc.Field{{Name: "a", Type: "string"}},
		Results: []*yakdoc.Field{{Name: "r", Type: "bool"}},
	}
	gappy := &yakdoc.FuncDecl{
		LibName:    "demo",
		MethodName: "Gappy",
		Document:   "",
		Decl:       "Gappy(x int) (int, error)",
		Params:     []*yakdoc.Field{{Name: "x", Type: "int"}},
		Results:    []*yakdoc.Field{{Name: "n", Type: "int"}, {Name: "err", Type: "error"}},
	}

	libs := map[string]*yakdoc.ScriptLib{
		"demo": makeLib("demo", full, gappy),
	}
	report := webdoc.CollectDocCoverage(libs)

	if report.Total != 2 {
		t.Errorf("expected total 2, got %d", report.Total)
	}
	if report.WithGap != 1 {
		t.Errorf("expected 1 function with gaps, got %d", report.WithGap)
	}
	if len(report.Gaps) != 1 {
		t.Fatalf("expected 1 gap entry, got %d", len(report.Gaps))
	}
	g := report.Gaps[0]
	if g.Method != "Gappy" {
		t.Errorf("expected gap on Gappy, got %s", g.Method)
	}
	if !g.MissingDesc || !g.MissingExample {
		t.Errorf("expected MissingDesc and MissingExample for Gappy")
	}
	if len(g.ParamsNoExpl) != 1 || g.ParamsNoExpl[0] != "x" {
		t.Errorf("expected param x missing explanation, got %v", g.ParamsNoExpl)
	}
	if g.ResultsNoExpl != 2 {
		t.Errorf("expected 2 returns missing explanation, got %d", g.ResultsNoExpl)
	}
}
