package aid

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// BuildRootTaskFromPlanData parses plan loop output into an executable task tree.
func (c *Coordinator) BuildRootTaskFromPlanData(planData string, rawInput string) (*AiTask, error) {
	if c == nil {
		return nil, utils.Error("coordinator is nil")
	}
	planData = strings.TrimSpace(planData)
	if planData == "" {
		return nil, utils.Error("plan data is empty")
	}

	action, err := aicommon.ExtractAction(planData, "plan", "plan")
	if err != nil {
		return nil, utils.Errorf("extract action from plan data failed: %w", err)
	}

	rootTask := c.generateAITaskWithName(action.GetAnyToString("main_task"), action.GetAnyToString("main_task_goal"))
	if identifier := action.GetAnyToString("main_task_identifier"); identifier != "" {
		sanitized := aicommon.SanitizeTaskName(identifier)
		if sanitized != "" {
			rootTask.SetSemanticIdentifier(sanitized)
		}
	}

	rawInput = strings.TrimSpace(rawInput)
	if rawInput != "" && !strings.Contains(rootTask.GetUserInput(), rawInput) {
		nonce := utils.RandStringBytes(4)
		taskInput := rootTask.GetUserInput()
		enhanced := utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .RawUserInput }}
<|用户原始需求_END_{{.nonce}}|>
--- 
{{ .Origin }}
`,
			map[string]any{
				"nonce":        nonce,
				"RawUserInput": rawInput,
				"Origin":       taskInput,
			})
		rootTask.SetUserInput(enhanced)
	}

	for _, subtask := range action.GetInvokeParamsArray("tasks") {
		if subtask.GetAnyToString("subtask_name") == "" {
			continue
		}
		rootTask.Subtasks = append(rootTask.Subtasks, c.generateAITask(subtask))
	}
	if rootTask.Name == "" {
		return nil, utils.Error("plan action missing main_task")
	}
	if len(rootTask.Subtasks) <= 0 {
		return nil, utils.Error("plan has no subtasks")
	}

	return c.standardizeTaskTreeAndNotify(rootTask, "approved plan prepared"), nil
}

// CommitApprovedPlan stores an approved plan and persists plan_ready state when possible.
func (c *Coordinator) CommitApprovedPlan(root *AiTask, facts, document string) error {
	if c == nil {
		return utils.Error("coordinator is nil")
	}
	if root == nil {
		return utils.Error("root task is nil")
	}
	if len(root.Subtasks) <= 0 {
		return utils.Error("plan has no subtasks")
	}

	c.rootTask = root
	if c.ContextProvider != nil {
		c.ContextProvider.StoreRootTask(root)
	}
	if strings.TrimSpace(facts) != "" {
		appendPlanFactsFrozenPartition(c.Config, facts)
	}
	if strings.TrimSpace(document) != "" {
		appendPlanDocumentFrozenPartition(c.Config, document)
	}
	c.savePlanAndExecState(Phase_PlanReady, nil)
	return nil
}

// ReviewPlanThroughUser emits PLAN_REVIEW_REQUIRE and blocks until the user approves or revises the plan.
func (c *Coordinator) ReviewPlanThroughUser(ctx context.Context, planPayload string, rsp *PlanResponse) (*PlanResponse, error) {
	if c == nil {
		return nil, utils.Error("coordinator is nil")
	}
	if rsp == nil || rsp.RootTask == nil {
		return nil, utils.Error("plan response is nil")
	}

	c.planLoadingStatus("任务规划等待用户审查 / Waiting User to Review Plan...")

	planReq, err := c.createPlanRequest(planPayload)
	if err != nil {
		return nil, err
	}

	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	c.EmitRequireReviewForPlan(rsp, ep.GetId())

	if ctx == nil {
		ctx = c.GetContext()
	}
	c.waitPlanReviewAgree(ctx, ep)
	params := ep.GetParams()
	c.ReleaseInteractiveEvent(ep.GetId(), params)
	if params == nil {
		c.planLoadingStatus("用户审查失败 / User Review Failed")
		return nil, utils.Errorf("coordinator: user review params is nil")
	}

	c.planLoadingStatus("处理用户审查结果 / Processing User Review...")
	approved, err := planReq.handleReviewPlanResponse(rsp, params)
	if err != nil {
		return nil, err
	}
	if approved == nil || approved.RootTask == nil {
		return nil, utils.Error("approved plan root task is nil")
	}
	if len(approved.RootTask.Subtasks) <= 0 {
		return nil, utils.Error("approved plan has no subtasks")
	}
	c.standardizeTaskTreeAndNotify(approved.RootTask, "plan review approved")
	return approved, nil
}

func (c *Coordinator) waitPlanReviewAgree(ctx context.Context, ep *aicommon.Endpoint) {
	if c == nil || c.Config == nil || ep == nil {
		return
	}
	if ctx == nil {
		ctx = c.GetContext()
	}
	if c.ForceManualPlanReview {
		c.DoWaitAgreeWithPolicy(ctx, aicommon.AgreePolicyManual, ep)
		return
	}
	c.DoWaitAgree(ctx, ep)
}

func serializeAiTaskToPlanParams(task *AiTask) map[string]any {
	if task == nil {
		return nil
	}
	item := map[string]any{
		"subtask_name": task.Name,
		"subtask_goal": task.Goal,
	}
	if len(task.DependsOn) > 0 {
		item["depends_on"] = task.DependsOn
	}
	if identifier := task.GetSemanticIdentifier(); identifier != "" {
		item["subtask_identifier"] = identifier
	}
	if len(task.Subtasks) > 0 {
		children := make([]map[string]any, 0, len(task.Subtasks))
		for _, sub := range task.Subtasks {
			if child := serializeAiTaskToPlanParams(sub); child != nil {
				children = append(children, child)
			}
		}
		if len(children) > 0 {
			item["sub_subtasks"] = children
		}
	}
	return item
}

// SerializeRootTaskToPlanData converts an approved task tree back into plan loop JSON payload.
func SerializeRootTaskToPlanData(root *AiTask) string {
	if root == nil {
		return ""
	}
	tasks := make([]map[string]any, 0, len(root.Subtasks))
	for _, sub := range root.Subtasks {
		if item := serializeAiTaskToPlanParams(sub); item != nil {
			tasks = append(tasks, item)
		}
	}
	payload := map[string]any{
		"@action":        "plan",
		"main_task":      root.Name,
		"main_task_goal": root.Goal,
		"tasks":          tasks,
	}
	if identifier := root.GetSemanticIdentifier(); identifier != "" {
		payload["main_task_identifier"] = identifier
	}
	return string(utils.Jsonify(payload))
}
