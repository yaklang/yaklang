package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

func TestCheckBashCommand(t *testing.T) {
	t.Run("safe-ls", func(t *testing.T) {
		result := CheckBashCommand(context.Background(), "ls -la")
		require.True(t, result.Available)
		require.Equal(t, schema.SFR_ACTION_ALLOW, result.Action)
		require.False(t, result.Blocked())
	})

	t.Run("rm-root", func(t *testing.T) {
		result := CheckBashCommand(context.Background(), "rm -rf /")
		require.True(t, result.Available)
		require.Equal(t, schema.SFR_ACTION_DENY, result.Action)
		require.True(t, result.Blocked())
		blocked := result.BlockedResult("bash")
		require.Equal(t, "blocked_by_bash_guard", blocked["termination_reason"])
		require.Equal(t, "deny", blocked["verdict"])
		require.NotContains(t, blocked, "check")
	})

	t.Run("disabled", func(t *testing.T) {
		t.Setenv(disableBashRuleEnv, "1")
		result := CheckBashCommand(context.Background(), "rm -rf /")
		require.False(t, result.Blocked())
		require.Equal(t, schema.SFR_ACTION_ALLOW, result.Action)
	})
}

func TestCheckBashConfigDisablesGuard(t *testing.T) {
	ctx := context.Background()
	preflight := CheckBashCommand(ctx, "rm -rf /", WithDisableBuiltinBashRules(true))
	require.False(t, preflight.Blocked())
	require.Equal(t, schema.SFR_ACTION_ALLOW, preflight.Action)

	gate := CheckBuiltinBashCommand(ctx, "rm -rf /", WithDisableBuiltinBashRules(true))
	require.False(t, gate.Blocked())
}
