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
	home := t.TempDir()
	env := map[string]string{
		"VULINBOX":      addr,
		"VULINBOX_HOST": utils.ExtractHostPort(addr),
		"YAKIT_HOME":    home,
	}
	// Scripts reference VULINBOX directly (f`${VULINBOX}/...`); yak.Execute
	// injects it as a var, so the AOT CLI must see it at compile time too.
	t.Setenv("VULINBOX", addr)
	t.Setenv("VULINBOX_HOST", utils.ExtractHostPort(addr))
	primeMustPassYakitHome(t, home, env)

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			script := filepath.Join(mpDir, name)
			output := RunYakScriptFileWithCLITimeout(t, 90*time.Second, script, env)
			if strings.Contains(output, "panic") {
				t.Fatalf("ssa2llvm run of %s produced a runtime panic:\n%s", name, output)
			}
		})
	}
}

// primeMustPassYakitHome fills the shared profile database once.
// A cold CallPostInitDatabase parses the embedded AI tools and core plugins
// locally (73 tools changed, 49 plugins changed; measured 5m22s, no network).
// The same binary against that home then spends about 3s in post-init.
// Each script below keeps the 90s run budget; this prime is the one-time cost.
func primeMustPassYakitHome(t *testing.T, home string, env map[string]string) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "prime-yakit.yak")
	// log is not in the core or net tiers, so this binary links the runtime
	// that opens the real yakit database during startup.
	err := os.WriteFile(script, []byte("log.Info(\"prime yakit database\")\n"), 0o644)
	require.NoError(t, err)
	started := time.Now()
	output := RunYakScriptFileWithCLITimeout(t, 8*time.Minute, script, env)
	if strings.Contains(output, "panic") {
		t.Fatalf("yakit prime panicked:\n%s", output)
	}
	t.Logf("primed YAKIT_HOME %s in %s", home, time.Since(started))
}
