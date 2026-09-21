package sca

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func TestRPMFrozenRequireConstraint(t *testing.T) {
	raw, err := os.ReadFile("testdata/rpm/rpmdb.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	fs := fstest.MapFS{"var/lib/rpm/rpmdb.sqlite": &fstest.MapFile{Data: raw, Mode: 0444}}
	r, err := ScanReport(context.Background(), fs, WithSnapshotID("rpm-capability"))
	if r == nil {
		t.Fatal(err)
	}
	if !r.Complete || err != nil {
		t.Fatalf("complete=%v err=%v", r.Complete, err)
	}
	c := mustNamed(t, r, "mariner-release", "2.0")
	if !strings.Contains(c.Key.Verification, "f7bd337ae2962162ac73a509ed7129f0") {
		t.Fatalf("md5: %q", c.Key.Verification)
	}
	found := false
	for _, q := range r.Requirements {
		if q.Target == "config(mariner-release)" {
			found = true
			if q.Constraint != "= 2.0-4.cm2" {
				t.Fatalf("constraint %q want = 2.0-4.cm2", q.Constraint)
			}
			if !strings.Contains(q.Condition, "268435464") {
				t.Fatalf("flags not propagated: %q", q.Condition)
			}
		}
	}
	if !found {
		t.Fatalf("missing config(mariner-release): %s", extra5149JSON(t, r.Requirements))
	}
	bom := dxtypes.CreateCycloneDXSBOMFromReport(r)
	if bom == nil {
		t.Fatal("nil SBOM")
	}
	var observations []model.Observation
	var requirements []model.Requirement
	for _, p := range bom.Properties {
		switch p.Name {
		case "sca:observations":
			if err := json.Unmarshal([]byte(p.Value), &observations); err != nil {
				t.Fatalf("sca:observations: %v", err)
			}
		case "sca:requirements":
			if err := json.Unmarshal([]byte(p.Value), &requirements); err != nil {
				t.Fatalf("sca:requirements: %v", err)
			}
		}
	}
	if len(observations) == 0 || len(requirements) == 0 {
		t.Fatal("SBOM omitted observations or requirements properties")
	}
	provideOK := false
	for _, o := range observations {
		for _, p := range o.Provides {
			if p == "config(mariner-release) = 2.0-4.cm2" {
				provideOK = true
			}
		}
	}
	if !provideOK {
		t.Fatalf("SBOM observations lost associated provide constraint: %s", extra5149JSON(t, observations))
	}
	reqOK := false
	for _, q := range requirements {
		if q.Target != "config(mariner-release)" {
			continue
		}
		reqOK = true
		if q.Constraint != "= 2.0-4.cm2" {
			t.Fatalf("SBOM requirement constraint %q", q.Constraint)
		}
		if q.Condition != "rpmflags=268435464" {
			t.Fatalf("SBOM requirement flags %q", q.Condition)
		}
		if q.From == "" {
			t.Fatalf("SBOM requirement missing From observation: %+v", q)
		}
	}
	if !reqOK {
		t.Fatalf("SBOM requirements lost associated constraint: %s", extra5149JSON(t, requirements))
	}
}

func TestRPMResourceLimitTyped(t *testing.T) {
	raw, err := os.ReadFile("testdata/rpm/rpmdb.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	fs := fstest.MapFS{"var/lib/rpm/rpmdb.sqlite": &fstest.MapFile{Data: raw, Mode: 0444}}
	r, err := ScanReport(context.Background(), fs, WithResourceLimits(ResourceLimits{MaxResolveSteps: 1}))
	if r == nil || err == nil || r.Complete {
		t.Fatalf("expected incomplete: complete=%v err=%v", r.Complete, err)
	}
	if !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("errors.Is resource_limit=false: %v", err)
	}
	var typed *scanerr.Error
	if !errors.As(err, &typed) || typed.Code != scanerr.ResourceLimit {
		t.Fatalf("errors.As: %v", err)
	}
	if scanerr.CodeOf(err) != scanerr.ResourceLimit {
		t.Fatalf("CodeOf=%s", scanerr.CodeOf(err))
	}
	seen := false
	for _, d := range r.Diagnostics {
		if d.Code == scanerr.ResourceLimit && d.Incomplete {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("diagnostic: %s", extra5149JSON(t, r.Diagnostics))
	}
}

func TestRPMCancelAndMalformedStayClassified(t *testing.T) {
	raw, err := os.ReadFile("testdata/rpm/rpmdb.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		fs := fstest.MapFS{"var/lib/rpm/rpmdb.sqlite": &fstest.MapFile{Data: raw, Mode: 0444}}
		r, err := ScanReport(ctx, fs)
		if r == nil || err == nil || r.Complete {
			t.Fatalf("cancel: complete=%v err=%v", r.Complete, err)
		}
		if !errors.Is(err, scanerr.ErrCancelled) && !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel class: %v", err)
		}
		if scanerr.CodeOf(err) != scanerr.Cancelled {
			t.Fatalf("CodeOf=%s err=%v", scanerr.CodeOf(err), err)
		}
	})
	t.Run("malformed-header", func(t *testing.T) {
		fs := fstest.MapFS{"var/lib/rpm/Packages": &fstest.MapFile{Data: []byte("not-an-rpm-database"), Mode: 0444}}
		r, err := ScanReport(context.Background(), fs)
		if r == nil {
			t.Fatal("nil report")
		}
		if r.Complete || err == nil {
			t.Fatalf("malformed must be incomplete: complete=%v err=%v", r.Complete, err)
		}
		if !errors.Is(err, scanerr.ErrMalformedInput) {
			t.Fatalf("want malformed_input, got %v code=%s", err, scanerr.CodeOf(err))
		}
		if errors.Is(err, scanerr.ErrResourceLimit) || scanerr.CodeOf(err) == scanerr.ResourceLimit {
			t.Fatalf("malformed became resource_limit: %v", err)
		}
		seen := false
		for _, d := range r.Diagnostics {
			if d.Code == scanerr.ResourceLimit {
				t.Fatalf("malformed diagnostic resource_limit: %s", extra5149JSON(t, r.Diagnostics))
			}
			if d.Code == scanerr.MalformedInput && d.Incomplete {
				seen = true
			}
		}
		if !seen {
			t.Fatalf("missing malformed diagnostic: %s", extra5149JSON(t, r.Diagnostics))
		}
	})
}

func TestRPMMultiProviderCandidatesAreNotChosen(t *testing.T) {
	mk := func(name, ver, provide string) *dxtypes.Package {
		p := &dxtypes.Package{Name: name, Version: ver}
		p.EnsureDetails()
		p.Ecosystem = "rpm"
		p.Evidence = "installed"
		p.Provides = []string{provide}
		p.Instance = name
		return p
	}
	a := mk("a", "1", "foo = 1.0")
	b := mk("b", "1", "foo = 2.0")
	c := mk("c", "1", "c")
	c.Requirements = []model.Requirement{{Target: "foo", Constraint: ">= 1.0", Operator: "and"}}
	for _, order := range [][]*dxtypes.Package{{a, b, c}, {c, b, a}, {b, c, a}} {
		r := &model.Report{Complete: true}
		if errs := fillReport(r, append([]*dxtypes.Package(nil), order...), ResourceLimits{MaxComponents: 100, MaxObservations: 100, MaxEdges: 1000}, nil); len(errs) != 0 {
			t.Fatal(errs)
		}
		want := map[string]struct{}{}
		for _, o := range r.Observations {
			if o.NativeID == "a" || o.NativeID == "b" {
				want[o.ID()] = struct{}{}
			}
		}
		if len(want) != 2 {
			t.Fatalf("want two provider observations, got %s", extra5149JSON(t, r.Observations))
		}
		got := map[string]struct{}{}
		nFoo := 0
		for _, q := range r.Requirements {
			if q.Target != "foo" {
				continue
			}
			nFoo++
			if len(q.Resolved) != 0 {
				t.Fatalf("chose a provider: %+v", q)
			}
			for _, id := range q.Candidates {
				got[id] = struct{}{}
			}
		}
		if nFoo != 1 {
			t.Fatalf("foo requirements=%d report=%s", nFoo, extra5149JSON(t, r.Requirements))
		}
		if len(got) != len(want) {
			t.Fatalf("candidates %v want %v report=%s", got, want, extra5149JSON(t, r.Requirements))
		}
		for id := range want {
			if _, ok := got[id]; !ok {
				t.Fatalf("missing provider candidate %s got=%v report=%s", id, got, extra5149JSON(t, r.Requirements))
			}
		}
		for id := range got {
			if _, ok := want[id]; !ok {
				t.Fatalf("extra candidate %s got=%v want=%v", id, got, want)
			}
		}
	}
}
