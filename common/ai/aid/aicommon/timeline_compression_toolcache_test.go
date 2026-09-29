package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// Exact cached schemas bypass AI summarization. The
// retained journal must still restore runtime membership and accept a small
// REUSE after compression, rather than duplicating or losing the schema.
func TestTimelineToolCacheSurvivesHistoryCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	tool := aitool.NewWithoutCallback("compression_probe", aitool.WithDescription("EXACT_CACHE_SCHEMA"), aitool.WithStringParam("path"))
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithToolManager(buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(tool))),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			require.NotContains(t, req.GetPrompt(), "EXACT_CACHE_SCHEMA")
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Verified observation; further checks pending.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.Timeline
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old observation ", 100))
	cfg.RecordRecentlyUsedTool(tool)
	tl.PushText(cfg.AcquireId(), strings.Repeat("second observation ", 100))
	tl.PushText(cfg.AcquireId(), "recent observation")
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, requests)
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
}
