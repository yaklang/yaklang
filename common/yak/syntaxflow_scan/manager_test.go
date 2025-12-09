package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestManager(t *testing.T) {
	newConfig := func(opts ...ssaconfig.Option) *Config {
		ret, err := NewConfig(opts...)
		require.NoError(t, err)
		return ret
	}

	t.Run("test save and resume scan task", func(t *testing.T) {
		taskId := uuid.NewString()
		task, err := createSyntaxflowTaskById(context.Background(), "",
			taskId,
			newConfig(
				ssaconfig.WithRuleFilterLanguage("java"),
				ssaconfig.WithProgramNames("a", "b", "c"),
			),
		)
		require.NoError(t, err)
		log.Infof("m; %v", task)

		task.markRuleSkipped(11)
		task.markRuleFailed(22)
		task.markRuleSuccess(33)
		task.setRiskCount(44)

		err = task.SaveTask()
		require.NoError(t, err)

		newTask, err := LoadSyntaxflowTaskFromDB(context.Background(), "",
			newConfig(ssaconfig.WithScanResumeTaskId(taskId)),
		)
		require.NoError(t, err)

		require.Equal(t, task.TaskId(), newTask.TaskId())
		require.Equal(t, task.status, newTask.status)
		require.Equal(t, task.GetTotalQuery(), newTask.GetTotalQuery())
		require.Equal(t, task.GetSkippedQuery(), newTask.GetSkippedQuery())
		require.Equal(t, task.GetFailedQuery(), newTask.GetFailedQuery())
		require.Equal(t, task.GetSuccessQuery(), newTask.GetSuccessQuery())
		require.Equal(t, task.GetRiskCount(), newTask.GetRiskCount())
		// require.Equal(t, task.programs, newTask.programs)

		require.NotNil(t, newTask.Config)
		require.NotNil(t, newTask.Config.GetRuleFilter())
		require.Equal(t, newTask.Config.GetRuleFilter().Language, []string{"java"})
	})

	t.Run("test scan batch increment", func(t *testing.T) {
		programName := uuid.NewString()
		taskId1 := uuid.NewString()
		task1, err := createSyntaxflowTaskById(context.Background(), "", taskId1,
			newConfig(
				ssaconfig.WithRuleFilterLanguage("java"),
				ssaconfig.WithProgramNames(programName),
			),
		)
		require.NoError(t, err)

		err = task1.SaveTask()
		require.NoError(t, err)

		taskId2 := uuid.NewString()
		task2, err := createSyntaxflowTaskById(context.Background(), "", taskId2,
			newConfig(
				ssaconfig.WithRuleFilterLanguage("java"),
				ssaconfig.WithProgramNames(programName),
			),
		)
		require.NoError(t, err)

		err = task2.SaveTask()
		require.NoError(t, err)

		require.Equal(t, task1.taskRecorder.ScanBatch+1, task2.taskRecorder.ScanBatch)
		log.Infof("Same program - Task1 scan batch: %d, Task2 scan batch: %d",
			task1.taskRecorder.ScanBatch, task2.taskRecorder.ScanBatch)

		taskId3 := uuid.NewString()
		newProgramName := uuid.NewString()
		task3, err := createSyntaxflowTaskById(context.Background(), "", taskId3,
			newConfig(
				ssaconfig.WithRuleFilterLanguage("java"),
				ssaconfig.WithProgramNames(newProgramName),
			),
		)
		require.NoError(t, err)

		err = task3.SaveTask()
		require.NoError(t, err)

		require.Equal(t, task1.taskRecorder.ScanBatch+1, task2.taskRecorder.ScanBatch)
		log.Infof("Same program - Task1 scan batch: %d, Task3 scan batch: %d",
			task1.taskRecorder.ScanBatch, task3.taskRecorder.ScanBatch)
	})

}
