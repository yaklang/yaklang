package tests

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMustPass_SSA2LLVM_Simple compiles and runs every script under
// tests/mustpass_simple with the real ssa2llvm CLI in AOT binary mode,
// asserting exit 0 and no runtime panic. Each file is a minimal regression
// case distilled from a concrete mustpass failure (semantic mismatches,
// compiler crashes, runtime panics), so a fix is always paired with a
// self-contained reproducer that stays green.
func TestMustPass_SSA2LLVM_Simple(t *testing.T) {
	repoRoot := RepoRoot(t)
	simpleDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "tests", "mustpass_simple")

	entries, err := os.ReadDir(simpleDir)
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yak") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	require.NotEmpty(t, names, "mustpass_simple must contain at least one .yak regression case")

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			script := filepath.Join(simpleDir, name)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			output := RunYakScriptFileWithCLITimeout(t, ctx, script, nil)
			if strings.Contains(output, "panic") {
				t.Fatalf("ssa2llvm run of %s produced a runtime panic:\n%s", name, output)
			}
		})
	}
}
