package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTimelineCompressionForkMergePreservesParentHistory(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "inherited history")
	fork, err := parent.ForkForTask("child", "investigation", nil, nil)
	require.NoError(t, err)
	fork.Branch.PushText(2, "branch investigation")
	parent.PushText(3, "parent concurrent work")
	bindCompressionMock(t, fork.Branch, func(*AIRequest) (string, error) {
		return compressionMockSummary("branch confirmed result"), nil
	})
	_, err = fork.Branch.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	result, err := fork.MergeBack()
	require.NoError(t, err)
	require.Equal(t, 1, result.CompressedHeadsMerged)
	view := RenderTimelineFrozenOpen(parent)
	require.Empty(t, view.Frozen)
	for _, text := range []string{"inherited history", "parent concurrent work", "branch confirmed result"} {
		require.Contains(t, view.Open, text)
	}
}

func TestTimelineCompressionForkMergeConflictIsAtomic(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent")
	fork, err := parent.ForkForTask("child", "investigation", nil, nil)
	require.NoError(t, err)
	fork.Branch.PushText(2, "first candidate must not leak")
	fork.Branch.PushText(3, "conflicting candidate")
	parent.PushText(3, "existing parent value")
	before, err := MarshalTimeline(parent)
	require.NoError(t, err)
	_, err = fork.MergeBack()
	require.Error(t, err)
	after, err := MarshalTimeline(parent)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTimelineFork_MergeBranchCompressedHead(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")
	parent.compressedHead = &TimelineCompressedHead{Text: "parent-only facts", CoveredEndItemID: 1, Version: 1}

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	fork.Branch.mu.Lock()
	fork.Branch.compressedHead = &TimelineCompressedHead{
		Text:             "branch compressed head",
		CoveredEndItemID: 9,
		CoveredEndAtMs:   123456789,
		Version:          1,
	}
	fork.Branch.mu.Unlock()

	mergeResult, err := fork.MergeBack()
	require.NoError(t, err)
	require.NotNil(t, mergeResult)
	require.GreaterOrEqual(t, mergeResult.CompressedHeadsMerged, 1)

	parent.mu.RLock()
	defer parent.mu.RUnlock()
	require.NotNil(t, parent.compressedHead)
	require.Equal(t, "parent-only facts", parent.compressedHead.Text)
	item, ok := parent.idToTimelineItem.Get(9)
	require.True(t, ok)
	require.Contains(t, item.String(), "branch compressed head")
}

func TestTimelineFork_InheritedCompressedHeadNotMerged(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")
	parent.compressedHead = &TimelineCompressedHead{
		Text:             "parent compressed head",
		CoveredEndItemID: 9,
		CoveredEndAtMs:   123456789,
		Version:          1,
	}

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)
	require.Equal(t, int64(9), fork.BaseMaxID)

	mergeResult, err := fork.MergeBack()
	require.NoError(t, err)
	require.NotNil(t, mergeResult)
	require.Equal(t, 0, mergeResult.CompressedHeadsMerged)

	parent.mu.RLock()
	defer parent.mu.RUnlock()
	require.NotNil(t, parent.compressedHead)
	require.Equal(t, "parent compressed head", parent.compressedHead.Text)
	require.Equal(t, int64(9), parent.compressedHead.CoveredEndItemID)
	require.Empty(t, parent.compressedHistory)
}
