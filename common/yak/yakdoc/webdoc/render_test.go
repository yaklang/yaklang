package webdoc

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

// 关键词: webdoc 单元测试, 渲染纯函数测试

func field(name, typ string) *yakdoc.Field {
	return &yakdoc.Field{Name: name, Type: typ}
}

func fn(lib, name, decl, doc string, params, results []*yakdoc.Field) *yakdoc.FuncDecl {
	return &yakdoc.FuncDecl{
		LibName:    lib,
		MethodName: name,
		Decl:       decl,
		Document:   doc,
		Params:     params,
		Results:    results,
	}
}

func mkLib(name string, instances map[string]*yakdoc.LibInstance, funcs ...*yakdoc.FuncDecl) *yakdoc.ScriptLib {
	m := make(map[string]*yakdoc.FuncDecl, len(funcs))
	for _, f := range funcs {
		m[f.MethodName] = f
	}
	if instances == nil {
		instances = map[string]*yakdoc.LibInstance{}
	}
	return &yakdoc.ScriptLib{Name: name, Functions: m, Instances: instances}
}

func TestLeadingProse(t *testing.T) {
	cases := []struct {
		name       string
		doc        string
		wantHas    []string
		wantHasNot []string
	}{
		{
			name:       "cut at params",
			doc:        "do something\nmore detail\n参数:\n- a: foo\n返回值:\n- bar",
			wantHas:    []string{"do something", "more detail"},
			wantHasNot: []string{"参数", "foo", "返回值", "bar"},
		},
		{
			name:       "cut at example",
			doc:        "desc line\nExample:\n```\ncode here\n```",
			wantHas:    []string{"desc line"},
			wantHasNot: []string{"code here", "Example"},
		},
		{
			name:       "cut at returns first",
			doc:        "only desc\n返回值:\n- r1",
			wantHas:    []string{"only desc"},
			wantHasNot: []string{"返回值", "r1"},
		},
		{
			name:       "empty doc",
			doc:        "",
			wantHas:    nil,
			wantHasNot: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := leadingProse(c.doc)
			for _, s := range c.wantHas {
				if !strings.Contains(got, s) {
					t.Fatalf("leadingProse(%q)=%q, want contains %q", c.doc, got, s)
				}
			}
			for _, s := range c.wantHasNot {
				if strings.Contains(got, s) {
					t.Fatalf("leadingProse(%q)=%q, want NOT contains %q", c.doc, got, s)
				}
			}
		})
	}
}

func TestClassifyFunctions(t *testing.T) {
	opt := fn("demo", "WithTimeout", "WithTimeout(i float64) DemoOption", "", nil, []*yakdoc.Field{field("", "DemoOption")})
	core := fn("demo", "Do", "Do() error", "", nil, []*yakdoc.Field{field("", "error")})
	twoResults := fn("demo", "Pair", "Pair() (int, DemoOption)", "", nil, []*yakdoc.Field{field("", "int"), field("", "DemoOption")})
	noResults := fn("demo", "Run", "Run()", "", nil, nil)

	gotCore, gotOpts := classifyFunctions([]*yakdoc.FuncDecl{opt, core, twoResults, noResults})
	if len(gotOpts) != 1 || gotOpts[0].MethodName != "WithTimeout" {
		t.Fatalf("expected only WithTimeout as option, got %v", names(gotOpts))
	}
	if len(gotCore) != 3 {
		t.Fatalf("expected 3 core funcs, got %v", names(gotCore))
	}
}

func names(fs []*yakdoc.FuncDecl) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.MethodName)
	}
	return out
}

func TestNeutralizeBareURLAutolinks(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`see http://127.0.0.1:8080 now`, `see http&#58;//127.0.0.1:8080 now`},
		{`q &#34;https://a.com&#34;`, `q &#34;https&#58;//a.com&#34;`},
		{`[link](http://a.com)`, `[link](http://a.com)`},
		{`no url here`, `no url here`},
		{`mixed [x](https://k) and http://bare`, `mixed [x](https://k) and http&#58;//bare`},
	}
	for _, c := range cases {
		if got := neutralizeBareURLAutolinks(c.in); got != c.want {
			t.Fatalf("neutralize(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestEscapeTableCell(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a|b", `a\|b`},
		{"a<b>&c", "a&lt;b&gt;&amp;c"},
		{"line1\nline2", "line1 line2"},
		{"  trim  ", "trim"},
		{`visit http://x"）`, `visit http&#58;//x&#34;）`},
	}
	for _, c := range cases {
		if got := escapeTableCell(c.in); got != c.want {
			t.Fatalf("escapeTableCell(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestExtractExampleCodeAndDedent(t *testing.T) {
	doc := "summary\nExample:\n    a = 1\n    b = 2\n"
	got := extractExampleCode(doc)
	if got != "a = 1\nb = 2" {
		t.Fatalf("extractExampleCode=%q", got)
	}

	docFenced := "x\nExample:\n```\ncode = 1\n```\n"
	if got := extractExampleCode(docFenced); got != "code = 1" {
		t.Fatalf("extractExampleCode fenced=%q", got)
	}

	if got := extractExampleCode("no example"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCollapseBlankLines(t *testing.T) {
	if got := collapseBlankLines("a\n\n\n\nb"); got != "a\n\nb" {
		t.Fatalf("collapse=%q", got)
	}
	// 围栏内空行保留
	in := "p\n\n```\nx\n\n\ny\n```\n\nq"
	got := collapseBlankLines(in)
	if !strings.Contains(got, "x\n\n\ny") {
		t.Fatalf("fence blank lines should be preserved, got %q", got)
	}
}

func TestAssignAnchorsDedup(t *testing.T) {
	a := fn("demo", "Foo", "", "", nil, nil)
	b := fn("demo", "foo", "", "", nil, nil)
	c := fn("demo", "FOO", "", "", nil, nil)
	m := assignAnchors([]*yakdoc.FuncDecl{a, b, c})
	seen := map[string]bool{}
	for _, f := range []*yakdoc.FuncDecl{a, b, c} {
		id := m[f]
		if id == "" {
			t.Fatalf("missing anchor for %s", f.MethodName)
		}
		if seen[id] {
			t.Fatalf("duplicate anchor id %q", id)
		}
		seen[id] = true
	}
	if m[a] != "foo" {
		t.Fatalf("first should keep base id, got %q", m[a])
	}
}

func TestStripInlineCodeAndColumns(t *testing.T) {
	if got := stripInlineCode("| `a|b` | c |"); strings.Contains(got, "a|b") {
		t.Fatalf("inline code should be stripped, got %q", got)
	}
	if cols := countTableColumns(stripInlineCode("| `a|b` | c |")); cols != 3 {
		t.Fatalf("columns=%d want 3", cols)
	}
	if cols := countTableColumns(`| a \| b | c |`); cols != 3 {
		t.Fatalf("escaped pipe columns=%d want 3", cols)
	}
}

// TestRenderLibMarkdownInvariants 对一批"对抗性"合成库走真实渲染 + 不变量校验，
// 这是 Markdown 构建健壮性的核心保障：任何渲染产物都必须通过 CheckMarkdownInvariants。
func TestRenderLibMarkdownInvariants(t *testing.T) {
	libs := []*yakdoc.ScriptLib{
		// 混合库：核心函数 + 配置选项 + 实例，含裸 URL/全角标点/转义/示例围栏
		mkLib("demo",
			map[string]*yakdoc.LibInstance{
				"Args": {LibName: "demo", InstanceName: "Args", Type: "[]string", ValueStr: `a|b<c>&d`},
			},
			fn("demo", "HTTP", "HTTP(raw string) (*Response, *Request, error)",
				"send http request\nmore prose\n参数:\n- raw: the raw request, e.g. http://127.0.0.1:8080\"）\n返回值:\n- rsp: the response\nExample:\n```\nrsp = demo.HTTP(\"GET / HTTP/1.1\")\n```",
				[]*yakdoc.Field{field("raw", "string")},
				[]*yakdoc.Field{field("rsp", "*Response"), field("req", "*Request"), field("err", "error")}),
			fn("demo", "timeout", "timeout(i float64) DemoOption", "set timeout\n参数:\n- i: seconds",
				[]*yakdoc.Field{field("i", "float64")}, []*yakdoc.Field{field("", "DemoOption")}),
			fn("demo", "proxy", "proxy(p ...string) DemoOption",
				"set proxy 参数: - p: addr like \"http://127.0.0.1:8080\"（多个）",
				[]*yakdoc.Field{field("p", "...string")}, []*yakdoc.Field{field("", "DemoOption")}),
			fn("demo", "Map", "Map(c chan<- int) map[string]int", "build a map<k,v>",
				[]*yakdoc.Field{field("c", "chan<- int")}, []*yakdoc.Field{field("", "map[string]int")}),
		),
		// 纯核心库（不应出现配置选项分组）
		mkLib("core", nil,
			fn("core", "A", "A()", "alpha", nil, nil),
			fn("core", "B", "B() error", "beta", nil, []*yakdoc.Field{field("", "error")}),
		),
		// 纯配置选项库
		mkLib("opts", nil,
			fn("opts", "WithA", "WithA() OptsOption", "a", nil, []*yakdoc.Field{field("", "OptsOption")}),
			fn("opts", "WithB", "WithB() OptsOption", "b", nil, []*yakdoc.Field{field("", "OptsOption")}),
		),
		// 库名 == 函数名（锚点冲突回归：cve/diff/ping）
		mkLib("cve", nil,
			fn("cve", "cve", "cve(q string) error", "query cve", []*yakdoc.Field{field("q", "string")}, []*yakdoc.Field{field("", "error")}),
		),
		// 大小写仅差的方法名（锚点去重）
		mkLib("dup", nil,
			fn("dup", "Parse", "Parse() error", "parse upper", nil, []*yakdoc.Field{field("", "error")}),
			fn("dup", "parse", "parse() error", "parse lower", nil, []*yakdoc.Field{field("", "error")}),
		),
		// 空文档/无参无返回
		mkLib("empty", nil,
			fn("empty", "Nop", "Nop()", "", nil, nil),
		),
		// 仅实例无函数
		mkLib("insonly",
			map[string]*yakdoc.LibInstance{"X": {LibName: "insonly", InstanceName: "X", Type: "int", ValueStr: "1"}},
		),
		// 含三反引号与十四反引号片段的描述
		mkLib("fences", nil,
			fn("fences", "Tricky", "Tricky()", "desc with ```inline``` fence\n参数:\n- none", nil, nil),
		),
	}

	for _, lib := range libs {
		t.Run(lib.Name, func(t *testing.T) {
			md := RenderLibMarkdown(lib, "", nil)
			if err := CheckMarkdownInvariants(md); err != nil {
				t.Fatalf("invariants failed for lib %s:\n%v\n---rendered---\n%s", lib.Name, err, md)
			}
		})
	}
}

// TestRenderLibMarkdownOptionLinkage 校验"选项随主函数渲染"：被消费的选项函数从顶层索引剔除，
// 改在消费它的主函数详情下以"可选参数 / 选项"小节展示。
func TestRenderLibMarkdownOptionLinkage(t *testing.T) {
	// Do 消费 ...DemoOption；WithX 生产 DemoOption，应被识别为选项函数。
	mixed := mkLib("demo", nil,
		fn("demo", "Do", "Do(a int, opts ...DemoOption) error", "do a thing\n参数:\n- a: the input",
			[]*yakdoc.Field{field("a", "int"), field("opts", "...DemoOption")},
			[]*yakdoc.Field{field("", "error")}),
		fn("demo", "WithX", "WithX(v int) DemoOption", "set x value",
			[]*yakdoc.Field{field("v", "int")}, []*yakdoc.Field{field("", "DemoOption")}),
	)
	md := RenderLibMarkdown(mixed, "", nil)

	// WithX 不应作为顶层索引/详情条目出现
	if strings.Contains(md, "[demo.WithX](#withx)") {
		t.Fatalf("option producer WithX should be removed from top-level index:\n%s", md)
	}
	if strings.Contains(md, "### WithX {#withx}") {
		t.Fatalf("option producer WithX should not get a top-level detail heading:\n%s", md)
	}
	// 主函数 Do 详情应出现"必填参数"与"可选参数"，可选参数用"函数索引同款表格"列出选项 WithX
	for _, want := range []string{
		"### Do {#do}", "**必填参数**", "**可选参数**", "...DemoOption",
		"|选项函数|参数|返回值|说明|", // 选项表头与索引同款(含参数/返回值)
		"| `demo.WithX` |",         // 选项函数以行内代码(非链接)展示
		"| `v int` |",             // 选项参数列
		"| `DemoOption` |",        // 选项返回值列
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("consumer detail missing %q:\n%s", want, md)
		}
	}
	// 选项函数无独立锚点，故不应出现成链接形式
	if strings.Contains(md, "[demo.WithX]") {
		t.Fatalf("option function should be inline code, not a link:\n%s", md)
	}
	if err := CheckMarkdownInvariants(md); err != nil {
		t.Fatalf("invariants failed:\n%v\n%s", err, md)
	}

	// 无选项的库：保持单一"参数"小节，无"必填参数/可选参数"
	coreOnly := mkLib("core", nil,
		fn("core", "Do", "Do(a int) error", "do\n参数:\n- a: x", []*yakdoc.Field{field("a", "int")}, []*yakdoc.Field{field("", "error")}),
	)
	md2 := RenderLibMarkdown(coreOnly, "", nil)
	if strings.Contains(md2, "**可选参数 / 选项**") || strings.Contains(md2, "**必填参数**") {
		t.Fatalf("core-only lib should use plain 参数 section:\n%s", md2)
	}
	if !strings.Contains(md2, "**参数**") {
		t.Fatalf("core-only lib should contain 参数 section:\n%s", md2)
	}
}

// TestRenderLibMarkdownPlainVariadicSplit 校验普通可变参数(...T，非选项)也拆分到"可选参数"。
func TestRenderLibMarkdownPlainVariadicSplit(t *testing.T) {
	lib := mkLib("yakit", nil,
		fn("yakit", "Info", "Info(tmp string, items ...any)",
			"输出日志\n参数:\n- tmp: 格式字符串\n- items: 格式化参数",
			[]*yakdoc.Field{field("tmp", "string"), field("items", "...any")}, nil),
	)
	md := RenderLibMarkdown(lib, "", nil)
	if !strings.Contains(md, "**必填参数**") || !strings.Contains(md, "**可选参数**") {
		t.Fatalf("plain variadic should split required/optional:\n%s", md)
	}
	// tmp 在必填，items 在可选；items 的可变类型应原样展示
	reqIdx := strings.Index(md, "**必填参数**")
	optIdx := strings.Index(md, "**可选参数**")
	tmpIdx := strings.Index(md, "| tmp |")
	itemsIdx := strings.Index(md, "| items |")
	if !(reqIdx < tmpIdx && tmpIdx < optIdx && optIdx < itemsIdx) {
		t.Fatalf("tmp must be under 必填参数 and items under 可选参数:\n%s", md)
	}
	if !strings.Contains(md, "`...any`") {
		t.Fatalf("variadic type should be shown:\n%s", md)
	}
	if err := CheckMarkdownInvariants(md); err != nil {
		t.Fatalf("invariants failed:\n%v\n%s", err, md)
	}
}

// TestStripLeadingFuncName 校验描述前导函数名清洗(等名/内部名后缀)，且不误删正文。
func TestStripLeadingFuncName(t *testing.T) {
	cases := []struct{ in, method, want string }{
		{"YakitInfo 向 Yakit 输出日志", "Info", "向 Yakit 输出日志"},
		{"SetKey 写入键值", "SetKey", "写入键值"},
		{"saveHTTPFlowFromRawWithOption 保存流量", "SaveHTTPFlowFromRawWithOption", "保存流量"},
		{"yakitStatusCard 输出卡片", "StatusCard", "输出卡片"},
		// 不应误删：首词是中文
		{"向数据库写入", "SetKey", "向数据库写入"},
		// 不应误删：小写正文词且非导出名后缀(区分大小写)
		{"widget does get things", "Get", "widget does get things"},
		// 多行：只动第一行
		{"Info 第一行\n第二行 Info", "Info", "第一行\n第二行 Info"},
	}
	for _, c := range cases {
		if got := stripLeadingFuncName(c.in, c.method); got != c.want {
			t.Errorf("stripLeadingFuncName(%q,%q)=%q want %q", c.in, c.method, got, c.want)
		}
	}
}

// TestRenderLibMarkdownVariadicIndexSplit 校验函数索引/详情按"是否含可变参数"拆成两块。
func TestRenderLibMarkdownVariadicIndexSplit(t *testing.T) {
	lib := mkLib("demo", nil,
		fn("demo", "Plain", "Plain(a int) error", "plain func",
			[]*yakdoc.Field{field("a", "int")}, []*yakdoc.Field{field("", "error")}),
		fn("demo", "Vari", "Vari(a int, rest ...string) error", "variadic func",
			[]*yakdoc.Field{field("a", "int"), field("rest", "...string")}, []*yakdoc.Field{field("", "error")}),
	)
	md := RenderLibMarkdown(lib, "", nil)
	for _, want := range []string{
		"## 函数索引", "## 可变参数函数索引", "## 函数详情", "## 可变参数函数详情",
		"|函数|参数|返回值|说明|", // 索引表必须含 参数/返回值 两列
		"| `a int` |",            // 参数列内容
		"| `error` |",            // 返回值列内容
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing section %q:\n%s", want, md)
		}
	}
	// Plain 在普通索引、Vari 在可变参数索引：普通索引段应含 Plain、不含 Vari
	regIdx := strings.Index(md, "## 函数索引")
	variIdx := strings.Index(md, "## 可变参数函数索引")
	detIdx := strings.Index(md, "## 函数详情")
	plainLink := strings.Index(md, "[demo.Plain](#plain)")
	variLink := strings.Index(md, "[demo.Vari](#vari)")
	if !(regIdx < plainLink && plainLink < variIdx) {
		t.Fatalf("Plain should be under 函数索引:\n%s", md)
	}
	if !(variIdx < variLink && variLink < detIdx) {
		t.Fatalf("Vari should be under 可变参数函数索引:\n%s", md)
	}
	if err := CheckMarkdownInvariants(md); err != nil {
		t.Fatalf("invariants failed:\n%v\n%s", err, md)
	}

	// 仅普通函数的库不应出现"可变参数"段
	onlyPlain := mkLib("p", nil, fn("p", "A", "A() error", "a", nil, []*yakdoc.Field{field("", "error")}))
	md2 := RenderLibMarkdown(onlyPlain, "", nil)
	if strings.Contains(md2, "可变参数函数索引") {
		t.Fatalf("plain-only lib should not show variadic section:\n%s", md2)
	}
}

// TestRenderLibMarkdownOverview 校验模块总览被注入到 H1 之后、概览统计行之前。
func TestRenderLibMarkdownOverview(t *testing.T) {
	lib := mkLib("demo", nil,
		fn("demo", "Do", "Do() error", "do", nil, []*yakdoc.Field{field("", "error")}),
	)
	md := RenderLibMarkdown(lib, "这是模块总览正文。", nil)
	h1 := strings.Index(md, "# demo {#library-demo}")
	ov := strings.Index(md, "这是模块总览正文。")
	stat := strings.Index(md, "> 共 ")
	if h1 < 0 || ov < 0 || stat < 0 || !(h1 < ov && ov < stat) {
		t.Fatalf("overview should sit between H1 and stats line:\n%s", md)
	}
	if err := CheckMarkdownInvariants(md); err != nil {
		t.Fatalf("invariants failed:\n%v\n%s", err, md)
	}
}

func TestRenderLibMarkdownStructure(t *testing.T) {
	lib := mkLib("demo", nil,
		fn("demo", "Do", "Do(a int) error", "do a thing\n参数:\n- a: the input\n返回值:\n- e: error",
			[]*yakdoc.Field{field("a", "int")}, []*yakdoc.Field{field("e", "error")}),
	)
	md := RenderLibMarkdown(lib, "", nil)
	wantContains := []string{
		"# demo {#library-demo}",
		"## 函数索引",
		"## 函数详情",
		"### Do {#do}",
		"```go\nDo(a int) error\n```",
		"[demo.Do](#do)",
		"**参数**",
		"**返回值**",
		"---",
	}
	for _, s := range wantContains {
		if !strings.Contains(md, s) {
			t.Fatalf("rendered markdown missing %q:\n%s", s, md)
		}
	}
	// 描述去重：参数说明 the input 只应出现在参数表，不应在描述正文重复 dump
	if strings.Count(md, "the input") != 1 {
		t.Fatalf("param explanation should appear exactly once, got %d:\n%s", strings.Count(md, "the input"), md)
	}
}
