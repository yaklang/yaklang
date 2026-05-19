package syntaxflow_utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRiskEvidence_WriteDisposal_BlockedInAnalyzeMode(t *testing.T) {
	ev := NewRiskEvidence(nil)
	_, err := ev.WriteDisposal([]int64{1}, "not_issue", "x", RiskReviewModeAnalyze)
	require.Error(t, err)
	require.Contains(t, err.Error(), "analyze-only")
}

func TestRiskEvidence_ResolveRiskIDs_NilDB(t *testing.T) {
	ev := NewRiskEvidence(nil)
	ids, total, err := ev.ResolveRiskIDs(&ypb.SSARisksFilter{}, 10)
	require.Error(t, err)
	require.Nil(t, ids)
	require.Zero(t, total)
}

func TestParseRiskReviewMode(t *testing.T) {
	require.Equal(t, RiskReviewModeAnalyze, ParseRiskReviewMode(""))
	require.Equal(t, RiskReviewModeAnalyze, ParseRiskReviewMode("READONLY"))
	require.Equal(t, RiskReviewModeAnalyzeDispose, ParseRiskReviewMode("analyze_dispose"))
	require.Equal(t, RiskReviewModeAnalyzeDispose, ParseRiskReviewMode("WRITE"))
}
