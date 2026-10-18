//go:build cgo

package minirehs

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Inspect actual serialized units, without asking the producer to rebuild an
// optional table a second time and then mistaking that for its original work.
func programNativeUnit(tb testing.TB, blob []byte, npat, idx int) mvsUnit {
	tb.Helper()
	unit := int(int32(binary.LittleEndian.Uint32(blob[20+idx*4:])))
	if unit < 0 {
		tb.Fatal("no native unit", idx)
	}
	nu := int(binary.LittleEndian.Uint32(blob[16:]))
	off := int(binary.LittleEndian.Uint32(blob[20+npat*4+unit*4:]))
	size := int(binary.LittleEndian.Uint32(blob[20+npat*4+nu*4+unit*4:]))
	u, err := decodeImageUnit(blob[off:off+size], npat)
	if err != nil {
		tb.Fatal(err)
	}
	return u
}

func TestProgramConsumerCensus(t *testing.T) {
	dir := programEvidenceDir(t)
	for _, set := range []string{"GetRequest", "MITM"} {
		patterns := getRequestImagePatterns(t)
		var records [][]byte
		if set == "MITM" {
			patterns, _ = compilableMITMPatterns(t)
			records, _ = loadCorpus(t)
			if len(records) != 1332 {
				t.Fatal("requires all 1332 corpus records")
			}
		}
		for _, data := range programRuntimeInputs() {
			records = append(records, data)
		}
		for _, locate := range []bool{false, true} {
			db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			m := db.(*database).imageMVS()
			type need struct {
				Index                                                             int
				ID                                                                PatternID
				Native, GoReverse, GoLocationFallback, Verifier, OracleDebug, DFA bool
				Positions, Words, DFAStates, DFATableBytes                        int
			}
			needs := make([]need, 0, len(m.nfas))
			dfaIdx := make(map[int]bool)
			var dfaCalls int
			var mu sync.Mutex
			m.kernel.testStatus = func(stage string, idx, status int) int {
				if (stage == "exists" || stage == "exists_many") && dfaIdx[idx] {
					mu.Lock()
					dfaCalls++
					mu.Unlock()
				}
				return status
			}
			for i, n := range m.nfas {
				r := need{Index: i, ID: m.all[i].id, Verifier: true, OracleDebug: true}
				if n != nil {
					u := programNativeUnit(t, m.blob, m.n, i)
					r.Native = true
					r.Positions = n.npos
					r.Words = n.nword
					r.GoLocationFallback = locate
					r.GoReverse = m.revNFAs[i] != nil
					r.DFA = u.hasDFA
					r.DFAStates = int(u.dfaNstates)
					r.DFATableBytes = len(u.dfaNext)*4 + len(u.dfaAccept)
					if u.hasDFA {
						dfaIdx[i] = true
						if m.kernel.dfaPlanKind(i) != 1 {
							t.Fatal("expected canonical optional DFA")
						}
					}
				}
				needs = append(needs, r)
			}
			sc, err := db.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
			for _, data := range records {
				if err := db.Scan(data, sc, nil); err != nil {
					t.Fatal(err)
				}
			}
			sc.Close()
			image, err := db.MarshalImage()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-locate-%v.rehs", set, locate)), image, 0600); err != nil {
				t.Fatal(err)
			}
			writeProgramJSON(t, dir, fmt.Sprintf("%s-locate-%v-consumers.json", set, locate), map[string]interface{}{"set": set, "locate": locate, "units": needs, "patterns": len(patterns), "records": len(records), "optional_dfas": len(dfaIdx), "dfa_exists_calls_in_scans": dfaCalls, "dfa_always_on_plan": m.dfaAlwaysOnIdxs, "blob_bytes": len(m.blob), "image_bytes": len(image), "note": "Native/debug/Go-location fields are potential consumers including fallbacks, not CPU shares. DFA scan call count is diagnostic; standalone DFA-batch and direct native entry remain real optional-table consumers."})
			db.Close()
		}
	}
}

func BenchmarkProgramDFAConstruction(b *testing.B) {
	patterns, _ := compilableMITMPatterns(b)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	for idx, n := range m.nfas {
		if n == nil {
			continue
		}
		u := programNativeUnit(b, m.blob, m.n, idx)
		if !u.hasDFA {
			continue
		}
		b.Run(fmt.Sprintf("MITM_rule%d", m.all[idx].id), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d := buildDFAFromNFA(n)
				if d == nil || d.nstates != int(u.dfaNstates) {
					b.Fatal("DFA shape changed")
				}
			}
			b.ReportMetric(float64(u.dfaNstates), "states/op")
			b.ReportMetric(float64(len(u.dfaNext)*4+len(u.dfaAccept)), "table-B")
		})
	}
}
