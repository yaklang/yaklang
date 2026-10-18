package minirehs

import (
	"errors"
	"strings"
	"testing"

	pcrelite "github.com/VillanCh/go-pcre2-lite"
)

func requireGroupPanic(t *testing.T, call func(), cause error) {
	t.Helper()
	defer func() {
		got := recover()
		err, ok := got.(error)
		if !ok || !errors.Is(err, cause) {
			t.Fatalf("legacy API failed silently or lost cause: panic=%v", got)
		}
	}()
	call()
}

func TestGroupErrorsAndLegacyFailure(t *testing.T) {
	g, err := BuildGroup([]string{programLimitedPCRE})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	data := strings.Repeat("a", 64) + "!"
	if found, err := g.MatchErr(data); found || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("MatchErr: %v %v", found, err)
	}
	if found, err := g.MatchStringErr(data); found || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("MatchStringErr: %v %v", found, err)
	}
	if found, err := g.MatchBytesErr([]byte(data)); found || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("MatchBytesErr: %v %v", found, err)
	}
	if out, err := g.FindErr(data); len(out) != 0 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("FindErr: %v %v", out, err)
	}
	if out, err := g.MatchedIndexesErr(data); len(out) != 0 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("MatchedIndexesErr: %v %v", out, err)
	}
	if out, err := g.MatchedPatternsErr(data); len(out) != 0 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("MatchedPatternsErr: %v %v", out, err)
	}
	if count, err := g.CountErr(data); count != 0 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("CountErr: %v %v", count, err)
	}
	if err := g.ScanErr(data, nil); !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("ScanErr: %v", err)
	}
	for name, call := range map[string]func(){
		"Match": func() { g.Match(data) }, "MatchString": func() { g.MatchString(data) }, "MatchBytes": func() { g.MatchBytes([]byte(data)) },
		"Find": func() { g.Find(data) }, "MatchedIndexes": func() { g.MatchedIndexes(data) },
		"MatchedPatterns": func() { g.MatchedPatterns(data) }, "Count": func() { g.Count(data) }, "Scan": func() { g.Scan(data, nil) },
	} {
		t.Run(name, func(t *testing.T) { requireGroupPanic(t, call, pcrelite.ErrMatchLimit) })
	}
	if found, err := MatchAny([]string{programLimitedPCRE}, data); found || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("MatchAny discarded execution failure: %v %v", found, err)
	}
}

func TestGroupErrorAPIsRetainPrefixes(t *testing.T) {
	g, err := BuildGroup([]string{`safe`, programLimitedPCRE})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	data := "safe " + strings.Repeat("a", 64) + "!"
	if out, err := g.FindErr(data); len(out) != 1 || out[0].Index != 0 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("FindErr prefix=%v err=%v", out, err)
	}
	if out, err := g.MatchedIndexesErr(data); len(out) != 1 || out[0] != 0 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("index prefix=%v err=%v", out, err)
	}
	if out, err := g.MatchedPatternsErr(data); len(out) != 1 || out[0] != "safe" || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("pattern prefix=%v err=%v", out, err)
	}
	if n, err := g.CountErr(data); n != 1 || !errors.Is(err, pcrelite.ErrMatchLimit) {
		t.Fatalf("count prefix=%v err=%v", n, err)
	}
	if found, err := g.MatchErr(data); !found || err != nil {
		t.Fatalf("early existence result observed later error: %v %v", found, err)
	}
	if err := g.ScanErr(data, func(*GroupMatch) bool { return false }); err != nil {
		t.Fatalf("callback early stop observed later error: %v", err)
	}
}

func TestClosedDatabaseAndGroupReturnErrors(t *testing.T) {
	for _, backend := range []BackendKind{BackendEngine, BackendStdlib, BackendMVS} {
		db, err := Compile([]Pattern{{ID: 0, Expr: `safe`}}, WithBackend(backend), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal("repeat Close", err)
		}
		if _, err := db.NewScratch(); !errors.Is(err, ErrClosed) {
			t.Fatalf("closed NewScratch: %v", err)
		}
		if err := db.Scan([]byte("safe"), nil, nil); !errors.Is(err, ErrClosed) {
			t.Fatalf("closed Scan: %v", err)
		}
		if err := db.ScanBatch([][]byte{[]byte("safe")}, nil, nil); !errors.Is(err, ErrClosed) {
			t.Fatalf("closed batch: %v", err)
		}
	}
	g, err := BuildGroup([]string{`safe`})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.MatchErr("safe"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed group: %v", err)
	}
	requireGroupPanic(t, func() { g.Match("safe") }, ErrClosed)
}
