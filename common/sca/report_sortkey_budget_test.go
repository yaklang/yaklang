package sca

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func sameIdentitySortPkgs(n, reqs, targetBytes int) []*dxtypes.Package {
	target := strings.Repeat("t", targetBytes)
	pkgs := make([]*dxtypes.Package, n)
	for i := 0; i < n; i++ {
		p := &dxtypes.Package{Name: "same", Version: "1.0.0", Potential: true}
		p.EnsureDetails()
		p.Ecosystem = "npm"
		p.DownStreamPackages = map[string]*dxtypes.Package{"keep": {}}
		p.Locations = []dxtypes.SourceRange{{StartLine: i + 1, EndLine: i + 1}}
		p.Requirements = make([]model.Requirement, reqs)
		for j := 0; j < reqs; j++ {
			p.Requirements[j] = model.Requirement{Target: target, Constraint: "1.0.0", Scope: "runtime"}
		}
		pkgs[i] = p
	}
	return pkgs
}

func TestFillReportSortKeyProbe(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("SCA_SORTKEY_N"))
	if n <= 0 {
		t.Skip("SCA_SORTKEY_N")
	}
	limit, _ := strconv.ParseInt(os.Getenv("SCA_SORTKEY_LIMIT"), 10, 64)
	reqs, _ := strconv.Atoi(os.Getenv("SCA_SORTKEY_REQS"))
	if reqs <= 0 {
		reqs = 4
	}
	targetBytes, _ := strconv.Atoi(os.Getenv("SCA_SORTKEY_TARGET"))
	if targetBytes <= 0 {
		targetBytes = 80
	}
	l := ResourceLimits{MaxResultBytes: limit, MaxComponents: 200000, MaxObservations: 400000, MaxEdges: 1000000}
	nl, err := l.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	pkgs := sameIdentitySortPkgs(n, reqs, targetBytes)
	st := &budget.State{Limits: nl}
	r := &model.Report{Complete: true}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	errs := fillReport(r, pkgs, nl, st)
	ns := time.Since(start).Nanoseconds()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	errStr := "<nil>"
	limitHit := false
	for _, e := range errs {
		if e != nil {
			errStr = e.Error()
			if errors.Is(e, scanerr.ErrResourceLimit) {
				limitHit = true
			}
			break
		}
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	mallocs := after.Mallocs - before.Mallocs
	fmt.Printf("n=%d reqs=%d tgt=%d limit=%d rec=%d charged=%d alloc=%d mallocs=%d rss=%d ns=%d err_limit=%t err=%s complete=%t\n",
		n, reqs, targetBytes, nl.MaxResultBytes, len(r.Components)+len(r.Observations)+len(r.Requirements), st.ResultBytes(), alloc, mallocs, ru.Maxrss, ns, limitHit, errStr, r.Complete)
}

func collideIdentityLocPkgs(n, reqs, targetBytes int, target string) []*dxtypes.Package {
	pkgs := make([]*dxtypes.Package, n)
	for i := 0; i < n; i++ {
		p := &dxtypes.Package{Name: "same", Version: "1.0.0", Potential: true}
		p.EnsureDetails()
		p.Ecosystem = "npm"
		p.DownStreamPackages = map[string]*dxtypes.Package{"keep": {}}
		p.Locations = []dxtypes.SourceRange{{StartLine: 1, EndLine: 1}}
		p.Requirements = make([]model.Requirement, reqs)
		for j := 0; j < reqs; j++ {
			p.Requirements[j] = model.Requirement{Target: target, Constraint: "1.0.0", Scope: "runtime", Candidates: []string{target}}
		}
		pkgs[i] = p
	}
	return pkgs
}

func TestFillReportChargesSortKeysBeforeCopy(t *testing.T) {
	l, err := (ResourceLimits{MaxResultBytes: 8000, MaxComponents: 200000, MaxObservations: 400000, MaxEdges: 1000000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	pkgs := collideIdentityLocPkgs(8, 4, 80, strings.Repeat("t", 80))
	st := &budget.State{Limits: l}
	r := &model.Report{Complete: true}
	errs := fillReport(r, pkgs, l, st)
	if len(errs) == 0 || !errors.Is(errs[0], scanerr.ErrResourceLimit) {
		t.Fatalf("identity+location collision JSON must be charged before marshal: complete=%v rec=%d charged=%d err=%v", r.Complete, len(r.Requirements), st.ResultBytes(), errs)
	}
	if r.Complete {
		t.Fatal("limit must mark report incomplete")
	}
	if len(r.Components) != 0 {
		t.Fatalf("Potential packages must not become components: %d", len(r.Components))
	}

	ok := sameIdentitySortPkgs(6, 2, 8)
	for i, p := range ok {
		p.Potential = false
		p.Name = fmt.Sprintf("pkg%d", i)
		p.Instance = fmt.Sprintf("id%d", i)
	}
	posLimits, err := (ResourceLimits{MaxComponents: 100, MaxObservations: 100, MaxEdges: 1000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	type ordered struct{ name, id, loc, req string }
	var want []ordered
	for _, p := range ok {
		want = append(want, ordered{p.Name, p.Identifier(), locationKey(p), requirementKey(p)})
	}
	sort.SliceStable(want, func(i, j int) bool {
		if want[i].id != want[j].id {
			return want[i].id < want[j].id
		}
		if want[i].loc != want[j].loc {
			return want[i].loc < want[j].loc
		}
		return want[i].req < want[j].req
	})
	pos := &model.Report{Complete: true}
	if errs = fillReport(pos, ok, posLimits, nil); len(errs) != 0 || !pos.Complete || len(pos.Components) != 6 {
		t.Fatalf("positive fillReport: complete=%v n=%d err=%v", pos.Complete, len(pos.Components), errs)
	}
	for i := range want {
		if pos.Components[i].Key.Name != want[i].name {
			t.Fatalf("component %d: got %s want %s (id/loc/req order)", i, pos.Components[i].Key.Name, want[i].name)
		}
	}
}

func TestFillReportSkipsJSONWhenIdentitiesDiffer(t *testing.T) {
	l, err := (ResourceLimits{MaxResultBytes: 20000, MaxComponents: 100, MaxObservations: 100, MaxEdges: 1000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", 8000)
	pkgs := make([]*dxtypes.Package, 4)
	for i := 0; i < 4; i++ {
		p := &dxtypes.Package{Name: fmt.Sprintf("pkg%d", i), Version: "1.0.0", Potential: true}
		p.EnsureDetails()
		p.Ecosystem = "npm"
		p.Instance = fmt.Sprintf("id%d", i)
		p.DownStreamPackages = map[string]*dxtypes.Package{"keep": {}}
		p.Requirements = []model.Requirement{{Target: huge, Constraint: "1", Scope: "runtime", Candidates: []string{huge, huge}}}
		pkgs[i] = p
	}
	st := &budget.State{Limits: l}
	r := &model.Report{Complete: true}
	errs := fillReport(r, pkgs, l, st)
	if len(errs) != 0 || !r.Complete {
		t.Fatalf("distinct identities must not marshal requirement JSON: complete=%v charged=%d err=%v", r.Complete, st.ResultBytes(), errs)
	}
}

func TestFillReportSortOrderControlCharsAndLongLocations(t *testing.T) {
	posLimits, err := (ResourceLimits{MaxComponents: 100, MaxObservations: 200, MaxEdges: 1000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	mk := func(name, target string, locs []dxtypes.SourceRange, cands []string) *dxtypes.Package {
		p := &dxtypes.Package{Name: name, Version: "1.0.0"}
		p.EnsureDetails()
		p.Ecosystem = "npm"
		p.Locations = locs
		p.Requirements = []model.Requirement{{Target: target, Constraint: "1", Scope: "runtime", Candidates: cands}}
		return p
	}
	longLoc := make([]dxtypes.SourceRange, 40)
	for i := range longLoc {
		longLoc[i] = dxtypes.SourceRange{StartLine: i + 1, EndLine: i + 2}
	}
	pkgs := []*dxtypes.Package{
		mk("same", "a", []dxtypes.SourceRange{{StartLine: 1, EndLine: 1}}, nil),
		mk("same", "\x00\n\"", []dxtypes.SourceRange{{StartLine: 1, EndLine: 1}}, []string{"\x01", "z"}),
		mk("same", "a", longLoc, []string{"one", "two", "three"}),
		mk("other", "a", []dxtypes.SourceRange{{StartLine: 1, EndLine: 1}}, nil),
	}
	for _, p := range pkgs {
		p.EnsureDetails()
		sort.SliceStable(p.Locations, func(i, j int) bool {
			if p.Locations[i].StartLine != p.Locations[j].StartLine {
				return p.Locations[i].StartLine < p.Locations[j].StartLine
			}
			return p.Locations[i].EndLine < p.Locations[j].EndLine
		})
		if len(p.Locations) > 0 {
			p.StartLine, p.EndLine = p.Locations[0].StartLine, p.Locations[0].EndLine
		}
	}
	fp := func(p *dxtypes.Package) string {
		return p.Identifier() + "|" + locationKey(p) + "|" + requirementKey(p)
	}
	want := make([]string, len(pkgs))
	for i, p := range pkgs {
		want[i] = fp(p)
	}
	sort.SliceStable(want, func(i, j int) bool { return want[i] < want[j] })
	r := &model.Report{Complete: true}
	if errs := fillReport(r, pkgs, posLimits, nil); len(errs) != 0 || !r.Complete {
		t.Fatalf("control/long-location sort: complete=%v err=%v", r.Complete, errs)
	}
	for i, p := range pkgs {
		if got := fp(p); got != want[i] {
			t.Fatalf("order %d got %q want %q", i, got, want[i])
		}
	}
}

func TestFillReportOverflowSentinelNotAdded(t *testing.T) {
	p := &dxtypes.Package{Name: "x", Version: "1"}
	p.EnsureDetails()
	p.Locations = make([]dxtypes.SourceRange, 1<<20)
	if _, err := locationWorkingBytes(p); err != nil && !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatal(err)
	}
	if n, err := budget.SizeAdd(-1, 1000); err == nil || n != 0 {
		t.Fatalf("overflow sentinel added: %d %v", n, err)
	}
}
