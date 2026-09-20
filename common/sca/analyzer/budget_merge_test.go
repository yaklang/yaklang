package analyzer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/model"
)

func TestMergePackagesResultBudget(t *testing.T) {
	l, err := (budget.Limits{MaxResultBytes: 256}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	st := budget.From(budget.Bind(context.Background(), l))
	pkgs := make([]*dxtypes.Package, 40)
	for i := range pkgs {
		pkgs[i] = &dxtypes.Package{Name: fmt.Sprintf("pkg-%d", i), Version: "1.0.0"}
	}
	out, err := mergePackagesBudget(st, pkgs)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("adapter without context must still use a bounded default internally; small budget: out=%d err=%v", len(out), err)
	}
	if out != nil {
		t.Fatalf("budget path must not return a merged slice on error: %d", len(out))
	}
}

func TestMergePackagesBudgetDoesNotMutateThenRollback(t *testing.T) {
	first := &dxtypes.Package{
		Name:         "dup",
		Version:      "1.0.0",
		License:      []string{"MIT"},
		FromFile:     []string{"a.go"},
		FromAnalyzer: []string{"one"},
	}
	second := &dxtypes.Package{
		Name:         "dup",
		Version:      "1.0.0",
		License:      []string{"BSD"},
		FromFile:     []string{"b.go"},
		FromAnalyzer: []string{"two"},
		PackageDetails: &dxtypes.PackageDetails{
			RawLicenses: []string{"BSD"},
			Evidence:    "",
		},
	}
	up := &dxtypes.Package{Name: "up", Version: "1.0.0"}
	first.LinkDepend(up)
	extra := &dxtypes.Package{Name: "extra", Version: "1.0.0"}
	pkgs := []*dxtypes.Package{first, second, extra}

	wantLic := append([]string(nil), first.License...)
	wantFile := append([]string(nil), first.FromFile...)
	wantAn := append([]string(nil), first.FromAnalyzer...)
	if first.PackageDetails != nil {
		t.Fatal("first identity must start without merged details")
	}
	if len(first.UpStreamPackages) != 1 {
		t.Fatal("precondition: first identity has one upstream")
	}

	// Map+sort index for 3 pointers is 48+24=72. SizeOfPackage+ptr is ~700.
	// 800 bytes covers the index and the first unique identity (dup) and
	// fails on the later unique key. The discarded-error path used to merge
	// dup's licenses before that failure, then return nil as an empty set.
	l, err := (budget.Limits{MaxResultBytes: 800}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	st := budget.From(budget.Bind(context.Background(), l))
	out, err := MergePackagesBudget(st, pkgs)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("expected resource_limit after first identity, out=%d err=%v", len(out), err)
	}
	if out != nil {
		t.Fatalf("error path must not return a merged result: %#v", out)
	}
	if got := fmt.Sprint(first.License); got != fmt.Sprint(wantLic) {
		t.Fatalf("license mutated before failed merge: %s", got)
	}
	if got := fmt.Sprint(first.FromFile); got != fmt.Sprint(wantFile) {
		t.Fatalf("FromFile mutated before failed merge: %s", got)
	}
	if got := fmt.Sprint(first.FromAnalyzer); got != fmt.Sprint(wantAn) {
		t.Fatalf("FromAnalyzer mutated before failed merge: %s", got)
	}
	if first.PackageDetails != nil {
		t.Fatal("MergeDetails ran before charges finished")
	}
	if len(first.UpStreamPackages) != 1 || first.UpStreamPackages[up.Identifier()] != up {
		t.Fatal("graph cleared or rewritten on failed merge")
	}

	kept := mergePackagesOrKeep(budget.From(budget.Bind(context.Background(), l)), pkgs)
	if len(kept) != 3 || kept[0] != first || kept[1] != second || kept[2] != extra {
		t.Fatalf("compatibility path must return the original slice, not nil/empty: %#v", kept)
	}

	ok, err := MergePackagesBudget(budget.From(context.Background()), pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ok) != 2 {
		t.Fatalf("successful merge identities: %d", len(ok))
	}
	var merged *dxtypes.Package
	for _, p := range ok {
		if p.Name == "dup" {
			merged = p
		}
	}
	if merged == nil {
		t.Fatal("dup identity missing after successful merge")
	}
	if fmt.Sprint(merged.License) != fmt.Sprint([]string{"MIT", "BSD"}) {
		t.Fatalf("successful merge licenses: %v", merged.License)
	}
	again, err := MergePackagesBudget(budget.From(context.Background()), ok)
	if err != nil {
		t.Fatal(err)
	}
	var mergedAgain *dxtypes.Package
	for _, p := range again {
		if p.Name == "dup" {
			mergedAgain = p
		}
	}
	if mergedAgain == nil || fmt.Sprint(mergedAgain.License) != fmt.Sprint([]string{"MIT", "BSD"}) {
		t.Fatalf("retry after success double-appended licenses: %v", mergedAgain)
	}
}

func TestMergePackagesAliasDoesNotDouble(t *testing.T) {
	p := &dxtypes.Package{Name: "a", Version: "1", License: []string{"MIT"}, FromFile: []string{"lock"}}
	out := MergePackages([]*dxtypes.Package{p, p})
	if len(out) != 1 || out[0] != p {
		t.Fatalf("alias merge identities: %#v", out)
	}
	if len(p.License) != 1 || p.License[0] != "MIT" {
		t.Fatalf("same pointer doubled licenses: %#v", p.License)
	}
	if len(p.FromFile) != 1 || p.FromFile[0] != "lock" {
		t.Fatalf("same pointer doubled FromFile: %#v", p.FromFile)
	}

	up := &dxtypes.Package{Name: "up", Version: "1"}
	a := &dxtypes.Package{
		Name:     "a",
		Version:  "1",
		License:  []string{"MIT"},
		FromFile: []string{"a.lock"},
		PackageDetails: &dxtypes.PackageDetails{
			Locations:    []dxtypes.SourceRange{{StartLine: 1, EndLine: 1}},
			Requirements: []model.Requirement{{Target: "x", Constraint: "1"}},
		},
	}
	b := &dxtypes.Package{
		Name:     "a",
		Version:  "1",
		License:  []string{"BSD"},
		FromFile: []string{"b.lock"},
		PackageDetails: &dxtypes.PackageDetails{
			Locations:    []dxtypes.SourceRange{{StartLine: 2, EndLine: 2}},
			Requirements: []model.Requirement{{Target: "y", Constraint: "2"}},
		},
	}
	a.LinkDepend(up)
	b.LinkDepend(up)
	got := MergePackages([]*dxtypes.Package{a, b, a, b, a})
	if len(got) != 1 || got[0] != a {
		t.Fatalf("interleaved identities: %#v", got)
	}
	merged := got[0]
	if len(merged.License) != 2 || merged.License[0] != "MIT" || merged.License[1] != "BSD" {
		t.Fatalf("two instances must contribute once: %#v", merged.License)
	}
	if len(merged.FromFile) != 2 || merged.FromFile[0] != "a.lock" || merged.FromFile[1] != "b.lock" {
		t.Fatalf("FromFile doubled or dropped: %#v", merged.FromFile)
	}
	if len(merged.Locations) != 2 || merged.Locations[0] != (dxtypes.SourceRange{StartLine: 1, EndLine: 1}) || merged.Locations[1] != (dxtypes.SourceRange{StartLine: 2, EndLine: 2}) {
		t.Fatalf("Locations doubled or dropped: %#v", merged.Locations)
	}
	if len(merged.Requirements) != 2 || merged.Requirements[0].Target != "x" || merged.Requirements[1].Target != "y" {
		t.Fatalf("Requirements doubled or dropped: %#v", merged.Requirements)
	}
	if len(merged.UpStreamPackages) != 1 || merged.UpStreamPackages[up.Identifier()] != up {
		t.Fatalf("graph doubled or dropped: %#v", merged.UpStreamPackages)
	}
	if len(b.License) != 1 || b.License[0] != "BSD" {
		t.Fatalf("source instance mutated: %#v", b.License)
	}
	again := MergePackages(got)
	if len(again) != 1 {
		t.Fatal("retry changed identity count")
	}
	for _, x := range again {
		if x.Name == "a" && (len(x.License) != 2 || len(x.FromFile) != 2 || len(x.Locations) != 2 || len(x.Requirements) != 2 || len(x.UpStreamPackages) != 1) {
			t.Fatalf("retry doubled evidence: license=%d file=%d loc=%d req=%d up=%d", len(x.License), len(x.FromFile), len(x.Locations), len(x.Requirements), len(x.UpStreamPackages))
		}
	}
}

func TestMergePackagesBudgetSameIdentityMetadata(t *testing.T) {
	up := &dxtypes.Package{Name: "up", Version: "1"}
	n := 12
	pkgs := make([]*dxtypes.Package, 0, n)
	for i := 0; i < n; i++ {
		p := &dxtypes.Package{
			Name:         "dup",
			Version:      "1.0.0",
			License:      []string{fmt.Sprintf("L%d", i)},
			FromFile:     []string{fmt.Sprintf("f%d", i)},
			FromAnalyzer: []string{fmt.Sprintf("a%d", i)},
			PackageDetails: &dxtypes.PackageDetails{
				Locations:    []dxtypes.SourceRange{{StartLine: i + 1, EndLine: i + 1}},
				Requirements: []model.Requirement{{Target: fmt.Sprintf("t%d", i)}},
				Diagnostics:  []model.Diagnostic{{Code: "malformed_input", Reason: fmt.Sprintf("r%d", i)}},
			},
		}
		p.LinkDepend(up)
		pkgs = append(pkgs, p)
	}
	lic0 := append([]string(nil), pkgs[0].License...)
	upCount := len(pkgs[0].UpStreamPackages)

	// Unique identities are only dup+up. A unique-name precheck would
	// accept a few kilobytes; extra same-identity evidence/edges must not.
	l, err := (budget.Limits{MaxResultBytes: 2500}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	st := budget.From(budget.Bind(context.Background(), l))
	out, err := MergePackagesBudget(st, pkgs)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("same-identity metadata must be charged: out=%d err=%v", len(out), err)
	}
	if out != nil {
		t.Fatal("error path returned a merged slice")
	}
	if fmt.Sprint(pkgs[0].License) != fmt.Sprint(lic0) || len(pkgs[0].UpStreamPackages) != upCount {
		t.Fatal("small budget mutated same-identity input")
	}
	for i, p := range pkgs {
		if len(p.License) != 1 || p.License[0] != fmt.Sprintf("L%d", i) {
			t.Fatalf("record %d licenses mutated: %#v", i, p.License)
		}
		if len(p.Locations) != 1 || len(p.Requirements) != 1 {
			t.Fatalf("record %d details mutated", i)
		}
		if len(p.UpStreamPackages) != 1 {
			t.Fatalf("record %d graph mutated", i)
		}
	}

	ok, err := MergePackagesBudget(budget.From(context.Background()), pkgs)
	if err != nil {
		t.Fatal(err)
	}
	var merged *dxtypes.Package
	for _, p := range ok {
		if p.Name == "dup" {
			merged = p
		}
	}
	if merged == nil || len(merged.License) != n || len(merged.FromFile) != n || len(merged.Locations) != n || len(merged.Requirements) != n || len(merged.Diagnostics) != n {
		t.Fatalf("positive merge dropped evidence: %#v", merged)
	}
	if merged.License[0] != "L0" || merged.License[n-1] != fmt.Sprintf("L%d", n-1) {
		t.Fatalf("license order: %#v", merged.License)
	}
	if len(merged.UpStreamPackages) != 1 {
		t.Fatalf("edges: %d", len(merged.UpStreamPackages))
	}
	kept := mergePackagesOrKeep(budget.From(budget.Bind(context.Background(), l)), pkgs)
	if len(kept) != n || kept[0] != pkgs[0] {
		t.Fatal("compatibility path must keep original records")
	}
}

func TestHandlerParsedResultBudget(t *testing.T) {
	l, err := (budget.Limits{MaxResultBytes: 256}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	ctx := budget.Bind(context.Background(), l)
	libs := make(types.Libraries, 40)
	for i := range libs {
		libs[i] = types.Library{Name: fmt.Sprintf("pkg-%d", i), Version: "1.0.0", ID: fmt.Sprintf("id-%d", i)}
	}
	_, err = handlerParsedBudget(ctx, libs, nil)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("merge DTO created without result-memory precheck: %v", err)
	}
}
