package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// Exact cached schemas bypass both AI reduction and emergency summaries. The
// retained journal must still restore runtime membership and accept a small
// REUSE after compression, rather than duplicating or losing the schema.
func TestTimelineToolCacheSurvivesHistoryCompression(t *testing.T) {
	for _, mode := range []string{"batch", "emergency"} {
		t.Run(mode, func(t *testing.T) {
			registerTimelineTestLiteForge(t)
			tool := aitool.NewWithoutCallback("compression_probe", aitool.WithDescription("EXACT_CACHE_SCHEMA"), aitool.WithStringParam("path"))
			requests := 0
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
				WithToolManager(buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(tool))),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
					requests++
					require.NotContains(t, req.GetPrompt(), "EXACT_CACHE_SCHEMA")
					return (&mockedAI{}).CallSpeedPriorityAI(req)
				}))
			tl := cfg.Timeline
			tl.autoCompressDisabled = true
			tl.SetTimelineBucketByteSize(-1)
			tl.PushText(cfg.AcquireId(), strings.Repeat("old observation ", 100))
			cfg.RecordRecentlyUsedTool(tool)
			tl.PushText(cfg.AcquireId(), strings.Repeat("second observation ", 100))
			tl.PushText(cfg.AcquireId(), "recent observation")
			if mode == "batch" {
				var selected []*TimelineItem
				for _, id := range tl.getActiveTimelineItemIDs() {
					item, _ := tl.idToTimelineItem.Get(id)
					selected = append(selected, item)
				}
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
			view := RenderTimelineFrozenOpen(tl)
			require.Contains(t, view.PromotedSemiDynamic1, "EXACT_CACHE_SCHEMA")
			require.NotContains(t, view.Frozen+view.Open, "EXACT_CACHE_SCHEMA")
			reloaded := cacheRuntimeConfig(tool)
			restoreCacheTimeline(t, tl, reloaded)
			require.Equal(t, []string{tool.Name}, reloaded.GetAiToolManager().GetRecentToolNames())
			require.Equal(t, view.PromotedSemiDynamic1, RenderTimelineFrozenOpen(reloaded.Timeline).PromotedSemiDynamic1)
			reloaded.RecordRecentlyUsedTool(tool)
			require.Contains(t, RenderTimelineFrozenOpen(reloaded.Timeline).Open, "[REUSE] compression_probe")
			require.NotContains(t, RenderTimelineFrozenOpen(reloaded.Timeline).Open, "EXACT_CACHE_SCHEMA")
		})
	}
}
