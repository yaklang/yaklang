package sca

import (
	"context"
	"errors"
	"os"
	"sort"
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
	body := extra5149JSON(t, r)
	if !strings.Contains(body, "config(mariner-release)") || !strings.Contains(body, "2.0-4.cm2") {
		t.Fatal("SBOM/report dump lost original qualifier")
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
		if err != nil && errors.Is(err, scanerr.ErrResourceLimit) {
			t.Fatalf("malformed became resource_limit: %v", err)
		}
		for _, d := range r.Diagnostics {
			if d.Code == scanerr.ResourceLimit {
				t.Fatalf("malformed diagnostic resource_limit: %s", extra5149JSON(t, r.Diagnostics))
			}
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
		var got []string
		for _, q := range r.Requirements {
			if q.Target == "foo" {
				got = append(got, q.Candidates...)
			}
		}
		sort.Strings(got)
		if len(got) < 2 {
			t.Fatalf("expected both providers as candidates, got %v report=%s", got, extra5149JSON(t, r.Requirements))
		}
		for _, q := range r.Requirements {
			if q.Target == "foo" && len(q.Resolved) != 0 {
				t.Fatalf("chose a provider: %+v", q)
			}
		}
	}
}
