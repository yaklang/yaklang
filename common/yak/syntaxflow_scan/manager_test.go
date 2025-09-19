package syntaxflow_scan

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestManager(t *testing.T) {
	t.Run("test save and resume scan task", func(t *testing.T) {
		taskId := uuid.NewString()
		task, err := createSyntaxflowTaskById(taskId, context.Background(), &ypb.SyntaxFlowScanRequest{
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
		task1, err := createSyntaxflowTaskById(taskId1, context.Background(), &ypb.SyntaxFlowScanRequest{
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
		task2, err := createSyntaxflowTaskById(taskId2, context.Background(), &ypb.SyntaxFlowScanRequest{
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
		task3, err := createSyntaxflowTaskById(taskId3, context.Background(), &ypb.SyntaxFlowScanRequest{
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
		task, err := createSyntaxflowTaskById(taskId, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode:  "",
			SSAProjectId: uint64(schemaProject.ID),
			ProgramName:  []string{}, // 空程序名，应该从项目配置中获取
		}, nil, &scanInputConfig{
			ProcessCallback:     func(progress float64) {},
			RuleProcessCallback: func(progName, ruleName string, progress float64) {},
		})
		require.NoError(t, err)
		require.NotNil(t, task)

		// 验证任务配置是否正确从项目中读取
		config, err := schemaProject.GetConfig()
		require.NoError(t, err)
		sc := config.ScanConfig
		require.Equal(t, []string{schemaProject.ProjectName}, task.programs)
		require.Equal(t, sc.IgnoreLanguage, task.ignoreLanguage)
		require.Equal(t, sc.Memory, task.memory)
		require.Equal(t, sc.Concurrency, task.concurrency)

		// 测试项目配置被正确覆盖
		taskId2 := uuid.NewString()
		task2, err := createSyntaxflowTaskById(taskId2, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode:    "",
			SSAProjectId:   uint64(schemaProject.ID),
			ProgramName:    []string{"custom-program"}, // 自定义程序名
			IgnoreLanguage: false,                      // 覆盖项目设置
			Memory:         false,                      // 覆盖项目设置
			Concurrency:    16,                         // 覆盖项目设置
		}, nil, &scanInputConfig{
			ProcessCallback:     func(progress float64) {},
			RuleProcessCallback: func(progName, ruleName string, progress float64) {},
		})
		require.NoError(t, err)
		require.NotNil(t, task2)

		// 验证自定义配置优先于项目配置
		require.Equal(t, []string{"custom-program"}, task2.programs)
		require.Equal(t, false, task2.ignoreLanguage)
		require.Equal(t, false, task2.memory)
		require.Equal(t, uint32(16), task2.concurrency)
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
		task, err := createSyntaxflowTaskById(taskId, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode:  "",
			SSAProjectId: uint64(schemaProject.ID),
			ProgramName:  []string{schemaProject.ProjectName},
		}, nil, &scanInputConfig{
			ProcessCallback:     func(progress float64) {},
			RuleProcessCallback: func(progName, ruleName string, progress float64) {},
		})
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
		_, err := createSyntaxflowTaskById(taskId, context.Background(), &ypb.SyntaxFlowScanRequest{
			ControlMode:  "",
			SSAProjectId: 99999, // 不存在的项目ID
			ProgramName:  []string{},
		}, nil, &scanInputConfig{
			ProcessCallback:     func(progress float64) {},
			RuleProcessCallback: func(progName, ruleName string, progress float64) {},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "query ssa project by id failed")
	})
}
