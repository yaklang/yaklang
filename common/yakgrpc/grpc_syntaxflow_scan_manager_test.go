package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"
)

func TestGRPCMUSTPASS_SyntaxFlow_Scan_Manager(t *testing.T) {
	t.Run("test create and get scan task", func(t *testing.T) {
		taskId := uuid.NewString()
		task, err := CreateSyntaxFlowTask(taskId, context.Background())
		require.NoError(t, err)
		cacheTask, err := GetSyntaxFlowTask(taskId)
		require.NoError(t, err)
		require.Equal(t, task, cacheTask)
		RemoveSyntaxFlowTask(taskId)
		_, err = GetSyntaxFlowTask(taskId)
		require.Error(t, err)
	})

	t.Run("test save and resume scan task", func(t *testing.T) {
		taskId := uuid.NewString()
		task, err := CreateSyntaxFlowTask(taskId, context.Background())
		require.NoError(t, err)
		{
			task.status = yakit.SYNTAXFLOWSCAN_DONE
			task.totalQuery = 60
			task.skipQuery = 11
			task.failedQuery = 22
			task.successQuery = 33
			task.riskCount = 44
			task.programs = []string{"a", "b", "c"}
		}
		task.SaveTask()

		newTask := &SyntaxFlowScanManager{taskID: taskId}
		require.NoError(t, err)
		err = newTask.ResumeManagerFromTask()
		require.NoError(t, err)
		{
			require.Equal(t, task.status, newTask.status)
			require.Equal(t, task.totalQuery, newTask.totalQuery)
			require.Equal(t, task.skipQuery, newTask.skipQuery)
			require.Equal(t, task.failedQuery, newTask.failedQuery)
			require.Equal(t, task.successQuery, newTask.successQuery)
			require.Equal(t, task.riskCount, newTask.riskCount)
			require.Equal(t, task.programs, newTask.programs)
		}
		err = yakit.DeleteSyntaxFlowScanTask(consts.GetGormProjectDatabase(), taskId)
		require.NoError(t, err)
	})
}
