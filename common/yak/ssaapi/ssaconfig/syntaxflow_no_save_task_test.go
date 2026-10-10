package ssaconfig

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyntaxFlowNoSaveTaskIsIndependentAndRoundTrips(t *testing.T) {
	var nilConfig *Config
	require.False(t, nilConfig.IsNoSaveTask())
	require.False(t, (&Config{}).IsNoSaveTask())

	defaults, err := NewCLIScanConfig(WithNoSaveRisk(true))
	require.NoError(t, err)
	require.False(t, defaults.IsNoSaveTask(), "no_save_risk must keep its existing task persistence contract")

	cfg, err := NewSyntaxFlowScanConfig(WithNoSaveTask(true), WithSyntaxFlowResultKind(SFResultSaveDatabase))
	require.NoError(t, err)
	require.True(t, cfg.IsNoSaveTask())
	require.False(t, cfg.IsNoSaveRisk(), "task persistence must not change result persistence")
	require.Equal(t, SFResultSaveDatabase, cfg.GetSyntaxFlowResultKind())
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	restored, err := NewCLIScanConfig(WithJsonRawConfig(raw))
	require.NoError(t, err)
	require.True(t, restored.IsNoSaveTask())
	require.NoError(t, WithNoSaveTask(false)(restored))
	require.False(t, restored.IsNoSaveTask())
}
