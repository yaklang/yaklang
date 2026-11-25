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
		})
		require.Nil(t, decision)
	})

	t.Run("bash-dangerous-denied", func(t *testing.T) {
		decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
		})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("bash-tool-ignores-shell-param", func(t *testing.T) {
		decision := securityRuleGate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
			"shell":   "powershell",
		})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("builtin-disabled-by-config", func(t *testing.T) {
		gate := securityRuleGateWithOptions([]aicommon.BashRuleCheckOption{
			aicommon.WithDisableBuiltinBashRules(true),
		})
		require.Nil(t, gate(ctx, "bash", aitool.InvokeParams{
			"command": "rm -rf /",
		}))
	})

	t.Run("other-tool-ignored", func(t *testing.T) {
		decision := securityRuleGate(ctx, "write_file", aitool.InvokeParams{
			"command": "rm -rf /",
		})
		require.Nil(t, decision)
	})
}
