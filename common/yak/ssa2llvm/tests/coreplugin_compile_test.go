package tests

import (
	"os"
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

func TestCorePlugin_RunSSADetectNewConfig(t *testing.T) {
	code := `
yakit.AutoInitYakit()
config, err = ssa.NewConfig(ssa.ModeAll, ssa.withProgramName("t"), ssa.withLanguage("php"))
if err != nil { die("new config: %v", err) }
if config == nil { die("config nil") }
_, err = config.ToJSONString()
if err != nil { die("to json: %v", err) }
println("detect-config-ok")
`
	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withCompilePluginType(compiler.YakPluginTypeYak))
	require.Contains(t, output, "detect-config-ok")
}

func TestCorePlugin_RunSSADetectLocalProject(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := string(coreplugin.GetCorePluginData("SSA 项目探测"))
	require.NotEmpty(t, code)

	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withCompilePluginType(compiler.YakPluginTypeYak), withArgs("--target", projectDir, "--compile-immediately", "--language", "php"))
	require.Contains(t, output, `"compile_immediately": true`)
	require.Contains(t, output, `"kind": "local"`)
	require.Contains(t, output, `"language": "php"`)
	require.Contains(t, output, `"local_file": "`+projectDir+`"`)
	require.Contains(t, output, `"file_count": 1`)
	require.Contains(t, output, `"program_names": [`)
	require.Contains(t, output, `"project_name": "001"`)
	require.Contains(t, output, `"project_exists": false`)
	require.NotContains(t, output, "YakVM Code DIE")
	require.NotContains(t, output, "unexpected end of JSON input")
}

func TestCorePlugin_RunSSADetectDefaultCompileImmediatelyFalse(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "index.php"), []byte("<?php echo 'ok';"), 0o644))

	code := string(coreplugin.GetCorePluginData("SSA 项目探测"))
	require.NotEmpty(t, code)

	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withCompilePluginType(compiler.YakPluginTypeYak), withArgs("--target", projectDir, "--language", "php"))
	require.Contains(t, output, `"compile_immediately": false`)
	require.NotContains(t, output, `"compile_immediately": true`)
	require.NotContains(t, output, "YakVM Code DIE")
	require.NotContains(t, output, "unexpected end of JSON input")
}

func TestCorePlugin_RunResetKnowledgeBase(t *testing.T) {
	code := string(coreplugin.GetCorePluginData("重置知识库"))
	require.NotEmpty(t, code)

	output := runBinaryWithEnv(t, code, "", map[string]string{
		"YAKIT_HOME": t.TempDir(),
	}, withCompilePluginType(compiler.YakPluginTypeYak))
	require.Contains(t, output, "请确认重置操作")
}
