package loop_ssa_risk_overview

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractRiskIDsFromFindings_range(t *testing.T) {
	ids := extractRiskIDsFromFindings("风险ID：9829-9832，共4条")
	require.Equal(t, []string{"9829", "9830", "9831", "9832"}, ids)
}

func TestExistingCoversRiskCluster(t *testing.T) {
	existing := "聚类\n风险ID: 9829, 9830, 9831, 9832\n"
	require.True(t, existingCoversRiskCluster(existing, []string{"9829", "9832"}))
	require.False(t, existingCoversRiskCluster(existing, []string{"9829", "9999"}))
}
