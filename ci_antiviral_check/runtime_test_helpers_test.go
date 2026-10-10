package ci_antiviral_check

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestYakRuntimeExcludesTestHelpers guards the production import boundary. A
// helper in an ordinary .go file can otherwise silently enter release builds.
func TestYakRuntimeExcludesTestHelpers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "./common/yak/cmd/yak.go")
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list production dependencies: %v\n%s", err, output)
	}
	for _, pkg := range strings.Fields(string(output)) {
		if pkg == "testing" || strings.HasPrefix(pkg, "github.com/stretchr/testify/") ||
			pkg == "github.com/yaklang/yaklang/common/syntaxflow/sfanalysis/sfanalysistest" ||
			pkg == "github.com/yaklang/yaklang/common/ai/rag/ragtest" {
			t.Errorf("production engine imports test helper %s", pkg)
		}
	}
}
