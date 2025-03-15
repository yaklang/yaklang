package taskstack

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed jsonschema/task.json
var taskJsonSchema string

//go:embed prompts/execute-task.txt
var executeTaskPromptTemplate string

//go:embed prompts/describe-tool.txt
var describeToolPromptTemplate string

// 用于识别工具操作请求的正则表达式
var toolActionRegex = regexp.MustCompile(`(?s)\` + "`" + `jsonschema.*?\{.*?"@action"\s*:\s*"([^"]+)".*?"tool"\s*:\s*"([^"]+)".*?\}\s*\` + "`" + ``)

// TaskAICallback 定义Task执行过程中AI调用回调函数类型
type TaskAICallback func(prompt string) (io.Reader, error)

// TaskProgress 记录任务执行的进度信息
type TaskProgress struct {
	TotalTasks     int    `json:"total_tasks"`     // 总任务数
	CompletedTasks int    `json:"completed_tasks"` // 已完成任务数
	CurrentTask    string `json:"current_task"`    // 当前执行的任务
	CurrentGoal    string `json:"current_goal"`    // 当前任务的目标
}

// TaskOption 定义配置Task的选项接口
type TaskOption interface {
	Apply(t *Task)
}

// taskOptionFunc 是一个适配器，允许将普通函数转换为TaskOption
type taskOptionFunc func(t *Task)

// Apply 实现TaskOption接口
func (f taskOptionFunc) Apply(t *Task) {
	f(t)
}

// WithTask_Tools 设置Task使用的工具集
func WithTask_Tools(tools []*Tool) TaskOption {
	return taskOptionFunc(func(t *Task) {
		t.tools = tools
	})
}

// WithTask_Callback 设置Task的AI回调函数
func WithTask_Callback(callback TaskAICallback) TaskOption {
	return taskOptionFunc(func(t *Task) {
		t.SetAICallback(callback)
	})
}

// WithTask_Metadata 设置Task的元数据
func WithTask_Metadata(metadata map[string]interface{}) TaskOption {
	return taskOptionFunc(func(t *Task) {
		if t.metadata == nil {
			t.metadata = make(map[string]interface{})
		}

		for k, v := range metadata {
			t.metadata[k] = v
		}
	})
}

// WithTask_Subtasks 设置Task的子任务
func WithTask_Subtasks(subtasks []Task) TaskOption {
	return taskOptionFunc(func(t *Task) {
		t.Subtasks = subtasks

		// 如果已经设置了AICallback，则为子任务也设置相同的回调
		if t.AICallback != nil {
			for i := range t.Subtasks {
				t.Subtasks[i].SetAICallback(t.AICallback)
			}
		}
	})
}

// WithTask_ParentTask 设置任务的父任务
func WithTask_ParentTask(parent *Task) TaskOption {
	return taskOptionFunc(func(t *Task) {
		// 从父任务继承回调函数和工具
		if parent.AICallback != nil {
			t.SetAICallback(parent.AICallback)
		}

		if parent.tools != nil && len(parent.tools) > 0 {
			t.tools = parent.tools
		}

		// 可以选择性地从父任务继承一些元数据
		if parent.metadata != nil {
			if t.metadata == nil {
				t.metadata = make(map[string]interface{})
			}

			// 只复制某些特定的元数据，避免覆盖子任务的特定设置
			for k, v := range parent.metadata {
				if _, exists := t.metadata[k]; !exists {
					t.metadata[k] = v
				}
			}
		}
	})
}

type Task struct {
	Name       string
	Goal       string
	Subtasks   []Task
	AICallback TaskAICallback // AI回调函数

	// 新增字段，存储默认工具和元数据
	tools    []*Tool
	metadata map[string]interface{}
}

// MarshalJSON 实现自定义的JSON序列化，跳过AICallback字段
func (t Task) MarshalJSON() ([]byte, error) {
	type TaskAlias Task // 创建一个别名类型以避免递归调用

	// 创建一个不包含AICallback的结构体
	return json.Marshal(struct {
		Name     string `json:"name"`
		Goal     string `json:"goal"`
		Subtasks []Task `json:"subtasks,omitempty"`
	}{
		Name:     t.Name,
		Goal:     t.Goal,
		Subtasks: t.Subtasks,
	})
}

// UnmarshalJSON 实现自定义的JSON反序列化，跳过AICallback字段
func (t *Task) UnmarshalJSON(data []byte) error {
	// 创建一个临时结构体，不包含AICallback
	aux := struct {
		Name     string `json:"name"`
		Goal     string `json:"goal"`
		Subtasks []Task `json:"subtasks,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	t.Name = aux.Name
	t.Goal = aux.Goal
	t.Subtasks = aux.Subtasks
	return nil
}

// SetAICallback 设置Task的AI回调函数
func (t *Task) SetAICallback(callback TaskAICallback) {
	t.AICallback = callback

	// 递归设置子任务的回调函数
	for i := range t.Subtasks {
		t.Subtasks[i].SetAICallback(callback)
	}
}

// NewTask 创建一个新的Task，可以通过选项进行配置
func NewTask(name, goal string, options ...TaskOption) *Task {
	task := &Task{
		Name:     name,
		Goal:     goal,
		metadata: make(map[string]interface{}),
	}

	// 应用所有选项
	for _, option := range options {
		option.Apply(task)
	}

	return task
}

// ApplyOptions 对Task应用新的选项
func (t *Task) ApplyOptions(options ...TaskOption) {
	for _, option := range options {
		option.Apply(t)
	}
}

// DeepCopy 创建Task的深度复制，包括其子任务
func (t *Task) DeepCopy() *Task {
	copy := &Task{
		Name:       t.Name,
		Goal:       t.Goal,
		AICallback: t.AICallback,
		tools:      t.tools, // 工具集和回调函数可以共享引用
	}

	// 复制元数据
	if t.metadata != nil {
		copy.metadata = make(map[string]interface{})
		for k, v := range t.metadata {
			copy.metadata[k] = v
		}
	}

	// 深度复制子任务
	if len(t.Subtasks) > 0 {
		copy.Subtasks = make([]Task, len(t.Subtasks))
		for i, subtask := range t.Subtasks {
			subtaskCopy := subtask.DeepCopy()
			copy.Subtasks[i] = *subtaskCopy
		}
	}

	return copy
}

// Invoke 执行当前任务，如果有子任务则依次执行子任务
// 现在支持通过选项覆盖默认设置
func (t *Task) Invoke(options ...TaskOption) (string, error) {
	// 创建一个Task的深度副本，以便应用临时选项
	taskCopy := t.DeepCopy()

	// 应用临时选项
	for _, option := range options {
		option.Apply(taskCopy)
	}

	if taskCopy.AICallback == nil {
		return "", errors.New("no AI callback function set")
	}

	// 使用Task中存储的tools和metadata，如果存在的话
	tools := taskCopy.tools
	metadata := make(map[string]interface{})

	// 复制元数据
	if taskCopy.metadata != nil {
		for k, v := range taskCopy.metadata {
			metadata[k] = v
		}
	}

	// 如果有子任务，先执行所有子任务
	if len(taskCopy.Subtasks) > 0 {
		results := make([]string, 0, len(taskCopy.Subtasks))

		// 计算总任务数（当前任务+所有子任务）
		totalTasks := 1 + len(taskCopy.Subtasks)

		for i, subtask := range taskCopy.Subtasks {
			// 更新进度信息
			progress := TaskProgress{
				TotalTasks:     totalTasks,
				CompletedTasks: i,
				CurrentTask:    subtask.Name,
				CurrentGoal:    subtask.Goal,
			}

			// 添加进度信息到metadata
			metadataCopy := make(map[string]interface{})
			for k, v := range metadata {
				metadataCopy[k] = v
			}
			metadataCopy["progress"] = progress

			// 为子任务提供父任务的tools和更新的metadata
			subtaskCopy := subtask
			if tools != nil {
				subtaskCopy.tools = tools
			}

			// 确保子任务继承父任务的AICallback
			if subtaskCopy.AICallback == nil {
				subtaskCopy.AICallback = taskCopy.AICallback
			}

			// 执行子任务
			result, err := (&subtaskCopy).Invoke(WithTask_Metadata(metadataCopy))
			if err != nil {
				return "", fmt.Errorf("error executing subtask '%s': %w", subtask.Name, err)
			}

			results = append(results, result)
		}

		// 所有子任务完成后，执行当前任务
		progress := TaskProgress{
			TotalTasks:     totalTasks,
			CompletedTasks: len(taskCopy.Subtasks),
			CurrentTask:    taskCopy.Name,
			CurrentGoal:    taskCopy.Goal,
		}

		// 添加进度信息到metadata
		metadataCopy := make(map[string]interface{})
		for k, v := range metadata {
			metadataCopy[k] = v
		}
		metadataCopy["progress"] = progress
		metadataCopy["subtask_results"] = results

		return taskCopy.executeTask(tools, metadataCopy)
	}

	// 没有子任务，直接执行当前任务
	progress := TaskProgress{
		TotalTasks:     1,
		CompletedTasks: 0,
		CurrentTask:    taskCopy.Name,
		CurrentGoal:    taskCopy.Goal,
	}

	// 添加进度信息到metadata
	metadataCopy := make(map[string]interface{})
	for k, v := range metadata {
		metadataCopy[k] = v
	}
	metadataCopy["progress"] = progress

	return taskCopy.executeTask(tools, metadataCopy)
}

// executeTask 实际执行任务并返回结果
func (t *Task) executeTask(tools []*Tool, metadata map[string]interface{}) (string, error) {
	// 使用Task的内部字段，如果传入的参数为nil则使用内部字段
	actualTools := tools
	if actualTools == nil && t.tools != nil {
		actualTools = t.tools
	}

	actualMetadata := metadata
	if actualMetadata == nil && t.metadata != nil {
		actualMetadata = t.metadata
	}

	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt(actualTools, actualMetadata)
	if err != nil {
		return "", fmt.Errorf("error generating task prompt: %w", err)
	}

	// 开始交互式执行
	var finalResponse string
	currentPrompt := prompt
	conversationHistory := []string{prompt}

	for {
		// 调用AI回调函数
		responseReader, err := t.AICallback(currentPrompt)
		if err != nil {
			return "", fmt.Errorf("error calling AI: %w", err)
		}

		// 读取AI的响应
		responseBytes, err := io.ReadAll(responseReader)
		if err != nil {
			return "", fmt.Errorf("error reading AI response: %w", err)
		}

		response := string(responseBytes)
		conversationHistory = append(conversationHistory, response)

		// 检查是否有工具操作请求
		matches := toolActionRegex.FindStringSubmatch(response)
		if len(matches) > 2 {
			action := matches[1]
			toolName := matches[2]

			// 处理不同的操作
			switch action {
			case "describe-tool":
				// 生成工具描述响应
				toolDescription, err := t.handleDescribeTool(actualTools, toolName)
				if err != nil {
					toolDescription = fmt.Sprintf("错误：%s", err.Error())
				}
				conversationHistory = append(conversationHistory, toolDescription)
				currentPrompt = strings.Join(conversationHistory, "\n\n")
				continue
			default:
				// 未知操作，继续进行
				finalResponse = response
				break
			}
		} else {
			// 没有工具操作请求，将当前响应作为最终响应
			finalResponse = response
			break
		}
	}

	return finalResponse, nil
}

// handleDescribeTool 处理描述工具的请求
func (t *Task) handleDescribeTool(tools []*Tool, toolName string) (string, error) {
	// 查找请求的工具
	var targetTool *Tool
	for _, tool := range tools {
		if tool.Name == toolName {
			targetTool = tool
			break
		}
	}

	if targetTool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	// 生成工具的JSONSchema描述
	toolJSONSchema := targetTool.ToJSONSchemaString()

	// 创建模板数据
	templateData := map[string]interface{}{
		"Tool":           targetTool,
		"ToolJSONSchema": toolJSONSchema,
	}

	// 解析工具描述模板
	tmpl, err := template.New("describe-tool").Parse(describeToolPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing tool description template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing tool description template: %w", err)
	}

	return promptBuilder.String(), nil
}

// generateTaskPrompt 生成执行任务的prompt
func (t *Task) generateTaskPrompt(tools []*Tool, metadata map[string]interface{}) (string, error) {
	// 创建模板数据
	templateData := map[string]interface{}{
		"Task":     t,
		"Tools":    tools,
		"Metadata": metadata,
	}

	// 解析prompt模板
	tmpl, err := template.New("execute-task").Parse(executeTaskPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing task prompt template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing task prompt template: %w", err)
	}

	return promptBuilder.String(), nil
}

type Runtime struct {
	Freeze bool
	Task   Task
	Stack  *utils.Stack[Task]
}

// NewTaskFromJSON 从JSON字符串创建Task
func NewTaskFromJSON(jsonStr string, options ...TaskOption) (*Task, error) {
	var task Task
	err := json.Unmarshal([]byte(jsonStr), &task)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal task from JSON: %w", err)
	}

	// 应用默认值和选项
	task.metadata = make(map[string]interface{})

	// 应用所有选项
	for _, option := range options {
		option.Apply(&task)
	}

	return &task, nil
}

// ValidateTask 检查Task是否有效并准备好执行
func ValidateTask(task *Task) error {
	if task == nil {
		return errors.New("task is nil")
	}

	if task.Name == "" {
		return errors.New("task name is empty")
	}

	if task.AICallback == nil {
		return errors.New("task has no AI callback function")
	}

	// 检查子任务
	for i, subtask := range task.Subtasks {
		if subtask.Name == "" {
			return fmt.Errorf("subtask at index %d has empty name", i)
		}
	}

	return nil
}

// ExtractTaskFromRawResponse 从原始响应中提取Task
func ExtractTaskFromRawResponse(rawResponse string, options ...TaskOption) (*Task, error) {
	task, err := extractTaskWithoutOptions(rawResponse)
	if err != nil {
		return nil, err
	}

	// 应用所有选项
	for _, option := range options {
		option.Apply(task)
	}

	return task, nil
}

// extractTaskWithoutOptions 是原始的提取功能，不应用选项
func extractTaskWithoutOptions(rawResponse string) (*Task, error) {
	for _, item := range jsonextractor.ExtractObjectIndexes(rawResponse) {
		start, end := item[0], item[1]
		taskJSON := rawResponse[start:end]

		// 尝试解析为完整的 task 结构
		var taskObj struct {
			Tasks []struct {
				SubtaskName string `json:"subtask_name"`
				SubtaskGoal string `json:"subtask_goal"`
			} `json:"tasks"`
		}

		err := json.Unmarshal([]byte(taskJSON), &taskObj)
		if err == nil && len(taskObj.Tasks) > 0 {
			// 找到了合法的任务结构
			mainTask := &Task{
				Name:     taskObj.Tasks[0].SubtaskName,
				Goal:     taskObj.Tasks[0].SubtaskGoal,
				Subtasks: make([]Task, 0),
			}

			// 如果有多个任务，将后续任务作为子任务
			if len(taskObj.Tasks) > 1 {
				for _, subtask := range taskObj.Tasks[1:] {
					if subtask.SubtaskName != "" {
						mainTask.Subtasks = append(mainTask.Subtasks, Task{
							Name: subtask.SubtaskName,
							Goal: subtask.SubtaskGoal,
						})
					}
				}
			}

			// 检查主任务 Name 是否存在
			if mainTask.Name != "" {
				return mainTask, nil
			}
		}

		// 尝试直接解析为单个 Task 对象
		var simpleTask Task
		err = json.Unmarshal([]byte(taskJSON), &simpleTask)
		if err == nil && simpleTask.Name != "" {
			return &simpleTask, nil
		}

		// 尝试解析为一个简单的 map 并创建 Task
		var taskMap map[string]interface{}
		err = json.Unmarshal([]byte(taskJSON), &taskMap)
		if err == nil {
			if name, ok := taskMap["name"].(string); ok && name != "" {
				task := &Task{
					Name: name,
				}

				if goal, ok := taskMap["goal"].(string); ok {
					task.Goal = goal
				}

				if subtasks, ok := taskMap["subtasks"].([]interface{}); ok {
					for _, st := range subtasks {
						if subtaskMap, ok := st.(map[string]interface{}); ok {
							if stName, ok := subtaskMap["name"].(string); ok && stName != "" {
								subtask := Task{
									Name: stName,
								}

								if stGoal, ok := subtaskMap["goal"].(string); ok {
									subtask.Goal = stGoal
								}

								task.Subtasks = append(task.Subtasks, subtask)
							}
						}
					}
				}

				return task, nil
			}
		}
	}
	return nil, errors.New("no task found")
}

// Plan 表示一组按顺序执行的任务
type Plan struct {
	Name     string
	Tasks    []*Task
	tools    []*Tool
	callback TaskAICallback
	metadata map[string]interface{}
}

// NewPlan 创建一个新的Plan，可以通过选项进行配置
func NewPlan(name string, options ...TaskOption) *Plan {
	plan := &Plan{
		Name:     name,
		Tasks:    []*Task{},
		metadata: make(map[string]interface{}),
	}

	// 应用所有选项到Plan
	for _, option := range options {
		applyToPlan(plan, option)
	}

	return plan
}

// applyToPlan 将TaskOption应用到Plan
func applyToPlan(p *Plan, option TaskOption) {
	// 创建一个临时Task来应用选项
	tmpTask := &Task{
		metadata:   p.metadata,
		tools:      p.tools,
		AICallback: p.callback,
	}

	// 应用选项到临时Task
	option.Apply(tmpTask)

	// 将临时Task的设置同步回Plan
	p.tools = tmpTask.tools
	p.callback = tmpTask.AICallback
	p.metadata = tmpTask.metadata
}

// AddTask 向Plan添加任务
func (p *Plan) AddTask(task *Task) {
	// 确保任务使用Plan的工具和回调
	if p.callback != nil {
		task.SetAICallback(p.callback)
	}

	if p.tools != nil {
		task.tools = p.tools
	}

	p.Tasks = append(p.Tasks, task)
}

// AddTaskWithOptions 创建并添加一个新任务到Plan
func (p *Plan) AddTaskWithOptions(name, goal string, options ...TaskOption) *Task {
	// 创建基础任务，继承Plan的设置
	task := NewTask(name, goal,
		WithTask_Callback(p.callback),
		WithTask_Tools(p.tools),
		WithTask_Metadata(p.metadata),
	)

	// 应用额外的选项
	task.ApplyOptions(options...)

	// 添加到Plan
	p.Tasks = append(p.Tasks, task)

	return task
}

// ApplyOptions 对Plan应用选项
func (p *Plan) ApplyOptions(options ...TaskOption) {
	for _, option := range options {
		applyToPlan(p, option)
	}

	// 将Plan的设置应用到所有任务的副本
	if p.Tasks != nil {
		for i, task := range p.Tasks {
			// 创建副本以避免修改原始任务
			taskCopy := task.DeepCopy()

			// 应用Plan的设置
			if p.callback != nil {
				taskCopy.SetAICallback(p.callback)
			}

			if p.tools != nil {
				taskCopy.tools = p.tools
			}

			// 更新任务引用
			p.Tasks[i] = taskCopy
		}
	}
}

// ExecutePlan 执行整个Plan的所有任务
func (p *Plan) ExecutePlan(options ...TaskOption) ([]string, error) {
	// 创建Plan的副本以应用临时选项
	planCopy := &Plan{
		Name:     p.Name,
		callback: p.callback,
		tools:    p.tools,
	}

	// 深度复制元数据
	if p.metadata != nil {
		planCopy.metadata = make(map[string]interface{})
		for k, v := range p.metadata {
			planCopy.metadata[k] = v
		}
	} else {
		planCopy.metadata = make(map[string]interface{})
	}

	// 深度复制任务列表
	if len(p.Tasks) > 0 {
		planCopy.Tasks = make([]*Task, len(p.Tasks))
		for i, task := range p.Tasks {
			planCopy.Tasks[i] = task.DeepCopy()
		}
	}

	// 应用临时选项
	planCopy.ApplyOptions(options...)

	// 检查是否设置了回调函数
	if planCopy.callback == nil {
		return nil, errors.New("no AI callback function set for plan")
	}

	results := make([]string, 0, len(planCopy.Tasks))

	// 逐个执行任务
	for i, task := range planCopy.Tasks {
		// 更新元数据中的计划执行进度
		planProgress := map[string]interface{}{
			"plan_name":         planCopy.Name,
			"plan_total_tasks":  len(planCopy.Tasks),
			"plan_current_task": i + 1,
			"plan_progress":     float64(i) / float64(len(planCopy.Tasks)),
		}

		// 合并Plan的元数据和任务进度元数据
		taskMetadata := make(map[string]interface{})
		for k, v := range planCopy.metadata {
			taskMetadata[k] = v
		}
		for k, v := range planProgress {
			taskMetadata[k] = v
		}

		// 为每个任务创建选项，确保使用Plan的tools和callback
		taskOptions := []TaskOption{
			WithTask_Callback(planCopy.callback),
			WithTask_Tools(planCopy.tools),
			WithTask_Metadata(taskMetadata),
		}

		// 执行任务
		result, err := task.Invoke(taskOptions...)
		if err != nil {
			return results, fmt.Errorf("error executing task %d (%s): %w", i+1, task.Name, err)
		}

		results = append(results, result)
	}

	return results, nil
}
