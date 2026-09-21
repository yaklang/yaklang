package analyzer

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"testing"
)

func BenchmarkExactIdentity(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 50000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for k := 0; k < b.N; k++ {
				packages := make([]*dxtypes.Package, n)
				for i := range packages {
					packages[i] = &dxtypes.Package{Name: "same-name", Version: fmt.Sprintf("1.0.%d", i)}
				}
				out := MergePackages(packages)
				if len(out) != n {
					b.Fatal("semantic loss", len(out), n)
				}
			}
		})
	}
}
