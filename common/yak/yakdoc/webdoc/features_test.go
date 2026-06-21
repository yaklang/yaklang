package webdoc

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

// TestExtractExamplesMultiMarker 校验多 example 标记解析(带标题/无标题/多个/未闭合容错)。
func TestExtractExamplesMultiMarker(t *testing.T) {
	doc := strings.Join([]string{
		"some description",
		"参数:",
		"- a: x",
		"<|EXAMPLE_START|> 基础用法",
		"```",
		"a = 1",
		"println(a)",
		"```",
		"<|EXAMPLE_END|>",
		"<|EXAMPLE_START|>",
		"b = 2",
		"<|EXAMPLE_END|>",
	}, "\n")

	got := extractExamples(doc)
	if len(got) != 2 {
		t.Fatalf("want 2 examples, got %d: %+v", len(got), got)
	}
	if got[0].Title != "基础用法" {
		t.Fatalf("first title=%q", got[0].Title)
	}
	if got[0].Code != "a = 1\nprintln(a)" {
		t.Fatalf("first code=%q", got[0].Code)
	}
	if got[1].Title != "" || got[1].Code != "b = 2" {
		t.Fatalf("second example=%+v", got[1])
	}
}

// TestExtractExamplesLegacyFallback 校验无新标记时回退到旧单 Example: 段。
func TestExtractExamplesLegacyFallback(t *testing.T) {
	doc := "desc\nExample:\n```\ncode = 1\n```"
	got := extractExamples(doc)
	if len(got) != 1 || got[0].Title != "" || got[0].Code != "code = 1" {
		t.Fatalf("legacy fallback got %+v", got)
	}
	if got := extractExamples("no example here"); got != nil {
		t.Fatalf("no example should return nil, got %+v", got)
	}
}

// TestExtractExamplesUnterminated 未闭合的 block 也应收尾，不丢示例。
func TestExtractExamplesUnterminated(t *testing.T) {
	doc := "<|EXAMPLE_START|> t\nx = 1"
	got := extractExamples(doc)
	if len(got) != 1 || got[0].Title != "t" || got[0].Code != "x = 1" {
		t.Fatalf("unterminated got %+v", got)
	}
}

// TestRenderExamplesLabels 校验示例标题渲染：单个无标题=示例；多个无标题=示例 N；有标题=示例：T。
func TestRenderExamplesLabels(t *testing.T) {
	single := renderExamples([]DocExample{{Code: "a = 1"}}, nil)
	if !strings.Contains(single, "**示例**") || strings.Contains(single, "示例 1") {
		t.Fatalf("single label wrong:\n%s", single)
	}
	multi := renderExamples([]DocExample{{Code: "a = 1"}, {Code: "b = 2"}}, nil)
	if !strings.Contains(multi, "**示例 1**") || !strings.Contains(multi, "**示例 2**") {
		t.Fatalf("multi labels wrong:\n%s", multi)
	}
	titled := renderExamples([]DocExample{{Title: "进阶", Code: "a = 1"}}, nil)
	if !strings.Contains(titled, "**示例：进阶**") {
		t.Fatalf("titled label wrong:\n%s", titled)
	}
	// 每个示例都应被 14 反引号 yak 围栏包裹
	if strings.Count(multi, exampleFence+"yak") != 2 {
		t.Fatalf("each example needs a yak fence:\n%s", multi)
	}
}

// TestBuildOptionIndex 校验选项关联：仅 ...Option/...Options 且有生产者的才算选项。
func TestBuildOptionIndex(t *testing.T) {
	funcs := []*yakdoc.FuncDecl{
		fn("db", "Save", "Save(u string, o ...yakit.CreateHTTPFlowOptions) error", "save",
			[]*yakdoc.Field{field("u", "string"), field("o", "...yakit.CreateHTTPFlowOptions")},
			[]*yakdoc.Field{field("", "error")}),
		fn("db", "WithTags", "WithTags(t string) yakit.CreateHTTPFlowOptions", "tag option",
			[]*yakdoc.Field{field("t", "string")}, []*yakdoc.Field{field("", "yakit.CreateHTTPFlowOptions")}),
		// ...string 不算选项(后缀不符)
		fn("db", "Plain", "Plain(s ...string)", "plain", []*yakdoc.Field{field("s", "...string")}, nil),
		// 返回值非被消费选项类型，不算生产者
		fn("db", "Other", "Other() SomeOption", "other", nil, []*yakdoc.Field{field("", "SomeOption")}),
	}
	oi := buildOptionIndex(funcs)

	if !oi.isProducer[funcs[1]] {
		t.Fatalf("WithTags should be a producer")
	}
	if oi.isProducer[funcs[3]] {
		t.Fatalf("Other should NOT be a producer (no consumer for SomeOption)")
	}
	// optionTypesOf 返回"基名"(去包限定)，以兼容跨包限定差异
	types := oi.optionTypesOf(funcs[0])
	if len(types) != 1 || types[0] != "CreateHTTPFlowOptions" {
		t.Fatalf("Save option types=%v", types)
	}
	if oi.isOptionParam(funcs[2].Params[0]) {
		t.Fatalf("...string param should not be option param")
	}
	// 选项参数是 Save 的第二个入参 o ...yakit.CreateHTTPFlowOptions：主函数下渲染锚点引用。
	ref := oi.renderOptionTypeRef(funcs[0].Params[1])
	if !strings.Contains(ref, "可作为可变参数") || !strings.Contains(ref, "#option-createhttpflowoptions") {
		t.Fatalf("option ref wrong:\n%s", ref)
	}
	// 页尾统一区按类型聚合，含选项函数表与锚点目标。
	list := oi.renderVariadicOptionList(funcs, map[*yakdoc.FuncDecl]string{funcs[0]: "save"})
	if !strings.Contains(list, "{#option-createhttpflowoptions}") || !strings.Contains(list, "`db.WithTags`") {
		t.Fatalf("variadic option list wrong:\n%s", list)
	}
}

// TestBuildOptionIndexCrossPackageQualifier 校验跨包限定差异下的选项关联：
// 消费侧形参为 ...fp.ConfigOption(带包名)，生产侧函数返回 ConfigOption(裸名)，
// 二者本是同一类型，应按基名关联——生产者被识别并从顶层剔除、列在主函数下。
func TestBuildOptionIndexCrossPackageQualifier(t *testing.T) {
	funcs := []*yakdoc.FuncDecl{
		fn("servicescan", "Scan", "Scan(t string, opts ...fp.ConfigOption) error", "scan",
			[]*yakdoc.Field{field("t", "string"), field("opts", "...fp.ConfigOption")},
			[]*yakdoc.Field{field("", "error")}),
		// 生产者返回裸名 ConfigOption(定义在 fp 包内)
		fn("servicescan", "proxy", "proxy(p ...string) ConfigOption", "set proxy",
			[]*yakdoc.Field{field("p", "...string")}, []*yakdoc.Field{field("", "ConfigOption")}),
	}
	oi := buildOptionIndex(funcs)
	if !oi.isProducer[funcs[1]] {
		t.Fatalf("proxy(ConfigOption) should be a producer for ...fp.ConfigOption consumer")
	}
	list := oi.renderVariadicOptionList(funcs, map[*yakdoc.FuncDecl]string{funcs[0]: "scan"})
	if !strings.Contains(list, "`servicescan.proxy`") {
		t.Fatalf("cross-package option not in consolidated list:\n%s", list)
	}
}

// TestExtractYakExamples 校验从生成的 Markdown 抽取 14 反引号 yak 围栏内的示例代码。
func TestExtractYakExamples(t *testing.T) {
	md := "intro\n" +
		exampleFence + "yak\n" + "a = 1\nprintln(a)\n" + exampleFence + "\n" +
		"middle\n" +
		exampleFence + "yak\n" + "b = 2\n" + exampleFence + "\n"
	got := ExtractYakExamples(md)
	if len(got) != 2 || got[0] != "a = 1\nprintln(a)" || got[1] != "b = 2" {
		t.Fatalf("extracted=%v", got)
	}
}

// TestCheckExampleSyntaxInjection 校验注入式语法检查器被调用。
func TestCheckExampleSyntaxInjection(t *testing.T) {
	called := false
	err := CheckExampleSyntax("a = 1", func(code string) error {
		called = true
		if code != "a = 1" {
			t.Fatalf("checker got %q", code)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("checker should be invoked, err=%v called=%v", err, called)
	}
	if err := CheckExampleSyntax("x", nil); err != nil {
		t.Fatalf("nil checker should be no-op, got %v", err)
	}
}
