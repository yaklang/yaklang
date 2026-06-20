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
			md := RenderLibMarkdown(lib, nil)
			if err := CheckMarkdownInvariants(md); err != nil {
				t.Fatalf("invariants failed for lib %s:\n%v\n---rendered---\n%s", lib.Name, err, md)
			}
		})
	}
}

func TestRenderLibMarkdownGrouping(t *testing.T) {
	mixed := mkLib("demo", nil,
		fn("demo", "Do", "Do() error", "do", nil, []*yakdoc.Field{field("", "error")}),
		fn("demo", "WithX", "WithX() DemoOption", "x", nil, []*yakdoc.Field{field("", "DemoOption")}),
	)
	md := RenderLibMarkdown(mixed, nil)
	if !strings.Contains(md, "**主要函数**") || !strings.Contains(md, "**配置选项**") {
		t.Fatalf("mixed lib should show both group labels:\n%s", md)
	}

	coreOnly := mkLib("core", nil,
		fn("core", "Do", "Do() error", "do", nil, []*yakdoc.Field{field("", "error")}),
	)
	md2 := RenderLibMarkdown(coreOnly, nil)
	if strings.Contains(md2, "**配置选项**") {
		t.Fatalf("core-only lib should NOT show option group:\n%s", md2)
	}
}

func TestRenderLibMarkdownStructure(t *testing.T) {
	lib := mkLib("demo", nil,
		fn("demo", "Do", "Do(a int) error", "do a thing\n参数:\n- a: the input\n返回值:\n- e: error",
			[]*yakdoc.Field{field("a", "int")}, []*yakdoc.Field{field("e", "error")}),
	)
	md := RenderLibMarkdown(lib, nil)
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
