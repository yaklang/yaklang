package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// A scan that fails before the manager is constructed must not dereference
// the nil manager in the defer block (source-scan CLI used to panic here).
func TestScan_ManagerNotConstructed_ReturnsErrorWithoutPanic(t *testing.T) {
	t.Run("empty query targets", func(t *testing.T) {
		err := Scan(context.Background(), ssaconfig.WithScanControlMode(ssaconfig.ControlModeStart))
		require.Error(t, err)
		require.Contains(t, err.Error(), "programs is empty")
	})

	t.Run("empty source directory", func(t *testing.T) {
		err := Scan(
			context.Background(),
			WithSourceDir("empty-program", "/definitely/not/exists"),
			ssaconfig.WithRuleFilterMode("source"),
		)
		require.Error(t, err)
	})

	t.Run("empty source dir before manager construction", func(t *testing.T) {
		err := Scan(
			context.Background(),
			WithSourceDir("empty-program", "/definitely/not/exists"),
			ssaconfig.WithRuleFilterMode("source"),
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such file or directory")
	})
}

func TestScan_JsonRawConfigDoesNotClearRuleFilterMode(t *testing.T) {
	configJSON, err := json.Marshal(map[string]any{
		"Mode": int(ssaconfig.ModeSyntaxFlowScan),
		"SyntaxFlowRule": map[string]any{
			"task_local": true,
		},
	})
	require.NoError(t, err)

	cfg, err := NewConfig(
		ssaconfig.WithJsonRawConfig(configJSON),
		ssaconfig.WithRuleFilterMode("source"),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"source"}, cfg.GetRuleFilterMode())
}
