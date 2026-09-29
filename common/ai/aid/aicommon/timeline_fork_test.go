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

func TestTimelineFork_RenderKeepsMergedTaskBoundariesWithoutRepeatingItemLabels(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "[doing] [task:task-root]:\nroot")

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithSequence(100))
	firstFork, err := parent.ForkForTask("1-1", "first", cfg, cfg)
	require.NoError(t, err)
	secondFork, err := parent.ForkForTask("1-2", "second", cfg, cfg)
	require.NoError(t, err)

	firstID := cfg.AcquireId()
	secondID := cfg.AcquireId()
	firstFork.Branch.PushText(firstID, "[doing] [task:task-child-a]:\nchild-a")
	secondFork.Branch.PushText(secondID, "[doing] [task:task-child-b]:\nchild-b")
	_, err = firstFork.MergeBack()
	require.NoError(t, err)
	_, err = secondFork.MergeBack()
	require.NoError(t, err)

	rendered := parent.GroupByMinutesAndBytes(3, -1).GetBlocks().Render("TIMELINE")
	require.Equal(t, 1, strings.Count(rendered, "task-root"))
	require.Equal(t, 1, strings.Count(rendered, "task-child-a"))
	require.Equal(t, 1, strings.Count(rendered, "task-child-b"))
	require.NotContains(t, rendered, "[task:")

	parent.mu.RLock()
	mergedItem, ok := parent.idToTimelineItem.Get(firstID)
	parent.mu.RUnlock()
	require.True(t, ok)
	require.Equal(t, "task-child-a", ParseTimelineItemHumanReadable(mergedItem).TaskID,
		"fork merge and structured TaskID parsing must remain untouched")
}
