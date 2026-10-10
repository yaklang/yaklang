package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

type recordingRiskHandler struct {
	items []schema.RiskUpdateItem
}

func (h *recordingRiskHandler) ApplyRiskUpdate(item schema.RiskUpdateItem) error {
	h.items = append(h.items, item)
	return nil
}

func runtimeRisk(feature, mode string, id uint) *schema.SSARisk {
	risk := &schema.SSARisk{
		Hash:            feature + "-" + mode,
		RiskFeatureHash: feature,
		ScanMode:        mode,
		FromRule:        mode + "-rule",
	}
	risk.ID = id
	return risk
}

func TestScanRuntime_SourceThenSSACoversOneRow(t *testing.T) {
	rt := NewScanRuntime()
	handler := &recordingRiskHandler{}
	rt.ListenRisk(handler)

	source := runtimeRisk("feature-1", string(schema.SFR_MODE_SOURCE), 5)
	require.True(t, rt.SubmitRisk(source))
	require.Len(t, handler.items, 1)
	require.Zero(t, handler.items[0].OldID)

	ssa := runtimeRisk("feature-1", string(schema.SFR_MODE_SSA), 0)
	require.True(t, rt.SubmitRisk(ssa))
	require.Len(t, handler.items, 2)
	require.Equal(t, uint(5), handler.items[1].OldID)
	require.Equal(t, source.Hash, handler.items[1].OldHash)
}

func TestScanRuntime_EarlierModePublishesNothing(t *testing.T) {
	rt := NewScanRuntime()
	handler := &recordingRiskHandler{}
	rt.ListenRisk(handler)

	require.True(t, rt.SubmitRisk(runtimeRisk("feature-1", string(schema.SFR_MODE_SSA), 1)))
	require.True(t, rt.SubmitRisk(runtimeRisk("feature-1", string(schema.SFR_MODE_SOURCE), 0)))
	require.Len(t, handler.items, 1, "an earlier mode must not overwrite a later finding")
}

func TestScanRuntime_NilRuntimeIsSafe(t *testing.T) {
	var rt *ScanRuntime
	require.False(t, rt.SubmitRisk(runtimeRisk("f", "ssa", 0)))
	require.NotPanics(t, func() { rt.ListenRisk(nil) })
}
