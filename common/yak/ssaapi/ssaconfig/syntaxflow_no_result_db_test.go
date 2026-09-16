package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSyntaxFlowNoResultDB_ForcesMemory verifies the read-only scan switch:
// when NoResultDB is on, the effective save kind must be memory no matter what
// the caller configured, so no risk / audit rows are written into the SSA
// database while the scan runs against an existing IR database.
func TestSyntaxFlowNoResultDB_ForcesMemory(t *testing.T) {
	t.Run("default keeps configured kind", func(t *testing.T) {
		cfg, err := NewCLIScanConfig(WithSyntaxFlowResultKind(SFResultSaveDatabase))
		require.NoError(t, err)
		require.False(t, cfg.IsSyntaxFlowResultNoDB())
		require.Equal(t, SFResultSaveDatabase, cfg.GetSyntaxFlowResultKind())
	})

	t.Run("no-result-db overrides database kind", func(t *testing.T) {
		cfg, err := NewCLIScanConfig(
			WithSyntaxFlowResultKind(SFResultSaveDatabase),
			WithSyntaxFlowNoResultDB(true),
		)
		require.NoError(t, err)
		require.True(t, cfg.IsSyntaxFlowResultNoDB())
		require.Equal(t, SFResultSaveMemory, cfg.GetSyntaxFlowResultKind(),
			"results must be kept in memory so nothing is written to the IR database")
	})

	t.Run("no-result-db can be turned off again", func(t *testing.T) {
		cfg, err := NewCLIScanConfig(
			WithSyntaxFlowNoResultDB(true),
			WithSyntaxFlowResultKind(SFResultSaveDatabase),
			WithSyntaxFlowNoResultDB(false),
		)
		require.NoError(t, err)
		require.False(t, cfg.IsSyntaxFlowResultNoDB())
		require.Equal(t, SFResultSaveDatabase, cfg.GetSyntaxFlowResultKind())
	})
}

// TestSyntaxFlowNoResultDB_NilSafety guards the accessor used by the scan
// runner: a missing SyntaxFlow section must not enable the read-only mode.
func TestSyntaxFlowNoResultDB_NilSafety(t *testing.T) {
	var nilCfg *Config
	require.False(t, nilCfg.IsSyntaxFlowResultNoDB())

	empty := &Config{}
	require.False(t, empty.IsSyntaxFlowResultNoDB())
}
