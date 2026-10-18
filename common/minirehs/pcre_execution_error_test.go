package minirehs

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	pcrelite "github.com/VillanCh/go-pcre2-lite"
	compat "github.com/VillanCh/go-pcre2-lite/regexp2"
)

type programLimitedVerifier struct {
	verifier
	limited *compat.Regexp
}

func (v programLimitedVerifier) findAllErr(data []byte) ([][2]int, error) {
	ok, err := v.limited.MatchString(string(data))
	if err != nil || !ok {
		return nil, err
	}
	return [][2]int{{-1, -1}}, nil
}

func TestPCREGatedWorkerErrorOrderAndReuse(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(oldProcs)
	for _, procs := range []int{1, 2} {
		runtime.GOMAXPROCS(procs)
		patterns := []Pattern{{ID: 10, Expr: `token(?=!)`}, {ID: 20, Expr: `common-prefix(?=(a+)+$)a`}}
		db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		if m.gateHead[0] < 0 || m.gateHead[1] < 0 || !m.gate[1] {
			db.Close()
			t.Fatal("expected actual localized-gate dispatcher")
		}
		// Same existing public compatibility API and flags as the Yak wrapper.
		// Configure before any scan; never mutate a published matching instance.
		limited, err := compat.Compile(patterns[1].Expr, compat.None)
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		if err := limited.SetMatchLimits(10, 10); err != nil {
			db.Close()
			t.Fatal(err)
		}
		m.all[1].v = programLimitedVerifier{verifier: m.all[1].v, limited: limited}
		originalFilter := m.pf
		m.pf = programOrderedHits{prefilter: originalFilter}
		sc, err := db.NewScratch()
		if err != nil {
			t.Fatal(err)
		}
		data := []byte("token! common-prefix" + strings.Repeat("a", 64) + "!")
		for _, stopEarly := range []bool{true, false, true, false} {
			var got []PatternID
			err := db.Scan(data, sc, func(match Match) bool {
				got = append(got, match.ID)
				return !stopEarly
			})
			if len(got) != 1 || got[0] != 10 {
				t.Fatalf("procs=%d callback prefix=%v", procs, got)
			}
			if stopEarly && err != nil || !stopEarly && !errors.Is(err, pcrelite.ErrMatchLimit) {
				t.Fatalf("procs=%d early=%v err=%v", procs, stopEarly, err)
			}
			if err != nil {
				var failure *ScanError
				if !errors.As(err, &failure) || failure.RuleID != 20 || failure.Category != "match_limit" || failure.NativeCode != nil {
					t.Fatalf("lost exposed error attributes or invented a missing native code: %v", err)
				}
			}
			root := sc.(*scratch)
			if root.verifierText != "" || root.gateAsyncScratch != nil && root.gateAsyncScratch.verifierText != "" {
				t.Fatal("worker retained subject after early stop or error")
			}
			if err := db.Scan([]byte("token!"), sc, nil); err != nil {
				t.Fatalf("scratch reuse after failed verification: %v", err)
			}
		}
		sc.Close()
		m.pf = originalFilter
		db.Close()
	}
}

func TestImageLazyVerifierFailureReturnsError(t *testing.T) {
	for _, expr := range []string{programLimitedPCRE, `a*`} {
		db, err := Compile([]Pattern{{ID: 0, Expr: expr}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		// Structurally representable image, deliberately invalid stored regex.
		// Load must remain parse-free; Scan must report failure, not NoMatch.
		db.(*database).imageMVS().all[0].expr = `(`
		image, err := db.MarshalImage()
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
		compiles := 0
		imageParseHook = func() { compiles++ }
		imageNFAHook = func() { compiles++ }
		loaded, err := LoadImage(image)
		imageParseHook, imageNFAHook = nil, nil
		if err != nil {
			t.Fatal(err)
		}
		if compiles != 0 {
			loaded.Close()
			t.Fatal("Load compiled a lazy verifier")
		}
		for i := 0; i < 2; i++ {
			err := loaded.Scan([]byte("aa!"), nil, nil)
			var failure *ScanError
			if !errors.As(err, &failure) || failure.RuleID != 0 || !failure.HasRule || !failure.Incomplete || failure.Category != "compile" {
				t.Fatalf("lazy failure became NoMatch or lost context: %v", err)
			}
		}
		loaded.Close()
	}
}

const programLimitedPCRE = `(*LIMIT_MATCH=10)(*NO_AUTO_POSSESS)(*NO_START_OPT)(a+)+$`

func TestPCREExecutionErrorsPropagate(t *testing.T) {
	for _, backend := range []BackendKind{BackendEngine, BackendStdlib, BackendMVS} {
		t.Run(backend.String(), func(t *testing.T) {
			patterns := []Pattern{{ID: 99, Expr: `safe`}, {ID: 0, Expr: programLimitedPCRE}}
			db, err := Compile(patterns, WithBackend(backend), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			candidates := []Database{db}
			if backend == BackendMVS {
				image, err := db.MarshalImage()
				if err != nil {
					t.Fatal(err)
				}
				loaded, err := LoadImage(image)
				if err != nil {
					t.Fatal(err)
				}
				defer loaded.Close()
				candidates = append(candidates, loaded)
			}
			data := []byte("safe " + strings.Repeat("a", 64) + "!")
			for _, current := range candidates {
				var callbacks []Match
				err = current.Scan(data, nil, func(m Match) bool {
					callbacks = append(callbacks, m)
					return true
				})
				if err == nil {
					t.Fatalf("PCRE resource failure became successful NoMatch; callbacks=%v", callbacks)
				}
				var failure *ScanError
				if !errors.Is(err, pcrelite.ErrMatchLimit) || !errors.As(err, &failure) || !failure.Incomplete ||
					!failure.HasRule || failure.RuleID != 0 || failure.Record != -1 {
					t.Fatalf("PCRE error lost cause/rule/incomplete context: %v", err)
				}
				if len(callbacks) != 1 || callbacks[0].ID != 99 {
					t.Fatalf("callbacks before error changed: %v", callbacks)
				}
				if err := current.Scan(data, nil, func(Match) bool { return false }); err != nil {
					t.Fatalf("earlier callback stop exposed a later error: %v", err)
				}
			}
		})
	}
}
