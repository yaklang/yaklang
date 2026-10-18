//go:build cgo

package minirehs

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"sort"
	"testing"
	"unicode/utf8"
)

func programMergedSingletonMode(tb testing.TB, db Database) string {
	tb.Helper()
	mode := os.Getenv("MINIREHS_SINGLETON_MODE")
	if mode == "" {
		mode = "Singleton"
	}
	m := db.(*database).imageMVS()
	if m.kernel == nil {
		tb.Fatal("native singleton qualification requires the actual kernel")
	}
	switch mode {
	case "Reference":
		m.kernel.mergedSingleton = -1
	case "Singleton":
		m.kernel.configureMergedSingleton(m)
	default:
		tb.Fatal("set MINIREHS_SINGLETON_MODE Reference/Singleton")
	}
	return mode
}

// Cut only at boundaries in the original decoded stream: a partial UTF rune
// could create a spurious RuneError. Qualified machines are prefix-monotone.
func TestProgramMergedSingletonCensus(t *testing.T) {
	dir := programEvidenceDir(t)
	pats, _ := compilableMITMPatterns(t)
	db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	idx, ok := singleMergedMember(m)
	if !ok || m.all[idx].id != 28 || m.kernel.mergedSingleton != idx {
		t.Fatal("actual MITM singleton did not qualify")
	}
	re, err := regexp.Compile(buildExprWithFlags(pats[idx]))
	if err != nil {
		t.Fatal(err)
	}
	corpus, _ := loadCorpus(t)
	if len(corpus) != 1332 {
		t.Fatal("full1332 corpus required")
	}
	type row struct {
		Input, SHA256                          string
		Bytes                                  int
		Match                                  bool
		NativeFirstEnd, OracleFirstEnd, Suffix int
	}
	var rows []row
	sc := &scratch{}
	defer sc.Close()
	inputs := programRuntimeInputs()
	for i := 0; i < len(corpus)+4; i++ {
		name := fmt.Sprintf("Corpus%04d", i)
		var data []byte
		if i < len(corpus) {
			data = corpus[i]
		} else {
			name = []string{"HTTP912", "Sparse16K", "DenseHTTP16K", "Zero16K"}[i-len(corpus)]
			data = inputs[name]
		}
		hits, err := m.kernel.mergedScanErr(data, sc)
		if err != nil {
			t.Fatal(err)
		}
		want := re.Match(data)
		if (len(hits) != 0) != want || (want && (len(hits) != 1 || hits[0] != idx)) {
			t.Fatal("complete original-expression oracle disagrees", name, hits, want)
		}
		r := row{Input: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: len(data), Match: want, NativeFirstEnd: -1, OracleFirstEnd: -1}
		if want {
			bounds := []int{0}
			for p := 0; p < len(data); {
				_, size := utf8.DecodeRune(data[p:])
				p += size
				bounds = append(bounds, p)
			}
			native := sort.Search(len(bounds), func(j int) bool {
				h, e := m.kernel.mergedScanErr(data[:bounds[j]], sc)
				if e != nil {
					t.Fatal(e)
				}
				return len(h) != 0
			})
			oracle := sort.Search(len(bounds), func(j int) bool { return re.Match(data[:bounds[j]]) })
			if native != oracle || native == len(bounds) {
				t.Fatal("first acceptance differs", name, native, oracle)
			}
			r.NativeFirstEnd, r.OracleFirstEnd = bounds[native], bounds[oracle]
			r.Suffix = len(data) - r.NativeFirstEnd
		}
		rows = append(rows, r)
	}
	writeProgramJSON(t, dir, "singleton-first-acceptance.json", map[string]interface{}{
		"patterns": len(pats), "index": idx, "id": m.all[idx].id, "positions": m.merged.npos, "words": m.merged.nword,
		"records": rows, "note": "Complete original-expression RE2 vs native merged scalar-presence truth and first acceptance at original UTF boundaries. Suffix is later state steps potentially avoided, not a multiplied system speedup or physical memory-read counter. Later assertion/other readers still finish.",
	})
}

func BenchmarkProgramMergedSingleton(b *testing.B) {
	for _, set := range []string{"GetRequest", "MITM"} {
		b.Run(set, func(b *testing.B) {
			pats := getRequestImagePatterns(b)
			if set == "MITM" {
				pats, _ = compilableMITMPatterns(b)
			}
			db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			programMergedSingletonMode(b, db)
			m := db.(*database).imageMVS()
			corpus, _ := loadCorpus(b)
			if len(corpus) != 1332 {
				b.Fatal("full1332 corpus required")
			}
			for _, name := range []string{"HTTP912", "Sparse16K", "DenseHTTP16K", "Zero16K", "Corpus1332"} {
				b.Run(name, func(b *testing.B) {
					records := [][]byte{programRuntimeInputs()[name]}
					if name == "Corpus1332" {
						records = corpus
					}
					sc := &scratch{}
					defer sc.Close()
					bytes := 0
					for _, data := range records {
						bytes += len(data)
						if _, err := m.kernel.mergedScanErr(data, sc); err != nil {
							b.Fatal(err)
						}
					}
					b.ReportAllocs()
					b.SetBytes(int64(bytes))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						for _, data := range records {
							if _, err := m.kernel.mergedScanErr(data, sc); err != nil {
								b.Fatal(err)
							}
						}
					}
				})
			}
		})
	}
}
