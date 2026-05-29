package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestCorePlugin_CompileAll(t *testing.T) {
	if testing.Short() {
		t.Skip("coreplugin compile sweep is slow")
	}

	t.Setenv("YAKIT_HOME", t.TempDir())
	coreplugin.InitDBForTest()

	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("profile database is not initialized")
	}
	plugins := yakit.QueryYakScriptByIsCore(db, true)
	if len(plugins) == 0 {
		t.Fatal("no coreplugin found in yakit database")
	}

	repoRoot := RepoRoot(t)
	EnsureRuntimeArchive(t, repoRoot)
	tmpDir := t.TempDir()

	var compiled, failed []string
	for _, plugin := range plugins {
		plugin := plugin
		t.Run(plugin.ScriptName, func(t *testing.T) {
			out := filepath.Join(tmpDir, plugin.ScriptName)
			_, err := compiler.CompileToExecutable(
				compiler.WithCompileSourceCode(plugin.Content),
				compiler.WithCompileLanguage("yak"),
				compiler.WithCompileEntryFunction("main"),
				compiler.WithCompileOutputFile(out),
			)
			if err != nil {
				failed = append(failed, plugin.ScriptName)
				t.Fatalf("compile %s: %v", plugin.ScriptName, err)
			}
			compiled = append(compiled, plugin.ScriptName)
		})
	}

	t.Logf("coreplugin compile summary: ok=%d fail=%d", len(compiled), len(failed))
	if len(failed) > 0 {
		t.Fatalf("failed to compile %d coreplugins: %v", len(failed), failed)
	}
}

func TestCorePlugin_RunResetKnowledgeBase(t *testing.T) {
	code := string(coreplugin.GetCorePluginData("重置知识库"))
	require.NotEmpty(t, code)

	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withCompilePluginType(compiler.YakPluginTypeYak))
	require.Contains(t, output, "请确认重置操作")
}
