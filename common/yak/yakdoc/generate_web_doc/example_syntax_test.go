package main

import (
	"runtime/debug"
	"sort"
	"testing"

	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// TestAllLibsExampleSyntax 对【全部】导出库的真实生成产物，逐个抽取示例代码做 antlr
// 语法/编译检查(进程内、不联网、不执行)。这是文档"内容质量"的强约束：任何库的任何示例
// 语法错误即红，确保文档站(Docusaurus/MDX)不会因示例代码块破损而崩溃，也保证示例对用户
// 可复制即用。新增/修改示例时若引入语法错误，本测试会在 CI(Essential-test)中拦截。
// 关键词: 全库示例语法校验, antlr FormattedAndSyntaxChecking, 文档质量强约束
func TestAllLibsExampleSyntax(t *testing.T) {
	// 规避 vendored ANTLR4 运行时在 GC 标记期偶发的堆损坏(与生成器 main 同因)。
	debug.SetGCPercent(-1)

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	checker := func(code string) error {
		_, err := antlr4yak.New().FormattedAndSyntaxChecking(code)
		return err
	}

	names := make([]string, 0, len(helper.Libs))
	for name := range helper.Libs {
		names = append(names, name)
	}
	sort.Strings(names)

	totalExamples, totalFailed := 0, 0
	for _, name := range names {
		lib := helper.Libs[name]
		md := webdoc.RenderLibMarkdown(lib, "", nil)
		examples := webdoc.ExtractYakExamples(md)
		totalExamples += len(examples)
		for i, code := range examples {
			if err := webdoc.CheckExampleSyntax(code, checker); err != nil {
				totalFailed++
				t.Errorf("lib %q example #%d failed syntax check: %v\n--- example code ---\n%s\n--- end ---", name, i+1, err, code)
			}
		}
	}
	t.Logf("checked %d libs, %d examples, %d failed", len(names), totalExamples, totalFailed)
}

// TestBenchmarkLibsHaveExamples 额外要求标杆库必须包含示例(内容丰满度的下限约束)。
// 关键词: 标杆库示例存在性, yakit/db/servicescan/synscan
func TestBenchmarkLibsHaveExamples(t *testing.T) {
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	for _, name := range []string{"yakit", "db", "servicescan", "synscan"} {
		lib, ok := helper.Libs[name]
		if !ok {
			t.Fatalf("benchmark lib %q not found in document helper", name)
		}
		md := webdoc.RenderLibMarkdown(lib, "", nil)
		if examples := webdoc.ExtractYakExamples(md); len(examples) == 0 {
			t.Errorf("lib %q produced no examples; benchmark libs must contain examples", name)
		}
	}
}
