//go:build cgo

package minirehs

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
)

// Census the tasks the unchanged production loop actually executed, rather
// than counting every NFA call or inventing candidates from the rule set.
func TestProgramRangeTaskCensus(t *testing.T) {
	dir := programEvidenceDir(t)
	corpus, _ := loadCorpus(t)
	if len(corpus) != 1332 {
		t.Fatal("range census requires MINIREHS_FULL_CORPUS=1 and all1332 records")
	}
	type task struct {
		Index, Lo, Hi, Words int
		ID                   PatternID
		Eligible             bool
		Reason               string
	}
	type row struct {
		Set, Input, SHA256               string
		Locate                           bool
		Bytes                            int
		Tasks                            []task
		StatusObservations               map[string]int
		Eligible, WindowBytes            int
		CommonWindowCalls, CommonBatches int
		Gated, Anchored, BiAnchored      int
		Actual                           programObservation
	}
	var rows []row
	for _, set := range []string{"GetRequest", "MITM"} {
		pats := getRequestImagePatterns(t)
		if set == "MITM" {
			pats, _ = compilableMITMPatterns(t)
		}
		for _, locate := range []bool{false, true} {
			db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			m := db.(*database).imageMVS()
			programSmallWorkerMode(t, db)
			if m.kernel == nil {
				t.Fatal("census requires the actual native kernel")
			}
			s, err := db.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
			sc := s.(*scratch)
			var mu sync.Mutex
			var statuses map[string]int
			m.kernel.testStatus = func(stage string, idx, status int) int {
				mu.Lock()
				statuses[stage]++
				mu.Unlock()
				return status
			}
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
				statuses = make(map[string]int)
				r := row{Set: set, Input: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Locate: locate, Bytes: len(data), StatusObservations: statuses}
				err := db.Scan(data, sc, func(match Match) bool {
					r.Actual.Callbacks = append(r.Actual.Callbacks, match)
					return true
				})
				if err != nil {
					t.Fatal(set, name, err)
				}
				r.Actual.Completed = true
				r.Gated, r.Anchored, r.BiAnchored = len(sc.gateTasks), len(sc.anchorBatch), len(sc.biBatch)
				for _, idx32 := range sc.batchIdx {
					idx := int(idx32)
					n := m.nfas[idx]
					x := task{Index: idx, ID: m.all[idx].id, Lo: int(sc.winLo[idx]), Hi: int(sc.winHi[idx])}
					if x.Lo < 0 || x.Hi < x.Lo || x.Hi > len(data) {
						t.Fatal("invalid actual range", x)
					}
					switch {
					case locate:
						x.Reason = "location"
					case n == nil:
						x.Reason = "no-NFA"
					case !m.batchable[idx] || n.hasAssert:
						x.Reason = "assert-or-not-batchable"
					case n.anchoredStart || n.requireEnd:
						x.Reason = "text-anchor"
					case n.nword > 8:
						x.Reason = "wide"
					default:
						x.Eligible = true
						r.Eligible++
						r.WindowBytes += x.Hi - x.Lo
					}
					if n != nil {
						x.Words = n.nword
					}
					r.Tasks = append(r.Tasks, x)
				}
				for j := 0; j < len(r.Tasks); {
					x := r.Tasks[j]
					end := j + 1
					if x.Eligible {
						bytes := x.Hi - x.Lo
						for end < len(r.Tasks) && end-j < 32 && r.Tasks[end].Eligible && r.Tasks[end].Lo == x.Lo && r.Tasks[end].Hi == x.Hi && bytes <= 64<<10-(x.Hi-x.Lo) {
							bytes += x.Hi - x.Lo
							end++
						}
					}
					r.CommonWindowCalls++
					if end-j > 1 {
						r.CommonBatches++
					}
					j = end
				}
				rows = append(rows, r)
			}
			s.Close()
			db.Close()
		}
	}
	writeProgramJSON(t, dir, "range-task-census.json", map[string]interface{}{"records": rows, "note": "Post-full-Scan scratch batchIdx/winLo/winHi reflect actual sequential range tasks. StatusObservations are hook observations per task/status, not Cgo boundary counts; many() emits multiple statuses per one boundary. CommonWindowCalls estimates adjacent equal-window32task/64KiB blocks only; no new production execution enabled. Both full1332 corpus and4 fixed inputs, both reporting modes, both rule sets."})
}

func BenchmarkProgramEmptyNativeBoundary(b *testing.B) {
	db, err := Compile([]Pattern{{ID: 0, Expr: `[ab]`}}, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	k := db.(*database).imageMVS().kernel
	for _, count := range []int{1, 4, 8, 32} {
		for _, many := range []bool{false, true} {
			b.Run(fmt.Sprintf("Tasks%d/Many%v", count, many), func(b *testing.B) {
				idxs := make([]int32, count)
				sc := &scratch{}
				defer sc.Close()
				k.nfaExistsMany(idxs, nil, sc)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if many {
						out := k.nfaExistsMany(idxs, nil, sc)
						if len(out) != count {
							b.Fatal("incomplete batch")
						}
					} else {
						for j := 0; j < count; j++ {
							if _, err := k.nfaExistsErr(0, nil); err != nil {
								b.Fatal(err)
							}
						}
					}
				}
				b.ReportMetric(float64(count), "tasks/op")
			})
		}
	}
}
