package test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newMinimalCoordinator(t *testing.T, userInput string) *aid.Coordinator {
	t.Helper()
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	c, err := aid.NewCoordinator(
		userInput,
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	return c
}

func TestExtractTaskFromRawResponse_OldFormat_NoIdentifierNoDependsOn(t *testing.T) {
	c := newMinimalCoordinator(t, "test backward compat")
	raw := `{
		"@action": "plan",
		"main_task": "扫描目录结构",
		"main_task_goal": "扫描并列出目录中所有文件",
		"tasks": [
			{
				"subtask_name": "遍历文件",
				"subtask_goal": "递归遍历目录中所有文件"
			},
			{
				"subtask_name": "统计大小",
				"subtask_goal": "计算每个文件的大小"
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, "扫描目录结构", task.Name)
	assert.Equal(t, "扫描并列出目录中所有文件", task.Goal)
	assert.NotEmpty(t, task.SemanticIdentifier, "SemanticIdentifier should be auto-generated even without main_task_identifier")
	require.Len(t, task.Subtasks, 2)

	sub1 := task.Subtasks[0]
	assert.Equal(t, "遍历文件", sub1.Name)
	assert.Equal(t, "递归遍历目录中所有文件", sub1.Goal)
	assert.Equal(t, []string{"1"}, sub1.DependsOn, "missing depends_on should default to previous DFS task")
	assert.NotEmpty(t, sub1.SemanticIdentifier, "SemanticIdentifier should be auto-generated")

	sub2 := task.Subtasks[1]
	assert.Equal(t, "统计大小", sub2.Name)
	assert.Equal(t, []string{"1-1"}, sub2.DependsOn)
}

func TestExtractTaskFromRawResponse_OldFormat_WithTaskNameTaskDescription(t *testing.T) {
	c := newMinimalCoordinator(t, "test old field names")
	raw := `{
		"@action": "plan",
		"main_task": "为项目添加自动化检查",
		"main_task_goal": "集成CI/CD检查工具",
		"tasks": [
			{
				"subtask_name": "配置工具",
				"subtask_goal": "安装并配置分析工具"
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, "为项目添加自动化检查", task.Name)
	require.Len(t, task.Subtasks, 1)
	assert.Equal(t, "配置工具", task.Subtasks[0].Name)
}

func TestExtractTaskFromRawResponse_NewFormat_WithIdentifierAndDependsOn(t *testing.T) {
	c := newMinimalCoordinator(t, "test new format")
	raw := `{
		"@action": "plan",
		"main_task": "为项目添加代码质量检查",
		"main_task_identifier": "add_code_quality_check",
		"main_task_goal": "在CI/CD中集成代码质量检查",
		"tasks": [
			{
				"subtask_name": "配置静态分析工具",
				"subtask_identifier": "setup_static_analysis",
				"subtask_goal": "安装并配置静态代码分析工具",
				"depends_on": []
			},
			{
				"subtask_name": "集成到CI/CD",
				"subtask_identifier": "integrate_cicd",
				"subtask_goal": "修改CI/CD配置，添加检查步骤",
				"depends_on": ["配置静态分析工具"]
			},
			{
				"subtask_name": "编写文档",
				"subtask_identifier": "write_docs",
				"subtask_goal": "编写使用文档",
				"depends_on": ["配置静态分析工具", "集成到CI/CD"]
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, "为项目添加代码质量检查", task.Name)
	assert.Equal(t, "在CI/CD中集成代码质量检查", task.Goal)

	require.Len(t, task.Subtasks, 3)

	sub1 := task.Subtasks[0]
	assert.Equal(t, "配置静态分析工具", sub1.Name)
	assert.Equal(t, "setup_static_analysis", sub1.SemanticIdentifier)
	assert.Equal(t, []string{"1"}, sub1.DependsOn)

	sub2 := task.Subtasks[1]
	assert.Equal(t, "集成到CI/CD", sub2.Name)
	assert.Equal(t, "integrate_cicd", sub2.SemanticIdentifier)
	assert.Equal(t, []string{"配置静态分析工具"}, sub2.DependsOn)

	sub3 := task.Subtasks[2]
	assert.Equal(t, "编写文档", sub3.Name)
	assert.Equal(t, "write_docs", sub3.SemanticIdentifier)
	assert.Equal(t, []string{"配置静态分析工具", "集成到CI/CD"}, sub3.DependsOn)
}

func TestExtractTaskFromRawResponse_MixedFormat_SomeWithIdentifier(t *testing.T) {
	c := newMinimalCoordinator(t, "test mixed format")
	raw := `{
		"@action": "plan",
		"main_task": "混合格式测试",
		"main_task_goal": "测试混合的任务格式",
		"tasks": [
			{
				"subtask_name": "有标识符的任务",
				"subtask_identifier": "task_with_id",
				"subtask_goal": "这个任务有标识符",
				"depends_on": []
			},
			{
				"subtask_name": "没有标识符的任务",
				"subtask_goal": "这个任务没有标识符"
			},
			{
				"subtask_name": "只有依赖的任务",
				"subtask_goal": "这个任务只有依赖关系",
				"depends_on": ["有标识符的任务"]
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Len(t, task.Subtasks, 3)

	sub1 := task.Subtasks[0]
	assert.Equal(t, "task_with_id", sub1.SemanticIdentifier)
	assert.Equal(t, []string{"1"}, sub1.DependsOn)

	sub2 := task.Subtasks[1]
	assert.NotEmpty(t, sub2.SemanticIdentifier, "auto-generated identifier expected")
	assert.NotEqual(t, "task_with_id", sub2.SemanticIdentifier)
	assert.Equal(t, []string{"1-1"}, sub2.DependsOn)

	sub3 := task.Subtasks[2]
	assert.NotEmpty(t, sub3.SemanticIdentifier)
	assert.Equal(t, []string{"有标识符的任务"}, sub3.DependsOn)
}

func TestExtractTaskFromRawResponse_EmptyDependsOn(t *testing.T) {
	c := newMinimalCoordinator(t, "test empty depends_on")
	raw := `{
		"@action": "plan",
		"main_task": "空依赖测试",
		"main_task_goal": "测试空依赖数组",
		"tasks": [
			{
				"subtask_name": "独立任务",
				"subtask_goal": "不依赖任何任务",
				"depends_on": []
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.Len(t, task.Subtasks, 1)
	assert.Equal(t, []string{"1"}, task.Subtasks[0].DependsOn)
}

func TestExtractPlan_OldFormat_BackwardCompat(t *testing.T) {
	c := newMinimalCoordinator(t, "test ExtractPlan old format")
	raw := `{
		"@action": "plan",
		"main_task": "找出最大文件",
		"main_task_goal": "在目录中找到最大的文件",
		"tasks": [
			{
				"subtask_name": "扫描目录",
				"subtask_goal": "递归扫描目录"
			},
			{
				"subtask_name": "比较大小",
				"subtask_goal": "比较文件大小并排序"
			}
		]
	}`

	planResp, err := aid.ExtractPlan(c, raw)
	require.NoError(t, err)
	require.NotNil(t, planResp)
	require.NotNil(t, planResp.RootTask)

	assert.Equal(t, "找出最大文件", planResp.RootTask.Name)
	require.Len(t, planResp.RootTask.Subtasks, 2)

	assert.Equal(t, "1", planResp.RootTask.Index)
	assert.Equal(t, "1-1", planResp.RootTask.Subtasks[0].Index)
	assert.Equal(t, "1-2", planResp.RootTask.Subtasks[1].Index)
	assert.Equal(t, []string{"1"}, planResp.RootTask.Subtasks[0].DependsOn)
	assert.Equal(t, []string{"1-1"}, planResp.RootTask.Subtasks[1].DependsOn)
}

func TestAiTask_DependsOn_Field(t *testing.T) {
	t.Run("NoDependsOn", func(t *testing.T) {
		task := &aid.AiTask{
			Name: "independent",
			Goal: "runs independently",
		}
		assert.Nil(t, task.DependsOn)
	})

	t.Run("WithDependsOn", func(t *testing.T) {
		task := &aid.AiTask{
			Name:      "dependent",
			Goal:      "depends on others",
			DependsOn: []string{"task_a", "task_b"},
		}
		assert.Equal(t, []string{"task_a", "task_b"}, task.DependsOn)
	})

	t.Run("EmptyDependsOn", func(t *testing.T) {
		task := &aid.AiTask{
			Name:      "empty_deps",
			Goal:      "empty deps list",
			DependsOn: []string{},
		}
		assert.Empty(t, task.DependsOn)
	})
}

func TestAiTask_SemanticIdentifier_GetSet(t *testing.T) {
	t.Run("DefaultEmpty", func(t *testing.T) {
		task := &aid.AiTask{Name: "test"}
		assert.Equal(t, "test", task.GetSemanticIdentifier())
	})

	t.Run("ExplicitSet", func(t *testing.T) {
		task := &aid.AiTask{
			Name:               "my task",
			SemanticIdentifier: "my_task_id",
		}
		assert.Equal(t, "my_task_id", task.GetSemanticIdentifier())
	})

	t.Run("SetViaMethod", func(t *testing.T) {
		task := &aid.AiTask{
			Name:               "my task",
			AIStatefulTaskBase: aicommon.NewStatefulTaskBase("test", "goal", context.Background(), nil),
		}
		task.SetSemanticIdentifier("custom_id")
		assert.Equal(t, "custom_id", task.GetSemanticIdentifier())
		assert.Equal(t, "custom_id", task.SemanticIdentifier)
	})
}

func TestPlanMocker_OldFormat_NoIdentifier(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	planMockerCalled := false
	c, err := aid.NewCoordinator(
		"test plan mocker backward compat",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			select {
			case outputChan <- event:
			default:
			}
		}),
		aid.WithPlanMocker(func(coordinator *aid.Coordinator) *aid.PlanResponse {
			planMockerCalled = true
			return &aid.PlanResponse{
				RootTask: &aid.AiTask{
					Name: "old-format-root",
					Goal: "test old format plan mocker",
					Subtasks: []*aid.AiTask{
						{
							Name: "subtask-without-identifier",
							Goal: "this subtask has no identifier or depends_on",
						},
						{
							Name: "another-subtask",
							Goal: "another subtask without new fields",
						},
					},
				},
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "done", "task_short_summary": "done", "task_long_summary": "done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		_ = c.Run()
	}()

	require.Eventually(t, func() bool {
		return planMockerCalled
	}, 10*1000*1000*1000, 100*1000*1000, "PlanMocker should be called")
}

func TestPlanMocker_NewFormat_WithIdentifierAndDependsOn(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	planMockerCalled := false
	c, err := aid.NewCoordinator(
		"test plan mocker new format",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			select {
			case outputChan <- event:
			default:
			}
		}),
		aid.WithPlanMocker(func(coordinator *aid.Coordinator) *aid.PlanResponse {
			planMockerCalled = true
			return &aid.PlanResponse{
				RootTask: &aid.AiTask{
					Name:               "new-format-root",
					Goal:               "test new format plan mocker",
					SemanticIdentifier: "new_format_root",
					Subtasks: []*aid.AiTask{
						{
							Name:               "setup-env",
							Goal:               "setup development environment",
							SemanticIdentifier: "setup_dev_env",
						},
						{
							Name:               "write-tests",
							Goal:               "write unit tests",
							SemanticIdentifier: "write_unit_tests",
							DependsOn:          []string{"setup-env"},
						},
					},
				},
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "done", "task_short_summary": "done", "task_long_summary": "done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		_ = c.Run()
	}()

	require.Eventually(t, func() bool {
		return planMockerCalled
	}, 10*1000*1000*1000, 100*1000*1000, "PlanMocker should be called")
}

func TestExtractTaskFromRawResponse_IndexGeneration(t *testing.T) {
	c := newMinimalCoordinator(t, "test index generation")
	raw := `{
		"@action": "plan",
		"main_task": "索引生成测试",
		"main_task_goal": "验证任务索引自动生成",
		"tasks": [
			{
				"subtask_name": "任务A",
				"subtask_goal": "目标A",
				"depends_on": []
			},
			{
				"subtask_name": "任务B",
				"subtask_goal": "目标B",
				"depends_on": ["任务A"]
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, "1", task.Index, "root index")
	require.Len(t, task.Subtasks, 2)
	assert.Equal(t, "1-1", task.Subtasks[0].Index, "first subtask index")
	assert.Equal(t, "1-2", task.Subtasks[1].Index, "second subtask index")

	for _, sub := range task.Subtasks {
		assert.Equal(t, task, sub.ParentTask, "subtask parent should be set")
	}
}

func TestExtractTaskFromRawResponse_SkipsEmptySubtaskName(t *testing.T) {
	c := newMinimalCoordinator(t, "test skip empty name")
	raw := `{
		"@action": "plan",
		"main_task": "过滤空名任务",
		"main_task_goal": "跳过空名子任务",
		"tasks": [
			{
				"subtask_name": "有效任务",
				"subtask_goal": "这是有效任务"
			},
			{
				"subtask_name": "",
				"subtask_goal": "这个任务没有名字应该被跳过"
			},
			{
				"subtask_goal": "这个任务也没有名字"
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.Len(t, task.Subtasks, 1)
	assert.Equal(t, "有效任务", task.Subtasks[0].Name)
}

func TestExtractTaskFromRawResponse_MainTaskIdentifier(t *testing.T) {
	t.Run("WithMainTaskIdentifier", func(t *testing.T) {
		c := newMinimalCoordinator(t, "test main task identifier")
		raw := `{
			"@action": "plan",
			"main_task": "创建市场推广计划",
			"main_task_identifier": "create_marketing_plan",
			"main_task_goal": "制定完整的市场推广策略",
			"tasks": [
				{
					"subtask_name": "市场调研",
					"subtask_goal": "分析目标市场"
				}
			]
		}`

		task, err := aid.ExtractTaskFromRawResponse(c, raw)
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, "创建市场推广计划", task.Name)
	})

	t.Run("WithoutMainTaskIdentifier", func(t *testing.T) {
		c := newMinimalCoordinator(t, "test without main task identifier")
		raw := `{
			"@action": "plan",
			"main_task": "创建市场推广计划",
			"main_task_goal": "制定完整的市场推广策略",
			"tasks": [
				{
					"subtask_name": "市场调研",
					"subtask_goal": "分析目标市场"
				}
			]
		}`

		task, err := aid.ExtractTaskFromRawResponse(c, raw)
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, "创建市场推广计划", task.Name)
		assert.NotEmpty(t, task.SemanticIdentifier, "should auto-generate SemanticIdentifier")
	})
}

func TestExtractTaskFromRawResponse_SubtaskIdentifierOverridesAutoGenerated(t *testing.T) {
	c := newMinimalCoordinator(t, "test identifier override")
	raw := `{
		"@action": "plan",
		"main_task": "标识符覆盖测试",
		"main_task_goal": "验证显式标识符覆盖自动生成",
		"tasks": [
			{
				"subtask_name": "非常非常长的任务名称用来测试自动生成的标识符是否会被显式覆盖",
				"subtask_identifier": "short_id",
				"subtask_goal": "验证覆盖",
				"depends_on": []
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.Len(t, task.Subtasks, 1)

	assert.Equal(t, "short_id", task.Subtasks[0].SemanticIdentifier,
		"explicit subtask_identifier should be used instead of auto-generated")
}

func TestExtractTaskFromRawResponse_MultipleDependencies(t *testing.T) {
	c := newMinimalCoordinator(t, "test multiple deps")
	raw := `{
		"@action": "plan",
		"main_task": "多依赖测试",
		"main_task_goal": "验证多个依赖关系正确解析",
		"tasks": [
			{
				"subtask_name": "基础任务A",
				"subtask_goal": "基础任务",
				"depends_on": []
			},
			{
				"subtask_name": "基础任务B",
				"subtask_goal": "基础任务",
				"depends_on": []
			},
			{
				"subtask_name": "聚合任务",
				"subtask_goal": "依赖A和B",
				"depends_on": ["基础任务A", "基础任务B"]
			}
		]
	}`

	task, err := aid.ExtractTaskFromRawResponse(c, raw)
	require.NoError(t, err)
	require.Len(t, task.Subtasks, 3)

	assert.Equal(t, []string{"1"}, task.Subtasks[0].DependsOn)
	assert.Equal(t, []string{"1-1"}, task.Subtasks[1].DependsOn)
	assert.Equal(t, []string{"基础任务A", "基础任务B"}, task.Subtasks[2].DependsOn)
}
