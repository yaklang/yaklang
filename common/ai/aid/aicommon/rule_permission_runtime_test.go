package aicommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRulePermissionRuntimeConfig(t *testing.T) {
	t.Run("env-document", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, `{"permissions":{"deny":["Bash(rm -rf *)","Read(./.env)"]}}`)
		t.Setenv(EnvToolCallPermissionsFile, "")
		runtimeBashPermissionCache.mu.Lock()
		runtimeBashPermissionCache.key = ""
		runtimeBashPermissionCache.rules = nil
		runtimeBashPermissionCache.mu.Unlock()

		rules := loadRuntimeBashPermissionSyntaxFlowRules()
		require.Len(t, rules, 1)
		require.Contains(t, rules[0].Content, `/^rm$/ as $cmd`)
	})

	t.Run("file-document", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "perm.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"permissions":{"deny":["Bash(rm -rf *)"]}}`), 0o644))
		t.Setenv(EnvToolCallPermissions, "")
		t.Setenv(EnvToolCallPermissionsFile, path)
		runtimeBashPermissionCache.mu.Lock()
		runtimeBashPermissionCache.key = ""
		runtimeBashPermissionCache.rules = nil
		runtimeBashPermissionCache.mu.Unlock()

		rules := loadRuntimeBashPermissionSyntaxFlowRules()
		require.Len(t, rules, 1)
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, "")
		t.Setenv(EnvToolCallPermissionsFile, "")
		runtimeBashPermissionCache.mu.Lock()
		runtimeBashPermissionCache.key = "force-miss"
		runtimeBashPermissionCache.rules = nil
		runtimeBashPermissionCache.mu.Unlock()
		require.Empty(t, loadRuntimeBashPermissionSyntaxFlowRules())
	})
}
