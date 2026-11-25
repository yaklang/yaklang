package aicommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRulePermissionRuntimeConfig(t *testing.T) {
	t.Run("env-document", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, `{"permissions":{"deny":["Read(./.env)"]}}`)
		t.Setenv(EnvToolCallPermissionsFile, "")
		runtimePermissionCache.mu.Lock()
		runtimePermissionCache.key = ""
		runtimePermissionCache.gate = nil
		runtimePermissionCache.bashRules = nil
		runtimePermissionCache.mu.Unlock()

		gate := CompileRulePermissionsFromRuntime()
		require.NotNil(t, gate)
	})

	t.Run("file-document", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "perm.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"permissions":{"deny":["Read(./.env)"]}}`), 0o644))
		t.Setenv(EnvToolCallPermissions, "")
		t.Setenv(EnvToolCallPermissionsFile, path)
		runtimePermissionCache.mu.Lock()
		runtimePermissionCache.key = ""
		runtimePermissionCache.gate = nil
		runtimePermissionCache.bashRules = nil
		runtimePermissionCache.mu.Unlock()

		gate := CompileRulePermissionsFromRuntime()
		require.NotNil(t, gate)
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, "")
		t.Setenv(EnvToolCallPermissionsFile, "")
		runtimePermissionCache.mu.Lock()
		runtimePermissionCache.key = "force-miss"
		runtimePermissionCache.gate = nil
		runtimePermissionCache.bashRules = nil
		runtimePermissionCache.mu.Unlock()
		require.Nil(t, CompileRulePermissionsFromRuntime())
	})
}
