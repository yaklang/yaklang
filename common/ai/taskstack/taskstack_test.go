package taskstack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

// 模拟执行任务的函数
func mockExecuteTask(task Task) string {
	return fmt.Sprintf("已执行任务: %s - %s", task.Name, task.Goal)
}

// 模拟处理工具请求的函数
func mockProcessToolRequire(task Task, tools []*Tool) (*ToolResult, error) {
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
	} else if strings.Contains(taskNameLower, "景点") || strings.Contains(taskGoalLower, "景点") ||
		strings.Contains(taskGoalLower, "attraction") || strings.Contains(taskGoalLower, "attractionapi") {
		// 寻找景点工具
		for _, tool := range tools {
			if strings.Contains(strings.ToLower(tool.Name), "attraction") {
				selectedTool = tool
				break
			}
		}
	} else if strings.Contains(taskNameLower, "餐") || strings.Contains(taskGoalLower, "餐") ||
		strings.Contains(taskNameLower, "饮") || strings.Contains(taskGoalLower, "饮") ||
		strings.Contains(taskNameLower, "食") || strings.Contains(taskGoalLower, "食") {
		// 寻找餐厅工具
		for _, tool := range tools {
			if strings.Contains(strings.ToLower(tool.Name), "restaurant") {
				selectedTool = tool
				break
			}
		}
	} else if strings.Contains(taskNameLower, "交通") || strings.Contains(taskGoalLower, "交通") ||
		strings.Contains(taskGoalLower, "路线") || strings.Contains(taskGoalLower, "路程") {
		// 寻找交通工具
		for _, tool := range tools {
			if strings.Contains(strings.ToLower(tool.Name), "transport") {
				selectedTool = tool
				break
			}
		}
	} else if strings.Contains(taskNameLower, "时间") || strings.Contains(taskGoalLower, "时间") ||
		strings.Contains(taskNameLower, "行程") || strings.Contains(taskGoalLower, "行程") {
		// 寻找时间评估工具
		for _, tool := range tools {
			if strings.Contains(strings.ToLower(tool.Name), "time") {
				selectedTool = tool
				break
			}
		}
	}

	// 如果通过关键词没找到，则尝试原来的通用匹配方法
	if selectedTool == nil {
		for _, tool := range tools {
			// 简单的匹配逻辑：检查任务名称或目标中是否包含工具名称的关键词
			toolNameLower := strings.ToLower(tool.Name)

			// 去掉API后缀进行匹配
			toolKeyword := strings.Replace(toolNameLower, "api", "", -1)

			if strings.Contains(taskNameLower, toolKeyword) || strings.Contains(taskGoalLower, toolKeyword) {
				selectedTool = tool
				break
			}
		}
	}

	if selectedTool == nil {
		return nil, fmt.Errorf("找不到适合任务 '%s' 的工具", task.Name)
	}

	// 构造工具调用参数
	toolParams := map[string]interface{}{}

	// 根据任务填充参数（简化版，实际应用中可能需要更复杂的参数提取）
	switch selectedTool.Name {
	case "WeatherAPI":
		toolParams["city"] = "北京"
		toolParams["date"] = "2025-03-16"
	case "AttractionAPI":
		toolParams["city"] = "北京"
		toolParams["preference"] = "历史文化"
	case "RestaurantAPI":
		toolParams["location"] = "故宫附近"
		toolParams["budget"] = "中等"
		toolParams["cuisine"] = "中式"
	case "TransportAPI":
		toolParams["origin"] = "故宫"
		toolParams["destination"] = "颐和园"
	case "TimeEstimateAPI":
		toolParams["locations"] = []string{"游览故宫", "午餐", "游览颐和园", "晚餐"}
	}

	// 创建JSON调用字符串
	invokeParams := ToolInvokeParams{
		Tool:   selectedTool.Name,
		Action: "invoke",
		Params: toolParams,
	}

	jsonBytes, err := json.Marshal(invokeParams)
	if err != nil {
		return nil, fmt.Errorf("构造工具调用参数失败: %v", err)
	}

	// 调用工具
	return selectedTool.InvokeWithJSON(string(jsonBytes))
}

// 判断任务是否需要工具的函数
func taskNeedsTool(task Task, tools []*Tool) bool {
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
	// 模拟复杂任务规划响应
	rawResponse := `{
    "@action": "plan",
    "query": "为一个带着8岁孩子的三口之家，在北京规划一个春季周末的一日游行程。预算为1000元，偏好历史文化景点和当地美食体验。",
    "tasks": [
        {
            "subtask_name": "检查北京天气情况",
            "subtask_goal": "获取计划出游日期的天气预报，包括温度范围、降水概率和活动建议，确定是否适合户外游览"
        },
        {
            "subtask_name": "筛选适合家庭的历史文化景点",
            "subtask_goal": "从AttractionAPI获取2-3个适合8岁儿童的历史文化景点，包括开放时间、票价、推荐游览时长和位置信息"
        },
        {
            "subtask_name": "规划早餐安排",
            "subtask_goal": "选择一个靠近第一个景点或住宿地的适合家庭用餐的早餐地点，考虑儿童友好程度和价格"
        },
        {
            "subtask_name": "规划午餐安排",
            "subtask_goal": "选择一个位于上午景点附近、提供当地特色美食的餐厅，须适合儿童且价格符合预算"
        },
        {
            "subtask_name": "规划晚餐安排",
            "subtask_goal": "选择一个位于下午景点附近或返程路线上的有特色餐厅，须适合家庭用餐且符合预算"
        },
        {
            "subtask_name": "规划景点间交通方式",
            "subtask_goal": "确定各个景点之间最合适的交通方式，包括时间、成本和便捷性分析，考虑有8岁儿童的实际需求"
        },
        {
            "subtask_name": "创建详细行程时间表",
            "subtask_goal": "制定从早到晚的完整时间安排，包括各景点游览时长、用餐时间、交通时间和必要的休息时间，确保合理可行"
        },
        {
            "subtask_name": "计算总预算花费",
            "subtask_goal": "汇总所有景点门票、餐饮费用和交通费用，确认总花费不超过1000元预算，并留出应急资金"
        },
        {
            "subtask_name": "制定应急备选方案",
            "subtask_goal": "针对可能的天气变化、景点关闭或其他突发情况，准备至少一套备选景点和活动方案，确保旅程顺利进行"
        }
    ]
}`

	// 1. 从原始响应中提取任务
	mainTask, err := ExtractTaskFromRawResponse(rawResponse)
	assert.NoError(t, err)
	assert.NotNil(t, mainTask)

	// 2. 创建运行时环境
	runtime := &Runtime{
		Task:  *mainTask,
		Stack: utils.NewStack[Task](),
	}

	// 3. 设置可用工具
	tools := GetAllMockTools()

	// 4. 将主任务压入堆栈，确保它被处理
	runtime.Stack.Push(*mainTask)

	// 5. 将子任务压入堆栈，顺序是从后往前，这样弹出时就是从前往后
	for i := len(mainTask.Subtasks) - 1; i >= 0; i-- {
		runtime.Stack.Push(mainTask.Subtasks[i])
	}

	// 创建一个自定义的工具需求检查函数
	customTaskNeedsTool := func(task Task, tools []*Tool) bool {
		// 对于计算预算和制定应急方案任务，不使用工具
		if strings.Contains(strings.ToLower(task.Name), "预算") ||
			strings.Contains(strings.ToLower(task.Name), "计算") {
			return false
		}
		if strings.Contains(strings.ToLower(task.Name), "应急") ||
			strings.Contains(strings.ToLower(task.Name), "备选") {
			return false
		}
		return taskNeedsTool(task, tools)
	}

	// 6. 执行任务堆栈
	var buffer bytes.Buffer
	for !runtime.Stack.IsEmpty() && !runtime.Freeze {
		// 弹出一个任务
		currentTask := runtime.Stack.Pop()

		// 判断任务是否需要工具（使用自定义函数）
		if customTaskNeedsTool(currentTask, tools) {
			// 需要工具，创建require-tool请求
			fmt.Fprintf(&buffer, "需要工具执行任务: %s\n", currentTask.Name)

			// 处理工具请求
			result, err := mockProcessToolRequire(currentTask, tools)
			if err != nil {
				fmt.Fprintf(&buffer, "工具请求失败: %v\n", err)
				continue
			}

			// 判断工具执行是否成功
			if !result.Success {
				fmt.Fprintf(&buffer, "工具执行失败: %s\n", result.Error)
				continue
			}

			// 提取执行结果
			execResult, ok := result.Data.(*ToolExecutionResult)
			if !ok {
				fmt.Fprintf(&buffer, "工具结果类型错误\n")
				continue
			}

			// 记录执行输出
			fmt.Fprintf(&buffer, "工具执行成功:\n")
			fmt.Fprintf(&buffer, "- 标准输出: %s\n", execResult.Stdout)
			if execResult.Stderr != "" {
				fmt.Fprintf(&buffer, "- 标准错误: %s\n", execResult.Stderr)
			}

			// 记录工具返回结果（简化显示）
			resultJSON, _ := json.MarshalIndent(execResult.Result, "", "  ")
			fmt.Fprintf(&buffer, "- 结果: %s\n", string(resultJSON))
		} else {
			// 不需要工具，直接执行任务
			result := mockExecuteTask(currentTask)
			fmt.Fprintf(&buffer, "%s\n", result)
		}
	}

	// 7. 输出执行结果
	t.Logf("任务执行日志:\n%s", buffer.String())

	// 8. 进行断言确保测试正确性
	executionLog := buffer.String()

	// 确保至少有一些任务是用工具执行的
	assert.Contains(t, executionLog, "需要工具执行任务")

	// 确保至少有一些任务是直接执行的
	assert.Contains(t, executionLog, "已执行任务")

	// 确保天气检查任务被执行
	assert.Contains(t, executionLog, "天气")

	// 确保景点筛选任务被执行
	assert.Contains(t, executionLog, "景点")

	// 确保至少有一个餐饮相关任务被执行
	assert.True(t, strings.Contains(executionLog, "餐"))

	// 确保交通规划被执行
	assert.Contains(t, executionLog, "交通")

	// 确保时间安排被执行
	assert.Contains(t, executionLog, "时间")

	// 确保预算相关任务被执行
	assert.Contains(t, executionLog, "预算")
}

// 测试工具选择逻辑
func TestToolSelectionFromTask(t *testing.T) {
	tools := GetAllMockTools()

	testCases := []struct {
		taskName       string
		taskGoal       string
		needsTool      bool
		outputContains string
	}{
		{
			taskName:       "检查北京天气情况",
			taskGoal:       "获取天气预报",
			needsTool:      true,
			outputContains: "天气",
		},
		{
			taskName:       "筛选历史文化景点",
			taskGoal:       "获取景点信息",
			needsTool:      true,
			outputContains: "景点",
		},
		{
			taskName:       "规划午餐安排",
			taskGoal:       "选择一个餐厅",
			needsTool:      true,
			outputContains: "餐厅",
		},
		{
			taskName:       "规划交通方式",
			taskGoal:       "确定交通路线",
			needsTool:      true,
			outputContains: "交通",
		},
		{
			taskName:       "制作详细时间表",
			taskGoal:       "安排时间",
			needsTool:      true,
			outputContains: "行程时间",
		},
		{
			taskName:       "计算总预算",
			taskGoal:       "确认总花费不超过预算",
			needsTool:      false,
			outputContains: "",
		},
		{
			taskName:       "制定应急方案",
			taskGoal:       "准备备选方案",
			needsTool:      false,
			outputContains: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.taskName, func(t *testing.T) {
			task := Task{
				Name: tc.taskName,
				Goal: tc.taskGoal,
			}

			// 测试任务是否需要工具
			needsTool := taskNeedsTool(task, tools)
			assert.Equal(t, tc.needsTool, needsTool, "任务是否需要工具判断错误")

			if tc.needsTool && tc.outputContains != "" {
				// 测试工具选择
				result, err := mockProcessToolRequire(task, tools)

				if !assert.NoError(t, err, "工具调用出错") {
					return
				}
				assert.NotNil(t, result, "结果不应为空")

				// 检查选择的工具是否符合预期
				if err == nil && result != nil {
					execResult, ok := result.Data.(*ToolExecutionResult)
					assert.True(t, ok, "结果类型错误")

					if ok {
						t.Logf("任务 '%s' 选择了工具，输出: %s", tc.taskName, execResult.Stdout)
						assert.Contains(t, execResult.Stdout, tc.outputContains, "选择的工具输出不正确")
					}
				}
			}
		})
	}
}
