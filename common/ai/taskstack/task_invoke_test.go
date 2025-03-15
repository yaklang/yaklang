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

	return strings.NewReader("这是AI的响应，成功执行了任务"), nil
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
            "const": "describe-tool",
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

func TestTask_Invoke_NoTools(t *testing.T) {
	// 创建一个简单的任务
	task := &Task{
		Name:       "测试任务",
		Goal:       "测试Task的Invoke方法（无工具）",
		AICallback: mockAICallback,
	}

	// 执行任务
	result, err := task.Invoke(nil, map[string]interface{}{
		"test_key": "test_value",
	})

	// 验证结果
	require.NoError(t, err)
	assert.Equal(t, "这是AI的响应，成功执行了任务", result)
}

func TestTask_Invoke_WithTools(t *testing.T) {
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

	// 创建一个简单的任务
	task := &Task{
		Name:       "测试任务",
		Goal:       "测试Task的Invoke方法（带工具）",
		AICallback: mockAICallback,
	}

	// 执行任务
	result, err := task.Invoke([]*Tool{testTool}, map[string]interface{}{
		"test_key": "test_value",
	})

	// 验证结果
	require.NoError(t, err)
	assert.Equal(t, "这是AI的响应，成功执行了任务", result)
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

	// 创建一个带有工具描述请求的任务
	task := &Task{
		Name:       "工具描述测试",
		Goal:       "测试请求工具描述功能",
		AICallback: mockAIToolDescriptionCallback,
	}

	// 获取工具描述
	toolDescription, err := task.handleDescribeTool([]*Tool{testTool}, "test_tool")
	require.NoError(t, err)
	assert.Contains(t, toolDescription, "JSONSchema描述")
	assert.Contains(t, toolDescription, "test_tool")

	// 模拟完整的处理流程
	// 1. 第一次调用AI回调，获取请求工具描述的响应
	firstPrompt, err := task.generateTaskPrompt([]*Tool{testTool}, map[string]interface{}{
		"progress": TaskProgress{
			TotalTasks:     1,
			CompletedTasks: 0,
			CurrentTask:    task.Name,
			CurrentGoal:    task.Goal,
		},
	})
	require.NoError(t, err)

	firstResponse, err := task.AICallback(firstPrompt)
	require.NoError(t, err)
	firstResponseBytes, err := io.ReadAll(firstResponse)
	require.NoError(t, err)

	// 2. 检查响应是否包含工具描述请求
	firstResponseStr := string(firstResponseBytes)
	assert.Contains(t, firstResponseStr, "describe-tool")
	assert.Contains(t, firstResponseStr, "test_tool")

	// 3. 构建包含工具描述的第二次提示
	secondPrompt := firstPrompt + "\n\n" + firstResponseStr + "\n\n" + toolDescription

	// 4. 第二次调用AI回调，应返回最终响应
	secondResponse, err := task.AICallback(secondPrompt)
	require.NoError(t, err)
	secondResponseBytes, err := io.ReadAll(secondResponse)
	require.NoError(t, err)

	// 5. 验证最终响应
	finalResponse := string(secondResponseBytes)
	assert.Equal(t, "这是申请工具描述后的最终响应", finalResponse)
}

func TestTask_Invoke_WithSubtasks(t *testing.T) {
	// 创建一个带子任务的任务
	task := &Task{
		Name:       "主任务",
		Goal:       "测试带子任务的Task Invoke方法",
		AICallback: mockAICallback,
		Subtasks: []Task{
			{
				Name:       "子任务1",
				Goal:       "子任务1的目标",
				AICallback: mockAICallback,
			},
			{
				Name:       "子任务2",
				Goal:       "子任务2的目标",
				AICallback: mockAICallback,
			},
		},
	}

	// 执行任务
	result, err := task.Invoke(nil, map[string]interface{}{
		"test_key": "test_value",
	})

	// 验证结果
	require.NoError(t, err)
	assert.Equal(t, "这是AI的响应，成功执行了任务", result)
}

func TestTask_NoAICallback(t *testing.T) {
	// 创建一个没有AI回调函数的任务
	task := &Task{
		Name: "无回调任务",
		Goal: "测试没有AI回调函数的情况",
	}

	// 执行任务，应该返回错误
	_, err := task.Invoke(nil, map[string]interface{}{})

	// 验证错误
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no AI callback function set")
}

func TestTask_ProgressInfo(t *testing.T) {
	// 创建一个检查进度信息的AI回调函数
	progressCheckCallback := func(prompt string) (io.Reader, error) {
		if os.Getenv("DEBUG_TASK_PROMPT") == "true" {
			fmt.Println("DEBUG_TASK_PROMPT: " + prompt)
		}

		// 检查prompt中是否包含正确的进度信息
		assert.Contains(t, prompt, "总任务数")
		assert.Contains(t, prompt, "已完成任务数")
		assert.Contains(t, prompt, "当前执行的任务")

		return strings.NewReader("进度信息验证成功"), nil
	}

	// 创建一个带子任务的任务
	task := &Task{
		Name:       "进度测试任务",
		Goal:       "测试任务进度信息",
		AICallback: progressCheckCallback,
		Subtasks: []Task{
			{
				Name:       "进度子任务",
				Goal:       "测试子任务进度信息",
				AICallback: progressCheckCallback,
			},
		},
	}

	// 执行任务
	result, err := task.Invoke(nil, map[string]interface{}{})

	// 验证结果
	require.NoError(t, err)
	assert.Equal(t, "进度信息验证成功", result)
}

// 测试JSON序列化和反序列化
func TestTask_JSONSerialization(t *testing.T) {
	// 创建一个任务
	originalTask := Task{
		Name: "JSON测试任务",
		Goal: "测试任务的JSON序列化和反序列化",
		Subtasks: []Task{
			{
				Name: "JSON子任务",
				Goal: "子任务的JSON测试",
			},
		},
	}

	// 序列化为JSON
	jsonData, err := json.Marshal(originalTask)
	require.NoError(t, err)

	// 反序列化
	var deserializedTask Task
	err = json.Unmarshal(jsonData, &deserializedTask)
	require.NoError(t, err)

	// 验证反序列化后的任务
	assert.Equal(t, originalTask.Name, deserializedTask.Name)
	assert.Equal(t, originalTask.Goal, deserializedTask.Goal)
	assert.Equal(t, len(originalTask.Subtasks), len(deserializedTask.Subtasks))
	assert.Equal(t, originalTask.Subtasks[0].Name, deserializedTask.Subtasks[0].Name)
	assert.Equal(t, originalTask.Subtasks[0].Goal, deserializedTask.Subtasks[0].Goal)
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

	// 创建一个使用工具描述的测试任务
	schemaCheckCallback := func(prompt string) (io.Reader, error) {
		if strings.Contains(prompt, "JSONSchema描述") {
			// 确保提示包含工具的JSONSchema
			assert.Contains(t, prompt, "complex_tool")
			assert.Contains(t, prompt, "具有多种参数类型的复杂工具")
		}
		return strings.NewReader("JSONSchema测试成功"), nil
	}

	task := &Task{
		Name:       "JSONSchema测试",
		Goal:       "测试工具JSONSchema描述",
		AICallback: schemaCheckCallback,
	}

	// 模拟一个请求工具描述的操作
	result, err := task.handleDescribeTool([]*Tool{complexTool}, "complex_tool")
	require.NoError(t, err)
	assert.Contains(t, result, "JSONSchema描述")
	assert.Contains(t, result, "complex_tool")
}

func TestTask_DirectAnswer(t *testing.T) {
	// 创建一些工具
	testTool1, err := NewTool("test_tool1",
		WithTool_Description("第一个测试工具"),
		WithTool_Param(NewToolParam("param1", "string",
			WithTool_ParamDescription("参数1"),
			WithTool_ParamRequired(true),
		)),
		WithTool_Callback(func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
			return "工具1执行成功", nil
		}),
	)
	require.NoError(t, err)

	testTool2, err := NewTool("test_tool2",
		WithTool_Description("第二个测试工具"),
		WithTool_Param(NewToolParam("param2", "number",
			WithTool_ParamDescription("参数2"),
			WithTool_ParamDefault(42),
		)),
		WithTool_Callback(func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
			return "工具2执行成功", nil
		}),
	)
	require.NoError(t, err)

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

	// 执行任务
	result, err := task.Invoke([]*Tool{testTool1, testTool2}, map[string]interface{}{})

	// 验证结果
	require.NoError(t, err)
	assert.Contains(t, result, "直接回答")
	assert.Contains(t, result, "不需要使用工具")
	assert.Contains(t, result, "任务已完成")
}
