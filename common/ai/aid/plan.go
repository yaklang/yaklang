package aid

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"sync/atomic"
)

type planRequest struct {
	config   *Config
	rawInput string

	// interactCount
	disableInteract bool // 是否禁用用户交互
	interactCount   *int64
}

func (pr *planRequest) deltaInteractCount(i int64) {
	if pr.interactCount == nil {
		pr.interactCount = new(int64)
	}
	atomic.AddInt64(pr.interactCount, i)
}

func (pr *planRequest) GetInteractCount() int64 {
	if pr.interactCount == nil {
		return 0
	}
	return atomic.LoadInt64(pr.interactCount)
}

func (pr *planRequest) callAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		pr.config.planAICallback,
		pr.config.coordinatorAICallback,
		pr.config.taskAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(pr.config, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

type PlanResponse struct {
	RootTask *aiTask `json:"root_task"`
}

func (p *PlanResponse) recursiveMergeSubtask(subtask *aiTask, callback func(i *aiTask) error, stopped *utils.AtomicBool) {
	if subtask == nil || (stopped != nil && stopped.IsSet()) {
		return
	}

	err := callback(subtask)
	if err != nil {
		stopped.Set()
		return
	}
	if subtask.Subtasks == nil || len(subtask.Subtasks) <= 0 {
		return
	}
	for _, st := range subtask.Subtasks {
		p.recursiveMergeSubtask(st, callback, stopped)
	}
}

func (p *PlanResponse) MergeSubtask(parentIndex string, name string, goal string) {
	if p.RootTask == nil {
		return
	}
	p.RootTask.GenerateIndex()

	p.recursiveMergeSubtask(p.RootTask, func(i *aiTask) error {
		if i.Index != parentIndex {
			return nil
		}

		i.Subtasks = append(i.Subtasks, &aiTask{
			config:     p.RootTask.config,
			Name:       name,
			Goal:       goal,
			ParentTask: i,
		})
		return utils.Error("normal exit")
	}, utils.NewBool(false))
}

// GenerateFirstPlanPrompt 根据PlanRequest生成prompt
func (pr *planRequest) GenerateFirstPlanPrompt() (string, error) {
	if pr.config.allowPlanUserInteract && !pr.disableInteract {
		return pr.config.quickBuildPrompt(__prompt_GenerateTaskListPromptWithUserInteract, map[string]any{
			"Memory": pr.config.memory,
		})
	} else {
		return pr.config.quickBuildPrompt(__prompt_GenerateTaskListPrompt, map[string]any{
			"Memory": pr.config.memory,
		})
	}
}

func (pr *Config) newPlanResponse(rootTask *aiTask) *PlanResponse {
	pr.SetSyncCallback(SYNC_TYPE_PLAN, func() any {
		return rootTask
	})
	return &PlanResponse{
		RootTask: rootTask,
	}
}

// Invoke 执行规划请求，调用AI生成任务列表并返回解析后的Task
func (pr *planRequest) Invoke() (*PlanResponse, error) {
	if pr.config.planMocker != nil {
		planRes := pr.config.planMocker(pr.config)
		if utils.IsNil(planRes) {
			return nil, utils.Error("planMocker returns nil, unknown error")
		}
		return planRes, nil
	}
	// 生成 Prompt
	prompt, err := pr.GenerateFirstPlanPrompt()
	if err != nil {
		return nil, fmt.Errorf("生成规划 prompt 失败: %v", err)
	}

	var rootTask = &aiTask{}
	defer func() {
		// Ensure config is propagated to the new task and its subtasks
		var propagateConfig func(task *aiTask)
		propagateConfig = func(task *aiTask) {
			if task == nil {
				return
			}
			task.config = pr.config
			if task.toolCallResultIds == nil {
				task.toolCallResultIds = omap.NewOrderedMap(make(map[int64]*aitool.ToolResult))
			}
			for _, sub := range task.Subtasks {
				sub.ParentTask = task // Ensure parent is set
				propagateConfig(sub)
			}
		}
		propagateConfig(rootTask)
		rootTask.GenerateIndex()
	}()

	var interactAction *Action

	needInteract := func() bool {
		return interactAction != nil && interactAction.ActionType() == "require-user-interact"
	}

	err = pr.config.callAiTransaction(
		prompt, pr.callAI,
		func(rsp *AIResponse) error {
			action, err := ExtractActionFromStream(rsp.GetOutputStreamReader("plan", false, pr.config), "plan", "require-user-interact")
			if err != nil {
				return utils.Error("parse action from AI response failed: " + err.Error())
			}
			switch action.ActionType() {
			case "plan":
				rootTask.Name = action.GetString("main_task")
				rootTask.Goal = action.GetString("main_task_goal")
				for _, subtask := range action.GetInvokeParamsArray("tasks") {
					if subtask.GetString("subtask_name") == "" {
						continue
					}
					rootTask.Subtasks = append(rootTask.Subtasks, &aiTask{
						config: pr.config,
						Name:   subtask.GetString("subtask_name"),
						Goal:   subtask.GetString("subtask_goal"),
					})
				}
				if rootTask.Name == "" {
					return fmt.Errorf("AI response does not contain any tasks, please check your AI model or prompt")
				}
				return nil
			case "require-user-interact":
				interactAction = action
				return nil
			}
			return utils.Error("no any ai callback is set, cannot found ai config")
		},
	)
	if err != nil {
		pr.config.EmitError(err.Error())
		return nil, err
	}

	if needInteract() {
		return pr.handlePlanWithUserInteract(interactAction)
	}

	if rootTask.Name == "" {
		return nil, utils.Error("cannot found any task in AI response, please check your AI model or prompt")
	}

	return pr.config.newPlanResponse(rootTask), nil
}

func (c *Coordinator) createPlanRequest(rawUserInput string) (*planRequest, error) {
	req, err := createPlanRequest(rawUserInput)
	if err != nil {
		return nil, err
	}
	req.config = c.config
	req.rawInput = rawUserInput
	return req, nil
}

func createPlanRequest(rawUserInput string) (*planRequest, error) {
	request := &planRequest{
		rawInput: rawUserInput,
	}
	return request, nil
}
