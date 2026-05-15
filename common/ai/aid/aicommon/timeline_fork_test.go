package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/linktable"
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

func TestTimelineFork_MergeBranchReducers(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "parent-A")

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := parent.ForkForTask("1-1", "task-1", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	fork.Branch.mu.Lock()
	fork.Branch.reducers.Set(9, linktable.NewUnlimitedStringLinkTable("branch reducer"))
	fork.Branch.reducerTs.Set(9, 123456789)
	fork.Branch.mu.Unlock()

	mergeResult, err := fork.MergeBack()
	require.NoError(t, err)
	require.NotNil(t, mergeResult)
	require.GreaterOrEqual(t, mergeResult.ReducersMerged, 1)

	parent.mu.RLock()
	defer parent.mu.RUnlock()
	found := false
	parent.reducers.ForEach(func(_ int64, reducer *linktable.LinkTable[string]) bool {
		if reducer != nil && reducer.Value() == "branch reducer" {
			found = true
			return false
		}
		return true
	})
	require.True(t, found)
}

func TestTimelineFork_CompressDoesNotTouchProtectedPrefix(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "base-1")
	parent.PushText(2, "base-2")
	baseReducers := parent.reducers.Len()

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
	require.Equal(t, baseReducers, parent.reducers.Len(), "parent reducers should stay unchanged before merge")
}
