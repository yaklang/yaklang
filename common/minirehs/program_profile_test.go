package minirehs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

func programRuntimeInputs() map[string][]byte {
	return map[string][]byte{
		"HTTP912":      bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\nContent-Type: text/html\r\n\r\n<html> token=abc123 header-042: abcd;ef </html>\n"), 8),
		"Sparse16K":    append(bytes.Repeat([]byte("zqxv "), 3276), []byte(" tok00042zdeadbeef common-prefix42token042 ")...),
		"DenseHTTP16K": bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n"), 512),
		"Zero16K":      make([]byte, 16<<10),
	}
}

func TestProgramRouteCensus(t *testing.T) {
	dir := programEvidenceDir(t)
	type rule struct {
		Pattern                                                 Pattern
		Verifier                                                string
		Literals                                                []string
		Positions, Words, Symbols, ExceptionRows                int
		Assert, Gate, RE2Location, Window, Anchored, BiAnchored bool
		Head, Tail, GateHead                                    int32
		ConditionalFirst, ConditionalFollow, ConditionalAccept  int
		ConditionalFollowMask                                   uint64
		ZeroConditionalFollowEligible                           bool
	}
	for _, set := range []string{"GetRequest", "MITM"} {
		patterns := getRequestImagePatterns(t)
		if set == "MITM" {
			patterns, _ = compilableMITMPatterns(t)
		}
		for _, locate := range []bool{false, true} {
			var parses, attempts int
			imageParseHook = func() { parses++ }
			imageNFAHook = func() { attempts++ }
			db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			imageParseHook, imageNFAHook = nil, nil
			if err != nil {
				t.Fatal(err)
			}
			m := db.(*database).imageMVS()
			label := fmt.Sprintf("%s-locate-%v", set, locate)
			rules := make([]rule, 0, len(m.all))
			eligible, logicalFollowBytes := 0, 0
			for i, cp := range m.all {
				r := rule{Pattern: Pattern{ID: cp.id, Expr: cp.expr}, Literals: cp.literals,
					Gate: m.gate[i], RE2Location: m.re2Loc[i], Window: m.windowable[i], Anchored: m.anchorable[i], BiAnchored: m.biAnchorable[i],
					Head: m.win[i].head, Tail: m.win[i].tail, GateHead: m.gateHead[i]}
				r.Verifier = "PCRE2"
				if cp.v.exact() {
					r.Verifier = "RE2"
				}
				if n := m.nfas[i]; n != nil {
					r.Positions, r.Words, r.Symbols, r.ExceptionRows = n.npos, n.nword, n.nsym, n.excCount
					r.Assert, r.ConditionalFirst, r.ConditionalAccept = n.hasAssert, len(n.condFirst), len(n.condAccept)
					for _, edges := range n.condFollow {
						r.ConditionalFollow += len(edges)
					}
					r.ConditionalFollowMask = n.condFollowMask1
					r.ZeroConditionalFollowEligible = n.single && n.hasAssert && r.ConditionalFollow == 0 && n.condFollowMask1 == 0
					if r.ZeroConditionalFollowEligible {
						eligible++
						logicalFollowBytes += 64 * n.npos * 8
					}
				}
				rules = append(rules, r)
			}
			writeProgramJSON(t, dir, label+"-census.json", map[string]interface{}{
				"set": set, "locate": locate, "original_patterns": patterns, "db_info": db.Info(), "primary_rules": rules,
				"frontend_parse_calls": parses, "try_compile_nfa": attempts, "literal_count": len(m.literals),
				"merged_members": m.mergedCount, "assert_always_on": m.assertAlwaysOn, "other_always_on": m.otherAlwaysOn,
				"zero_cond_follow_eligible_units": eligible, "zero_cond_follow_logical_bytes": logicalFollowBytes,
				"logical_bytes_note": "Logical per-pattern table extent; actual C allocations/consumers are measured separately, not production RSS.",
			})
			if !locate {
				if err := os.WriteFile(filepath.Join(dir, set+".blob"), m.blob, 0600); err != nil {
					t.Fatal(err)
				}
				image, err := db.MarshalImage()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, set+".rehs"), image, 0600); err != nil {
					t.Fatal(err)
				}
			}
			type inputWork struct{ Bytes, RawHits, Fanout, UniqueLiteralMembers int }
			work := make(map[string]inputWork)
			for name, data := range programRuntimeInputs() {
				sc := &scratch{}
				hits := m.pf.scanHits(data, sc)
				seen := make(map[int32]bool)
				fanout := 0
				for _, hit := range hits {
					members := m.litToPat[hit.litID]
					fanout += len(members)
					for _, idx := range members {
						seen[idx] = true
					}
				}
				work[name] = inputWork{len(data), len(hits), fanout, len(seen)}
				sc.Close()
			}
			writeProgramJSON(t, dir, label+"-literal-work.json", work)
			db.Close()
			t.Logf("%s: rules=%d parse=%d NFA=%d zero-follow eligible=%d logical bytes=%d", label, len(patterns), parses, attempts, eligible, logicalFollowBytes)
		}
	}
}

func TestProgramBaselineObservations(t *testing.T) {
	dir := programEvidenceDir(t)
	type pair struct {
		Set, Input       string
		Data             []byte
		Locate, Loaded   bool
		Stop             int
		Actual, Expected programObservation
		Ordered          bool
	}
	var pairs []pair
	for _, set := range []string{"GetRequest", "MITM"} {
		patterns := getRequestImagePatterns(t)
		if set == "MITM" {
			patterns, _ = compilableMITMPatterns(t)
		}
		for _, locate := range []bool{false, true} {
			db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			m := db.(*database).imageMVS()
			// A RE2 rule with no executable NFA uses the documented exact-span
			// fallback even in existence mode. Only route policy comes from the
			// plan; the reference matcher and all positions are independent.
			fallbackExact := make(map[PatternID]bool)
			for i, cp := range m.all {
				fallbackExact[cp.id] = m.nfas[i] == nil && cp.v.exact()
			}
			image, err := db.MarshalImage()
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			loaded, err := LoadImage(image)
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			for name, data := range programRuntimeInputs() {
				want := programObservation{Completed: true}
				for _, p := range patterns {
					one := programOracle([]Pattern{p}, data, BackendMVS, locate, 0, fallbackExact[p.ID])
					want.Callbacks = append(want.Callbacks, one.Callbacks...)
					if one.Error != nil {
						want.Error, want.Completed = one.Error, false
						break
					}
				}
				for _, source := range []struct {
					db     Database
					loaded bool
				}{{db, false}, {loaded, true}} {
					got := programScan(t, source.db, data, 0)
					pairs = append(pairs, pair{set, name, data, locate, source.loaded, 0, got, want, false})
					if err := programCompare(got, want, false); err != nil {
						t.Errorf("%s/%s/loc=%v/load=%v: %v", set, name, locate, source.loaded, err)
					}
				}
				// Across Compile/Load, preserve the actual observable candidate
				// order as well as the stop position. Whole-reference iteration
				// order is deliberately not imposed on the optimized engine.
				for _, stop := range []int{1, 2} {
					a, b := programScan(t, db, data, stop), programScan(t, loaded, data, stop)
					pairs = append(pairs, pair{set, name, data, locate, true, stop, b, a, true})
					if err := programCompare(b, a, true); err != nil {
						t.Error(err)
					}
				}
			}
			loaded.Close()
			db.Close()
		}
	}
	writeProgramJSON(t, dir, "baseline-complete-results.json", pairs)
	t.Logf("saved %d complete actual/reference or ordered Compile/Load result pairs", len(pairs))
}

// CPU profiles start after fixture/setup, isolating the selected phase.
// This is separate from the uninstrumented outcome benchmarks. Allocation
// profiles include process setup; only MemStats deltas describe the phase.
func TestProgramProfileStage(t *testing.T) {
	stage := os.Getenv("MINIREHS_PROFILE_STAGE")
	if stage == "" {
		t.Skip("set MINIREHS_PROFILE_STAGE for an isolated diagnostic profile")
	}
	dir := programEvidenceDir(t)
	patterns := getRequestImagePatterns(t)
	var records [][]byte
	if strings.HasPrefix(stage, "mitm_") {
		patterns, _ = compilableMITMPatterns(t)
		records, _ = loadCorpus(t)
		if len(records) != 1332 {
			t.Fatal("profile requires full actual corpus")
		}
	} else if stage == "http" || stage == "dense" || stage == "zero" || stage == "sparse" {
		key := map[string]string{"http": "HTTP912", "dense": "DenseHTTP16K", "zero": "Zero16K", "sparse": "Sparse16K"}[stage]
		records = [][]byte{programRuntimeInputs()[key]}
	}
	opts := []Option{WithBackend(BackendMVS), WithReportLocation(stage == "mitm_locate"), WithLogger(silentLogger{})}
	var base Database
	var image []byte
	var sc Scratch
	var err error
	if stage != "compile" {
		base, err = Compile(patterns, opts...)
		if err != nil {
			t.Fatal(err)
		}
		if stage == "load" {
			image, err = base.MarshalImage()
			base.Close()
			base = nil
			if err != nil {
				t.Fatal(err)
			}
		} else {
			defer base.Close()
			sc, err = base.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
			defer sc.Close()
			for _, record := range records {
				if err := base.Scan(record, sc, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if stage != "compile" && stage != "load" && len(records) == 0 {
		t.Fatalf("unknown profile stage %q", stage)
	}
	runtime.GC()
	file, err := os.Create(filepath.Join(dir, stage+".cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	iterations, inputBytes, inputRecords := 0, int64(0), int64(0)
	started := time.Now()
	for iterations == 0 || time.Since(started) < 4*time.Second {
		if stage == "compile" || stage == "load" {
			var db Database
			if stage == "compile" {
				db, err = Compile(patterns, opts...)
			} else {
				db, err = LoadImage(image)
			}
			if err != nil {
				break
			}
			db.Close()
		} else {
			for _, record := range records {
				err = base.Scan(record, sc, nil)
				if err != nil {
					break
				}
				inputBytes += int64(len(record))
				inputRecords++
			}
			if err != nil {
				break
			}
		}
		iterations++
	}
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	pprof.StopCPUProfile()
	file.Close()
	writeProgramJSON(t, dir, stage+"-profile.json", map[string]interface{}{
		"stage": stage, "iterations": iterations, "input_bytes": inputBytes, "input_records": inputRecords,
		"phase_wall_ns": elapsed.Nanoseconds(), "phase_go_total_alloc_bytes": after.TotalAlloc - before.TotalAlloc,
		"phase_go_mallocs": after.Mallocs - before.Mallocs, "error": programError(err),
		"cpu_note": "Isolated sampled CPU profile includes native work behind runtime.cgocall; no native stack or PMU attribution claimed.",
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Qualify the already-present safe subject sharing independently of the new
// pool work. Candidates are synthetic; reductions here are not a production
// weighted speedup. Existing PCRE flags/normalization remain authoritative.
func BenchmarkProgramSubjectReuse(b *testing.B) {
	data := append(bytes.Repeat([]byte("zqxv "), 3276), []byte("common-prefix!")...)
	for _, candidates := range []int{1, 8, 64} {
		b.Run(fmt.Sprint(candidates), func(b *testing.B) {
			confirmers := make([]*pcre2Verifier, candidates)
			for i := range confirmers {
				v := &pcre2Verifier{expr: `common-prefix(?=!)`}
				if locs, err := v.findAllErr(data); err != nil || len(locs) != 1 {
					b.Fatalf("PCRE warmup: %v %v", locs, err)
				}
				confirmers[i] = v
			}
			for _, shared := range []bool{false, true} {
				b.Run(map[bool]string{false: "Independent", true: "Shared"}[shared], func(b *testing.B) {
					sc := &scratch{}
					defer sc.Close()
					b.SetBytes(int64(len(data)))
					b.ReportAllocs()
					b.ResetTimer()
					b.ReportMetric(float64(candidates), "candidates/op")
					for i := 0; i < b.N; i++ {
						for _, v := range confirmers {
							var locs [][2]int
							var err error
							if shared {
								locs, err = findAllInRecord(v, data, 0, len(data), sc)
							} else {
								locs, err = v.findAllErr(data)
							}
							if err != nil || len(locs) != 1 {
								b.Fatalf("PCRE result: %v %v", locs, err)
							}
						}
						sc.clearVerifierText()
					}
				})
			}
		})
	}
}
