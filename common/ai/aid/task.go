package aid

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
)

// TaskResponseCallback 定义Task执行过程中响应回调函数类型
type TaskResponseCallback func(ctx *PromptContextProvider, details ...aispec.ChatDetail) (continueThinking bool, prompt string, err error)

// TaskProgress 记录任务执行的进度信息
type TaskProgress struct {
	TotalTasks     int    `json:"total_tasks"`     // 总任务数
	CompletedTasks int    `json:"completed_tasks"` // 已完成任务数
	CurrentTask    string `json:"current_task"`    // 当前执行的任务
	CurrentGoal    string `json:"current_goal"`    // 当前任务的目标
}

type AiTask struct {
	*aicommon.Emitter
	*Coordinator

	Index      string    `json:"index"`
	Name       string    `json:"name"`
	Goal       string    `json:"goal"`
	ParentTask *AiTask   `json:"parent_task"`
	Subtasks   []*AiTask `json:"subtasks"`

	ResponseCallback TaskResponseCallback `json:"-"` // 响应回调函数

	// 新增字段，存储默认工具和元数据
	metadata map[string]interface{}

	executing bool
	executed  bool

	// runtime
	//ToolCallResults   []*aitool.ToolResult `json:"tool_call_results"`
	toolCallResultIds *omap.OrderedMap[int64, *aitool.ToolResult]
	StatusSummary     string `json:"status_summary"`
	TaskSummary       string `json:"task_summary"`
	ShortSummary      string `json:"short_summary"`
	LongSummary       string `json:"long_summary"`

	ToolCallCount int64 `json:"tool_call_count"`

	// task continue count
	TaskContinueCount int64 `json:"task_continue_count"` // 任务继续执行的次数
}

func (t *AiTask) GetSummary() string {
	if t.TaskSummary != "" {
		return t.TaskSummary
	}
	if t.ShortSummary != "" {
		return t.ShortSummary
	}
	if t.LongSummary != "" {
		return t.LongSummary
	}
	if t.StatusSummary != "" {
		return t.StatusSummary
	}
	return ""
}

func (t *AiTask) GetSuccessCallCount() int {
	count := 0
	t.toolCallResultIds.ForEach(func(i int64, v *aitool.ToolResult) bool {
		if v.Success {
			count++
		}
		return true
	})
	return count
}

func (t *AiTask) GetFailCallCount() int {
	count := 0
	t.toolCallResultIds.ForEach(func(i int64, v *aitool.ToolResult) bool {
		if !v.Success {
			count++
		}
		return true
	})
	return count
}

func (t *AiTask) GetEmitter() *aicommon.Emitter {
	if t.Emitter == nil {
		return t.Coordinator.GetEmitter()
	}
	return t.Emitter

}

func (t *AiTask) GetIndex() string {
	return t.Index
}

func (t *AiTask) GetName() string {
	return t.Name
}


func (t *AiTask) CallAI(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	for _, cb := range []aicommon.AICallbackType{
		t.Coordinator.QualityPriorityAICallback,
		t.Coordinator.SpeedPriorityAICallback,
		t.Coordinator.OriginalAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(t.Coordinator, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

func (t *AiTask) PushToolCallResult(i *aitool.ToolResult) {
	t.toolCallResultIds.Set(i.GetID(), i)
	t.Coordinator.Memory.PushToolCallResults(i)
	atomic.AddInt64(&t.ToolCallCount, 1)
}

// MarshalJSON 实现自定义的JSON序列化，跳过AICallback字段
func (t *AiTask) MarshalJSON() ([]byte, error) {
	type TaskAlias AiTask // 创建一个别名类型以避免递归调用
	var progress string
	if t.executed {
		progress = "success"
	} else if t.executing {
		progress = "in-progress"
	}

	// 创建一个不包含AICallback的结构体
	return json.Marshal(struct {
		Index                string    `json:"index"`
		Name                 string    `json:"name"`
		Goal                 string    `json:"goal"`
		Subtasks             []*AiTask `json:"subtasks,omitempty"`
		Progress             string    `json:"progress"` // 添加进度字段
		Summary              string    `json:"summary"`
		TotalToolCallCount   int64     `json:"total_tool_call_count"`
		SuccessToolCallCount int       `json:"success_tool_call_count"`
		FailToolCallCount    int       `json:"fail_tool_call_count"`
	}{
		Index:                t.Index,
		Name:                 t.Name,
		Goal:                 t.Goal,
		Subtasks:             t.Subtasks,
		Progress:             progress,
		Summary:              t.GetSummary(),
		TotalToolCallCount:   t.ToolCallCount,
		SuccessToolCallCount: t.GetSuccessCallCount(),
		FailToolCallCount:    t.GetFailCallCount(),
	})
}

// UnmarshalJSON 实现自定义的JSON反序列化，跳过AICallback字段
func (t *AiTask) UnmarshalJSON(data []byte) error {
	// 创建一个临时结构体，不包含AICallback
	aux := struct {
		Index    string    `json:"index"`
		Name     string    `json:"name"`
		Goal     string    `json:"goal"`
		Subtasks []*AiTask `json:"subtasks,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	t.Index = aux.Index
	t.Name = aux.Name
	t.Goal = aux.Goal
	t.Subtasks = aux.Subtasks
	if t.toolCallResultIds == nil {
		t.toolCallResultIds = omap.NewOrderedMap(make(map[int64]*aitool.ToolResult))
	}
	return nil
}

func ExtractPlan(c *Coordinator, rawResponse string) (*PlanResponse, error) {
	at, err := ExtractTaskFromRawResponse(c, rawResponse)
	if err != nil {
		return nil, err
	}
	return &PlanResponse{RootTask: at}, nil
}

func ExtractNextPlanTaskFromRawResponse(c *Coordinator, rawResponse string) ([]*AiTask, error) {
	for _, item := range jsonextractor.ExtractObjectIndexes(rawResponse) {
		start, end := item[0], item[1]
		taskJSON := rawResponse[start:end]

		// 尝试解析为新的 aiTask schema 结构
		var planObj struct {
			Action       string    `json:"@action"`
			NextPlanTask []*AiTask `json:"next_plans"`
		}

		err := json.Unmarshal([]byte(taskJSON), &planObj)
		if err == nil && planObj.Action == "re-plan" && len(planObj.NextPlanTask) > 0 {
			return planObj.NextPlanTask, nil
		}
	}
	return nil, errors.New("no aiTask found in next-plans")
}

// _assignHierarchicalIndicesRecursive 递归地为任务及其子任务分配层级索引。
// currentTask 是当前要处理的任务。
// currentIndex 是为 currentTask 计算好的索引字符串 (例如 "1", "1-2", "1-2-3")。
func _assignHierarchicalIndicesRecursive(currentTask *AiTask, currentIndex string) {
	if currentTask == nil {
		return
	}
	currentTask.Index = currentIndex

	for i, subTask := range currentTask.Subtasks {
		// 子任务的索引是父任务索引加上自己的序号 (1-based)
		// 例如，如果父任务索引是 "1-2", 第一个子任务是 "1-2-1", 第二个是 "1-2-2"
		subTaskIndex := fmt.Sprintf("%s-%d", currentIndex, i+1)
		_assignHierarchicalIndicesRecursive(subTask, subTaskIndex)
	}
}

// GenerateIndex 为任务树生成层级索引。
// 调用此方法的任务 (a) 所在树的根节点索引将被设为 "1"。
// 其子任务将相应地获得如 "1-1", "1-2" 等索引，孙任务如 "1-1-1" 等。
func (a *AiTask) GenerateIndex() {
	if a == nil {
		return
	}

	root := a
	// 向上遍历以找到树的实际根节点。
	// 包含一个针对极深树或潜在循环依赖的安全中断。
	for i := 0; i < 1000 && root.ParentTask != nil; i++ {
		root = root.ParentTask
	}

	// 循环结束后，'root' 要么是真正的根节点 (ParentTask == nil)，
	// 要么是经过1000次迭代后到达的节点。
	// 从这个 'root' 开始进行索引。
	// 根任务的索引被指定为 "1"。
	_assignHierarchicalIndicesRecursive(root, "1")
}

// ExtractTaskFromRawResponse 从原始响应中提取Task
func ExtractTaskFromRawResponse(c *Coordinator, rawResponse string) (retTask *AiTask, err error) {
	defer func() {
		if retTask == nil {
			return
		}
		// Ensure config is propagated to the new task and its subtasks
		var propagateConfig func(task *AiTask)
		propagateConfig = func(task *AiTask) {
			if task == nil {
				return
			}
			task.Coordinator = c
			if task.toolCallResultIds == nil {
				task.toolCallResultIds = omap.NewOrderedMap(make(map[int64]*aitool.ToolResult))
			}
			for _, sub := range task.Subtasks {
				sub.ParentTask = task // Ensure parent is set
				propagateConfig(sub)
			}
		}
		propagateConfig(retTask)
		retTask.GenerateIndex()
	}()
	var extraReason bytes.Buffer
	_ = extraReason
	for _, item := range jsonextractor.ExtractObjectIndexes(rawResponse) {
		start, end := item[0], item[1]
		taskJSON := rawResponse[start:end]

		// 尝试解析为新的 aiTask schema 结构
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

		err = json.Unmarshal([]byte(taskJSON), &planObj)
		if err != nil {
			log.Debugf("Failed to parse taskJSON as planObj structure: %v. JSON: %s", err, taskJSON)
		}
		if err == nil && planObj.Action == "plan" && len(planObj.Tasks) > 0 {
			// 创建主任务
			mainTask := &AiTask{
				Coordinator: c,
				Name:        planObj.MainTask,
				Goal:        planObj.MainTaskGoal,
				Subtasks:    make([]*AiTask, 0),
				metadata: map[string]interface{}{
					"query": planObj.Query,
				},
				toolCallResultIds: omap.NewOrderedMap(make(map[int64]*aitool.ToolResult)),
			}

			// 如果主任务名称为空，则使用第一个子任务的名称
			if mainTask.Name == "" {
				mainTask.Name = planObj.Tasks[0].SubtaskName
				mainTask.Goal = planObj.Tasks[0].SubtaskGoal

				// 如果有多个子任务，使用除第一个外的所有任务作为子任务
				if len(planObj.Tasks) > 1 {
					for _, subtask := range planObj.Tasks[1:] {
						mainTask.Subtasks = append(mainTask.Subtasks, &AiTask{
							Coordinator:       c,
							Name:              subtask.SubtaskName,
							Goal:              subtask.SubtaskGoal,
							ParentTask:        mainTask,
							metadata:          map[string]interface{}{},
							toolCallResultIds: omap.NewOrderedMap(make(map[int64]*aitool.ToolResult)),
						})
					}
				}
			} else {
				// 主任务名称存在，将所有任务作为子任务
				for _, subtask := range planObj.Tasks {
					mainTask.Subtasks = append(mainTask.Subtasks, &AiTask{
						Coordinator:       c,
						Name:              subtask.SubtaskName,
						Goal:              subtask.SubtaskGoal,
						ParentTask:        mainTask,
						metadata:          map[string]interface{}{},
						toolCallResultIds: omap.NewOrderedMap(make(map[int64]*aitool.ToolResult)),
					})
				}
			}

			retTask = mainTask
			err = nil
			return
		}

		// 尝试直接解析为单个 aiTask 对象
		var simpleTask AiTask
		err = json.Unmarshal([]byte(taskJSON), &simpleTask)
		if err != nil {
			log.Debugf("Failed to parse taskJSON as simpleTask: %v. JSON: %s", err, taskJSON)
		}
		if err == nil && simpleTask.Name != "" {
			retTask = &simpleTask
			err = nil
			return
		}

		// 尝试解析为一个简单的 map 并创建 aiTask
		var taskMap map[string]interface{}
		err = json.Unmarshal([]byte(taskJSON), &taskMap)
		if err != nil {
			log.Debugf("Failed to parse taskJSON as taskMap: %v. JSON: %s", err, taskJSON)
		}
		if err == nil {
			if name, ok := taskMap["name"].(string); ok && name != "" {
				taskIns := &AiTask{
					Name:              name,
					Coordinator:       c,
					metadata:          map[string]interface{}{},
					toolCallResultIds: omap.NewOrderedMap(make(map[int64]*aitool.ToolResult)),
				}

				if goal, ok := taskMap["goal"].(string); ok {
					taskIns.Goal = goal
				}

				if subtasks, ok := taskMap["subtasks"].([]interface{}); ok {
					for _, st := range subtasks {
						if subtaskMap, ok := st.(map[string]interface{}); ok {
							if stName, ok := subtaskMap["name"].(string); ok && stName != "" {
								subtask := &AiTask{
									Name:              stName,
									metadata:          map[string]interface{}{},
									toolCallResultIds: omap.NewOrderedMap(make(map[int64]*aitool.ToolResult)),
								}

								if stGoal, ok := subtaskMap["goal"].(string); ok {
									subtask.Goal = stGoal
								}

								taskIns.Subtasks = append(taskIns.Subtasks, subtask)
							}
						}
					}
				}
				retTask = taskIns
				err = nil
				return
			}
		}
	}
	err = errors.New("no aiTask found in raw response")
	retTask = nil
	return
}

func (t *AiTask) SingleLineStatusSummary() string {
	return strings.ReplaceAll(t.StatusSummary, "\n", " ")
}

func (t *AiTask) QuoteName() string {
	return strconv.Quote(t.Name)
}

func (t *AiTask) QuoteGoal() string {
	return strconv.Quote(t.Goal)
}

func (t *AiTask) CanContinue() bool {
	return t.TaskContinueCount < t.Coordinator.MaxTaskContinue
}
