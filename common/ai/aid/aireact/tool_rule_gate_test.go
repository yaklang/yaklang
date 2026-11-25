package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSecurityRuleGate(t *testing.T) {
	ctx := context.Background()

	t.Run("bash-safe-allowed", func(t *testing.T) {
		decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
			"command": "echo hello",
			"shell":   "bash",
		})
		require.Nil(t, decision)
	})

	t.Run("bash-dangerous-denied", func(t *testing.T) {
		decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "bash",
		})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("non-bash-shell-passes-through", func(t *testing.T) {
		for _, shell := range []string{"cmd", "powershell", "pwsh", "unknown-shell"} {
			for _, command := range []string{"echo hello", "rm -rf /"} {
				decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
					"command": command,
					"shell":   shell,
				})
				require.Nil(t, decision, "shell=%s command=%s", shell, command)
			}
		}
	})

	t.Run("bash-exe-alias-still-denied", func(t *testing.T) {
		decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "bash.exe",
		})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("builtin-disabled-by-config", func(t *testing.T) {
		gate := securityRuleGateWithOptions([]aicommon.BashCommandCheckOption{
			aicommon.WithDisableBuiltinBashRules(true),
		})
		require.Nil(t, gate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "bash",
		}))
		require.Nil(t, gate(ctx, "bash", aitool.InvokeParams{
			"command": "echo hello",
			"shell":   "powershell",
		}))
	})

	t.Run("other-tool-ignored", func(t *testing.T) {
		decision := securityRuleGate(ctx, "write_file", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "bash",
		})
		require.Nil(t, decision)
	})
}
