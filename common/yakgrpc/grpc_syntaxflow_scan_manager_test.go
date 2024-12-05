package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_Scan_Manager(t *testing.T) {
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
		}, nil)
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
}
