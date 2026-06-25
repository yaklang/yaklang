package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestReconcileLanguage_GoModOverridesJavaHint(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0o644))

	rec, err := ReconcileLanguage(dir, "java")
	require.NoError(t, err)
	require.Equal(t, ssaconfig.GO, rec.Language)
	require.Equal(t, ssaconfig.GO, rec.Detected)
	require.NotEmpty(t, rec.Warnings)
}

func TestReconcileLanguage_EmptyHintUsesDetected(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.21\n"), 0o644))

	rec, err := ReconcileLanguage(dir, "")
	require.NoError(t, err)
	require.Equal(t, ssaconfig.GO, rec.Language)
	require.Equal(t, "detected", rec.Source)
}

func TestResolveLanguage_MatchesReconcile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.21\n"), 0o644))

	lang, err := ResolveLanguage(dir, "java")
	require.NoError(t, err)
	require.Equal(t, ssaconfig.GO, lang)
}
