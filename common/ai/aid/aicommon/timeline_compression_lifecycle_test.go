package aicommon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimelineCompressionEntriesShareExecution(t *testing.T) {
	tl := NewTimeline(nil, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		close(entered)
		<-release
		return compressionMockSummary("summary"), nil
	})
	tl.SetTimelineContentLimit(600)
	tl.PushText(1, strings.Repeat("original observation ", 1000))
	explicit := make(chan error, 1)
	go func() { _, err := tl.CompressOnce(compressionTestOptions()); explicit <- err }()
	<-entered
	waiting := make(chan TimelineCompressionStage, 1)
	options := compressionTestOptions()
	options.OnProgress = func(stage TimelineCompressionStage) { waiting <- stage }
	automatic := make(chan error, 1)
	go func() { _, err := tl.CompressBeforePrompt(options); automatic <- err }()
	select {
	case stage := <-waiting:
		require.Equal(t, TimelineCompressionWaiting, stage)
	case <-time.After(time.Second):
		t.Fatal("automatic entry did not wait for the explicit transaction")
	}
	_, err := tl.CompressOnce(compressionTestOptions())
	require.ErrorIs(t, err, errTimelineCompressionBusy)
	close(release)
	require.NoError(t, <-explicit)
	require.NoError(t, <-automatic)
	require.Equal(t, 1, calls)
}

func TestTimelineCompressionConcurrentAppendWaitsForNextCheck(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "original history that still needs one summary")
	calls, notifications := 0, 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		if calls == 1 {
			tl.PushText(2, "new observation that arrives during compression")
			return compressionMockSummary("first summary"), nil
		}
		return compressionMockSummary("final summary"), nil
	})
	tl.SetTimelineContentLimit(1)
	tl.RegisterSummaryCallback("reentrant", func(TimelineSummaryEvent) {
		notifications++
		require.False(t, tl.compressing)
		if notifications != 1 {
			return
		}
		// The append arrived during the only AI round. The callback's own check
		// owns the next round; this call does not fold or compress it again.
		require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "new observation")
		result, err := tl.CompressBeforePrompt(compressionTestOptions())
		require.NoError(t, err)
		require.Equal(t, "final summary", result.Summary)
	})
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 2, notifications)
	require.Equal(t, "first summary", result.Summary)
}

func TestTimelineCompressionResultCollectsWithoutOverwritingReduction(t *testing.T) {
	reduction := &TimelineCompressionResult{ThroughID: 2, RetiredIDs: []int64{1}, RetainedIDs: []int64{2},
		RetainedRange: "2", Summary: "summary", InputTokens: 100, SummaryTokens: 10}
	compaction := &TimelineCompressionResult{ThroughID: 4, RetiredIDs: []int64{3, 4}, RetiredPromotedEntryIDs: []int64{3, 4}}
	result := collectCompressionResults([]*timelineCompressionCommit{{Result: reduction}, {Result: compaction}})
	require.Equal(t, int64(4), result.ThroughID)
	require.Equal(t, "summary", result.Summary)
	require.Equal(t, 100, result.InputTokens)
	require.Equal(t, []int64{1, 3, 4}, result.RetiredIDs)
	require.Equal(t, []int64{2}, result.RetainedIDs)
	require.Equal(t, []int64{1}, reduction.RetiredIDs, "collection must not change observer receipts")
}

func TestTimelineCompressionLimitChangeDoesNotStartBackoff(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "original history that still needs one summary")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		tl.SetTimelineContentLimit(2)
		return compressionMockSummary("summary"), nil
	})
	tl.SetTimelineContentLimit(1)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.ErrorIs(t, err, errTimelineCompressionLimitChanged)
	require.Empty(t, tl.compressionLastFailure)
	require.True(t, tl.compressionRetryAfter.IsZero())
}

func TestTimelineCompressionOptionsPreservePromptProjection(t *testing.T) {
	tl := NewTimeline(nil, nil)
	options := TimelineCompressionOptions{Context: context.Background(), PromptOptions: TimelinePromptOptions{
		IncludeLatestModelReplay: true, ExcludeToolCache: true, PromoteUserInput: false,
	}}
	require.Equal(t, options.PromptOptions, tl.prepareCompressionOptions(options).PromptOptions)
}
