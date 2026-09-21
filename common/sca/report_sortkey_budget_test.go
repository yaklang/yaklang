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

func TestFillReportChargesSortKeysBeforeCopy(t *testing.T) {
	l, err := (ResourceLimits{MaxResultBytes: 8000, MaxComponents: 200000, MaxObservations: 400000, MaxEdges: 1000000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	pkgs := sameIdentitySortPkgs(40, 4, 80)
	st := &budget.State{Limits: l}
	r := &model.Report{Complete: true}
	errs := fillReport(r, pkgs, l, st)
	if len(errs) == 0 || !errors.Is(errs[0], scanerr.ErrResourceLimit) {
		t.Fatalf("sort-key copies must be charged before marshal: complete=%v rec=%d charged=%d err=%v", r.Complete, len(r.Requirements), st.ResultBytes(), errs)
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
	type ordered struct{ name, id string }
	var want []ordered
	for _, p := range ok {
		want = append(want, ordered{p.Name, p.Identifier()})
	}
	sort.SliceStable(want, func(i, j int) bool { return want[i].id < want[j].id })
	pos := &model.Report{Complete: true}
	if errs = fillReport(pos, ok, posLimits, nil); len(errs) != 0 || !pos.Complete || len(pos.Components) != 6 {
		t.Fatalf("positive fillReport: complete=%v n=%d err=%v", pos.Complete, len(pos.Components), errs)
	}
	for i := range want {
		if pos.Components[i].Key.Name != want[i].name {
			t.Fatalf("component %d: got %s want %s (id order)", i, pos.Components[i].Key.Name, want[i].name)
		}
	}
}
