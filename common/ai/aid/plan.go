package aid

import (
	_ "embed"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_plan"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

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
		pr.cod.EmitThoughtStream("mock task", "使用模版预设任务")
		planRes := pr.cod.PlanMocker(pr.cod)
		if utils.IsNil(planRes) {
			return nil, utils.Error("planMocker returns nil, unknown error")
		}
		// Ensure all tasks from PlanMocker have proper initialization:
		// AIStatefulTaskBase, Coordinator reference, and SemanticIdentifier.
		// This handles cases where PlanMocker constructs AiTask objects manually
		// without going through generateAITaskWithName.
		if planRes.RootTask != nil {
			pr.cod.ensureTaskTreeInitialized(planRes.RootTask)
			planRes.RootTask.GenerateIndex()
		}
		return planRes, nil
	}

	var rootTask = pr.cod.generateAITaskWithName("root-default", "root-default")
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
		true,
	)

	// Set PlanPrompt to KeyValueConfig for Plan Loop to use
	// This content appears only during plan initialization
	if pr.cod.Config.PlanPrompt != "" {
		pr.cod.Config.SetConfig(loop_plan.PLAN_PROMPT_KEY, pr.cod.Config.PlanPrompt)
	}

	err := pr.cod.ExecuteLoopTask(
		schema.AI_REACT_LOOP_NAME_PLAN,
		planTask,
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, _ *reactloops.OnPostIterationOperator) {
			if isDone {
				planData := loop.Get(loop_plan.PLAN_DATA_KEY)
				action, err := aicommon.ExtractAction(planData, "plan", "plan")
				if err != nil {
					log.Errorf("extract action from plan data failed: %v", err)
					return
				}
				rootTask = pr.cod.generateAITaskWithName(action.GetAnyToString("main_task"), action.GetAnyToString("main_task_goal"))

				if !strings.Contains(rootTask.GetUserInput(), pr.rawInput) {
					// keep raw user input context
					nonce := utils.RandStringBytes(4)
					taskInput := rootTask.GetUserInput()
					i := utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .RawUserInput }}
<|用户原始需求_END_{{.nonce}}|>
--- 
{{ .Origin }}
`,
						map[string]any{
							"nonce":        nonce,
							"RawUserInput": pr.rawInput,
							"Origin":       taskInput,
						})
					rootTask.SetUserInput(i)
				}

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
		"plan-task"+uuid.NewString(),
		fmt.Sprintf("任务名称: %s\n任务目标: %s", task.Name, task.Goal),
		c.Ctx,
		c.Emitter,
		true,
	)
	task.AIStatefulTaskBase = taskBase
	taskBase.SetName(name)

	// Generate semantic identifier for directory naming
	semanticId := c.generateSemanticIdentifier(name)
	task.SetSemanticIdentifier(semanticId)

	nonce := utils.RandStringBytes(4)
	taskInput := task.GetUserInput()
	i := utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .RawUserInput }}
<|用户原始需求_END_{{.nonce}}|>
--- 
{{ .Origin }}
`,
		map[string]any{
			"nonce":        nonce,
			"RawUserInput": c.userInput,
			"Origin":       taskInput,
		})
	task.SetUserInput(i)

	return task
}

// generateSemanticIdentifier generates a short semantic identifier from a task name.
// Logic:
//  1. If name is empty, return empty.
//  2. Sanitize the name; if short enough (≤20 runes), use it directly.
//  3. If too long, use SpeedPriority AI to generate a shorter identifier.
//  4. If AI is unavailable or fails, fall back to truncation.
func (c *Coordinator) generateSemanticIdentifier(name string) string {
	if name == "" {
		return ""
	}

	sanitized := aicommon.SanitizeTaskName(name)
	if sanitized == "" {
		return ""
	}

	const maxIdentifierRuneLen = 20
	runes := []rune(sanitized)
	if len(runes) <= maxIdentifierRuneLen {
		return sanitized
	}

	// Name is too long, use LiteForge to generate a shorter identifier via speed-priority AI
	truncateFallback := func() string {
		truncated := string(runes[:maxIdentifierRuneLen])
		return strings.TrimRight(truncated, "_")
	}

	aiCallback := c.SpeedPriorityAICallback
	if aiCallback == nil {
		aiCallback = c.OriginalAICallback
	}
	if aiCallback == nil {
		return truncateFallback()
	}

	prompt := fmt.Sprintf(`Generate a very short identifier (2-6 words, max 20 characters total) for the following task name.
The identifier should capture the core meaning. Chinese or English are both acceptable.
Reply with ONLY the JSON: {"@action":"object","identifier":"YOUR_IDENTIFIER"}

Task name: %s`, name)

	forgeResult, err := aicommon.InvokeLiteForge(prompt, aicommon.WithAICallback(aiCallback))
	if err != nil {
		log.Debugf("liteforge failed to generate semantic identifier for %q: %v, falling back to truncation", name, err)
		return truncateFallback()
	}
	if forgeResult == nil || forgeResult.Action == nil {
		log.Debugf("liteforge returned nil result for %q, falling back to truncation", name)
		return truncateFallback()
	}

	result := strings.TrimSpace(forgeResult.Action.GetString("identifier"))
	result = aicommon.SanitizeTaskName(result)
	if result == "" {
		return truncateFallback()
	}

	// Ensure the AI-generated identifier is within limits
	resultRunes := []rune(result)
	if len(resultRunes) > maxIdentifierRuneLen {
		result = string(resultRunes[:maxIdentifierRuneLen])
		result = strings.TrimRight(result, "_")
	}
	return result
}

// ensureTaskTreeInitialized recursively walks the task tree and ensures each task
// has proper AIStatefulTaskBase, Coordinator reference, SemanticIdentifier, and parent pointers.
// This is critical for tasks created via PlanMocker (preset tasks) which may bypass
// generateAITaskWithName and lack proper initialization.
func (c *Coordinator) ensureTaskTreeInitialized(task *AiTask) {
	if task == nil {
		return
	}

	// Ensure Coordinator is set
	task.Coordinator = c

	// Ensure AIStatefulTaskBase is initialized
	if task.AIStatefulTaskBase == nil {
		taskBase := aicommon.NewStatefulTaskBase(
			"plan-task"+uuid.NewString(),
			fmt.Sprintf("任务名称: %s\n任务目标: %s", task.Name, task.Goal),
			c.Ctx,
			c.Emitter,
			true,
		)
		task.AIStatefulTaskBase = taskBase
		taskBase.SetName(task.Name)
	}

	// Ensure SemanticIdentifier is set
	if task.SemanticIdentifier == "" && task.Name != "" {
		semanticId := c.generateSemanticIdentifier(task.Name)
		task.SetSemanticIdentifier(semanticId)
	}

	// Recursively process subtasks
	for _, sub := range task.Subtasks {
		sub.ParentTask = task
		c.ensureTaskTreeInitialized(sub)
	}
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
