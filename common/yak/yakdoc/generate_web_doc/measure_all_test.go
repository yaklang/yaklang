package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func funcHasExample(doc string) bool {
	return strings.Contains(doc, "<|EXAMPLE_START|>") ||
		strings.Contains(doc, "Example:") || strings.Contains(doc, "Example：") ||
		strings.Contains(doc, "example:") || strings.Contains(doc, "示例:") || strings.Contains(doc, "示例：")
}

// TestMeasureCoverage 度量全库文档完善度：overview 覆盖、函数数、有示例函数数、示例数、
// 示例语法失败数。输出报告与明细文件，用于规划"完善所有文档"。
// 关键词: 文档完善度度量, overview 覆盖, 示例覆盖, 语法失败
func TestMeasureCoverage(t *testing.T) {
	debug.SetGCPercent(-1)
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	checker := func(code string) error {
		_, err := antlr4yak.New().FormattedAndSyntaxChecking(code)
		return err
	}

	overviewDir := "overviews"
	hasOverview := func(name string) bool {
		_, err := os.Stat(filepath.Join(overviewDir, name+".md"))
		return err == nil
	}

	names := make([]string, 0, len(helper.Libs))
	for name := range helper.Libs {
		names = append(names, name)
	}
	sort.Strings(names)

	var noOverview []string
	var funcNoExample strings.Builder
	var libFuncs strings.Builder
	totFuncs, totFuncsWithEx, totExamples, totSyntaxFail, libsNoOverview := 0, 0, 0, 0, 0

	for _, name := range names {
		lib := helper.Libs[name]
		ov := hasOverview(name)
		if !ov {
			noOverview = append(noOverview, name)
			libsNoOverview++
		}
		funcs := 0
		funcsWithEx := 0
		fnames := make([]string, 0, len(lib.Functions))
		for fn := range lib.Functions {
			fnames = append(fnames, fn)
		}
		sort.Strings(fnames)
		libFuncs.WriteString(fmt.Sprintf("\n== %s (%d funcs, overview=%v) ==\n", name, len(fnames), ov))
		for _, fn := range fnames {
			f := lib.Functions[fn]
			funcs++
			libFuncs.WriteString(fmt.Sprintf("%s\n", strings.TrimSpace(f.Decl)))
			if funcHasExample(f.Document) {
				funcsWithEx++
			} else {
				funcNoExample.WriteString(fmt.Sprintf("%s.%s\n", name, f.MethodName))
			}
		}
		md := webdoc.RenderLibMarkdown(lib, "", nil)
		examples := webdoc.ExtractYakExamples(md)
		fail := 0
		for _, code := range examples {
			if err := webdoc.CheckExampleSyntax(code, checker); err != nil {
				fail++
			}
		}
		totFuncs += funcs
		totFuncsWithEx += funcsWithEx
		totExamples += len(examples)
		totSyntaxFail += fail
		_ = yakdoc.FuncDecl{}
	}

	fmt.Printf("\n=== DOC COVERAGE ===\n")
	fmt.Printf("libs=%d  libsNoOverview=%d  funcs=%d  funcsWithExample=%d (%.1f%%)  examples=%d  syntaxFail=%d\n",
		len(names), libsNoOverview, totFuncs, totFuncsWithEx,
		100*float64(totFuncsWithEx)/float64(totFuncs), totExamples, totSyntaxFail)
	fmt.Printf("libs without overview (%d):\n%s\n", len(noOverview), strings.Join(noOverview, " "))
	_ = os.WriteFile("/tmp/doc_no_example.txt", []byte(funcNoExample.String()), 0o644)
	_ = os.WriteFile("/tmp/doc_no_overview.txt", []byte(strings.Join(noOverview, "\n")), 0o644)
	_ = os.WriteFile("/tmp/lib_funcs.txt", []byte(libFuncs.String()), 0o644)
	fmt.Printf("details: /tmp/doc_no_example.txt /tmp/doc_no_overview.txt /tmp/lib_funcs.txt\n")
}
