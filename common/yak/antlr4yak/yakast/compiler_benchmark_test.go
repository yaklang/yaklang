package yakast

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

var compilerBenchmarkCodes []*yakvm.Code

// This benchmark never asks for formatted output. Comparing the same benchmark
// before/after cleanup measures the formatter tax on ordinary compilation.
func BenchmarkYakCompiler(b *testing.B) {
	sources := map[string]string{
		"small":          "f=func(a){if(a>0){return a+1};return 0}\n",
		"blocks_54KiB":   strings.Repeat("f = func(a) { if (a > 0) { return a + 1 }; return 0 }\n", 1000),
		"list_10k":       "a=[" + strings.Repeat("1,", 10000) + "0]",
		"chain_10k":      "a=1" + strings.Repeat(" + 1", 10000),
		"select_1k":      "select{" + strings.Repeat("case ch<-1:a=1;", 1000) + "default:a=0}",
		"switch_1k":      "switch a{" + strings.Repeat("case 1:a=1;", 1000) + "default:a=0}",
		"unicode_64KiB":  strings.Repeat("a = f\"你好 ${1 + 2}\"\n", 2700),
		"comments_64KiB": strings.Repeat("// comment with formatting trivia\na=1\n", 1771),
		"heredoc_64KiB":  "a=<<<TAG\n" + strings.Repeat("  raw text {} // ;\n", 3641) + "TAG",
		"template_64KiB": "a=f\"" + strings.Repeat("x", 65536) + "${1+2}\"",
	}
	for _, size := range []int{1024, 8192, 65536, 524288} {
		sources[fmt.Sprintf("flat_%dB", size)] = strings.Repeat("a = 1 + 2 * 3\n", size/14)
	}
	params := make([]string, 1000)
	pairs := make([]string, 1000)
	for i := range params {
		params[i] = fmt.Sprintf("arg%d /* annotation */ map[string][]int", i)
		pairs[i] = fmt.Sprintf("\"key%d\":%d", i, i)
	}
	sources["params_1k"] = "f=func(" + strings.Join(params, ",") + ") (int,error){return 1,nil}"
	sources["map_1k"] = "a={" + strings.Join(pairs, ",") + "}"
	for name, source := range sources {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for i := 0; i < b.N; i++ {
				c := NewYakCompiler()
				if !c.Compiler(source) {
					b.Fatal(c.GetErrors())
				}
				compilerBenchmarkCodes = c.GetOpcodes()
			}
		})
	}
}

// Explicit formatting reparses once; cached reads should allocate nothing.
// This compatibility path is separate from Engine.Format's direct formatter.
func BenchmarkYakCompilerFormatting(b *testing.B) {
	source := strings.Repeat("a=1+2*3\n", 1000)
	b.Run("CompileAndFormat", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(source)))
		for i := 0; i < b.N; i++ {
			c := NewYakCompiler()
			if !c.Compiler(source) {
				b.Fatal(c.GetErrors())
			}
			formatterResult = c.GetFormattedCode()
		}
	})
	b.Run("CachedFormat", func(b *testing.B) {
		c := NewYakCompiler()
		if !c.Compiler(source) {
			b.Fatal(c.GetErrors())
		}
		if c.GetFormattedCode() == "" {
			b.Fatal("missing formatted output")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			formatterResult = c.GetFormattedCode()
		}
	})
}
