package syntaxflow_scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func writeLiveSourceTree(t *testing.T, dir string) {
	t.Helper()
	write := func(rel, content string) {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	}
	write("main.go", "package main\n")
	write("main_test.go", "package main\n")
	write("internal/handler/handler.go", "package handler\n")
	write("internal/handler/handler_test.go", "package handler\n")
	write("internal/testdata/fixture.go", "package fixture\n")
	write("generated/consts.go", "package generated\n")
}

func liveSourceFiles(t *testing.T, cfg *Config, dir string) map[string]string {
	t.Helper()
	require.NoError(t, attachLiveSourceTarget(cfg, dir))
	require.Len(t, cfg.QueryTargets, 1)
	target, ok := cfg.QueryTargets[0].(*ssaapi.SourceQueryTarget)
	require.True(t, ok, "live source target should be a *ssaapi.SourceQueryTarget")
	return target.Files()
}

func TestAttachLiveSourceTarget_AppliesDefaultCompileExcludes(t *testing.T) {
	dir := t.TempDir()
	writeLiveSourceTree(t, dir)

	cfg, err := NewConfig()
	require.NoError(t, err)

	files := liveSourceFiles(t, cfg, dir)
	require.Contains(t, files, "main.go")
	require.Contains(t, files, "internal/handler/handler.go")
	require.NotContains(t, files, "main_test.go",
		"the source stage must drop root-level *_test.go like the compile walk does")
	require.NotContains(t, files, "internal/handler/handler_test.go",
		"the source stage must drop nested *_test.go like the compile walk does")
	require.NotContains(t, files, "internal/testdata/fixture.go",
		"the source stage must drop testdata directories like the compile walk does")
}

func TestAttachLiveSourceTarget_UserExcludePatternsApply(t *testing.T) {
	dir := t.TempDir()
	writeLiveSourceTree(t, dir)

	cfg, err := NewConfig()
	require.NoError(t, err)
	cfg.SetCompileExcludeFiles([]string{"generated/**"})

	files := liveSourceFiles(t, cfg, dir)
	require.NotContains(t, files, "generated/consts.go",
		"user --exclude-file patterns must scope the source stage too")
	require.NotContains(t, files, "internal/handler/handler_test.go",
		"built-in defaults must still apply alongside user patterns")
	require.Contains(t, files, "main.go")
}

func TestAttachLiveSourceTarget_DisableDefaultExcludesKeepsTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeLiveSourceTree(t, dir)

	cfg, err := NewConfig()
	require.NoError(t, err)
	cfg.SSACompile = &ssaconfig.SSACompileConfig{DisableDefaultExcludes: true}

	files := liveSourceFiles(t, cfg, dir)
	require.Contains(t, files, "main_test.go",
		"WithDisableDefaultCompileExcludes must restore the full source scope")
	require.Contains(t, files, "internal/handler/handler_test.go")
	require.Contains(t, files, "internal/testdata/fixture.go")
}
