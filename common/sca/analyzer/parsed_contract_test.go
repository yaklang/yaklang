package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"testing"
)

func TestUnresolvedAndScopedReferences(t *testing.T) {
	pkgs, err := handlerParsed([]types.Library{{ID: "root", Name: "root", Version: "1"}}, []types.Dependency{{ID: "root", DependsOn: []string{"@scope/pkg@^0.2.3", "malformed", "@scope/without-version"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || len(pkgs[0].UnresolvedDependencies) != 3 || len(pkgs[0].UpStreamPackages) != 0 {
		t.Fatalf("fabricated component or lost reference: %+v", pkgs)
	}
}
