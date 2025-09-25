package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestManager(t *testing.T) {
	t.Run("test save and resume scan task", func(t *testing.T) {
		taskId := uuid.NewString()
		task, err := createSyntaxflowTaskById(context.Background(), "",
			taskId,
			NewScanConfig(
				WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
					Language: []string{"java"},
				}),
				WithProgramNames("a", "b", "c"),
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
			NewScanConfig(WithResumeTaskId(taskId)),
		)
		require.NoError(t, err)

		require.Equal(t, task.TaskId(), newTask.TaskId())
		require.Equal(t, task.status, newTask.status)
		require.Equal(t, task.GetTotalQuery(), newTask.GetTotalQuery())
		require.Equal(t, task.GetSkippedQuery(), newTask.GetSkippedQuery())
		require.Equal(t, task.GetFailedQuery(), newTask.GetFailedQuery())
		require.Equal(t, task.GetSuccessQuery(), newTask.GetSuccessQuery())
		require.Equal(t, task.GetRiskCount(), newTask.GetRiskCount())
		require.Equal(t, task.programs, newTask.programs)

		require.NotNil(t, newTask.ssaConfig)
		require.NotNil(t, newTask.ssaConfig.GetRuleFilter())
		require.Equal(t, newTask.ssaConfig.GetRuleFilter().Language, []string{"java"})
	})

	t.Run("test scan batch increment", func(t *testing.T) {
		programName := uuid.NewString()
		task1, err := createSyntaxflowTaskById(context.Background(), "", taskId1, NewScanConfig(
			WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			}),
			WithProgramNames(programName),
		))
		require.NoError(t, err)

		err = task1.SaveTask()
		require.NoError(t, err)

		taskId2 := uuid.NewString()
		task2, err := createSyntaxflowTaskById(context.Background(), "", taskId2, NewScanConfig(
			WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			}),
			WithProgramNames(programName),
		))
		require.NoError(t, err)

		err = task2.SaveTask()
		require.NoError(t, err)

		require.Equal(t, task1.taskRecorder.ScanBatch+1, task2.taskRecorder.ScanBatch)
		log.Infof("Same program - Task1 scan batch: %d, Task2 scan batch: %d",
			task1.taskRecorder.ScanBatch, task2.taskRecorder.ScanBatch)

		taskId3 := uuid.NewString()
		newProgramName := uuid.NewString()
		task3, err := createSyntaxflowTaskById(context.Background(), "", taskId3, NewScanConfig(
			WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			}),
			WithProgramNames(newProgramName),
		))
		require.NoError(t, err)

		err = task3.SaveTask()
		require.NoError(t, err)

		require.Equal(t, task1.taskRecorder.ScanBatch+1, task3.taskRecorder.ScanBatch)
		log.Infof("Same program - Task1 scan batch: %d, Task3 scan batch: %d",
			task1.taskRecorder.ScanBatch, task3.taskRecorder.ScanBatch)
	})

	t.Run("test SSA project configuration initialization", func(t *testing.T) {
		// 创建测试用的SSA项目
		testProject := &ypb.SSAProject{
			ProjectName:      "test-project-" + uuid.NewString(),
			Description:      "Test project for SSA configuration",
			Language:         "java",
			CodeSourceConfig: `{"kind":"local","local_file":"/test/path"}`,
			CompileConfig: &ypb.SSAProjectCompileConfig{
				StrictMode:   true,
				PeepholeSize: 10,
				ReCompile:    false,
			},
			ScanConfig: &ypb.SSAProjectScanConfig{
				Concurrency:    8,
				Memory:         true,
				IgnoreLanguage: true,
			},
			RuleConfig: &ypb.SSAProjectScanRuleConfig{
				RuleFilter: &ypb.SyntaxFlowRuleFilter{
					Language: []string{"java", "go"},
					Severity: []string{"high", "critical"},
				},
			},
		}

		// 创建项目到数据库
		schemaProject, err := yakit.CreateSSAProject(consts.GetGormProfileDatabase(), testProject)
		require.NoError(t, err)
		require.NotNil(t, schemaProject)

		defer func() {
			// 清理测试数据
			_ = consts.GetGormProfileDatabase().Unscoped().Delete(schemaProject)
		}()

		// 测试使用项目配置初始化扫描任务
		taskId := uuid.NewString()
		task, err := createSyntaxflowTaskById(context.Background(), "", taskId, NewScanConfig(
			WithControlMode(""),
			WithProjectId(uint64(schemaProject.ID)),
		))
		require.NoError(t, err)
		require.NotNil(t, task)

		// TODO: FIX ME
		// // 验证任务配置是否正确从项目中读取
		// config, err := schemaProject.GetConfig()
		// require.NoError(t, err)
		// sc := config.ScanConfig
		// require.Equal(t, []string{schemaProject.ProjectName}, task.programs)
		// require.Equal(t, sc.IgnoreLanguage, task.ignoreLanguage)
		// require.Equal(t, sc.Memory, task.memory)
		// require.Equal(t, sc.Concurrency, task.concurrency)

		// 测试项目配置被正确覆盖
		taskId2 := uuid.NewString()

		task2, err := createSyntaxflowTaskById(context.Background(), "", taskId2,
			NewScanConfig(
				WithControlMode(""),
				WithProjectId(uint64(schemaProject.ID)),
				// 覆盖项目配置
				WithProgramNames("custom-program"),
				WithIgnoreLanguage(false),
				WithMemory(false),
				WithConcurrency(16),
			),
		)
		require.NoError(t, err)
		require.NotNil(t, task2)

		// // 验证自定义配置优先于项目配置
		// require.Equal(t, []string{"custom-program"}, task2.programs)
		// require.Equal(t, false, task2.ignoreLanguage)
		// require.Equal(t, false, task2.memory)
		// require.Equal(t, uint32(16), task2.concurrency)
	})

	t.Run("test SSA project rule configuration", func(t *testing.T) {
		// 创建测试规则
		testRule := &schema.SyntaxFlowRule{
			RuleName:    "test-rule-" + uuid.NewString(),
			Content:     "test rule content",
			Language:    "java",
			Severity:    "high",
			Type:        "audit",
			Purpose:     "test",
			Description: "Test rule for SSA project",
		}

		err := consts.GetGormProfileDatabase().Create(testRule).Error
		require.NoError(t, err)

		defer func() {
			// 清理测试数据
			_ = consts.GetGormProfileDatabase().Unscoped().Delete(testRule)
		}()

		// 创建测试用的SSA项目，配置规则过滤器
		testProject := &ypb.SSAProject{
			ProjectName:      "test-project-rules-" + uuid.NewString(),
			Description:      "Test project for rule configuration",
			CodeSourceConfig: `{"kind":"local","local_file":"/test/path"}`,
			Language:         "java",
			RuleConfig: &ypb.SSAProjectScanRuleConfig{
				RuleFilter: &ypb.SyntaxFlowRuleFilter{
					Language: []string{"java"},
					Severity: []string{"high"},
				},
			},
		}

		schemaProject, err := yakit.CreateSSAProject(consts.GetGormProfileDatabase(), testProject)
		require.NoError(t, err)
		require.NotNil(t, schemaProject)

		defer func() {
			// 清理测试数据
			_ = consts.GetGormProfileDatabase().Unscoped().Delete(schemaProject)
		}()

		// 测试使用项目规则配置初始化扫描任务
		taskId := uuid.NewString()
		task, err := createSyntaxflowTaskById(context.Background(), "", taskId, NewScanConfig(
			WithControlMode(""),
			WithProjectId(uint64(schemaProject.ID)),
			WithProgramNames(schemaProject.ProjectName),
		))
		require.NoError(t, err)
		require.NotNil(t, task)

		// 验证规则数量正确获取
		require.True(t, task.rulesCount >= 0)
		require.NotNil(t, task.ruleChan)

		// 验证任务类型设置正确
		require.Equal(t, schema.SFResultKindScan, task.kind)
	})

	t.Run("test invalid SSA project ID", func(t *testing.T) {
		// 测试使用无效的项目ID
		taskId := uuid.NewString()
		_, err := createSyntaxflowTaskById(context.Background(), "", taskId, NewScanConfig(
			WithControlMode(""),
			WithProjectId(99999), // 不存在的项目ID
		))
		require.Error(t, err)
		require.Contains(t, err.Error(), "query ssa project by id failed")
	})
}
