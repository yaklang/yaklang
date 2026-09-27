package aicommon

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

func cacheRuntimeConfig(tools ...*aitool.Tool) *Config {
	return NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithToolManager(buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(tools...))))
}

func restoreCacheTimeline(t *testing.T, source *Timeline, cfg *Config) {
	t.Helper()
	raw, err := MarshalTimeline(source)
	require.NoError(t, err)
	tl, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	tl.ReassignIDs(cfg.AcquireId)
	cfg.Timeline = tl
	cfg.restoreRecentToolsFromTimeline()
}

// Load A/B -> freeze -> reuse A -> reload -> reuse B. Reload must retain the
// pending recency without sealing it or copying either schema into Open.
// Schema changes and removals are then appended as new deltas and only replace
// the old aggregate when the next freeze commits.
func TestTimelineToolCacheRuntimeRestoreLifecycle(t *testing.T) {
	a := aitool.NewWithoutCallback("alpha", aitool.WithStringParam("path"))
	b := aitool.NewWithoutCallback("beta", aitool.WithStringParam("value"))
	cfg := cacheRuntimeConfig(a, b)
	cfg.RecordRecentlyUsedTool(a)
	cfg.RecordRecentlyUsedTool(b)
	tl := cfg.GetTimeline()
	tl.FreezeAll()
	frozen := RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1
	cfg.RecordRecentlyUsedTool(a)
	require.Equal(t, frozen, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1)
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Open, "Direct Params Schema")
	next := cacheRuntimeConfig(a, b)
	restoreCacheTimeline(t, tl, next)
	require.Equal(t, []string{"beta", "alpha"}, next.GetAiToolManager().GetRecentToolNames())
	require.Equal(t, frozen, RenderTimelineFrozenOpen(next.Timeline).PromotedSemiDynamic1)
	before, err := MarshalTimeline(next.Timeline)
	require.NoError(t, err)
	next.restoreRecentToolsFromTimeline()
	after, err := MarshalTimeline(next.Timeline)
	require.NoError(t, err)
	require.Equal(t, before, after, "unchanged restore is read-only and idempotent")
	next.RecordRecentlyUsedTool(b)
	require.Equal(t, frozen, RenderTimelineFrozenOpen(next.Timeline).PromotedSemiDynamic1)
	next.Timeline.FreezeAll()
	requireToolCacheOrder(t, next.Timeline, "## Tool: alpha", "## Tool: beta")

	// Same name, changed schema; beta no longer exists. Old Semi stays visible
	// with correction deltas in Open, instead of silently rewriting the prefix.
	a2 := aitool.NewWithoutCallback("alpha", aitool.WithStringParam("new_path"))
	changed := cacheRuntimeConfig(a2)
	restoreCacheTimeline(t, next.Timeline, changed)
	view := RenderTimelineFrozenOpen(changed.Timeline)
	require.NotContains(t, view.PromotedSemiDynamic1, "new_path")
	require.Contains(t, view.PromotedSemiDynamic1, "## Tool: beta")
	require.Contains(t, view.Open, "[UPSERT] alpha")
	require.Contains(t, view.Open, "new_path")
	require.Contains(t, view.Open, "[DELETE] beta")
	require.Equal(t, []string{"alpha"}, changed.GetAiToolManager().GetRecentToolNames())
	id := changed.Timeline.GetMaxID()
	changed.restoreRecentToolsFromTimeline()
	require.Equal(t, id, changed.Timeline.GetMaxID())
	changed.Timeline.FreezeAll()
	view = RenderTimelineFrozenOpen(changed.Timeline)
	require.Contains(t, view.PromotedSemiDynamic1, "new_path")
	require.NotContains(t, view.PromotedSemiDynamic1, "## Tool: beta")
	require.Empty(t, view.Open)
}

func TestTimelineToolCacheRuntimeEvictionAndSmallerRestoreBudget(t *testing.T) {
	a := aitool.NewWithoutCallback("alpha", aitool.WithStringParam("x"))
	b := aitool.NewWithoutCallback("beta", aitool.WithStringParam("x"))
	cfg := cacheRuntimeConfig(a, b)
	cfg.RecordRecentlyUsedTool(a)
	cfg.RecordRecentlyUsedTool(b)
	cfg.Timeline.FreezeAll()
	frozen := RenderTimelineFrozenOpen(cfg.Timeline).PromotedSemiDynamic1
	cfg.RecordRecentlyUsedTool(a) // A is newest, including after restore.
	manager := buildinaitools.NewToolManager(buildinaitools.WithOnlyTools(a, b), buildinaitools.WithRecentToolCacheMaxTokens(1))
	next := NewConfig(context.Background(), WithDisableAutoSkills(true), WithToolManager(manager))
	restoreCacheTimeline(t, cfg.Timeline, next)
	require.Equal(t, []string{"alpha"}, manager.GetRecentToolNames())
	require.Equal(t, frozen, RenderTimelineFrozenOpen(next.Timeline).PromotedSemiDynamic1)
	require.Contains(t, RenderTimelineFrozenOpen(next.Timeline).Open, "[DELETE] beta")
	next.Timeline.FreezeAll()
	sealed := RenderTimelineFrozenOpen(next.Timeline).PromotedSemiDynamic1
	require.NotContains(t, sealed, "## Tool: beta")

	mutation := next.RecordRecentlyUsedTool(b)
	require.Len(t, mutation.Deleted, 1)
	require.Equal(t, "alpha", mutation.Deleted[0].Name)
	require.Equal(t, []string{"beta"}, manager.GetRecentToolNames())
	view := RenderTimelineFrozenOpen(next.Timeline)
	require.Equal(t, sealed, view.PromotedSemiDynamic1)
	require.Contains(t, view.Open, "[UPSERT] beta")
	require.Contains(t, view.Open, "[DELETE] alpha")
	next.Timeline.FreezeAll()
	require.NotContains(t, RenderTimelineFrozenOpen(next.Timeline).PromotedSemiDynamic1, "## Tool: alpha")

	// A stale pre-seeded manager must not resurrect a deleted journal key.
	reloaded := cacheRuntimeConfig(a, b)
	reloaded.GetAiToolManager().AddRecentlyUsedTool(a)
	restoreCacheTimeline(t, next.Timeline, reloaded)
	require.Equal(t, []string{"beta"}, reloaded.GetAiToolManager().GetRecentToolNames())
}

func TestTimelineToolCacheRuntimeSeedsMissingJournalAndDoesNotInferFromText(t *testing.T) {
	a := aitool.NewWithoutCallback("alpha", aitool.WithStringParam("x"))
	cfg := cacheRuntimeConfig(a)
	cfg.GetAiToolManager().AddRecentlyUsedTool(a)
	require.NotNil(t, cfg.RecordRecentlyUsedTool(a).Reuse)
	require.Contains(t, RenderTimelineFrozenOpen(cfg.Timeline).Open, "[UPSERT] alpha")
	cfg.RecordRecentlyUsedTool(a)
	view := RenderTimelineFrozenOpen(cfg.Timeline).Open
	require.Equal(t, 1, strings.Count(view, "Direct Params Schema"))
	require.Contains(t, view, "[REUSE] alpha")

	legacy := NewTimeline(nil, nil)
	legacy.PushText(1, "CACHE_TOOL_CALL: alpha was called successfully")
	restored := cacheRuntimeConfig(a)
	restored.GetAiToolManager().AddRecentlyUsedTool(a)
	restoreCacheTimeline(t, legacy, restored)
	require.Empty(t, restored.GetAiToolManager().GetRecentToolNames())
	require.Contains(t, restored.Timeline.Dump(), "alpha was called successfully")
}

func TestTimelineToolCacheRuntimeConcurrentWritersAgreeWithJournal(t *testing.T) {
	a := aitool.NewWithoutCallback("alpha", aitool.WithStringParam("x"))
	b := aitool.NewWithoutCallback("beta", aitool.WithStringParam("x"))
	cfg := cacheRuntimeConfig(a, b)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg.RecordRecentlyUsedTool([]*aitool.Tool{a, b}[i%2])
		}(i)
	}
	wg.Wait()
	require.Equal(t, cfg.GetAiToolManager().GetRecentToolNames(),
		cfg.Timeline.effectivePromotedKeys(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool))
	open := RenderTimelineFrozenOpen(cfg.Timeline).Open
	require.Equal(t, 2, strings.Count(open, "[UPSERT]"))
	require.Equal(t, 18, strings.Count(open, "[REUSE]"))
	cfg.Timeline.FreezeAll()
	reloaded := cacheRuntimeConfig(a, b)
	restoreCacheTimeline(t, cfg.Timeline, reloaded)
	require.Equal(t, cfg.GetAiToolManager().GetRecentToolNames(), reloaded.GetAiToolManager().GetRecentToolNames())
}

func TestTimelineToolCacheRuntimeForkMergeAndRollback(t *testing.T) {
	a := aitool.NewWithoutCallback("alpha", aitool.WithStringParam("x"))
	b := aitool.NewWithoutCallback("beta", aitool.WithStringParam("x"))
	c := aitool.NewWithoutCallback("gamma", aitool.WithStringParam("x"))
	parent := cacheRuntimeConfig(a, b, c)
	parent.RecordRecentlyUsedTool(a)
	parent.RecordRecentlyUsedTool(b)
	parent.Timeline.FreezeAll()
	checkpoint := parent.Timeline.GetMaxID()
	frozen := RenderTimelineFrozenOpen(parent.Timeline).PromotedSemiDynamic1
	child := cacheRuntimeConfig(a, b, c)
	child.SeqIdProvider = parent.SeqIdProvider
	fork, err := parent.Timeline.ForkForTask("cache-child", "cache child", child, child)
	require.NoError(t, err)
	child.Timeline = fork.Branch
	child.restoreRecentToolsFromTimeline()
	child.RecordRecentlyUsedTool(a)
	child.RecordRecentlyUsedTool(c)
	require.Equal(t, frozen, RenderTimelineFrozenOpen(parent.Timeline).PromotedSemiDynamic1)
	require.NotContains(t, RenderTimelineFrozenOpen(parent.Timeline).Open, "gamma")
	require.Equal(t, []string{"alpha", "beta"}, parent.GetAiToolManager().GetRecentToolNames())
	_, err = fork.MergeBack()
	require.NoError(t, err)
	parent.restoreRecentToolsFromTimeline()
	require.Equal(t, []string{"beta", "alpha", "gamma"}, parent.GetAiToolManager().GetRecentToolNames())
	parent.Timeline.FreezeAll()
	require.Contains(t, RenderTimelineFrozenOpen(parent.Timeline).PromotedSemiDynamic1, "## Tool: gamma")
	parent.Timeline.TruncateAfter(checkpoint)
	parent.restoreRecentToolsFromTimeline()
	require.Equal(t, []string{"alpha", "beta"}, parent.GetAiToolManager().GetRecentToolNames())
	require.Equal(t, frozen, RenderTimelineFrozenOpen(parent.Timeline).PromotedSemiDynamic1)
}
