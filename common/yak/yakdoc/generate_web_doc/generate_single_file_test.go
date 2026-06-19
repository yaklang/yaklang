package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

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
	GenerateSingleFile(dir, lib)
	raw, err := os.ReadFile(filepath.Join(dir, lib.Name+".md"))
	if err != nil {
		t.Fatalf("read generated file failed: %v", err)
	}
	return string(raw)
}

// 关键词: 文档生成测试, 二次转义回归, 代码块漏表回归, 内联代码转义回归, 参数返回值解释列
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

	// 索引表格摘要：引号应只转义一次（&#34;），绝不出现二次转义（&amp;#34; / &amp;lt;）
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
	if strings.Contains(indexLine, "```") {
		t.Errorf("code fence leaked into index table row: %q", indexLine)
	}
	if strings.Contains(indexLine, "Foo(\"x\")") || strings.Contains(indexLine, "example") {
		t.Errorf("example content leaked into index table row: %q", indexLine)
	}

	// 内联代码（定义/类型）不应被 HTML 转义：chan<- int 必须原样，不能出现 chan&lt;-
	if !strings.Contains(out, "`Foo(a string, b chan<- int) (n int, err error)`") {
		t.Errorf("expected raw inline decl with chan<- int, got:\n%s", out)
	}
	if strings.Contains(out, "chan&lt;-") {
		t.Errorf("inline code was HTML-escaped (chan&lt;-), regression:\n%s", out)
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
}

// 空文档应给详细描述加占位，避免标题下空白
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

// 摘要辅助函数的纯逻辑校验
func TestSummarizeDocument_StripsCodeAndExample(t *testing.T) {
	raw := "hello world\n```yak\ncode()\n```\nexample：\nshould-drop"
	got := summarizeDocument(raw)
	if strings.Contains(got, "code()") {
		t.Errorf("summary should strip fenced code, got %q", got)
	}
	if strings.Contains(got, "should-drop") || strings.Contains(got, "example") {
		t.Errorf("summary should cut at example marker, got %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("summary should keep prose, got %q", got)
	}
}

// renderDetailDoc：示例标记之后的内容（含缩进示例代码）应原样保留，不被 HTML 转义
func TestRenderDetailDoc_KeepsExampleRaw(t *testing.T) {
	doc := "prose <x> & \"y\"\n" +
		"example:\n" +
		"\thttp.Do(\"https://a\", []byte(`<link href=\"/x\">`))\n"
	got := renderDetailDoc(doc)
	// 标记前的 prose 应被转义
	if !strings.Contains(got, "prose &lt;x&gt; &amp; &#34;y&#34;") {
		t.Errorf("prose before example should be escaped, got:\n%s", got)
	}
	// 标记后的示例代码应原样保留
	if !strings.Contains(got, "http.Do(\"https://a\", []byte(`<link href=\"/x\">`))") {
		t.Errorf("example after marker should be kept raw, got:\n%s", got)
	}
	if strings.Contains(got, "&lt;link") || strings.Contains(got, "[]byte(`&#34;") {
		t.Errorf("example code must not be HTML escaped, got:\n%s", got)
	}
}

// escapeProseKeepCode 应保留围栏代码块内容、转义正文
func TestEscapeProseKeepCode(t *testing.T) {
	raw := "prose <x> & y\n```\nkeep <raw> &\n```"
	got := escapeProseKeepCode(raw)
	if !strings.Contains(got, "prose &lt;x&gt; &amp; y") {
		t.Errorf("prose should be HTML escaped, got %q", got)
	}
	if !strings.Contains(got, "keep <raw> &") {
		t.Errorf("fenced code should be kept raw, got %q", got)
	}
}

// fence14 是 MANUAL_EXAMPLE_SPEC §2 的 14 反引号围栏，测试里独立定义一份避免依赖实现常量。
const fence14 = "``````````````"

// extractFence14Blocks 复刻 verify-manual-examples.py 抽取 14 反引号 yak 块的逻辑：
// 开围栏行须严格等于 14 反引号+yak，闭围栏行须严格等于 14 反引号。
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

// T1: extractExampleCode 应去标记、去围栏、去公共缩进
func TestExtractExampleCode(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "fenced",
			doc:  "desc\nexample：\n```yak\nfoo(\"x\")\nbar()\n```\n",
			want: "foo(\"x\")\nbar()",
		},
		{
			name: "indented-no-fence",
			doc:  "desc\nExample:\n\thttp.Do(\"u\", []byte(`<a>`))\n\tprintln(1)\n",
			want: "http.Do(\"u\", []byte(`<a>`))\nprintln(1)",
		},
		{
			name: "no-example",
			doc:  "desc only\n参数:\n- a: x\n",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractExampleCode(c.doc); got != c.want {
				t.Errorf("extractExampleCode mismatch:\nwant: %q\ngot:  %q", c.want, got)
			}
		})
	}
}

// T1: renderDetailDoc 的示例应输出为可被 verify-manual-examples.py 抽取的 14 反引号块
func TestRenderDetailDoc_FenceExtractable(t *testing.T) {
	doc := "prose desc\n" +
		"example:\n" +
		"\tassert 1+1 == 2\n" +
		"\tprintln(\"ok\")\n"
	got := renderDetailDoc(doc)
	blocks := extractFence14Blocks(got)
	if len(blocks) != 1 {
		t.Fatalf("expected exactly 1 fenced block, got %d:\n%s", len(blocks), got)
	}
	if !strings.Contains(blocks[0], "assert 1+1 == 2") || !strings.Contains(blocks[0], "println(\"ok\")") {
		t.Errorf("fenced block lost example content: %q", blocks[0])
	}
}

// T1: GenerateSingleFile 产出的 .md 示例为 14 反引号块且原样保留代码
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

// T2: collectDocCoverage 对夹具返回正确缺口集合
func TestCollectDocCoverage(t *testing.T) {
	// full: 描述/参数解释/返回解释/示例齐全 -> 无缺口
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
	// gappy: 无描述/参数缺解释/返回缺解释/无示例
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
	report := collectDocCoverage(libs)

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
	if !g.MissingDesc {
		t.Errorf("expected MissingDesc=true for Gappy")
	}
	if !g.MissingExample {
		t.Errorf("expected MissingExample=true for Gappy")
	}
	if len(g.ParamsNoExpl) != 1 || g.ParamsNoExpl[0] != "x" {
		t.Errorf("expected param x missing explanation, got %v", g.ParamsNoExpl)
	}
	if g.ResultsNoExpl != 2 {
		t.Errorf("expected 2 returns missing explanation, got %d", g.ResultsNoExpl)
	}
}
