package sca

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"testing"
)

func TestPackageSortBudgetAndIdentity(t *testing.T) {
	a, b := &dxtypes.Package{Name: "a"}, &dxtypes.Package{Name: "b"}
	pkgs := []*dxtypes.Package{b, a}
	low := budget.From(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 1}))
	if err := sortPackagesByIdentifier(pkgs, low); scanerr.CodeOf(err) != scanerr.ResourceLimit {
		t.Fatalf("err=%v", err)
	}
	if pkgs[0] != b || pkgs[1] != a {
		t.Fatal("budget failure mutated input")
	}
	st := budget.From(context.Background())
	for _, name := range []string{"b", "changed"} {
		b.Name = name
		if err := sortPackagesByIdentifier(pkgs, st); err != nil {
			t.Fatal(err)
		}
		if pkgs[0].Identifier() > pkgs[1].Identifier() {
			t.Fatal("sort ignored current identity")
		}
	}
	same := &dxtypes.Package{Name: "a"}
	pkgs = []*dxtypes.Package{same, a}
	if err := sortPackagesByIdentifier(pkgs, st); err != nil {
		t.Fatal(err)
	}
	if pkgs[0] != same || pkgs[1] != a {
		t.Fatal("equal identities reordered")
	}
}
