package syntaxflow_scan

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestManager(t *testing.T) {
	t.Run("test save and resume scan task", func(t *testing.T) {
		taskId := uuid.NewString()
		task, err := CreateSyntaxflowTaskById(taskId, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode: "",
			Filter: &ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			},
			ProgramName: []string{
				"a", "b", "c",
			},
		}, nil, nil)
		require.NoError(t, err)
		log.Infof("m; %v", task)

		task.totalQuery = 60
		task.skipQuery = 11
		task.failedQuery = 22
		task.successQuery = 33
		task.riskCount = 44

		err = task.SaveTask()
		require.NoError(t, err)

		newTask, err := LoadSyntaxflowTaskFromDB(taskId, context.Background(), nil)
		require.NoError(t, err)

		require.Equal(t, task.TaskId(), newTask.TaskId())
		require.Equal(t, task.status, newTask.status)
		require.Equal(t, task.totalQuery, newTask.totalQuery)
		require.Equal(t, task.skipQuery, newTask.skipQuery)
		require.Equal(t, task.failedQuery, newTask.failedQuery)
		require.Equal(t, task.successQuery, newTask.successQuery)
		require.Equal(t, task.riskCount, newTask.riskCount)
		require.Equal(t, task.programs, newTask.programs)

		require.NotNil(t, newTask.config)
		require.NotNil(t, newTask.config.Filter)
		require.Equal(t, newTask.config.Filter.Language, []string{"java"})
	})

	t.Run("test scan batch increment", func(t *testing.T) {
		taskId1 := uuid.NewString()
		programName := uuid.NewString()
		task1, err := CreateSyntaxflowTaskById(taskId1, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode: "",
			Filter: &ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			},
			ProgramName: []string{programName},
		}, nil, nil)
		require.NoError(t, err)

		err = task1.SaveTask()
		require.NoError(t, err)

		taskId2 := uuid.NewString()
		task2, err := CreateSyntaxflowTaskById(taskId2, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode: "",
			Filter: &ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			},
			ProgramName: []string{programName},
		}, nil, nil)
		require.NoError(t, err)

		err = task2.SaveTask()
		require.NoError(t, err)

		require.Equal(t, task1.taskRecorder.ScanBatch+1, task2.taskRecorder.ScanBatch)
		log.Infof("Same program - Task1 scan batch: %d, Task2 scan batch: %d",
			task1.taskRecorder.ScanBatch, task2.taskRecorder.ScanBatch)

		taskId3 := uuid.NewString()
		newProgramName := uuid.NewString()
		task3, err := CreateSyntaxflowTaskById(taskId3, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode: "",
			Filter: &ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			},
			ProgramName: []string{newProgramName},
		}, nil, nil)
		require.NoError(t, err)

		err = task3.SaveTask()
		require.NoError(t, err)

		require.Equal(t, uint64(1), task3.taskRecorder.ScanBatch)
		log.Infof("Different program - Task3 scan batch: %d", task3.taskRecorder.ScanBatch)
	})
}
