package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/bash/commandcheck"
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

	t.Run("non-bash-safe-requires-review", func(t *testing.T) {
		for _, shell := range []string{"cmd", "powershell", "pwsh", "unknown-shell"} {
			decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
				"command": "echo hello",
				"shell":   shell,
			})
			require.NotNil(t, decision, "shell=%s", shell)
			require.Equal(t, schema.SFR_ACTION_ASK, decision.Action, "shell=%s", shell)
			require.Equal(t, "shell-static-analysis-unavailable", decision.RuleID, "shell=%s", shell)
		}
	})

	t.Run("non-bash-dangerous-still-denied", func(t *testing.T) {
		decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "powershell",
		})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("builtin-disabled-by-config", func(t *testing.T) {
		gate := securityRuleGateWithOptions([]commandcheck.CheckOption{
			commandcheck.WithDisableBuiltinRules(true),
		})
		require.Nil(t, gate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "bash",
		}))
		require.Nil(t, gate(ctx, "bash", aitool.InvokeParams{
			"command": "echo hello",
			"shell":   "powershell",
		}), "disabled rules must not turn into a non-bash review prompt")
	})

	t.Run("other-tool-ignored", func(t *testing.T) {
		decision := securityRuleGate(ctx, "write_file", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "bash",
		})
		require.Nil(t, decision)
	})
}
