package aid

import (
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_plan"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type planRequest struct {
	cod      *Coordinator
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

func (pr *planRequest) CallAI(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	for _, cb := range []aicommon.AICallbackType{
		pr.cod.QualityPriorityAICallback,
		pr.cod.SpeedPriorityAICallback,
		pr.cod.OriginalAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(pr.cod, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

type PlanResponse struct {
	RootTask *AiTask `json:"root_task"`
}

func (p *PlanResponse) recursiveMergeSubtask(subtask *AiTask, callback func(i *AiTask) error, stopped *utils.AtomicBool) {
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

	p.recursiveMergeSubtask(p.RootTask, func(i *AiTask) error {
		if i.Index != parentIndex {
			return nil
		}

		thisTask := p.RootTask.Coordinator.generateAITaskWithName(name, goal)
		thisTask.ParentTask = i

		i.Subtasks = append(i.Subtasks, thisTask)
		return utils.Error("normal exit")
	}, utils.NewBool(false))
}

// Invoke 执行规划请求，调用AI生成任务列表并返回解析后的Task
func (pr *planRequest) Invoke() (*PlanResponse, error) {
	if pr.cod.PlanMocker != nil {
		planRes := pr.cod.PlanMocker(pr.cod)
		if utils.IsNil(planRes) {
			return nil, utils.Error("planMocker returns nil, unknown error")
		}
		return planRes, nil
	}

	var rootTask = &AiTask{}
	defer func() {
		// Ensure config is propagated to the new task and its subtasks
		var propagateConfig func(task *AiTask)
		propagateConfig = func(task *AiTask) {
			if task == nil {
				return
			}
			task.Coordinator = pr.cod
			for _, sub := range task.Subtasks {
				sub.ParentTask = task // Ensure parent is set
				propagateConfig(sub)
			}
		}
		propagateConfig(rootTask)
		rootTask.GenerateIndex()
	}()

	planTask := aicommon.NewStatefulTaskBase(
		"plan-task",
		pr.rawInput,
		pr.cod.Ctx,
		pr.cod.Emitter,
	)
	err := pr.cod.ExecuteLoopTask(
		schema.AI_REACT_LOOP_NAME_PLAN,
		planTask,
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
			if isDone {
				planData := loop.Get(loop_plan.PLAN_DATA_KEY)
				action, err := aicommon.ExtractAction(planData, "generate_plan", "plan")
				if err != nil {
					log.Errorf("extract action from plan data failed: %v", err)
					return
				}
				rootTask.Name = action.GetAnyToString("main_task")
				rootTask.Goal = action.GetAnyToString("main_task_goal")
				rootTask.AIStatefulTaskBase = aicommon.NewStatefulTaskBase(
					"root-task"+uuid.NewString(),
					fmt.Sprintf("任务名称: %s\n任务目标: %s", rootTask.Name, rootTask.Goal),
					pr.cod.Ctx,
					pr.cod.Emitter,
				)
				for _, subtask := range action.GetInvokeParamsArray("tasks") {
					if subtask.GetAnyToString("subtask_name") == "" {
						continue
					}
					rootTask.Subtasks = append(rootTask.Subtasks, pr.cod.generateAITask(subtask))
				}
				if rootTask.Name == "" {
					log.Errorf("plan action missing main_task")
				}
			}
		}))
	if err != nil {
		return nil, err
	}
	return pr.cod.newPlanResponse(rootTask), nil
}

func (c *Coordinator) generateAITask(params aitool.InvokeParams) *AiTask {
	return c.generateAITaskWithName(params.GetAnyToString("subtask_name"), params.GetAnyToString("subtask_goal"))
}

func (c *Coordinator) generateAITaskWithName(name, goal string) *AiTask {
	task := &AiTask{
		Coordinator: c,
		Name:        name,
		Goal:        goal,
	}

	taskBase := aicommon.NewStatefulTaskBase(
		"plan-subtask-"+uuid.NewString(),
		fmt.Sprintf("任务名称: %s\n任务目标: %s", task.Name, task.Goal),
		c.Ctx,
		c.Emitter,
	)
	task.AIStatefulTaskBase = taskBase
	return task
}

func (c *Coordinator) createPlanRequest(rawUserInput string) (*planRequest, error) {
	req, err := createPlanRequest(rawUserInput)
	if err != nil {
		return nil, err
	}
	req.cod = c
	req.rawInput = rawUserInput
	return req, nil
}

func createPlanRequest(rawUserInput string) (*planRequest, error) {
	request := &planRequest{
		rawInput: rawUserInput,
	}
	return request, nil
}
