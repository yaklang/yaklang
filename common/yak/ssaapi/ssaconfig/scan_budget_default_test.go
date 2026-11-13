package ssaconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScanRuleBudgetDefaults(t *testing.T) {
	// The work limit is still defaulted; the wall-clock timeout is off unless
	// explicitly configured.
	cfg, err := New(ModeSyntaxFlowScan)
	require.NoError(t, err)
	require.Nil(t, cfg.SyntaxFlowScan, "New stays lazy")
	require.Equal(t, time.Duration(0), cfg.GetScanRuleTimeout())
	require.Equal(t, DefaultScanRuleWorkLimit, cfg.GetScanRuleWorkLimit())

	// Explicit 0 remains the documented escape hatch.
	disabled, err := New(ModeSyntaxFlowScan,
		WithScanRuleTimeout(0),
		WithScanRuleWorkLimit(0),
	)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), disabled.GetScanRuleTimeout())
	require.Equal(t, int64(0), disabled.GetScanRuleWorkLimit())

	// Explicit non-zero values override the defaults unchanged.
	explicit, err := New(ModeSyntaxFlowScan,
		WithScanRuleTimeout(2*time.Minute),
		WithScanRuleWorkLimit(123),
	)
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, explicit.GetScanRuleTimeout())
	require.Equal(t, int64(123), explicit.GetScanRuleWorkLimit())

	// Non-scan configs keep the old zero-value behavior.
	nonScan, err := New(ModeProjectBase)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), nonScan.GetScanRuleTimeout())
	require.Equal(t, int64(0), nonScan.GetScanRuleWorkLimit())
}
