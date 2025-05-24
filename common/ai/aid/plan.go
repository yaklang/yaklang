package aid

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

type planRequest struct {
	config   *Config
	rawInput string
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
	if pr.config.allowPlanUserInteract {
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

	var task *aiTask = nil
	err = pr.config.callAiTransaction(
		prompt, pr.callAI,
		func(rsp *AIResponse) error {
			// 读取响应内容
			responseBytes, err := io.ReadAll(rsp.GetOutputStreamReader("plan", false, pr.config))
			if len(responseBytes) <= 0 {
				return utils.Error("ai resposne is empty")
			}
			response := string(responseBytes)

			// 从响应中提取任务
			task, err = ExtractTaskFromRawResponse(pr.config, response)
			if err != nil {
				return fmt.Errorf("从 AI 响应中提取任务失败: %v", err)
			}
			return nil
		},
	)
	if err != nil {
		pr.config.EmitError(err.Error())
		if task == nil || utils.IsNil(task) {
			return nil, err
		}
	}
	return pr.config.newPlanResponse(task), nil
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
