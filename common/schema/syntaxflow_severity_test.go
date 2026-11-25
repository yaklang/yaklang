package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyntaxFlowSeverityRank(t *testing.T) {
	ordered := []SyntaxFlowSeverity{
		SFR_SEVERITY_INFO,
		SFR_SEVERITY_LOW,
		SFR_SEVERITY_WARNING,
		SFR_SEVERITY_HIGH,
		SFR_SEVERITY_CRITICAL,
	}
	for i := 1; i < len(ordered); i++ {
		require.Greater(t, ordered[i].Rank(), ordered[i-1].Rank(), ordered[i])
	}
	require.Greater(t, SFR_SEVERITY_HIGH.Rank(), SyntaxFlowSeverity("unknown").Rank())
}
