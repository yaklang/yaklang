package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTimelineFork_BranchWriteInvisibleBeforeMerge(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	fork.Branch.PushText(2, "branch-B")
	require.NotContains(t, parent.Dump(), "branch-B")

	_, err = fork.MergeBack()
	require.NoError(t, err)
	require.Contains(t, parent.Dump(), "branch-B")
}

func TestTimelineFork_MergeBranchCompressedHead(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")

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
	require.Equal(t, "branch compressed head", parent.compressedHead.Text)
	require.Equal(t, int64(9), parent.compressedHead.CoveredEndItemID)
	require.Equal(t, int64(123456789), parent.compressedHead.CoveredEndAtMs)
}

func TestTimelineFork_MergePreservesGlobalIDs(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	globalID := cfg.AcquireId()
	fork.Branch.PushText(globalID, "branch-global-id")

	mergeResult, err := fork.MergeBack()
	require.NoError(t, err)
	require.NotNil(t, mergeResult)
	require.Equal(t, 1, mergeResult.ActiveItemsMerged)

	parent.mu.RLock()
	defer parent.mu.RUnlock()
	item, ok := parent.idToTimelineItem.Get(globalID)
	require.True(t, ok)
	require.NotNil(t, item)
	require.Contains(t, parent.Dump(), "branch-global-id")
}

func TestTimelineFork_MergeIDConflict(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	conflictID := cfg.AcquireId()
	fork.Branch.PushText(conflictID, "branch-conflict")

	parent.PushText(conflictID, "parent-conflict")

	_, err = fork.MergeBack()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists in parent timeline")
}

func TestTimelineFork_ConvertConfigToOptionsSharesSeqIdProvider(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableAutoSkills(true), WithSequence(100))
	require.NotNil(t, parent.SeqIdProvider)

	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.NotNil(t, child.SeqIdProvider)
	require.Same(t, parent.SeqIdProvider, child.SeqIdProvider)

	id1 := parent.AcquireId()
	id2 := child.AcquireId()
	require.Greater(t, id2, id1)
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

func TestTimelineFork_CompressDoesNotTouchProtectedPrefix(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "base-1")
	parent.PushText(2, "base-2")
	baseHead := cloneTimelineCompressedHead(parent.compressedHead)

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, &mockedAI{})
	require.NoError(t, err)
	require.NotNil(t, fork)

	fork.Branch.PushText(3, strings.Repeat("x", 400))
	fork.Branch.PushText(4, strings.Repeat("y", 400))

	baseItem, _ := fork.Branch.idToTimelineItem.Get(1)
	newItem, _ := fork.Branch.idToTimelineItem.Get(3)
	require.NotNil(t, baseItem)
	require.NotNil(t, newItem)

	fork.Branch.batchCompressOldestWithRecent([]*TimelineItem{baseItem, newItem}, nil)

	fork.Branch.mu.RLock()
	_, stillHasBase := fork.Branch.idToTimelineItem.Get(1)
	fork.Branch.mu.RUnlock()
	require.True(t, stillHasBase, "protected prefix item should not be deleted by fork compression")
	require.Equal(t, baseHead, parent.compressedHead, "parent compressed head should stay unchanged before merge")
}
