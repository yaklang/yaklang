package dxtypes

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

func TestPhaseScopedLinker(t *testing.T) {
	a, b := &Package{Name: "a", Version: "1"}, &Package{Name: "b", Version: "2"}
	for phase := 0; phase < 2; phase++ {
		l := NewLinker(budget.From(budget.Ensure(context.Background())))
		if err := l.Link(a, b); err != nil {
			t.Fatal(err)
		}
		if err := l.Link(a, b); err != nil {
			t.Fatal(err)
		}
		if len(a.UpStreamPackages) != 1 || len(b.DownStreamPackages) != 1 || a.UpStreamPackages[b.Identifier()] != b || b.DownStreamPackages[a.Identifier()] != a {
			t.Fatal("lost exact bidirectional relationship")
		}
		a.Version = "3"
		b.Version = "4"
		a.UpStreamPackages = nil
		b.DownStreamPackages = nil
	}
	l := NewLinker(budget.From(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 1})))
	if err := l.Link(a, b); err == nil {
		t.Fatal("budget bypass")
	}
	if a.UpStreamPackages != nil || b.DownStreamPackages != nil {
		t.Fatal("partial mutation before reservation")
	}
}
