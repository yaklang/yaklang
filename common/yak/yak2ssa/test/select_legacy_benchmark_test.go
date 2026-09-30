package test

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// Only existing syntax, with the API cache disabled on both revisions.
func BenchmarkSelectLegacySSA(b *testing.B) {
	for _, source := range []struct{ name, code string }{
		{"small", `select=3; f=(select)=>select+1; m={"select":f}; println(m.select(select))`},
		{"statements50", "a=0\n" + strings.Repeat("if a>1 {a+=2} else {a+=3}; println(a)\n", 50)},
	} {
		b.Run(source.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := ssaapi.Parse(source.code, ssaapi.WithEnableCache(false)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
