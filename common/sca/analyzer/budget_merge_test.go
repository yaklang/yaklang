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
