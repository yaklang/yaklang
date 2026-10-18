package minirehs

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	pcre2 "github.com/VillanCh/go-pcre2-lite/regexp2"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// Keep recoverable complete results, including multiplicity and errors. A digest
// or a set cannot detect duplicate output, swallowed errors, or selection drift.
type programObservedError struct {
	Type    string
	Message string
}

type programObservation struct {
	Callbacks []Match
	Stopped   bool
	Completed bool
	Error     *programObservedError
}

func programError(err error) *programObservedError {
	if err == nil {
		return nil
	}
	return &programObservedError{Type: fmt.Sprintf("%T", err), Message: err.Error()}
}

func programCompare(got, want programObservation, ordered bool) error {
	canonical := func(o programObservation) programObservation {
		o.Callbacks = append([]Match(nil), o.Callbacks...)
		if !ordered {
			sort.Slice(o.Callbacks, func(i, j int) bool {
				a, b := o.Callbacks[i], o.Callbacks[j]
				if a.ID != b.ID {
					return a.ID < b.ID
				}
				if a.From != b.From {
					return a.From < b.From
				}
				return a.To < b.To
			})
		}
		return o
	}
	if reflect.DeepEqual(canonical(got), canonical(want)) {
		return nil
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	return fmt.Errorf("complete result differs: got=%s want=%s", a, b)
}

func programScan(t *testing.T, db Database, data []byte, stopAfter int) programObservation {
	t.Helper()
	var out programObservation
	sc, err := db.NewScratch()
	if err != nil {
		out.Error = programError(err)
		return out
	}
	defer sc.Close()
	err = db.Scan(data, sc, func(m Match) bool {
		out.Callbacks = append(out.Callbacks, m)
		out.Stopped = stopAfter > 0 && len(out.Callbacks) >= stopAfter
		return !out.Stopped
	})
	out.Error = programError(err)
	out.Completed = err == nil && !out.Stopped
	return out
}

// The reference never calls minirehs' verifier or prefilter. RE2 executes the
// whole original subject. PCRE uses the existing compatibility wrapper, retaining
// its normalization/flags and its error instead of treating an error as NoMatch.
func programOracle(patterns []Pattern, data []byte, backend BackendKind, locate bool, stopAfter int, fallbackExact ...bool) programObservation {
	var out programObservation
	emit := func(m Match) bool {
		out.Callbacks = append(out.Callbacks, m)
		out.Stopped = stopAfter > 0 && len(out.Callbacks) >= stopAfter
		return !out.Stopped
	}
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		re, err := regexp.Compile(expr)
		if err != nil {
			ok, err := regexp_utils.NewYakRegexpUtils(expr).MatchString(string(data))
			if err != nil {
				out.Error = programError(err)
				return out
			}
			if ok && !emit(Match{ID: p.ID, From: -1, To: -1}) {
				return out
			}
			continue
		}
		if backend == BackendMVS && !locate && !(len(fallbackExact) > 0 && fallbackExact[0]) {
			if re.Match(data) && !emit(Match{ID: p.ID, From: -1, To: -1}) {
				return out
			}
			continue
		}
		if backend == BackendMVS {
			re.Longest()
		}
		for _, loc := range re.FindAllIndex(data, -1) {
			if !emit(Match{ID: p.ID, From: loc[0], To: loc[1]}) {
				return out
			}
		}
	}
	out.Completed = true
	return out
}

func TestProgramComparatorDetectsCorruption(t *testing.T) {
	want := programObservation{Callbacks: []Match{{ID: 1, From: 0, To: 2}, {ID: 2, From: -1, To: -1}}, Completed: true}
	for name, mutate := range map[string]func(*programObservation){
		"missing ID":       func(o *programObservation) { o.Callbacks = o.Callbacks[:1] },
		"wrong offset":     func(o *programObservation) { o.Callbacks[0].From++ },
		"extra duplicate":  func(o *programObservation) { o.Callbacks = append(o.Callbacks, o.Callbacks[0]) },
		"wrong sentinel":   func(o *programObservation) { o.Callbacks[1].From = 0 },
		"unexpected error": func(o *programObservation) { o.Error = programError(fmt.Errorf("limit")) },
		"wrong completion": func(o *programObservation) { o.Completed = false },
		"wrong early stop": func(o *programObservation) { o.Stopped = true },
	} {
		t.Run(name, func(t *testing.T) {
			got := want
			got.Callbacks = append([]Match(nil), want.Callbacks...)
			mutate(&got)
			if programCompare(got, want, false) == nil {
				t.Fatal("comparator accepted injected corruption")
			}
		})
	}
	reversed := want
	reversed.Callbacks = []Match{want.Callbacks[1], want.Callbacks[0]}
	if programCompare(reversed, want, false) != nil || programCompare(reversed, want, true) == nil {
		t.Fatal("unordered multiset and ordered callback contracts were conflated")
	}
	failed := want
	failed.Error = programError(fmt.Errorf("resource limit"))
	failed.Completed = false
	if programCompare(want, failed, false) == nil {
		t.Fatal("comparator accepted a swallowed error")
	}
}

func TestProgramOracleKeepsErrors(t *testing.T) {
	got := programOracle([]Pattern{{ID: 1, Expr: `(`}}, []byte("x"), BackendMVS, false, 0)
	if got.Error == nil || got.Completed || len(got.Callbacks) != 0 {
		t.Fatalf("invalid pattern became a successful NoMatch: %+v", got)
	}
	re, err := pcre2.Compile(`(*NO_AUTO_POSSESS)(*NO_START_OPT)^(a+)+$`, pcre2.None)
	if err != nil {
		t.Fatal(err)
	}
	if err := re.SetMatchLimits(10, 10); err != nil {
		t.Fatal(err)
	}
	ok, err := re.MatchString(strings.Repeat("a", 64) + "!")
	if ok || err == nil || programError(err) == nil {
		t.Fatalf("bounded resource-limit oracle lost error: matched=%v err=%v", ok, err)
	}
}

func TestProgramNullableFallbackContract(t *testing.T) {
	// The original MVS frontend explicitly sends nullable roots to the RE2
	// fallback. That existing route reports full-subject exact spans even in
	// existence mode. Preserve the observable exception rather than normalizing
	// it into a single sentinel merely to match the broad backend documentation.
	for _, expr := range []string{`(?:)`, `a*`, `a?`} {
		patterns := []Pattern{{ID: 17, Expr: expr}}
		db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		if m.nfas[0] != nil {
			db.Close()
			t.Fatalf("%q did not enter the nullable fallback", expr)
		}
		for _, data := range [][]byte{nil, []byte("aaa"), []byte("a\x00K")} {
			for _, stop := range []int{0, 1, 2} {
				got := programScan(t, db, data, stop)
				want := programOracle(patterns, data, BackendMVS, false, stop, true)
				if err := programCompare(got, want, true); err != nil {
					t.Error(err)
				}
			}
		}
		db.Close()
	}
}

func TestProgramCompleteOracle(t *testing.T) {
	type observationPair struct {
		Expr     string
		Data     []byte
		Backend  BackendKind
		Locate   bool
		Loaded   bool
		Stop     int
		Actual   programObservation
		Expected programObservation
	}
	var results []observationPair
	for _, tc := range []struct{ expr, data string }{
		{`a|aa`, "aaaa"},
		{`a(?=b)`, "abab"},
		{`(?i)kfoo`, "Kfoo kfoo"},
		{`(?i:sfoo)`, "ſfoo sfoo"},
		{`\bfoo\b`, "foo bar foo"},
		{`[ab]{2,3}`, "aab zz bb"},
		{`[^x]{1,3}`, "x\x00x\xffx"},
		{`(?m)^ab$`, "ab\nx\nab"},
	} {
		for _, backend := range []BackendKind{BackendEngine, BackendStdlib, BackendMVS} {
			for _, locate := range []bool{false, true} {
				name := fmt.Sprintf("%s/%s/loc=%v", tc.expr, backend, locate)
				t.Run(name, func(t *testing.T) {
					patterns := []Pattern{{ID: 17, Expr: tc.expr}}
					db, err := Compile(patterns, WithBackend(backend), WithReportLocation(locate), WithLogger(silentLogger{}))
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					check := func(db Database, loaded bool) {
						for _, stop := range []int{0, 1, 2} {
							got := programScan(t, db, []byte(tc.data), stop)
							want := programOracle(patterns, []byte(tc.data), backend, locate, stop)
							results = append(results, observationPair{tc.expr, []byte(tc.data), backend, locate, loaded, stop, got, want})
							if err := programCompare(got, want, false); err != nil {
								t.Error(err)
							}
						}
					}
					check(db, false)
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
						check(loaded, true)
					}
				})
			}
		}
	}
	if dir := os.Getenv("MINIREHS_EVIDENCE_DIR"); dir != "" {
		writeProgramJSON(t, programEvidenceDir(t), "complete-oracle-results.json", results)
	}
}

// Isolated known-failure probes are opt-in until each correct expectation is
// promoted into an ordinary regression with its corresponding T03/T04 fix.
// A nonzero exit here means an actual result mismatch, never successful repair.
func TestProgramSemanticProbes(t *testing.T) {
	if os.Getenv("MINIREHS_PROGRAM_PROBES") != "1" {
		t.Skip("set MINIREHS_PROGRAM_PROBES=1 for isolated research reproductions")
	}
	dir := programEvidenceDir(t)
	type probe struct {
		Name     string
		Patterns []Pattern
		Data     []byte
		Backend  BackendKind
		Locate   bool
		Actual   programObservation
		Expected programObservation
		Mismatch string
		Status   string
	}
	probes := []probe{
		{Name: "Engine global nonoverlap", Patterns: []Pattern{{ID: 1, Expr: `aa`}}, Data: []byte("aaaaaaaaaa"), Backend: BackendEngine, Locate: true},
		{Name: "gate min end", Patterns: []Pattern{{ID: 1, Expr: `(?:\A1fooAAAAAAAAAAAA(?=Q)|[0-9]{3}foo(?=A))`}}, Data: []byte("001fooAAAAAAAAAAAA"), Backend: BackendMVS},
		{Name: "PCRE sentinel", Patterns: []Pattern{{ID: 1, Expr: `\u0061bc`}}, Data: []byte(strings.Repeat(" ", 1024) + "abc"), Backend: BackendMVS, Locate: true},
		{Name: "PCRE lookbehind", Patterns: []Pattern{{ID: 1, Expr: `(?<=x)foo`}}, Data: []byte("xfoo"), Backend: BackendMVS},
		{Name: "PCRE control verb", Patterns: []Pattern{{ID: 1, Expr: `a(*SKIP)(*F)|foo`}}, Data: []byte("a foo"), Backend: BackendMVS},
	}
	for i := range probes {
		p := &probes[i]
		db, err := Compile(p.Patterns, WithBackend(p.Backend), WithReportLocation(p.Locate), WithLogger(silentLogger{}))
		if err != nil {
			p.Actual.Error = programError(err)
		} else {
			p.Actual = programScan(t, db, p.Data, 0)
			db.Close()
		}
		p.Expected = programOracle(p.Patterns, p.Data, p.Backend, p.Locate, 0)
		if err != nil && p.Expected.Error != nil {
			p.Status = "rejected by both current compiler and independent wrapper; no runtime route"
			continue
		}
		p.Status = "executed"
		if err := programCompare(p.Actual, p.Expected, false); err != nil {
			p.Mismatch = err.Error()
			t.Errorf("%s: %s", p.Name, err)
		}
	}
	writeProgramJSON(t, dir, "semantic-probes.json", probes)
}
