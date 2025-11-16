package aid

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aispec"
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
	*Coordinator
	*aicommon.AIStatefulTaskBase
	Index      string    `json:"index"`
	Name       string    `json:"name"`
	Goal       string    `json:"goal"`
	ParentTask *AiTask   `json:"parent_task"`
	Subtasks   []*AiTask `json:"subtasks"`

	StatusSummary string `json:"status_summary"`
	TaskSummary   string `json:"task_summary"`
	ShortSummary  string `json:"short_summary"`
	LongSummary   string `json:"long_summary"`

	toolCallResultIds *omap.OrderedMap[int64, *aitool.ToolResult]
}

func (t *AiTask) executed() bool {
	if len(t.Subtasks) > 0 {
		for _, subtask := range t.Subtasks {
			if !subtask.executed() { // 子任务有没有已完成的，本任务也视为未完成
				return false
			}
		}
		return true
	}
	return t.GetStatus() == aicommon.AITaskState_Completed
}

func (t *AiTask) executing() bool {
	if len(t.Subtasks) > 0 {
		for _, subtask := range t.Subtasks {
			if subtask.executing() { // 子任务有执行中的时候 本任务也为执行中
				return true
			}
		}
		return false
	}
	return t.GetStatus() == aicommon.AITaskState_Processing
}

func (t *AiTask) SetID(id string) {
	if t.AIStatefulTaskBase != nil {
		t.AIStatefulTaskBase.SetID(id)
	}
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
	for _, v := range t.GetAllToolCallResults() {
		if v.Success {
			count++
		}
	}
	return count
}

func (t *AiTask) GetFailCallCount() int {
	count := 0
	for _, v := range t.GetAllToolCallResults() {
		if !v.Success {
			count++
		}
	}
	return count
}

func (t *AiTask) GetEmitter() *aicommon.Emitter {
	if t.Emitter == nil {
		return t.Coordinator.GetEmitter()
	}
	return t.Emitter

}

// MarshalJSON 实现自定义的JSON序列化，跳过AICallback字段
func (t *AiTask) MarshalJSON() ([]byte, error) {
	type TaskAlias AiTask // 创建一个别名类型以避免递归调用
	var progress string
	if t.GetStatus() == aicommon.AITaskState_Completed {
		progress = "success"
	} else if t.GetStatus() == aicommon.AITaskState_Processing {
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
		TotalToolCallCount:   int64(len(t.GetAllToolCallResults())),
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
	t.AIStatefulTaskBase = aicommon.NewStatefulTaskBase(
		fmt.Sprintf("pe-task-%s", t.Index),
		aux.Goal,
		t.Ctx,
		nil)
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
	action, err := aicommon.ExtractAction(rawResponse, "re-plan")
	if err != nil {
		return nil, err
	}

	var taskList []*AiTask
	for _, params := range action.GetInvokeParamsArray("next_plans") {
		taskList = append(taskList, c.generateAITask(params))
	}
	if len(taskList) <= 0 {
		return nil, errors.New("no aiTask found in next-plan")
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
	currentTask.SetID(currentIndex)

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
func (t *AiTask) GenerateIndex() {
	if t == nil {
		return
	}

	root := t
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
	retTask = c.generateAITaskWithName("root-default", "root-default")
	action, err := aicommon.ExtractAction(rawResponse, "plan")
	if err != nil {
		log.Errorf("extract action from plan data failed: %v", err)
		return
	}
	switch action.ActionType() {
	case "plan":
		retTask = c.generateAITaskWithName(action.GetAnyToString("main_task"), action.GetAnyToString("main_task_goal"))
		for _, subtask := range action.GetInvokeParamsArray("tasks") {
			if subtask.GetAnyToString("subtask_name") == "" {
				continue
			}
			retTask.Subtasks = append(retTask.Subtasks, c.generateAITask(subtask))
		}
		if retTask.Name == "" {
			log.Errorf("plan action missing main_task")
		}
	}
	if retTask == nil {
		return nil, errors.New("no valid plan action found in response")
	}

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

// ToolCallCount 返回工具调用次数
func (t *AiTask) ToolCallCount() int {
	if t == nil {
		return 0
	}
	return len(t.GetAllToolCallResults())
}

// TaskContinueCount 返回任务继续执行的次数（从 ReActLoop 获取迭代次数）
func (t *AiTask) TaskContinueCount() int {
	if t == nil {
		return 0
	}
	// 尝试从 ReActLoop 获取当前迭代次数
	loop := t.GetReActLoop()
	if loop == nil {
		return 0
	}
	// 使用类型断言获取迭代次数（ReActLoopIF 接口中没有这个方法，需要类型断言）
	// 如果无法获取，返回 0
	if reactLoop, ok := loop.(interface{ GetCurrentIterationIndex() int }); ok {
		return reactLoop.GetCurrentIterationIndex()
	}
	return 0
}
