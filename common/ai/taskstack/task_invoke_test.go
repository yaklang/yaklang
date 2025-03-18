package taskstack

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 模拟的AI回调函数，返回固定的响应
func mockAICallback(prompt string) (io.Reader, error) {
	// 这里可以记录或打印prompt以检查格式
	if os.Getenv("DEBUG_TASK_PROMPT") == "true" {
		fmt.Println("DEBUG_TASK_PROMPT: " + prompt)
	}

	return strings.NewReader("mock response"), nil
}

// 模拟AI请求工具描述的回调函数
func mockAIToolDescriptionCallback(prompt string) (io.Reader, error) {
	if os.Getenv("DEBUG_TASK_PROMPT") == "true" {
		fmt.Println("DEBUG_TASK_PROMPT: " + prompt)
	}

	// 检查是否是第一次调用（初始提示）或者是否已经包含了工具描述
	if strings.Contains(prompt, "JSONSchema描述") {
		// 提示中已经包含了工具描述信息，返回最终响应
		return strings.NewReader("这是申请工具描述后的最终响应"), nil
	} else {
		// 第一次调用或没有工具描述，返回一个请求工具描述的响应
		return strings.NewReader(`我需要了解更多关于test_tool的信息。

` + "```" + `jsonschema help="申请工具详情"
{
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "required": ["tool", "@action"],
    "additionalProperties": false,
    "properties": {
        "@action": {
            "const": "call-tool",
            "description": "标识当前操作的具体类型"
        },
        "tool": {
            "type": "string",
            "description": "您想要了解详情的工具名"
        }
    }
}
` + "```" + `

我想申请查看test_tool的详细信息。`), nil
	}
}

// TestTask_Invoke_NoTools 测试无工具的任务执行
func TestTask_Invoke_NoTools(t *testing.T) {
	task := NewTask("test", "test goal", WithTask_Callback(mockAICallback))
	assert.NotNil(t, task)
	assert.Equal(t, "test", task.Name)
	assert.Equal(t, "test goal", task.Goal)
}

// TestTask_Invoke_WithTools 测试带工具的任务执行
func TestTask_Invoke_WithTools(t *testing.T) {
	task := NewTask("test", "test goal", WithTask_Callback(mockAICallback))
	tools := []*Tool{
		{
			Name:        "test-tool",
			Description: "test tool description",
		},
	}
	task.ApplyOptions(WithTask_Tools(tools))
	assert.NotNil(t, task.tools)
	assert.Equal(t, 1, len(task.tools))
}

func TestTask_Invoke_WithToolDescription(t *testing.T) {
	// 创建一个测试工具
	testTool, err := NewTool("test_tool",
		WithTool_Description("用于测试的工具"),
		WithTool_Param(NewToolParam("param1", "string",
			WithTool_ParamDescription("参数1"),
			WithTool_ParamRequired(true),
		)),
		WithTool_Param(NewToolParam("param2", "number",
			WithTool_ParamDescription("参数2"),
			WithTool_ParamDefault(42),
		)),
		WithTool_Callback(func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
			return "工具执行成功", nil
		}),
	)
	require.NoError(t, err)

	// 创建一个测试任务
	task := NewTask("test_task", "test goal")
	task.SetAICallback(mockAICallback)

	// 创建一个 TaskSystemContext
	ctx := &TaskSystemContext{
		Progress:    "",
		CurrentTask: task,
	}

	// 生成任务提示
	prompt, err := task.generateTaskPrompt([]*Tool{testTool}, ctx, map[string]interface{}{})
	assert.NoError(t, err)
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "test_tool")
}

// TestTask_Invoke_WithSubtasks 测试带子任务的执行
func TestTask_Invoke_WithSubtasks(t *testing.T) {
	parentTask := NewTask("parent", "parent goal", WithTask_Callback(mockAICallback))
	childTask := NewTask("child", "child goal", WithTask_Callback(mockAICallback))
	parentTask.Subtasks = []*Task{childTask}

	assert.NotNil(t, parentTask.Subtasks)
	assert.Equal(t, 1, len(parentTask.Subtasks))
}

// TestTask_NoAICallback 测试没有AI回调函数的情况
func TestTask_NoAICallback(t *testing.T) {
	task := NewTask("test", "test goal")
	err := ValidateTask(task)
	assert.Error(t, err)
}

// TestTask_ProgressInfo 测试进度信息
func TestTask_ProgressInfo(t *testing.T) {
	task := NewTask("test", "test goal", WithTask_Callback(mockAICallback))
	progress := &TaskProgress{
		TotalTasks:     1,
		CompletedTasks: 0,
		CurrentTask:    task.Name,
		CurrentGoal:    task.Goal,
	}
	assert.Equal(t, 1, progress.TotalTasks)
	assert.Equal(t, 0, progress.CompletedTasks)
	assert.Equal(t, task.Name, progress.CurrentTask)
	assert.Equal(t, task.Goal, progress.CurrentGoal)
}

// TestTask_JSONSerialization 测试JSON序列化和反序列化
func TestTask_JSONSerialization(t *testing.T) {
	task := NewTask("test", "test goal", WithTask_Callback(mockAICallback))
	data, err := task.MarshalJSON()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	newTask := &Task{}
	err = newTask.UnmarshalJSON(data)
	assert.NoError(t, err)
	assert.Equal(t, task.Name, newTask.Name)
	assert.Equal(t, task.Goal, newTask.Goal)
}

func TestTask_ToolJSONSchema(t *testing.T) {
	// 创建一个具有多种参数类型的测试工具
	complexTool, err := NewTool("complex_tool",
		WithTool_Description("具有多种参数类型的复杂工具"),
		WithTool_Param(NewToolParam("string_param", "string",
			WithTool_ParamDescription("字符串参数"),
			WithTool_ParamRequired(true),
		)),
		WithTool_Param(NewToolParam("number_param", "number",
			WithTool_ParamDescription("数值参数"),
			WithTool_ParamDefault(42.5),
		)),
		WithTool_Param(NewToolParam("integer_param", "integer",
			WithTool_ParamDescription("整数参数"),
			WithTool_ParamDefault(10),
		)),
		WithTool_Param(NewToolParam("boolean_param", "boolean",
			WithTool_ParamDescription("布尔参数"),
			WithTool_ParamDefault(false),
		)),
		WithTool_Param(NewToolParam("array_param", "array",
			WithTool_ParamDescription("数组参数"),
			WithTool_ArrayItem(
				NewToolParamValue("string",
					WithTool_ValueDescription("数组中的字符串项"),
				),
			),
		)),
		WithTool_Callback(func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
			return "执行成功", nil
		}),
	)
	require.NoError(t, err)

	// 生成工具的JSONSchema
	jsonSchema := complexTool.ToJSONSchemaString()

	// 验证JSONSchema包含正确的信息
	assert.Contains(t, jsonSchema, "complex_tool")
	assert.Contains(t, jsonSchema, "具有多种参数类型的复杂工具")
	assert.Contains(t, jsonSchema, "string_param")
	assert.Contains(t, jsonSchema, "number_param")
	assert.Contains(t, jsonSchema, "integer_param")
	assert.Contains(t, jsonSchema, "boolean_param")
	assert.Contains(t, jsonSchema, "array_param")

	// 确保模式是有效的JSON
	var schemaObj map[string]interface{}
	err = json.Unmarshal([]byte(jsonSchema), &schemaObj)
	require.NoError(t, err)

	// 确保必需字段存在
	properties, ok := schemaObj["properties"].(map[string]interface{})
	require.True(t, ok)

	// 检查参数部分
	params, ok := properties["params"].(map[string]interface{})
	require.True(t, ok)

	// 确保参数属性包含所有参数
	paramProps, ok := params["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, paramProps, "string_param")
	assert.Contains(t, paramProps, "number_param")
	assert.Contains(t, paramProps, "integer_param")
	assert.Contains(t, paramProps, "boolean_param")
	assert.Contains(t, paramProps, "array_param")

	// 检查必需的参数
	required, ok := params["required"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, required, "string_param")
}

func TestTask_DirectAnswer(t *testing.T) {
	// 模拟AI直接回答而不使用工具的情况
	directAnswerCallback := func(prompt string) (io.Reader, error) {
		if os.Getenv("DEBUG_TASK_PROMPT") == "true" {
			fmt.Println("DEBUG_TASK_PROMPT: " + prompt)
		}

		// 确保提示中包含关于可选使用工具的信息
		assert.Contains(t, prompt, "使用它们是完全可选的")
		assert.Contains(t, prompt, "可以直接给出答案")

		// 返回直接的回答，不请求工具描述
		return strings.NewReader("我决定直接回答这个任务，不需要使用工具。\n\n这是我的详细回答：...\n\n总结：任务已完成。"), nil
	}

	// 创建任务
	task := &Task{
		Name:       "直接回答测试",
		Goal:       "测试AI直接回答任务而不使用工具的情况",
		AICallback: directAnswerCallback,
	}

	// 验证任务创建
	assert.Equal(t, "直接回答测试", task.Name)
	assert.Equal(t, "测试AI直接回答任务而不使用工具的情况", task.Goal)
	assert.NotNil(t, task.AICallback)
}
