package taskstack

import (
	"errors"
	"io"
	"strings"
	"testing"
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

// 模拟工具回调函数
func mockToolCallback(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
	return "工具执行结果", nil
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
		WithTool_Callback(mockToolCallback),
	)

	// 创建工具2
	tool2, _ := NewTool("TestTool2",
		WithTool_Description("用于测试的工具2"),
		WithTool_Callback(mockToolCallback),
	)

	return []*Tool{tool1, tool2}
}

// 测试创建Task并应用选项
func TestNewTaskWithOptions(t *testing.T) {
	callback := createMockAICallback("测试响应")
	tools := createTestTools()
	metadata := map[string]interface{}{
		"test_key": "test_value",
	}

	// 创建任务并应用选项
	task := NewTask("测试任务", "执行测试",
		WithTask_Callback(callback),
		WithTask_Tools(tools),
		WithTask_Metadata(metadata),
	)

	// 验证选项已正确应用
	if task.Name != "测试任务" {
		t.Errorf("期望任务名为 '测试任务'，实际为 '%s'", task.Name)
	}

	if task.Goal != "执行测试" {
		t.Errorf("期望任务目标为 '执行测试'，实际为 '%s'", task.Goal)
	}

	if task.AICallback == nil {
		t.Error("回调函数未设置")
	}

	if task.tools == nil || len(task.tools) != 2 {
		t.Errorf("期望工具数量为 2，实际为 %d", len(task.tools))
	}

	if task.metadata == nil {
		t.Error("元数据未设置")
	} else if val, ok := task.metadata["test_key"]; !ok || val != "test_value" {
		t.Errorf("元数据值不正确，期望 'test_value'，实际为 '%v'", val)
	}
}

// 测试深度复制功能
func TestTaskDeepCopy(t *testing.T) {
	// 创建用于测试的对象
	callback := createMockAICallback("原始任务的响应")

	// 创建原始任务
	originalTask := NewTask("原始任务", "测试复制功能",
		WithTask_Callback(callback),
		WithTask_Tools(createTestTools()),
	)

	// 进行深度复制
	copiedTask := originalTask.DeepCopy()

	// 验证复制结果
	if copiedTask.Name != originalTask.Name {
		t.Errorf("复制后名称不匹配，原始: %s, 复制: %s", originalTask.Name, copiedTask.Name)
	}

	if copiedTask.Goal != originalTask.Goal {
		t.Errorf("复制后目标不匹配，原始: %s, 复制: %s", originalTask.Goal, copiedTask.Goal)
	}

	// 验证回调函数复制
	// 注意：我们只能验证回调函数不为nil，因为函数引用相等是预期的行为
	if copiedTask.AICallback == nil {
		t.Error("复制后回调函数为nil")
	}

	// 验证工具集复制
	if len(copiedTask.tools) != len(originalTask.tools) {
		t.Errorf("复制后工具数量不匹配，原始: %d, 复制: %d", len(originalTask.tools), len(copiedTask.tools))
	}

	// 修改复制对象不应影响原始对象
	copiedTask.Name = "修改后的任务名"
	if originalTask.Name == copiedTask.Name {
		t.Error("修改复制对象影响了原始对象")
	}
}

// 测试任务执行功能
func TestTaskInvoke(t *testing.T) {
	// 创建测试回调
	callback := createMockAICallback("任务执行成功")

	// 创建父任务
	parentTask := NewTask("父任务", "父任务目标", WithTask_Callback(callback))

	// 执行任务
	result, err := parentTask.Invoke()
	if err != nil {
		t.Fatalf("执行任务失败: %v", err)
	}

	if result != "任务执行成功" {
		t.Errorf("任务执行结果不符合预期，得到: %s", result)
	}
}

// 测试任务执行错误处理
func TestTaskInvokeError(t *testing.T) {
	// 创建会返回错误的回调
	errorCallback := createErrorAICallback()

	// 创建任务
	task := NewTask("错误任务", "测试错误", WithTask_Callback(errorCallback))

	// 执行任务，预期失败
	_, err := task.Invoke()
	if err == nil {
		t.Fatal("预期任务执行应该失败，但没有返回错误")
	}
}

// 测试创建计划
func TestNewPlan(t *testing.T) {
	// 创建测试工具和回调
	tools := createTestTools()
	callback := createMockAICallback("响应")
	metadata := map[string]interface{}{
		"plan_key": "plan_value",
	}

	// 创建计划
	plan := NewPlan("测试计划",
		WithTask_Callback(callback),
		WithTask_Tools(tools),
		WithTask_Metadata(metadata),
	)

	// 验证计划创建正确
	if plan.Name != "测试计划" {
		t.Errorf("计划名称不正确，期望: 测试计划, 实际: %s", plan.Name)
	}

	if plan.callback == nil {
		t.Error("计划回调未设置")
	}

	if plan.tools == nil || len(plan.tools) != 2 {
		t.Errorf("计划工具设置不正确，期望: 2, 实际: %d", len(plan.tools))
	}

	if val, ok := plan.metadata["plan_key"]; !ok || val != "plan_value" {
		t.Errorf("计划元数据不正确，期望: plan_value, 实际: %v", val)
	}
}

// 测试向计划添加任务
func TestPlanAddTask(t *testing.T) {
	// 创建测试回调
	callback := createMockAICallback("响应")

	// 创建计划
	plan := NewPlan("测试计划", WithTask_Callback(callback))

	// 创建独立任务
	task := NewTask("独立任务", "独立目标", WithTask_Callback(callback))
	plan.AddTask(task)

	// 使用AddTaskWithOptions添加任务
	_ = plan.AddTaskWithOptions("第二个任务", "第二个目标")

	// 验证
	if len(plan.Tasks) != 2 {
		t.Errorf("计划任务数量不正确，期望: 2, 实际: %d", len(plan.Tasks))
	}

	if plan.Tasks[0].Name != "独立任务" || plan.Tasks[1].Name != "第二个任务" {
		t.Error("计划中的任务名称不正确")
	}

	// 确认任务继承了计划的回调
	if plan.Tasks[0].AICallback == nil || plan.Tasks[1].AICallback == nil {
		t.Error("添加到计划的任务应该继承计划的回调")
	}
}

// 测试执行计划
func TestExecutePlan(t *testing.T) {
	// 创建测试回调
	callback := createMockAICallback("任务响应")

	// 创建计划
	plan := NewPlan("测试计划", WithTask_Callback(callback))

	// 添加任务
	plan.AddTaskWithOptions("任务1", "目标1")
	plan.AddTaskWithOptions("任务2", "目标2")

	// 执行计划
	results, err := plan.ExecutePlan()
	if err != nil {
		t.Fatalf("执行计划失败: %v", err)
	}

	// 验证结果
	if len(results) != 2 {
		t.Errorf("计划执行结果数量不正确，期望: 2, 实际: %d", len(results))
	}
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
	// 有效任务
	validTask := NewTask("有效任务", "有效目标", WithTask_Callback(createMockAICallback("响应")))
	err := ValidateTask(validTask)
	if err != nil {
		t.Errorf("有效任务验证失败: %v", err)
	}

	// 空任务
	err = ValidateTask(nil)
	if err == nil {
		t.Error("空任务应该验证失败")
	}

	// 无回调任务
	noCallbackTask := NewTask("无回调任务", "无回调目标")
	err = ValidateTask(noCallbackTask)
	if err == nil {
		t.Error("没有回调的任务应该验证失败")
	}

	// 空名称任务
	emptyNameTask := NewTask("", "空名称目标", WithTask_Callback(createMockAICallback("响应")))
	err = ValidateTask(emptyNameTask)
	if err == nil {
		t.Error("空名称的任务应该验证失败")
	}
}

// 测试从原始响应中提取任务
func TestExtractTaskFromRawResponse(t *testing.T) {
	response := `这是一些文本和一个任务JSON：
{"name":"提取的任务","goal":"测试提取功能","subtasks":[{"name":"子任务","goal":"测试子任务提取"}]}
更多文本...`

	task, err := ExtractTaskFromRawResponse(response, WithTask_Callback(createMockAICallback("响应")))
	if err != nil {
		t.Fatalf("从响应提取任务失败: %v", err)
	}

	if task.Name != "提取的任务" || task.Goal != "测试提取功能" {
		t.Errorf("提取任务结果不正确，名称: %s, 目标: %s", task.Name, task.Goal)
	}

	if len(task.Subtasks) != 1 || task.Subtasks[0].Name != "子任务" {
		t.Error("子任务提取不正确")
	}
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
