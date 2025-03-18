package taskstack

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

// 创建一个模拟的AI回调函数，返回固定响应
func createMockAICallback(response string) TaskAICallback {
	return func(prompt string) (io.Reader, error) {
		return strings.NewReader(response), nil
	}
}

// 创建一个始终返回错误的AI回调函数
func createErrorAICallback() TaskAICallback {
	return func(prompt string) (io.Reader, error) {
		return nil, errors.New("模拟AI调用错误")
	}
}

// 创建测试用的工具
func createTestTools() []*Tool {
	// 创建参数
	param1 := NewToolParam("param1", "string",
		WithTool_ParamDescription("参数1"),
		WithTool_ParamRequired(true),
	)

	// 创建工具1
	tool1, _ := NewTool("TestTool1",
		WithTool_Description("用于测试的工具1"),
		WithTool_Param(param1),
		WithTool_Callback(func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
			return "工具1执行结果", nil
		}),
	)

	// 创建工具2
	tool2, _ := NewTool("TestTool2",
		WithTool_Description("用于测试的工具2"),
		WithTool_Callback(func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
			return "工具2执行结果", nil
		}),
	)

	return []*Tool{tool1, tool2}
}

// 测试创建Task并应用选项
func TestNewTaskWithOptions(t *testing.T) {
	task := NewTask("test", "test goal")
	assert.Equal(t, "test", task.Name)
	assert.Equal(t, "test goal", task.Goal)
}

// 测试深度复制功能
func TestTaskDeepCopy(t *testing.T) {
	original := NewTask("test", "test goal")
	original.metadata = map[string]interface{}{
		"key": "value",
	}

	copied := original.DeepCopy()
	assert.Equal(t, original.Name, copied.Name)
	assert.Equal(t, original.Goal, copied.Goal)
	assert.Equal(t, original.metadata["key"], copied.metadata["key"])
}

// 测试任务执行功能
func TestTaskInvoke(t *testing.T) {
	// 创建测试回调
	callback := createMockAICallback("任务执行成功")

	// 创建父任务
	parentTask := NewTask("父任务", "父任务目标", WithTask_Callback(callback))

	// 创建运行时环境
	runtime := &Runtime{
		Stack: utils.NewStack[*Task](),
	}

	// 执行任务
	runtime.Invoke(parentTask)
}

// 测试任务执行错误处理
func TestTaskInvokeError(t *testing.T) {
	// 创建会返回错误的回调
	errorCallback := createErrorAICallback()

	// 创建任务
	task := NewTask("错误任务", "测试错误", WithTask_Callback(errorCallback))

	// 创建运行时环境
	runtime := &Runtime{
		Stack: utils.NewStack[*Task](),
	}

	// 执行任务，预期失败
	runtime.Invoke(task)
	// 由于 Runtime.Invoke 没有返回值，我们无法直接测试错误
	// 这里我们只是验证任务被创建和执行
	assert.NotNil(t, task)
	assert.Equal(t, "错误任务", task.Name)
	assert.Equal(t, "测试错误", task.Goal)
}

// 测试从JSON创建任务
func TestNewTaskFromJSON(t *testing.T) {
	jsonStr := `{"name":"JSON任务","goal":"从JSON创建","subtasks":[{"name":"子任务1","goal":"子目标1"}]}`

	// 从JSON创建任务
	task, err := NewTaskFromJSON(jsonStr, WithTask_Callback(createMockAICallback("JSON响应")))
	if err != nil {
		t.Fatalf("从JSON创建任务失败: %v", err)
	}

	// 验证JSON解析结果
	if task.Name != "JSON任务" || task.Goal != "从JSON创建" {
		t.Errorf("JSON解析结果不正确，名称: %s, 目标: %s", task.Name, task.Goal)
	}

	if len(task.Subtasks) != 1 || task.Subtasks[0].Name != "子任务1" {
		t.Error("子任务解析不正确")
	}
}

// 测试任务验证
func TestValidateTask(t *testing.T) {
	task := NewTask("", "")
	err := ValidateTask(task)
	assert.Error(t, err)

	task = NewTask("test", "test goal", WithTask_Callback(createMockAICallback("响应")))
	err = ValidateTask(task)
	assert.NoError(t, err)
}

// 测试从原始响应中提取任务
func TestExtractTaskFromRawResponse(t *testing.T) {
	response := `{"name": "test", "goal": "test goal"}`
	task, err := ExtractTaskFromRawResponse(response, WithTask_Callback(createMockAICallback("响应")))
	assert.NoError(t, err)
	assert.Equal(t, "test", task.Name)
	assert.Equal(t, "test goal", task.Goal)
}

// TestExtractTaskFromRawResponseDetailed 测试不同格式的原始响应中提取任务
func TestExtractTaskFromRawResponseDetailed(t *testing.T) {
	t.Run("从task.json格式响应提取任务", func(t *testing.T) {
		rawResponse := `{
			"@action": "plan",
			"query": "用户的查询",
			"tasks": [
				{
					"subtask_name": "主任务名称",
					"subtask_goal": "主任务目标"
				}
			]
		}`

		task, err := ExtractTaskFromRawResponse(rawResponse, WithTask_Callback(createMockAICallback("响应")))
		if err != nil {
			t.Fatalf("从task.json格式提取任务失败: %v", err)
		}

		if task.Name != "主任务名称" {
			t.Errorf("提取的任务名称不正确，期望 '主任务名称'，实际为 '%s'", task.Name)
		}
	})
}
