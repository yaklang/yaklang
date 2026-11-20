package test

import (
	"embed"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
)

//go:embed all:code
var codeFS embed.FS

// TestAllSyntaxForCpp_G4 parses every fixture with the C++ grammar only. It
// guards the grammar and the frontend entry point, not SSA lowering.
func TestAllSyntaxForCpp_G4(t *testing.T) {
	budget := 10 * time.Second
	if raw := os.Getenv("YAK_CPP_FIXTURE_PARSE_BUDGET_SEC"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		require.NoError(t, err, "invalid YAK_CPP_FIXTURE_PARSE_BUDGET_SEC")
		require.Positive(t, seconds, "invalid YAK_CPP_FIXTURE_PARSE_BUDGET_SEC")
		budget = time.Duration(seconds) * time.Second
	}

	entries, err := codeFS.ReadDir("code")
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".cpp" {
			continue
		}
		count++
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile("code/" + name)
			require.NoError(t, err)

			start := time.Now()
			ast, parseErr := cpp2ssa.Frontend(string(raw))
			elapsed := time.Since(start)
			if parseErr != nil {
				t.Fatalf("parse %s failed after %s: %v", name, elapsed, parseErr)
			}
			require.NotNil(t, ast)
			if elapsed > budget {
				t.Fatalf("parse %s took %s, over budget %s", name, elapsed, budget)
			}
		})
	}
	require.Positive(t, count, "no .cpp fixtures found in code/")
}
