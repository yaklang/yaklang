package aid

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 模拟执行任务的函数
func mockExecuteTask(task aiTask) string {
	return fmt.Sprintf("已执行任务: %s - %s", task.Name, task.Goal)
}

// 模拟处理工具请求的函数
func mockProcessToolRequire(task aiTask, tools []*Tool) (*ToolResult, error) {
	// 根据任务选择合适的工具
	var selectedTool *Tool

	taskNameLower := strings.ToLower(task.Name)
	taskGoalLower := strings.ToLower(task.Goal)

	// 特定任务类型的工具匹配
	if strings.Contains(taskNameLower, "天气") || strings.Contains(taskGoalLower, "天气") ||
		strings.Contains(taskGoalLower, "降水") || strings.Contains(taskGoalLower, "温度") {
		// 寻找天气工具
		for _, tool := range tools {
			if strings.Contains(strings.ToLower(tool.Name), "weather") {
				selectedTool = tool
				break
			}
		}
	}

	if selectedTool == nil {
		return nil, fmt.Errorf("未找到合适的工具")
	}

	// 执行工具
	result := &ToolExecutionResult{
		Stdout: fmt.Sprintf("执行工具 %s", selectedTool.Name),
		Stderr: "",
		Result: map[string]interface{}{
			"status": "success",
			"data":   "工具执行结果",
		},
	}

	return &ToolResult{
		Success: true,
		Data:    result,
	}, nil
}

// 判断任务是否需要工具的函数
func taskNeedsTool(task aiTask, tools []*Tool) bool {
	taskNameLower := strings.ToLower(task.Name)
	taskGoalLower := strings.ToLower(task.Goal)

	// 针对特定任务类型的关键词匹配
	if strings.Contains(taskNameLower, "天气") || strings.Contains(taskGoalLower, "天气") ||
		strings.Contains(taskGoalLower, "降水") || strings.Contains(taskGoalLower, "温度") {
		return true
	}

	if strings.Contains(taskNameLower, "景点") || strings.Contains(taskGoalLower, "景点") ||
		strings.Contains(taskGoalLower, "attraction") || strings.Contains(taskGoalLower, "attractionapi") {
		return true
	}

	if strings.Contains(taskNameLower, "餐") || strings.Contains(taskGoalLower, "餐") ||
		strings.Contains(taskNameLower, "饮") || strings.Contains(taskGoalLower, "饮") ||
		strings.Contains(taskNameLower, "食") || strings.Contains(taskGoalLower, "食") {
		return true
	}

	if strings.Contains(taskNameLower, "交通") || strings.Contains(taskGoalLower, "交通") ||
		strings.Contains(taskGoalLower, "路线") || strings.Contains(taskGoalLower, "路程") {
		return true
	}

	if strings.Contains(taskNameLower, "时间") || strings.Contains(taskGoalLower, "时间") ||
		strings.Contains(taskNameLower, "行程") || strings.Contains(taskGoalLower, "行程") {
		return true
	}

	// 原有的通用工具匹配逻辑
	for _, tool := range tools {
		toolNameLower := strings.ToLower(tool.Name)

		// 去掉API后缀进行匹配
		toolKeyword := strings.Replace(toolNameLower, "api", "", -1)

		if strings.Contains(taskNameLower, toolKeyword) || strings.Contains(taskGoalLower, toolKeyword) {
			return true
		}
	}
	return false
}

// 测试任务执行与工具调用的集成
func TestTaskAndToolIntegration(t *testing.T) {
	task := NewTask("test", "test goal", WithTask_Callback(mockAICallback))
	tools := []*Tool{
		newTool("test-tool",
			WithTool_Description("test tool description"),
		),
	}
	task.ApplyOptions(WithTask_Tools(tools))

	assert.NotNil(t, task.tools)
	assert.Equal(t, 1, len(task.tools))
}

// 测试工具选择逻辑
func TestToolSelectionFromTask(t *testing.T) {
	task := NewTask("test", "test goal", WithTask_Callback(mockAICallback))
	tools := []*Tool{
		newTool("tool1",
			WithTool_Description("tool1 description"),
		),
		newTool("tool2",
			WithTool_Description("tool2 description"),
		),
	}
	task.ApplyOptions(WithTask_Tools(tools))

	assert.NotNil(t, task.tools)
	assert.Equal(t, 2, len(task.tools))
	assert.Equal(t, "tool1", task.tools[0].Name)
	assert.Equal(t, "tool2", task.tools[1].Name)
}
