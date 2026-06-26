package aid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorTimelineDifferForTask_UsesBranchDiffer(t *testing.T) {
	c := newTestCoordinator(t)
	task := newStateTask(c, "branch-task")
	task.Index = "1-1"

	c.Timeline.PushText(c.AcquireId(), "main-marker")
	fork, err := c.Timeline.ForkForTask(task.Index, task.Name, c.Config, c.Config)
	require.NoError(t, err)
	require.NotNil(t, fork)

	restore := task.withTimelineFork(fork)
	defer restore()

	differ := c.timelineDifferForTask(task)
	require.NotNil(t, differ)

	c.Timeline.PushText(c.AcquireId(), "main-late-marker")
	fork.Branch.PushText(c.AcquireId(), "branch-marker")
	diff, err := differ.Diff()
	require.NoError(t, err)
	require.Contains(t, diff, "branch-marker")
	require.NotContains(t, diff, "main-late-marker")
}

func TestCoordinatorTimelineDifferForTask_DefaultsToCoordinatorDiffer(t *testing.T) {
	c := newTestCoordinator(t)
	task := newStateTask(c, "main-task")
	task.Index = "1-1"

	differ := c.timelineDifferForTask(task)
	require.Same(t, c.TimelineDiffer, differ)
}
