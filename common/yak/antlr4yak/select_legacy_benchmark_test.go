package antlr4yak

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
)

// These sources deliberately use only pre-select syntax. Copy this new file to
// the parent revision to run the same benchmark on both sides of the change.
func BenchmarkSelectLegacyFrontend(b *testing.B) {
	sources := []struct{ name, code string }{
		{"small", "select=3; f=(select)=>select+1; m={\"select\":f}; assert m.select(select)==4"},
		{"statements200", "a=0\n" + strings.Repeat("a = a + 1 * 2 - 3 / 4 % 5 && a > 1 || a < 10 ? a.b.c(1,2,3) : a[0]\n", 200)},
	}
	entries, err := os.ReadDir("../../coreplugin/base-yak-plugin")
	if err != nil {
		b.Fatal(err)
	}
	var plugins []struct{ name, code string }
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yak" {
			continue
		}
		code, err := os.ReadFile(filepath.Join("../../coreplugin/base-yak-plugin", entry.Name()))
		if err != nil {
			b.Fatal(err)
		}
		plugins = append(plugins, struct{ name, code string }{entry.Name(), string(code)})
	}
	sort.Slice(plugins, func(i, j int) bool {
		if len(plugins[i].code) == len(plugins[j].code) {
			return plugins[i].name < plugins[j].name
		}
		return len(plugins[i].code) > len(plugins[j].code)
	})
	if len(plugins) < 2 {
		b.Fatal("need two real core-plugin scripts")
	}
	sources = append(sources, plugins[:2]...)
	for _, source := range sources {
		b.Run(source.name, func(b *testing.B) {
			b.Run("parse", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source.code)))
				for i := 0; i < b.N; i++ {
					_, err := antlr4util.ParseASTWithSLLFirst(source.code,
						yak.NewYaklangLexer, yak.NewYaklangParser, nil, nil,
						func(p *yak.YaklangParser) yak.IProgramContext { return p.Program() })
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("compile", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source.code)))
				for i := 0; i < b.N; i++ {
					compiler := yakast.NewYakCompiler()
					if !compiler.Compiler(source.code) {
						b.Fatal(compiler.GetErrors())
					}
				}
			})
		})
	}
}

func BenchmarkSelectLegacyEngineNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		engine := New()
		if engine.GetVM() == nil {
			b.Fatal("missing VM")
		}
	}
}
