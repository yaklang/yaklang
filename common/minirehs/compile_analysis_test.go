package minirehs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp/syntax"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Include every semantic node field, not just String(), and revisit shared
// children as occurrences. Readers may borrow syntax nodes but may not edit them.
func analysisTreeShape(re *syntax.Regexp) string {
	if re == nil {
		return "nil"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "(%d/%d/%v/%d/%d/%d/%q", re.Op, re.Flags, re.Rune, re.Min, re.Max, re.Cap, re.Name)
	for _, sub := range re.Sub {
		b.WriteString(analysisTreeShape(sub))
	}
	b.WriteByte(')')
	return b.String()
}

func TestCompileASTReadOnlyAndOccurrences(t *testing.T) {
	for _, expr := range []string{`(ab){2}`, `(?:a?b?){3}x`, `\btoken\b`, `(?i)k[ab]{2}`, `^token[0-9]{2}$`, `\x{fffd}+x`} {
		re, tree, err := compileAndParse(expr)
		if err != nil {
			t.Fatal(err)
		}
		before := analysisTreeShape(tree)
		lits := extractRequiredLiterals(tree, 2)
		cp := &compiledPattern{id: 0, expr: expr, v: &re2Verifier{re: re}, literals: lits}
		parsed := map[*compiledPattern]*syntax.Regexp{cp: tree}
		cfg := newDefaultConfig()
		cfg.backend = BackendMVS
		cfg.logger = silentLogger{}
		db, err := (&mvsBackend{}).compileParsed([]*compiledPattern{cp}, cfg, parsed)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed) != 0 {
			db.close()
			t.Fatal("compile-only AST map retained processed rules")
		}
		if before != analysisTreeShape(tree) {
			db.close()
			t.Fatal("NFA/window readers mutated shared syntax")
		}
		m := db.(*mvsDB)
		if expr == `(ab){2}` && (m.nfas[0] == nil || m.nfas[0].npos != 4) {
			db.close()
			t.Fatal("shared AST occurrences collapsed")
		}
		if got, want := computeLitWindowTree(tree, lits), computeLitWindow(expr, lits); got != want {
			db.close()
			t.Fatalf("window parameter reuse: %v != %v", got, want)
		}
		db.close()
	}
}

func TestCompileASTParseCounts(t *testing.T) {
	old := compileASTReuseEnabled
	defer func() { compileASTReuseEnabled = old; imageParseHook = nil; imageNFAHook = nil }()
	for _, backend := range []BackendKind{BackendMVS, Auto, BackendEngine, BackendStdlib} {
		counts := make([]int, 2)
		for i, enabled := range []bool{false, true} {
			compileASTReuseEnabled = enabled
			parses, attempts := 0, 0
			imageParseHook = func() { parses++ }
			imageNFAHook = func() { attempts++ }
			db, err := Compile([]Pattern{{ID: 0, Expr: `token[0-9]{2}`}, {ID: 0, Expr: `other[ab]{2}`}}, WithBackend(backend), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			imageParseHook = nil
			imageNFAHook = nil
			counts[i] = parses
			if backend == BackendMVS && attempts != 2 {
				t.Fatalf("NFA attempts=%d", attempts)
			}
		}
		if backend == BackendMVS {
			if counts[0] != 6 || counts[1] != 2 {
				t.Fatalf("explicit package Parse counts: %v", counts)
			}
		} else if counts[0] != 2 || counts[1] != 2 {
			t.Fatalf("non-MVS analysis retention: %v", counts)
		}
	}
}

func compileASTPatterns() []Pattern {
	return []Pattern{{ID: 0, Expr: `abc`}, {ID: 0, Expr: `abc`, Flags: FlagCaseless}, {ID: 2, Expr: `(ab){2}`}, {ID: 3, Expr: `(?:a?b?){2}x`}, {ID: 4, Expr: `a*`}, {ID: 5, Expr: `\bword\b`}, {ID: 6, Expr: `k`, Flags: FlagCaseless}, {ID: 7, Expr: `\x{fffd}+x`}, {ID: 8, Expr: `token[0-9]{2}`}, {ID: 9, Expr: `abc(?=!)`}, {ID: 10, Expr: `(?i)word`}, {ID: 11, Expr: `word`, Flags: FlagCaseless}, {ID: 12, Expr: `(?:)`}}
}

func TestCompileASTCompleteResults(t *testing.T) {
	old := compileASTReuseEnabled
	defer func() { compileASTReuseEnabled = old }()
	patterns := compileASTPatterns()
	for _, locate := range []bool{false, true} {
		for _, minLit := range []int{1, 2, 3} {
			compileASTReuseEnabled = false
			reference, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithMinLiteralLen(minLit), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			a, err := reference.MarshalImage()
			if err != nil {
				t.Fatal(err)
			}
			compileASTReuseEnabled = true
			candidate, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithMinLiteralLen(minLit), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			b, err := candidate.MarshalImage()
			if err != nil || !bytes.Equal(a, b) || !reflect.DeepEqual(reference.Info(), candidate.Info()) {
				t.Fatal("AST reuse changed plan/image/reports", err)
			}
			loaded, err := LoadImage(b)
			if err != nil {
				t.Fatal(err)
			}
			for _, data := range [][]byte{nil, []byte("ABC abc ababab word token42 abc! K"), {0xff, 0xc0, 'x', 0}, []byte(" aaax WORD aaaa")} {
				m := candidate.(*database).imageMVS()
				oracle := programObservation{Completed: true}
				for idx, p := range patterns {
					one := programOracle([]Pattern{p}, data, BackendMVS, locate, 0, m.nfas[idx] == nil && m.all[idx].v.exact())
					oracle.Callbacks = append(oracle.Callbacks, one.Callbacks...)
					if one.Error != nil {
						oracle.Error = one.Error
						oracle.Completed = false
						break
					}
				}
				if err := programCompare(programScan(t, candidate, data, 0), oracle, false); err != nil {
					t.Fatal(err)
				}
				for _, stop := range []int{0, 1, 2, 4} {
					want := programScan(t, reference, data, stop)
					for _, src := range []Database{candidate, loaded} {
						if err := programCompare(programScan(t, src, data, stop), want, true); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			loaded.Close()
			candidate.Close()
			reference.Close()
		}
	}
	for _, patterns := range [][]Pattern{{{ID: 1, Expr: ""}}, {{ID: 2, Expr: "("}}, {{ID: 3, Expr: "good"}, {ID: 4, Expr: "("}}} {
		var a, b string
		for i, enabled := range []bool{false, true} {
			compileASTReuseEnabled = enabled
			db, err := Compile(patterns, WithBackend(BackendKind(255)), WithLogger(silentLogger{}))
			if db != nil {
				db.Close()
			}
			if i == 0 {
				a = fmt.Sprint(err)
			} else {
				b = fmt.Sprint(err)
			}
		}
		if a != b {
			t.Fatalf("compile error/order changed: %s != %s", a, b)
		}
	}
}

func TestCompileASTConcurrent(t *testing.T) {
	old := compileASTReuseEnabled
	compileASTReuseEnabled = true
	defer func() { compileASTReuseEnabled = old }()
	var wg sync.WaitGroup
	problems := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				db, err := Compile([]Pattern{{ID: 0, Expr: `(ab){2}`}, {ID: 1, Expr: `\btoken\b`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
				if err != nil {
					problems <- err
					return
				}
				matches, err := imageMatches(db, []byte("abab token"))
				db.Close()
				if err != nil || len(matches) != 2 {
					problems <- fmt.Errorf("concurrent AST: %v %v", matches, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(problems)
	for err := range problems {
		t.Error(err)
	}
}

func BenchmarkProgramCompileAST(b *testing.B) {
	old := compileASTReuseEnabled
	defer func() { compileASTReuseEnabled = old }()
	for _, set := range []string{"GetRequest", "MITM"} {
		b.Run(set, func(b *testing.B) {
			patterns := getRequestImagePatterns(b)
			if set == "MITM" {
				patterns, _ = compilableMITMPatterns(b)
			}
			for _, enabled := range []bool{false, true} {
				b.Run(map[bool]string{false: "Reference", true: "Reuse"}[enabled], func(b *testing.B) {
					compileASTReuseEnabled = enabled
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
						if err != nil {
							b.Fatal(err)
						}
						db.Close()
					}
				})
			}
		})
	}
}

func TestProgramCompileASTEvidence(t *testing.T) {
	dir := programEvidenceDir(t)
	reuse := os.Getenv("MINIREHS_AST_MODE") == "reuse"
	old := compileASTReuseEnabled
	compileASTReuseEnabled = reuse
	defer func() { compileASTReuseEnabled = old; imageParseHook = nil; imageNFAHook = nil }()
	type phase struct {
		Set                                                                          string
		Locate, Reuse                                                                bool
		Parses, Attempts                                                             int
		WallNS                                                                       int64
		GoAllocBytes, GoMallocs, HeapBefore, SampledHeapPeak, HeapAfter, HeapAfterGC uint64
		HeapSamples                                                                  int
		ImageBytes                                                                   int
	}
	var phases []phase
	type result struct {
		Set, Input string
		Locate     bool
		Stop       int
		Got, Want  programObservation
	}
	var results []result
	for _, set := range []string{"GetRequest", "MITM"} {
		patterns := getRequestImagePatterns(t)
		if set == "MITM" {
			patterns, _ = compilableMITMPatterns(t)
		}
		for _, locate := range []bool{false, true} {
			runtime.GC()
			var before, after, post runtime.MemStats
			runtime.ReadMemStats(&before)
			parses, attempts := 0, 0
			imageParseHook = func() { parses++ }
			imageNFAHook = func() { attempts++ }
			stop, done := make(chan struct{}), make(chan struct{})
			peak, samples := before.HeapAlloc, 0
			go func() {
				defer close(done)
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						return
					case <-ticker.C:
						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						if m.HeapAlloc > peak {
							peak = m.HeapAlloc
						}
						samples++
					}
				}
			}()
			started := time.Now()
			db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			elapsed := time.Since(started)
			close(stop)
			<-done
			imageParseHook = nil
			imageNFAHook = nil
			if err != nil {
				t.Fatal(err)
			}
			runtime.ReadMemStats(&after)
			if after.HeapAlloc > peak {
				peak = after.HeapAlloc
			}
			runtime.GC()
			runtime.ReadMemStats(&post)
			image, err := db.MarshalImage()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-locate-%v.rehs", set, locate)), image, 0600); err != nil {
				t.Fatal(err)
			}
			phases = append(phases, phase{set, locate, reuse, parses, attempts, elapsed.Nanoseconds(), after.TotalAlloc - before.TotalAlloc, after.Mallocs - before.Mallocs, before.HeapAlloc, peak, after.HeapAlloc, post.HeapAlloc, samples, len(image)})
			loaded, err := LoadImage(image)
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range programRuntimeInputs() {
				for _, n := range []int{0, 1, 2} {
					a, b := programScan(t, db, data, n), programScan(t, loaded, data, n)
					if err := programCompare(a, b, true); err != nil {
						t.Fatal(err)
					}
					results = append(results, result{set, name, locate, n, b, a})
				}
			}
			loaded.Close()
			db.Close()
		}
	}
	writeProgramJSON(t, dir, "compile-ast-phases.json", map[string]interface{}{"phases": phases, "note": "Compile diagnostics with package hooks and 10ms heap sampling; not uninstrumented performance, request percentiles or Go live/RSS. Heap peak is sampled HeapAlloc incl uncollected garbage. AST reuse affects only exact NFA/window in phase1."})
	writeProgramJSON(t, dir, "compile-ast-complete-results.json", results)
}
