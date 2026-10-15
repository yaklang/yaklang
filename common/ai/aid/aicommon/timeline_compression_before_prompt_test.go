package aicommon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Production lifecycle: append (including exact deltas) -> check before prompt
// assembly -> one AI request -> atomic freeze/summary/promotion -> grow Open again.
func TestTimelineCompressionBeforePromptLifecycle(t *testing.T) {
	tl := NewTimeline(nil, nil)
	calls := 0
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		calls++
		// The exact schema stays out of the ordinary history sections; it is
		// listed once as a retained promoted entry the model may delete.
		require.Equal(t, 1, strings.Count(req.GetPrompt(), "EXACT_BEFORE_PROMPT_SCHEMA"))
		requireCompressionPromotedEntry(t, req.GetPrompt(), 3,
			"kind=recent-tool-cache name=probe operation=upsert", "[UPSERT] probe\nEXACT_BEFORE_PROMPT_SCHEMA")
		if calls == 2 {
			require.Contains(t, req.GetPrompt(), "FIRST_SUMMARY")
			return compressionMockSummary("SECOND_SUMMARY"), nil
		}
		return compressionMockSummary("FIRST_SUMMARY"), nil
	})
	tl.SetTimelineContentLimit(500)
	tl.SetTimelineBucketByteSize(1)
	tl.PushText(1, "small history")
	var stages []TimelineCompressionStage
	opts := compressionTestOptions()
	opts.OnProgress = func(stage TimelineCompressionStage) {
		// Callbacks must be able to inspect history without deadlocking.
		tl.mu.RLock()
		tl.mu.RUnlock()
		stages = append(stages, stage)
	}
	result, err := tl.CompressBeforePrompt(opts)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Zero(t, calls)
	require.Empty(t, stages, "below-threshold history is not an optimization")
	importFreezeItem(tl, 2, time.Now().Add(10*time.Minute), &TextTimelineItem{ID: 2, Text: strings.Repeat("history observation ", 300)})
	require.True(t, tl.PushPromotable(3, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "probe", TimelinePromotedOperationUpsert, "EXACT_BEFORE_PROMPT_SCHEMA"))
	before := RenderTimelineFrozenOpen(tl)
	require.Empty(t, before.Frozen)
	require.Empty(t, before.PromotedRecentTools)
	require.Contains(t, before.Open, "EXACT_BEFORE_PROMPT_SCHEMA")
	require.Zero(t, calls)
	result, err = tl.CompressBeforePrompt(opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, calls)
	require.Equal(t, []TimelineCompressionStage{TimelineCompressionOptimizing}, stages)
	require.EqualValues(t, 2, tl.FreezeSnapshot().Version, "lossless compaction and AI commit have separate receipts")
	view := RenderTimelineFrozenOpen(tl)
	require.Empty(t, view.Open)
	require.Contains(t, view.Frozen, "FIRST_SUMMARY")
	require.Contains(t, view.PromotedRecentTools, "EXACT_BEFORE_PROMPT_SCHEMA")
	result, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Nil(t, result)
	tl.PushText(4, strings.Repeat("second phase ", 300))
	require.Equal(t, view.Frozen, RenderTimelineFrozenOpen(tl).Frozen)
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.EqualValues(t, 3, tl.FreezeSnapshot().Version)
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Frozen, "FIRST_SUMMARY")
}

func TestTimelineCompressionBeforePromptFailureBackoff(t *testing.T) {
	tl := NewTimeline(nil, nil)
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { calls++; return "", errors.New("temporary failure") })
	tl.SetTimelineContentLimit(1)
	tl.PushText(1, "preserve original history")
	before, _ := MarshalTimeline(tl)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.Error(t, err)
	for i := 0; i < 3; i++ {
		result, err := tl.CompressBeforePrompt(compressionTestOptions())
		require.NoError(t, err)
		require.Nil(t, result)
	}
	require.Equal(t, 1, calls)
	after, _ := MarshalTimeline(tl)
	require.Equal(t, before, after)
	tl.PushText(2, "new observation")
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	tl.mu.Lock()
	tl.compressionRetryAfter = time.Time{}
	tl.mu.Unlock()
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.Error(t, err)
	require.Equal(t, 2, calls)
	_, err = tl.CompressOnce(compressionTestOptions())
	require.Error(t, err)
	require.Equal(t, 3, calls, "explicit retry is available")
}

func TestTimelineCompressionBeforePromptSharedWaitAndCancellation(t *testing.T) {
	tl := NewTimeline(nil, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	calls := 0
	var enteredOnce sync.Once
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		enteredOnce.Do(func() { close(entered) })
		<-release
		return compressionMockSummary("summary"), nil
	})
	tl.PushText(1, "work completed with enough detail to cross the compression limit")
	tl.SetTimelineContentLimit(int64(mainTimelineTokens(tl) - 1))
	done := make(chan error, 1)
	go func() { _, err := tl.CompressBeforePrompt(compressionTestOptions()); done <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := compressionTestOptions()
	opts.Context = ctx
	_, err := tl.CompressBeforePrompt(opts)
	require.ErrorIs(t, err, context.Canceled)
	second := make(chan error, 1)
	waiting := make(chan TimelineCompressionStage, 1)
	waitOpts := compressionTestOptions()
	waitOpts.OnProgress = func(stage TimelineCompressionStage) { waiting <- stage }
	go func() { _, err := tl.CompressBeforePrompt(waitOpts); second <- err }()
	select {
	case stage := <-waiting:
		require.Equal(t, TimelineCompressionWaiting, stage)
	case <-time.After(2 * time.Second):
		t.Fatal("missing compression waiting status")
	}
	select {
	case <-second:
		t.Fatal("before-prompt check returned before active transaction finished")
	case <-time.After(10 * time.Millisecond):
	}
	unblock()
	require.NoError(t, <-done)
	require.NoError(t, <-second)
	require.Equal(t, 1, calls)
}

func TestTimelineCompressionBeforePromptDoesNotRepeatCompletedSnapshot(t *testing.T) {
	tl := NewTimeline(nil, nil)
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { calls++; return compressionMockSummary("new head"), nil })
	tl.SetTimelineContentLimit(1)
	tl.PushText(1, "ordinary input")
	checked, err := tl.buildCompressionSnapshot()
	require.NoError(t, err)
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	// Snapshot validity still belongs to the transaction. A later scheduler
	// check measures afresh under the shared execution reservation.
	tl.mu.Lock()
	_, err = tl.validateCompressionSourceLocked(checked)
	tl.mu.Unlock()
	require.ErrorContains(t, err, "changed")
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 2, calls, "a rendered result still over the tiny limit gets its own later round")
}

func TestTimelineCompressionBeforePromptWaitersShareFailure(t *testing.T) {
	tl := NewTimeline(nil, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		if calls == 1 {
			close(entered)
		}
		<-release
		return "", errors.New("mock unavailable")
	})
	tl.SetTimelineContentLimit(1)
	tl.PushText(1, "preserve original")
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, err := tl.CompressBeforePrompt(compressionTestOptions()); first <- err }()
	<-entered
	go func() { _, err := tl.CompressBeforePrompt(compressionTestOptions()); second <- err }()
	unblock()
	require.Error(t, <-first)
	require.NoError(t, <-second)
	require.Equal(t, 1, calls)
	require.Empty(t, RenderTimelineFrozenOpen(tl).Frozen)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "preserve original")
}
