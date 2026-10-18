package minirehs

import (
	"fmt"
	"strings"
	"testing"
)

var programStorageBlob []byte
var programStorageLimEx *mvsLimEx

// This file also builds against the unchanged parent to freeze a strict A/B.
func BenchmarkProgramStorage(b *testing.B) {
	for _, set := range []string{"GetRequest", "MITM"} {
		b.Run(set, func(b *testing.B) {
			pats := getRequestImagePatterns(b)
			if set == "MITM" {
				pats, _ = compilableMITMPatterns(b)
			}
			db, err := Compile(pats, WithBackend(BackendMVS), WithLogger(silentLogger{}))
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			m := db.(*database).imageMVS()
			b.Run("Blob", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(m.blob)))
				for i := 0; i < b.N; i++ {
					programStorageBlob = buildMVSBlob(m.nfas, m.merged, m.assertNecFactor, m.hasLiterals)
				}
			})
			b.Run("Compile", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					d, err := Compile(pats, WithBackend(BackendMVS), WithLogger(silentLogger{}))
					if err != nil {
						b.Fatal(err)
					}
					d.Close()
				}
			})
		})
	}
}

func BenchmarkProgramMergedDerivation(b *testing.B) {
	for _, p := range []int{64, 65, 512} {
		for _, branch := range []bool{false, true} {
			b.Run(fmt.Sprintf("P%d/Branch%v", p, branch), func(b *testing.B) {
				expr := strings.Repeat(`[ab]`, p)
				if branch {
					expr = strings.Repeat(`[ab]?`, p-1) + `[cd]`
				}
				n := compileExprToNFA(expr)
				if n == nil {
					b.Fatal("NFA missing")
				}
				m := buildMergedNFA([]mergeMember{{idx: 0, nfa: n}})
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					programStorageLimEx = buildLimEx(m)
				}
				b.ReportMetric(float64(programStorageLimEx.excCount), "retained-rows")
			})
		}
	}
}
