package taskstack

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
)

// TaskCallback 定义Task执行过程中调用回调函数类型
type TaskCallback func(ctx *TaskSystemContext, details ...aispec.ChatDetail) (io.Reader, error)

// TaskResponseCallback 定义Task执行过程中响应回调函数类型
type TaskResponseCallback func(ctx *TaskSystemContext, details ...aispec.ChatDetail) (continueThinking bool, prompt string, err error)

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
func WithTask_Callback(callback TaskCallback) TaskOption {
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

type Task struct {
	Name              string
	Goal              string
	ParentTask        *Task
	Subtasks          []*Task
	AICallback        TaskCallback         // AI回调函数
	ResponseCallback  TaskResponseCallback // 响应回调函数
	SummaryAICallback TaskCallback         // 总结回调函数

	// 新增字段，存储默认工具和元数据
	tools    []*Tool
	metadata map[string]interface{}

	executing bool
	executed  bool
}

func (t *Task) applyToolsForAllSubtasks() {
	for _, subtask := range t.Subtasks {
		subtask.tools = t.tools
		subtask.applyToolsForAllSubtasks()
	}
}

// MarshalJSON 实现自定义的JSON序列化，跳过AICallback字段
func (t Task) MarshalJSON() ([]byte, error) {
	type TaskAlias Task // 创建一个别名类型以避免递归调用

	// 创建一个不包含AICallback的结构体
	return json.Marshal(struct {
		Name     string  `json:"name"`
		Goal     string  `json:"goal"`
		Subtasks []*Task `json:"subtasks,omitempty"`
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
		Name     string  `json:"name"`
		Goal     string  `json:"goal"`
		Subtasks []*Task `json:"subtasks,omitempty"`
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
func (t *Task) SetAICallback(callback TaskCallback) {
	t.AICallback = callback

	// 递归设置子任务的回调函数
	for i := range t.Subtasks {
		t.Subtasks[i].SetAICallback(callback)
	}
}

// SetResponseAICallback 设置Task的响应回调函数
func (t *Task) SetResponseAICallback(callback TaskResponseCallback) {
	t.ResponseCallback = callback

	// 递归设置子任务的回调函数
	for i := range t.Subtasks {
		t.Subtasks[i].SetResponseAICallback(callback)
	}
}

// SetSummaryAICallback 设置Task的总结回调函数
func (t *Task) SetSummaryAICallback(callback TaskCallback) {
	t.SummaryAICallback = callback

	// 递归设置子任务的回调函数
	for i := range t.Subtasks {
		t.Subtasks[i].SetSummaryAICallback(callback)
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
		Name:              t.Name,
		Goal:              t.Goal,
		AICallback:        t.AICallback,
		ResponseCallback:  t.ResponseCallback,
		SummaryAICallback: t.SummaryAICallback,
		tools:             t.tools, // 工具集和回调函数可以共享引用
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
		copy.Subtasks = make([]*Task, len(t.Subtasks))
		for i, subtask := range t.Subtasks {
			subtaskCopy := subtask.DeepCopy()
			copy.Subtasks[i] = subtaskCopy
		}
	}

	return copy
}

type TaskSystemContext struct {
	Progress    string
	CurrentTask *Task
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

		// 尝试解析为新的 task schema 结构
		var planObj struct {
			Action       string `json:"@action"`
			Query        string `json:"query"`
			MainTask     string `json:"main_task"`
			MainTaskGoal string `json:"main_task_goal"`
			Tasks        []struct {
				SubtaskName string `json:"subtask_name"`
				SubtaskGoal string `json:"subtask_goal"`
			} `json:"tasks"`
		}

		err := json.Unmarshal([]byte(taskJSON), &planObj)
		if err == nil && planObj.Action == "plan" && len(planObj.Tasks) > 0 {
			// 创建主任务
			mainTask := &Task{
				Name:     planObj.MainTask,
				Goal:     planObj.MainTaskGoal,
				Subtasks: make([]*Task, 0),
				metadata: map[string]interface{}{
					"query": planObj.Query,
				},
			}

			// 如果主任务名称为空，则使用第一个子任务的名称
			if mainTask.Name == "" {
				mainTask.Name = planObj.Tasks[0].SubtaskName
				mainTask.Goal = planObj.Tasks[0].SubtaskGoal

				// 如果有多个子任务，使用除第一个外的所有任务作为子任务
				if len(planObj.Tasks) > 1 {
					for _, subtask := range planObj.Tasks[1:] {
						mainTask.Subtasks = append(mainTask.Subtasks, &Task{
							Name:       subtask.SubtaskName,
							Goal:       subtask.SubtaskGoal,
							ParentTask: mainTask,
							tools:      mainTask.tools,
						})
					}
				}
			} else {
				// 主任务名称存在，将所有任务作为子任务
				for _, subtask := range planObj.Tasks {
					mainTask.Subtasks = append(mainTask.Subtasks, &Task{
						Name:       subtask.SubtaskName,
						Goal:       subtask.SubtaskGoal,
						ParentTask: mainTask,
						tools:      mainTask.tools,
					})
				}
			}

			return mainTask, nil
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
								subtask := &Task{
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

func (t *Task) QuoteName() string {
	return strconv.Quote(t.Name)
}

func (t *Task) QuoteGoal() string {
	return strconv.Quote(t.Goal)
}
