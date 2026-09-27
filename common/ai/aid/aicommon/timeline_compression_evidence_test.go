package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

// Exercise the actual one-shot compression transaction, not just FreezeAll.
// Exact evidence never enters the reducer input, and remains recoverable after dump/restore.
func TestEvidenceSurvivesHistoryCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			require.NotContains(t, req.GetPrompt(), "EXACT_EVIDENCE_SECRET")
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Verified observation; further checks pending.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 100))
	saveTestEvidence(cfg, "fact", "EXACT_EVIDENCE_SECRET")
	tl.PushText(cfg.AcquireId(), strings.Repeat("another ordinary observation ", 100))
	tl.PushText(cfg.AcquireId(), "recent ordinary observation")
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	require.NotNil(t, tl.compressedHead)
	materials := RenderTimelineFrozenOpen(tl)
	require.Contains(t, materials.EvidenceSemiDynamic, "EXACT_EVIDENCE_SECRET")
	require.NotContains(t, materials.Frozen, "EXACT_EVIDENCE_SECRET")
	require.NotContains(t, materials.Open, "EXACT_EVIDENCE_SECRET")
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, materials, RenderTimelineFrozenOpen(restored))
	store, found := restored.evidenceStore()
	require.True(t, found)
	require.Contains(t, store.Render(), "EXACT_EVIDENCE_SECRET")
}

func TestTimelineEvidencePayloadWaitsForCompressionBeforePrompt(t *testing.T) {
	c := evidenceConfig(t)
	c.Timeline.SetTimelineBucketByteSize(256)
	saveTestEvidence(c, "large", strings.Repeat("confirmed observation ", 100))
	require.Empty(t, BuildPromptFrozenOpenMaterials(c).SessionEvidenceSemiDynamic)
	require.NotEmpty(t, BuildPromptFrozenOpenMaterials(c).TimelineOpen)
	c.Timeline.SetTimelineContentLimit(1)
	_, err := c.Timeline.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	blocks := BuildPromptFrozenOpenMaterials(c)
	require.NotEmpty(t, blocks.SessionEvidenceSemiDynamic)
	require.Empty(t, blocks.TimelineOpen)
	require.NotEmpty(t, c.Timeline.FreezeSnapshot().Batches)
}
