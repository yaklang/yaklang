package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

// Exercise the actual reducer and emergency compression, not just FreezeAll.
// Exact evidence never enters the reducer input, and remains recoverable after dump/restore.
func TestEvidenceSurvivesHistoryCompression(t *testing.T) {
	for _, mode := range []string{"batch", "emergency"} {
		t.Run(mode, func(t *testing.T) {
			registerTimelineTestLiteForge(t)
			requests := 0
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
					requests++
					require.NotContains(t, req.GetPrompt(), "EXACT_EVIDENCE_SECRET")
					return (&mockedAI{}).CallSpeedPriorityAI(req)
				}))
			tl := cfg.GetTimeline()
			tl.autoCompressDisabled = true
			tl.SetTimelineBucketByteSize(-1)
			tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 100))
			saveTestEvidence(cfg, "fact", "EXACT_EVIDENCE_SECRET")
			tl.PushText(cfg.AcquireId(), strings.Repeat("another ordinary observation ", 100))
			tl.PushText(cfg.AcquireId(), "recent ordinary observation")
			if mode == "batch" {
				active := tl.getActiveTimelineItemIDs()
				var selected []*TimelineItem
				for _, id := range active {
					item, _ := tl.idToTimelineItem.Get(id)
					selected = append(selected, item)
				}
				// Same boundary transaction as the compression scheduler, retaining the recent tail.
				tl.mu.Lock()
				tl.freezeLocked(true, selected[1].GetID())
				tl.mu.Unlock()
				tl.batchCompressOldestWithRecent(selected[:2], selected[2:])
				require.Positive(t, requests)
			} else {
				tl.emergencyCompress(1)
				require.Zero(t, requests)
			}
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
		})
	}
}
