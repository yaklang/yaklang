package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestTimelineCompressionBusyDoesNotFreezeAgain(t *testing.T) {
	registerTimelineTestLiteForge(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			once.Do(func() { close(started) })
			<-release
			return (&mockedAI{}).CallSpeedPriorityAI(req)
		}))
	tl := cfg.Timeline
	tl.autoCompressDisabled = true
	tl.totalDumpContentLimit = 100
	tl.SetTimelineBucketByteSize(-1)
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 12; i++ {
		injectTimelineItem(tl, i, base.Add(time.Duration(i)*time.Second), &TextTimelineItem{ID: i, Text: strings.Repeat("observation ", 50)})
	}
	tl.mu.Lock()
	initialVersion, initialThrough := tl.freezeState.Version, tl.frozenThroughLocked()
	tl.compressForSizeLimitLocked()
	tl.mu.Unlock()
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("reducer did not start")
	}
	tl.mu.Lock()
	version, through := tl.freezeState.Version, tl.frozenThroughLocked()
	// Simulate growth while the reducer is blocked, without a time/byte freeze.
	for i := int64(13); i <= 18; i++ {
		now := time.Now()
		item := &TimelineItem{createdAt: now, value: &TextTimelineItem{ID: i, Text: strings.Repeat("new observation ", 50)}}
		tl.idToTs.Set(i, now.UnixMilli())
		tl.OrderInsertId(i, item)
		tl.OrderInsertTs(now.UnixMilli(), item)
	}
	tl.compressForSizeLimitLocked()
	newVersion, newThrough := tl.freezeState.Version, tl.frozenThroughLocked()
	tl.mu.Unlock()
	require.Equal(t, initialVersion, version, "starting a reducer must not publish an intermediate freeze")
	require.Equal(t, initialThrough, through)
	require.Equal(t, version, newVersion)
	require.Equal(t, through, newThrough)
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		tl.mu.RLock()
		defer tl.mu.RUnlock()
		return !tl.compressing
	}, 5*time.Second, 10*time.Millisecond)
	require.NotNil(t, tl.compressedHead)
	require.Greater(t, tl.frozenThroughLocked(), initialThrough, "successful commit freezes the covered range")
	for i := int64(13); i <= 18; i++ {
		item, _ := tl.idToTimelineItem.Get(i)
		require.False(t, item.deleted, "new appends must survive the older snapshot")
	}
}

func TestTimelineCompressionSnapshotIsDetached(t *testing.T) {
	value := &TextTimelineItem{ID: 1, Text: "original observation"}
	item := &TimelineItem{value: value}
	snapshot := timelineCompressionPromptSnapshot([]*TimelineItem{item})
	value.Text = "changed observation"
	item.deleted = true
	require.Len(t, snapshot, 1)
	require.Equal(t, "original observation", snapshot[0].String())
	require.Equal(t, int64(1), snapshot[0].GetID())
}

func TestTimelineCompressionPreservesEditedCandidate(t *testing.T) {
	registerTimelineTestLiteForge(t)
	var timeline *Timeline
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			timeline.mu.Lock()
			item, _ := timeline.idToTimelineItem.Get(1)
			item.value.(*TextTimelineItem).Text = "new fact after snapshot"
			timeline.mu.Unlock()
			return (&mockedAI{}).CallSpeedPriorityAI(req)
		}))
	timeline = compressionBenchmarkFixture(cfg)
	items := timeline.idToTimelineItem.Values()
	timeline.batchCompressOldestWithRecent(items[:50], items[50:])
	require.Nil(t, timeline.compressedHead)
	require.Len(t, timeline.getActiveTimelineItemIDs(), 60)
	require.Equal(t, "new fact after snapshot", items[0].String())
}

func TestTimelineCompressionCoversAllChunksBeforeCommit(t *testing.T) {
	for _, failLast := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty_last_%v", failLast), func(t *testing.T) {
			registerTimelineTestLiteForge(t)
			var prompts []string
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
					prompts = append(prompts, req.GetPrompt())
					if failLast && strings.Contains(req.GetPrompt(), "SOURCE_5_END") {
						rsp := NewUnboundAIResponse()
						rsp.EmitOutputStream(strings.NewReader(`{"@action":"timeline-reducer","key_findings":[]}`))
						rsp.Close()
						return rsp, nil
					}
					return (&mockedAI{}).CallSpeedPriorityAI(req)
				}))
			tl := cfg.Timeline
			tl.autoCompressDisabled = true
			var items []*TimelineItem
			for i := int64(1); i <= 5; i++ {
				body := fmt.Sprintf("SOURCE_%d_BEGIN\n%s\nSOURCE_%d_END", i, strings.Repeat("source-data ", 2400), i)
				tl.PushText(i, body)
				item, _ := tl.idToTimelineItem.Get(i)
				items = append(items, item)
			}
			tl.PushText(6, "recent context")
			recent, _ := tl.idToTimelineItem.Get(6)
			tl.batchCompressOldestWithRecent(items, []*TimelineItem{recent})
			require.Greater(t, len(prompts), 1)
			joined := strings.Join(prompts, "\n")
			for i, item := range items {
				require.Equal(t, 1, strings.Count(joined, fmt.Sprintf("SOURCE_%d_BEGIN", i+1)))
				require.Equal(t, 1, strings.Count(joined, fmt.Sprintf("SOURCE_%d_END", i+1)))
				require.Equal(t, !failLast, item.deleted)
			}
			require.NotContains(t, joined, "more items truncated due to size limit")
			require.False(t, recent.deleted)
			if failLast {
				require.Nil(t, tl.compressedHead, "partial success must not change visible history")
			} else {
				require.Equal(t, int64(5), tl.compressedHead.CoveredEndItemID)
				require.Equal(t, int64(1), tl.compressedHead.Version, "all chunks commit one head")
			}
		})
	}
}

func TestTimelineCompressionOversizedItemStaysActive(t *testing.T) {
	items := []*TimelineItem{{value: &TextTimelineItem{ID: 1, Text: strings.Repeat("x", MaxBatchCompressPromptSize+1)}}}
	chunks, err := splitTimelineCompressionInput(items, nil)
	require.Error(t, err)
	require.Empty(t, chunks)
	require.False(t, items[0].deleted)
}

func TestTimelineCompressionRejectsStaleResult(t *testing.T) {
	registerTimelineTestLiteForge(t)
	var timeline *Timeline
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			// Simulate another history operation while the AI request is in flight.
			timeline.mu.Lock()
			timeline.updateCompressedHead(&TimelineCompressedHead{Text: "newer history", CoveredEndItemID: 1})
			timeline.mu.Unlock()
			return (&mockedAI{}).CallSpeedPriorityAI(req)
		}))
	timeline = compressionBenchmarkFixture(cfg)
	items := timeline.idToTimelineItem.Values()
	timeline.batchCompressOldestWithRecent(items[:50], items[50:])
	require.Equal(t, "newer history", timeline.compressedHead.Text)
	require.Len(t, timeline.getActiveTimelineItemIDs(), 60)
}

func TestTimelineCompressionReservationReleasedAfterFailure(t *testing.T) {
	for _, panicFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("panic_%v", panicFirst), func(t *testing.T) {
			registerTimelineTestLiteForge(t)
			var succeed atomic.Bool
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
					if !succeed.Load() {
						if panicFirst {
							panic("test reducer panic")
						}
						return nil, fmt.Errorf("test reducer unavailable")
					}
					return (&mockedAI{}).CallSpeedPriorityAI(req)
				}))
			tl := compressionBenchmarkFixture(cfg)
			tl.totalDumpContentLimit = 100
			run := func() {
				tl.mu.Lock()
				tl.compressForSizeLimitLocked()
				tl.mu.Unlock()
				require.Eventually(t, func() bool {
					tl.mu.RLock()
					defer tl.mu.RUnlock()
					return !tl.compressing
				}, 5*time.Second, 10*time.Millisecond)
			}
			run()
			require.Nil(t, tl.compressedHead)
			require.Len(t, tl.getActiveTimelineItemIDs(), 60)
			succeed.Store(true)
			run()
			require.NotNil(t, tl.compressedHead, "a failed job must not block future compression")
		})
	}
}

func TestTimelineCompressionPromptPreservesAppendedHeadPrefix(t *testing.T) {
	old := &TimelineCompressedHeadBlock{CoveredEndItemID: 4, CoveredEndAtMs: 10000, Version: 1, Text: strings.Repeat("old finding\n", 1000)}
	next := &TimelineCompressedHeadBlock{CoveredEndItemID: 8, CoveredEndAtMs: 20000, Version: 2, Text: strings.TrimSpace(old.Text) + "\n\nnew finding"}
	projectedOld := projectTimelineRenderableBlocksForPrompt(TimelineRenderableBlocks{old})[0]
	projectedNext := projectTimelineRenderableBlocksForPrompt(TimelineRenderableBlocks{next})[0]
	require.Equal(t, projectedOld.StableNonce(), projectedNext.StableNonce())
	require.True(t, strings.HasPrefix(projectedNext.Render(), projectedOld.Render()))
	// Diagnostics and serialized coverage keep their precise original metadata.
	require.Contains(t, next.Render(), "covered_end_item_id=8 covered_end_at_ms=20000 version=2")
	require.NotEqual(t, old.StableNonce(), next.StableNonce())
	require.Equal(t, int64(8), next.CoveredEndItemID)

	// Verify the property after real projection, not just in the intermediate
	// prompt: the model sees stable TIMELINE labels, never the process nonce.
	project := func(block TimelineRenderableBlock) string {
		prompt := aiprojection.CreateTag("AI_CACHE_SYSTEM", "high-static", strings.Repeat("system ", 200)) +
			aiprojection.CreateTag("AI_CACHE_FROZEN", "semi-dynamic", TimelineRenderableBlocks{block}.Render("TIMELINE")) +
			aiprojection.CreateTag("PROMPT_SECTION_dynamic", "tail", "continue")
		result := aiprojection.ProjectAndObserve("timeline-cache-test", prompt)
		require.NotNil(t, result)
		for _, message := range result.Messages {
			var text string
			switch content := message.Content.(type) {
			case string:
				text = content
			case []*aispec.ChatContent:
				for _, part := range content {
					if part != nil {
						text += part.Text
					}
				}
			}
			if strings.Contains(text, "old finding") {
				return text
			}
		}
		t.Fatal("projected summary missing")
		return ""
	}
	oldMessage, newMessage := project(projectedOld), project(projectedNext)
	prefix := oldMessage[:strings.LastIndex(oldMessage, "old finding")+len("old finding")]
	require.True(t, strings.HasPrefix(newMessage, prefix))
	require.Contains(t, newMessage, "<|TIMELINE_compressedhead|>")
	require.NotContains(t, newMessage, "covered_end_item_id=")
	t.Logf("append preserves %d bytes of projected summary prefix", len(prefix))
}
