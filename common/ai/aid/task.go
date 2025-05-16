package aid

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
)

// TaskResponseCallback 定义Task执行过程中响应回调函数类型
type TaskResponseCallback func(ctx *Memory, details ...aispec.ChatDetail) (continueThinking bool, prompt string, err error)

// TaskProgress 记录任务执行的进度信息
type TaskProgress struct {
	TotalTasks     int    `json:"total_tasks"`     // 总任务数
	CompletedTasks int    `json:"completed_tasks"` // 已完成任务数
	CurrentTask    string `json:"current_task"`    // 当前执行的任务
	CurrentGoal    string `json:"current_goal"`    // 当前任务的目标
}

type aiTask struct {
	config *Config

	Index      string    `json:"index"`
	Name       string    `json:"name"`
	Goal       string    `json:"goal"`
	ParentTask *aiTask   `json:"parent_task"`
	Subtasks   []*aiTask `json:"subtasks"`

	ResponseCallback TaskResponseCallback `json:"-"` // 响应回调函数

	// 新增字段，存储默认工具和元数据
	metadata map[string]interface{}

	executing bool
	executed  bool
	rerun     bool

	// runtime
	//ToolCallResults   []*aitool.ToolResult `json:"tool_call_results"`
	toolCallResultIds *omap.OrderedMap[int64, *aitool.ToolResult]
	StatusSummary     string `json:"status_summary"`
	TaskSummary       string `json:"task_summary"`
	ShortSummary      string `json:"short_summary"`
	LongSummary       string `json:"long_summary"`
}

func (t *aiTask) callAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		t.config.taskAICallback,
		t.config.coordinatorAICallback,
		t.config.planAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(t.config, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

func (t *aiTask) PushToolCallResult(i *aitool.ToolResult) {
	t.toolCallResultIds.Set(i.GetID(), i)
	t.config.memory.PushToolCallResults(i)
}

// MarshalJSON 实现自定义的JSON序列化，跳过AICallback字段
func (t aiTask) MarshalJSON() ([]byte, error) {
	type TaskAlias aiTask // 创建一个别名类型以避免递归调用

	// 创建一个不包含AICallback的结构体
	return json.Marshal(struct {
		Name     string    `json:"name"`
		Goal     string    `json:"goal"`
		Subtasks []*aiTask `json:"subtasks,omitempty"`
	}{
		Name:     t.Name,
		Goal:     t.Goal,
		Subtasks: t.Subtasks,
	})
}

// UnmarshalJSON 实现自定义的JSON反序列化，跳过AICallback字段
func (t *aiTask) UnmarshalJSON(data []byte) error {
	// 创建一个临时结构体，不包含AICallback
	aux := struct {
		Name     string    `json:"name"`
		Goal     string    `json:"goal"`
		Subtasks []*aiTask `json:"subtasks,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	t.Name = aux.Name
	t.Goal = aux.Goal
	t.Subtasks = aux.Subtasks
	return nil
}

func ExtractPlan(c *Config, rawResponse string) (*PlanResponse, error) {
	at, err := ExtractTaskFromRawResponse(c, rawResponse)
	if err != nil {
		return nil, err
	}
	return &PlanResponse{RootTask: at}, nil
}

func ExtractNextPlanTaskFromRawResponse(c *Config, rawResponse string) ([]*aiTask, error) {
	for _, item := range jsonextractor.ExtractObjectIndexes(rawResponse) {
		start, end := item[0], item[1]
		taskJSON := rawResponse[start:end]

		// 尝试解析为新的 aiTask schema 结构
		var planObj struct {
			Action       string    `json:"@action"`
			NextPlanTask []*aiTask `json:"next_plans"`
		}

		err := json.Unmarshal([]byte(taskJSON), &planObj)
		if err == nil && planObj.Action == "re-plan" && len(planObj.NextPlanTask) > 0 {
			return planObj.NextPlanTask, nil
		}
	}
	return nil, errors.New("no aiTask found in next-plans")
}

// _genIdx 递归地为任务及其子任务生成索引。
// currentStartIndex 是当前任务建议的起始索引。
// parentAssignedIndex 是其父任务被分配的索引值。
// 返回值是处理完此任务及其所有子任务后，下一个可用的起始索引。
func _genIdx(currentStartIndex int, parentAssignedIndex int, i *aiTask) int {
	if i == nil {
		return currentStartIndex
	}
	idx := fmt.Sprintf("%d-%d", currentStartIndex, parentAssignedIndex)
	i.Index = idx

	// 当前任务的 assignedIndex 就是 currentStartIndex
	assignedIndexForThisTask := currentStartIndex
	// 下一个子任务应该从 currentStartIndex + 1 开始
	nextSubTaskStartIndex := currentStartIndex + 1

	for _, subNode := range i.Subtasks {
		// 递归调用 _genIdx，它会返回其处理完后的下一个可用 startIndex
		nextSubTaskStartIndex = _genIdx(nextSubTaskStartIndex, assignedIndexForThisTask, subNode)
	}
	// 返回处理完当前节点及其所有子节点后，下一个兄弟节点可用的 startIndex
	return nextSubTaskStartIndex
}

func (a *aiTask) GenerateIndex() {
	if a == nil {
		return
	}
	if a.ParentTask != nil {
		var root *aiTask = a.ParentTask
		for i := 0; i < 1000; i++ {
			if root.ParentTask == nil {
				_genIdx(1, 0, root)
				return
			}
			root = root.ParentTask
		}
		if root != nil && root.ParentTask != nil {
			_genIdx(1, 0, root)
		}
	} else {
		_genIdx(1, 0, a)
	}
}

// ExtractTaskFromRawResponse 从原始响应中提取Task
func ExtractTaskFromRawResponse(c *Config, rawResponse string) (retTask *aiTask, err error) {
	defer func() {
		if retTask == nil {
			return
		}
		// Ensure config is propagated to the new task and its subtasks
		var propagateConfig func(task *aiTask)
		propagateConfig = func(task *aiTask) {
			if task == nil {
				return
			}
			task.config = c
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
			mainTask := &aiTask{
				config:   c,
				Name:     planObj.MainTask,
				Goal:     planObj.MainTaskGoal,
				Subtasks: make([]*aiTask, 0),
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
						mainTask.Subtasks = append(mainTask.Subtasks, &aiTask{
							config:            c,
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
					mainTask.Subtasks = append(mainTask.Subtasks, &aiTask{
						config:            c,
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
		var simpleTask aiTask
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
				taskIns := &aiTask{
					Name:              name,
					config:            c,
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
								subtask := &aiTask{
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

func (t *aiTask) SingleLineStatusSummary() string {
	return strings.ReplaceAll(t.StatusSummary, "\n", " ")
}

func (t *aiTask) QuoteName() string {
	return strconv.Quote(t.Name)
}

func (t *aiTask) QuoteGoal() string {
	return strconv.Quote(t.Goal)
}
