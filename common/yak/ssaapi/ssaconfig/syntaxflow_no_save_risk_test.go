package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSyntaxFlowNoSaveRisk_DoesNotChangeSaveKind verifies that persistence mode
// still reaches the normal final-save path, where only risk/audit persistence
// is suppressed.
func TestSyntaxFlowNoSaveRisk_DoesNotChangeSaveKind(t *testing.T) {
	t.Run("default keeps configured kind", func(t *testing.T) {
		cfg, err := NewCLIScanConfig(WithSyntaxFlowResultKind(SFResultSaveDatabase))
		require.NoError(t, err)
		require.False(t, cfg.IsNoSaveRisk())
		require.Equal(t, SFResultSaveDatabase, cfg.GetSyntaxFlowResultKind())
	})

	t.Run("no-save-risk keeps database kind", func(t *testing.T) {
		cfg, err := NewCLIScanConfig(
			WithSyntaxFlowResultKind(SFResultSaveDatabase),
			WithNoSaveRisk(true),
		)
		require.NoError(t, err)
		require.True(t, cfg.IsNoSaveRisk())
		require.Equal(t, SFResultSaveDatabase, cfg.GetSyntaxFlowResultKind())
	})

	t.Run("no-save-risk can be turned off again", func(t *testing.T) {
		cfg, err := NewCLIScanConfig(
			WithNoSaveRisk(true),
			WithSyntaxFlowResultKind(SFResultSaveDatabase),
			WithNoSaveRisk(false),
		)
		require.NoError(t, err)
		require.False(t, cfg.IsNoSaveRisk())
		require.Equal(t, SFResultSaveDatabase, cfg.GetSyntaxFlowResultKind())
	})
}

// TestSyntaxFlowNoSaveRisk_NilSafety guards the accessor used by the scan
// runner: a missing SyntaxFlow section must not enable the read-only mode.
func TestSyntaxFlowNoSaveRisk_NilSafety(t *testing.T) {
	var nilCfg *Config
	require.False(t, nilCfg.IsNoSaveRisk())

	empty := &Config{}
	require.False(t, empty.IsNoSaveRisk())
}
