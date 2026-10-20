package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskCompletionWaitsFinalizationAndStopBypasses(t *testing.T) {
	for _, abort := range []bool{false, true} {
		task := NewStatefulTaskBase("full-task", "input", context.Background(), NewDummyEmitter(), true)
		task.SetStatus(AITaskState_Processing)
		first, last := task.DeferCompletion(), task.DeferCompletion()
		task.SetStatus(AITaskState_Completed)
		require.Equal(t, AITaskState_Processing, task.GetStatus())
		require.NoError(t, task.GetContext().Err())
		first()
		first()
		require.Equal(t, AITaskState_Processing, task.GetStatus())
		if abort {
			task.SetStatus(AITaskState_Aborted)
			require.ErrorIs(t, task.GetContext().Err(), context.Canceled)
		}
		last()
		last()
		if abort {
			require.Equal(t, AITaskState_Aborted, task.GetStatus())
		} else {
			require.Equal(t, AITaskState_Completed, task.GetStatus())
		}
		require.ErrorIs(t, task.GetContext().Err(), context.Canceled)
	}
}

func mustMarshalMemoryTimeline(t *testing.T, tl *Timeline) string {
	t.Helper()
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	return raw
}
