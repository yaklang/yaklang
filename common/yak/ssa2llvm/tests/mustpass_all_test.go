package tests

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
)

// TestMustPass_SSA2LLVM_AllScripts mirrors common/yak/yaktest/mustpass's
// TestMustPass: every script under common/yak/yaktest/mustpass/files must run
// successfully. The yakvm suite executes them through yak.Execute; this test
// compiles each script with the real ssa2llvm CLI and runs the produced native
// binary (AOT mode) with the same VULINBOX environment, asserting exit 0 and no
// runtime panic.
func TestMustPass_SSA2LLVM_AllScripts(t *testing.T) {
	repoRoot := RepoRoot(t)
	mpDir := filepath.Join(repoRoot, "common", "yak", "yaktest", "mustpass", "files")

	entries, err := os.ReadDir(mpDir)
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yak") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	require.NotEmpty(t, names)

	// Same environment as the yakvm mustpass suite: a live vulinbox server.
	addr, err := vulinbox.NewVulinServer(context.Background())
	require.NoError(t, err, "vulinbox must start for mustpass scripts")
	env := map[string]string{
		"VULINBOX":      addr,
		"VULINBOX_HOST": utils.ExtractHostPort(addr),
		"YAKIT_HOME":    t.TempDir(),
	}

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			script := filepath.Join(mpDir, name)
			output := RunYakScriptFileWithCLI(t, script, env)
			if strings.Contains(output, "panic") {
				t.Fatalf("ssa2llvm run of %s produced a runtime panic:\n%s", name, output)
			}
		})
	}
}
