package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

	t.Run("test programs field saved when using ProjectID", func(t *testing.T) {
		// 创建一个简单的程序并保存到数据库
		programName := uuid.NewString()
		prog, err := ssaapi.Parse(`print("test")`,
			ssaapi.WithProgramName(programName),
			ssaapi.WithLanguage(ssaconfig.Yak),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programName)
		}()

		// 获取程序的数据库记录 ID
		progFromDB, err := ssadb.GetProgram(programName, ssadb.Application)
		require.NoError(t, err)
		require.NotNil(t, progFromDB)

		// 创建一个 SSA Project
		projectName := uuid.NewString()
		projectReq := &ypb.CreateSSAProjectRequest{
			Project: &ypb.SSAProject{
				ProjectName: projectName,
				Language:    "yak",
			},
		}
		project, err := yakit.CreateSSAProject(consts.GetGormProfileDatabase(), projectReq)
		require.NoError(t, err)
		require.NotNil(t, project)
		defer func() {
			// 清理项目
			yakit.DeleteSSAProject(consts.GetGormProfileDatabase(), &ypb.DeleteSSAProjectRequest{
				Filter: &ypb.SSAProjectFilter{
					IDs: []int64{int64(project.ID)},
				},
			})
		}()

		// 更新程序的 ProjectID
		err = yakit.UpdateIrProgramProjectID(ssadb.GetDB(), uint(progFromDB.ID), uint64(project.ID))
		require.NoError(t, err)

		// 通过 ProjectID 创建扫描任务（不传 ProgramNames）
		taskId := uuid.NewString()
		task, err := createSyntaxflowTaskById(context.Background(), "",
			taskId,
			newConfig(
				ssaconfig.WithRuleFilterLanguage("yak"),
				ssaconfig.WithProjectID(uint64(project.ID)),
				// 注意：这里不传 ProgramNames
			),
		)
		require.NoError(t, err)
		require.NotNil(t, task)

		// 验证 initByConfig 后 BaseInfo.ProgramNames 被正确设置
		require.NotNil(t, task.Config)
		require.NotNil(t, task.Config.Config)
		require.NotNil(t, task.Config.Config.BaseInfo)
		require.NotEmpty(t, task.Config.Config.BaseInfo.ProgramNames, "BaseInfo.ProgramNames should be set after initByConfig")
		require.Equal(t, programName, task.Config.Config.BaseInfo.ProgramNames[0], "ProgramNames should match the queried program name")

		// 验证 Programs 字段被设置
		require.NotEmpty(t, task.Config.Programs, "Programs should be set after initByConfig")
		require.Equal(t, programName, task.Config.Programs[0].GetProgramName(), "Program name should match")

		// 保存任务
		err = task.SaveTask()
		require.NoError(t, err)

		// 从数据库查询任务，验证 programs 字段不为空
		savedTask, err := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), taskId)
		require.NoError(t, err)
		require.NotNil(t, savedTask)
		require.NotEmpty(t, savedTask.Programs, "programs field should not be empty after SaveTask")
		require.Equal(t, programName, savedTask.Programs, "programs field should contain the program name")
	})

}
