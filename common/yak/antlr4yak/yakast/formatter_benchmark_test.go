package yakast

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakfmt"
)

var formatterResult string

// Legacy measures the previous public path: compile plus the visitor's original
// formatting buffer. GetFormattedCode now uses the independent printer lazily,
// so reading that method here would unfairly run BOTH formatters for the baseline.
func BenchmarkYakFormatter(b *testing.B) {
	sources := map[string]string{
		"small":         "f = func(a) { if (a > 0) { return a + 1 }; return 0 }\n",
		"flat_64KiB":    strings.Repeat("a = 1 + 2 * 3\n", 4682),
		"blocks_54KiB":  strings.Repeat("f = func(a) { if (a > 0) { return a + 1 }; return 0 }\n", 1000),
		"list_10k":      "a = [" + strings.Repeat("1,", 10000) + "0]\n",
		"chain_10k":     "a = 1" + strings.Repeat(" + 1", 10000) + "\n",
		"select_1k":     "select {" + strings.Repeat("case ch <- 1: a = 1;", 1000) + "default: a = 0 }\n",
		"unicode_64KiB": strings.Repeat("a = f\"你好 ${1 + 2}\"\n", 2700),
	}
	for _, size := range []int{1024, 8192, 65536, 524288} {
		sources[fmt.Sprintf("flat_%dB", size)] = strings.Repeat("a = 1 + 2 * 3\n", size/14)
	}
	for name, source := range sources {
		b.Run(name, func(b *testing.B) {
			b.Run("Independent", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for i := 0; i < b.N; i++ {
					var err error
					formatterResult, err = yakfmt.Format(source)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Legacy", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for i := 0; i < b.N; i++ {
					c := NewYakCompiler()
					if !c.Compiler(source) {
						b.Fatal(c.GetErrors())
					}
					formatterResult = strings.TrimSpace(c.formatted.String())
				}
			})
			b.Run("LayoutOnly", func(b *testing.B) {
				lex := parser.NewYaklangLexer(antlr.NewInputStream(source))
				ts := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
				c := NewYakCompiler()
				_, tree := c.parseProgramTwoStage(ts)
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					formatterResult = yakfmt.FormatTree(source, tree, ts)
				}
			})
		})
	}
}
