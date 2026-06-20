package main

import (
	"runtime/debug"
	"testing"

	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// TestYakitDBExampleSyntax 对标杆库(yakit、db、servicescan、synscan)的真实生成产物，逐个抽取
// 示例代码做 antlr 语法/编译检查(进程内、不联网、不执行)。这是文档"内容质量"的强约束：任何
// 示例语法错误即红。其它库暂不强约束(可后续逐步纳入)，避免一次性卡住全量。
// 关键词: 示例语法校验, antlr FormattedAndSyntaxChecking, yakit/db/servicescan/synscan 标杆
func TestYakitDBExampleSyntax(t *testing.T) {
	// 规避 vendored ANTLR4 运行时在 GC 标记期偶发的堆损坏(与生成器 main 同因)。
	debug.SetGCPercent(-1)

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	checker := func(code string) error {
		_, err := antlr4yak.New().FormattedAndSyntaxChecking(code)
		return err
	}

	for _, name := range []string{"yakit", "db", "servicescan", "synscan"} {
		lib, ok := helper.Libs[name]
		if !ok {
			t.Fatalf("benchmark lib %q not found in document helper", name)
		}
		md := webdoc.RenderLibMarkdown(lib, "", nil)
		examples := webdoc.ExtractYakExamples(md)
		if len(examples) == 0 {
			t.Errorf("lib %q produced no examples; benchmark libs must contain examples", name)
			continue
		}
		for i, code := range examples {
			if err := webdoc.CheckExampleSyntax(code, checker); err != nil {
				t.Errorf("lib %q example #%d failed syntax check: %v\n--- example code ---\n%s\n--- end ---", name, i+1, err, code)
			}
		}
		t.Logf("lib %q: %d example(s) passed syntax check", name, len(examples))
	}
}
