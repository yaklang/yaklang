package bundler

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"strings"
	"testing"
)

func TestNativeIdentityFieldBoundaries(t *testing.T) {
	for _, sep := range []string{"\x00", ":", "é"} {
		t.Run(sep, func(t *testing.T) {
			raw := "GEM\n  remote: https://example.invalid/\n  specs:\n    a" + sep + "b (c)\n      leaf (>= 1)\n    a (b" + sep + "c)\n      leaf (< 2)\n    leaf (1)\n"
			libs, deps, err := NewParser().Parse(nil, strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(libs) != 3 || len(deps) != 2 {
				t.Fatalf("libs=%+v deps=%+v", libs, deps)
			}
			ids := map[string]string{}
			for _, lib := range libs {
				ids[lib.Name] = lib.ID
			}
			if ids["a"] == ids["a"+sep+"b"] {
				t.Fatal("identity field boundary collision")
			}
			constraints := map[string]string{ids["a"+sep+"b"]: ">= 1", ids["a"]: "< 2"}
			for _, dep := range deps {
				if len(dep.Requirements) != 1 {
					t.Fatalf("requirements=%+v", dep)
				}
				q := dep.Requirements[0]
				if q.Target != "leaf" || q.Constraint != constraints[dep.ID] || q.Resolved != ids["leaf"] {
					t.Fatalf("misassociated dependency: %+v", dep)
				}
			}
		})
	}
}

func TestNativeIdentityBudgetBeforeBuild(t *testing.T) {
	st := budget.From(budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 8}))
	id, err := bundlerNativeID(st, "a", "1", "source", "")
	if id != "" || scanerr.CodeOf(err) != scanerr.ResourceLimit {
		t.Fatalf("id=%q err=%v", id, err)
	}
}
