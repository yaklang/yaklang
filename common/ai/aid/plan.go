package aid

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"text/template"
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
	tmpl, err := template.New("generateTaskList").Parse(__prompt_GenerateTaskListPrompt)
	if err != nil {
		return "", err
	}

	// 准备模板数据
	data := map[string]interface{}{
		"Memory": pr.config.memory,
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
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

	//// 调用 AI 回调函数
	//saverMutex := new(sync.Mutex)
	//var seqId int64
	//var saver CheckpointCommitHandler
	//
	//planCreator := func(overrideId int64) (*aiTask, error) {
	//	responseReader, err := pr.callAI(NewAIRequest(
	//		prompt,
	//		WithAIRequest_OnAcquireSeq(func(i int64) {
	//			saverMutex.Lock()
	//			defer saverMutex.Unlock()
	//			seqId = i
	//		}),
	//		WithAIRequest_SaveCheckpointCallback(func(f CheckpointCommitHandler) {
	//			saverMutex.Lock()
	//			defer saverMutex.Unlock()
	//			saver = f
	//		}), WithAIRequest_SeqId(seqId)),
	//	)
	//	if err != nil {
	//		return nil, fmt.Errorf("call ai err: %v", err)
	//	}
	//
	//	saverMutex.Lock()
	//	defer saverMutex.Unlock()
	//	if !utils.IsNil(saver) && saver != nil {
	//		pr.config.EmitInfo("start to save checkpoint into db: %v", seqId)
	//		cp, err := saver()
	//		if err != nil {
	//			pr.config.EmitError("cannot save checkpoint")
	//		} else {
	//			pr.config.EmitInfo("checkpoint cached in database: %v:%v", utils.ShrinkString(cp.CoordinatorUuid, 12), cp.Seq)
	//		}
	//	}
	//	return task, nil
	//}
	//
	//var task *aiTask = nil
	//for utils.IsNil(task) {
	//	task, err = planCreator(seqId)
	//	if err != nil {
	//		pr.config.EmitError("create plan err: %v", err)
	//		select {
	//		case <-pr.config.ctx.Done():
	//		case <-time.After(200 * time.Millisecond):
	//			pr.config.EmitError("retry to plan with original id: %v", seqId)
	//		}
	//	}
	//}
	//return pr.config.newPlanResponse(task), nil
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
