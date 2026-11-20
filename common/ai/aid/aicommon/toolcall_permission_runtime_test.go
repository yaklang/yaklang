package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestClaudeCodePermissionRuntimeConfig(t *testing.T) {
	t.Run("env-single-document", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, `{"permissions":{"deny":["Bash(rm -rf *)"]}}`)
		gate := CompileClaudeCodePermissionsFromRuntime()
		require.NotNil(t, gate)

		decision := gate(context.Background(), "bash", aitool.InvokeParams{"command": "rm -rf /tmp/x"})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("env-array-splices-documents", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, `[
			{"permissions":{"allow":["Bash(git *)"]}},
			{"permissions":{"deny":["Bash(git push *)"]}}
		]`)
		gate := CompileClaudeCodePermissionsFromRuntime()
		require.NotNil(t, gate)

		decision := gate(context.Background(), "bash", aitool.InvokeParams{"command": "git push origin main"})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("invalid-document-does-not-panic", func(t *testing.T) {
		t.Setenv(EnvToolCallPermissions, `{"permissions":{"deny":["Bash(rm -rf *"]}}`)
		require.Nil(t, CompileClaudeCodePermissionsFromRuntime())
	})
}
