package aid

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed jsonschema/plan-review/freedom-plan-review.json
var schemaFreedomReviewPlan string

type PlanReviewSuggestion struct {
	Id               string `json:"id"`
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`

	PromptBuilder    func(plan *planRequest, rt *runtime) `json:"-"`
	ResponseCallback func(reader io.Reader)               `json:"-"`
	ParamSchema      string                               `json:"param_schema"`
}

func (c *Config) getPlanReviewSuggestion() []*PlanReviewSuggestion {
	opt := []*PlanReviewSuggestion{
		{
			Value:            "freedom-review",
			Prompt:           "审阅模式",
			PromptEnglish:    "User freely review the plan, can add more details or modify the plan",
			AllowExtraPrompt: true,
			ParamSchema:      schemaFreedomReviewPlan,
		},
		{
			Value:            "unclear",
			Prompt:           "目标不明确",
			PromptEnglish:    `The plan is too vague and fuzzy, needs more specific objectives and clearer definition`,
			AllowExtraPrompt: true,
		},
		{
			Value:            "incomplete",
			Prompt:           "有遗漏",
			PromptEnglish:    "The plan is not complete enough, more details need to be added",
			AllowExtraPrompt: true,
		},
		{
			Value:            "create-subtask",
			Prompt:           "需要拆分子任务",
			PromptEnglish:    "Create Subtask for current level task, if user not specified, auto evaluate how to modify the task",
			AllowExtraPrompt: true,
		},
		{
			Value:         "continue",
			Prompt:        "计划合理，继续执行",
			PromptEnglish: "The plan is reasonable, continue execution",
		},
	}
	for idx, o := range opt {
		o.Id = fmt.Sprintf("plan-review-suggestion-%v-%d", c.id, idx)
	}
	return opt
}

func (p *planRequest) handleReviewPlanResponse(rsp *PlanResponse, param aitool.InvokeParams) (*PlanResponse, error) {
	reviewQuestion := "请审查当前的任务计划"

	if utils.IsNil(rsp) {
		return nil, utils.Error("plan response is nil")
	}

	// 1. 获取审查建议
	suggestion := param.GetString("suggestion")
	if suggestion == "" {
		p.config.EmitError("suggestion is empty, use default: continue")
		return rsp, nil
	}

	if suggestion == "continue" {
		p.config.EmitInfo("plan is reasonable, continue")
		// 继续执行现有计划
		return rsp, nil
	}

	// 2. 根据审查建议处理
	switch suggestion {
	case "unclear":
		p.config.EmitInfo("user suggestion: the plan is unclear, regenerate plan")
		extraPrompt := param.GetString("extra_prompt")
		if extraPrompt == "" {
			extraPrompt = param.GetString("prompt")
		}
		if extraPrompt == "" {
			extraPrompt = "用户认为当前计划不够明确，需要细化每个子任务具体的目标和清晰的定义，包含其中可能会用到的工具"
		}
		newPlan, err := p.generateNewPlan(suggestion, extraPrompt, rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return nil, utils.Errorf("generate new plan failed: %v", err)
		}

		ep := p.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		p.config.EmitRequireReviewForPlan(newPlan, ep.GetId())
		p.config.DoWaitAgree(nil, ep)
		params := ep.GetParams()
		p.config.ReleaseInteractiveEvent(ep.GetId(), params)
		if params == nil {
			p.config.EmitError("user review params is nil, plan failed")
			return newPlan, nil
		}
		p.config.CallAfterReview(ep.GetSeq(), reviewQuestion, params)
		return p.handleReviewPlanResponse(newPlan, params)
	case "incomplete":
		p.config.EmitInfo("plan is incomplete")
		// 重新生成计划，但保留现有任务
		extraPrompt := param.GetString("extra_prompt")
		if extraPrompt == "" {
			extraPrompt = param.GetString("prompt")
		}
		newPlan, err := p.generateNewPlan(suggestion, extraPrompt, rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return nil, utils.Errorf("generate new plan failed: %v", err)
		}

		ep := p.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		p.config.EmitRequireReviewForPlan(newPlan, ep.GetId())
		p.config.DoWaitAgree(nil, ep)
		params := ep.GetParams()
		p.config.ReleaseInteractiveEvent(ep.GetId(), params)
		if params == nil {
			p.config.EmitError("user review params is nil, plan failed")
			return newPlan, nil
		}
		p.config.CallAfterReview(ep.GetSeq(), reviewQuestion, params)
		return p.handleReviewPlanResponse(newPlan, params)
	case "create-subtask":
		p.config.EmitError("create-subtask required via user suggestion")
		extraPrompt := param.GetString("extra_prompt")
		if extraPrompt == "" {
			extraPrompt = "用户认为某些任务太宽泛了，需要被分割成更小的子任务以确保执行顺利，切分制定任务需要注意一下原则\n"
			extraPrompt += " S-M-A-R-T: SMART 代表：1. Specific（具体的） 2. Measurable（可衡量的） 3. Achievable（可实现的） 4. Relevant（相关的） 5. Time-bound（有时限的）。\n" +
				"SMART 是一个用于设定目标和评估目标达成度的标准。它帮助人们设定清晰、可行和可衡量的目标，以便更好地规划和实现个人或团队的愿景和任务。\n" +
				"从这几个角度考虑。\n## 注意\n1. 你运行在一个由外部思维链约束的任务中，尽量保持输出简短，保留任务相关元素，避免冗长描述\n" +
				"2. 如果重点是输出JSON，则只输出JSON\n" +
				"3. 不需要输出算法简介和背景相关知识\n" +
				"4. 无需额外解释"
		}
		targetPlans := param.GetStringSlice("target_plans")
		if len(targetPlans) > 0 {
			extraPrompt += "\n用户认为你应该重点关注的子任务：" + fmt.Sprint(targetPlans)
		} else {
			extraPrompt += "\n用户没有规定你需要具体拆分哪些子任务，你需要自己决定在当前任务树的叶节点拆分"
		}
		newPlan, err := p.generateCreateSubtaskPlan(extraPrompt, rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return nil, utils.Errorf("generate new plan failed: %v", err)
		}

		ep := p.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		p.config.EmitRequireReviewForPlan(newPlan, ep.GetId())
		p.config.DoWaitAgree(nil, ep)
		params := ep.GetParams()
		p.config.ReleaseInteractiveEvent(ep.GetId(), params)
		if params == nil {
			p.config.EmitError("user review params is nil, plan failed")
			return newPlan, nil
		}
		p.config.CallAfterReview(ep.GetSeq(), reviewQuestion, params)
		return p.handleReviewPlanResponse(newPlan, params)
	case "freedom-review":
		p.config.EmitInfo("user uses freedom review mode to review the plan")
		// 重新生成计划，但保留现有任务
		var userReviewTaskTree strings.Builder
		err := generateReviewedTaskTree(param.GetObject("reviewed-task-tree"), &userReviewTaskTree, "")
		if err != nil {
			return nil, err
		}

		newPlan, err := p.freedomReviewGenerateNewPlan(userReviewTaskTree.String(), rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return nil, utils.Errorf("generate new plan failed: %v", err)
		}

		ep := p.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		p.config.EmitRequireReviewForPlan(newPlan, ep.GetId())
		p.config.DoWaitAgree(nil, ep)
		params := ep.GetParams()
		p.config.ReleaseInteractiveEvent(ep.GetId(), params)
		if params == nil {
			p.config.EmitError("user review params is nil, plan failed")
			return newPlan, nil
		}
		p.config.CallAfterReview(ep.GetSeq(), reviewQuestion, params)
		return p.handleReviewPlanResponse(newPlan, params)
	default:
		p.config.EmitError("unknown review suggestion: %s", suggestion)
		return rsp, nil
	}
}

func generateReviewedTaskTree(task aitool.InvokeParams, buf *strings.Builder, prefix string) error {
	write := func(content string) {
		buf.WriteString(prefix)
		buf.WriteString(content)
	}
	write(fmt.Sprintf("- 任务名称:%s ; 任务目标: %s\n", task.GetString("name"), task.GetString("goal")))
	if remove := task.GetBool("isRemove"); remove {
		write(fmt.Sprintln("   **此任务已被用户移除**"))
	}
	if desc := task.GetString("description"); desc != "" {
		write(fmt.Sprintf("  任务描述: %s\n", desc))
	}
	if tools := task.GetStringSlice("tools"); len(tools) > 0 {
		write(fmt.Sprintf("  任务关键词: %s\n", strings.Join(tools, ", ")))
	}
	subtasks := task.GetObjectArray("subtasks")
	for _, subtask := range subtasks {
		err := generateReviewedTaskTree(subtask, buf, prefix+"\t")
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *planRequest) generateCreateSubtaskPlan(extraPrompt string, rsp *PlanResponse) (*PlanResponse, error) {
	tmpl, err := template.New("partial-replan").Parse(planReviewCreateSubtaskPrompts)
	if err != nil {
		return nil, utils.Errorf("error parsing plan review prompt: %v", err)
	}
	nonce := utils.RandStringBytes(6)
	params := map[string]any{
		"Memory":      p.config.memory,
		"CurrentPlan": rsp.RootTask,
		"ExtraPrompt": extraPrompt,
		"NONCE":       nonce,
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, params)
	if err != nil {
		return nil, err
	}
	err = p.config.callAiTransaction(buf.String(), p.CallAI, func(response *aicommon.AIResponse) error {
		reader := response.GetOutputStreamReader("create-subtasks", false, p.config.GetEmitter())
		if reader == nil {
			return utils.Error("get output stream failed")
		}
		raw, err := io.ReadAll(reader)
		if err != nil && len(raw) <= 0 {
			return utils.Errorf("read create-subtask stream failed: %v", err)
		}
		action, err := aicommon.ExtractAction(string(raw), "plan-create-subtask")
		if err != nil {
			return utils.Errorf("extract create-subtask action failed: %v", err)
		}
		count := 0
		for _, subtask := range action.GetInvokeParamsArray("subtasks") {
			count++
			parentIdx := subtask.GetString("parent_index")
			name, goal := subtask.GetString("name"), subtask.GetString("goal")
			log.Infof("create subtask for: %v, title: %v goal: %v", parentIdx, name, goal)
			rsp.MergeSubtask(parentIdx, name, goal)
		}
		if count <= 1 {
			return utils.Errorf("create subtask failed, no subtask found (<=1)")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	rsp.RootTask.GenerateIndex()
	return rsp, nil
}

func (p *planRequest) freedomReviewGenerateNewPlan(extraPrompt string, rsp *PlanResponse) (*PlanResponse, error) {
	// 生成新的计划，使用自由审查模式
	tmpl, err := template.New("freedom-plan-review").Parse(planFreedomReviewPrompts)
	if err != nil {
		return nil, utils.Errorf("error parsing freedom plan review prompt: %v", err)
	}

	nonce := utils.RandStringBytes(6)
	data := map[string]any{
		"Memory":           p.config.memory,
		"CurrentPlan":      rsp.RootTask,
		"USER_REVIEW_PLAN": extraPrompt,
		"NONCE":            nonce,
	}

	var planPrompt bytes.Buffer
	err = tmpl.Execute(&planPrompt, data)
	if err != nil {
		return nil, utils.Errorf("error executing freedom plan review prompt: %v", err)
	}

	p.config.EmitInfo("freedom plan review prompt: %s", planPrompt.String())
	prompt := planPrompt.String()

	var task *AiTask
	err = p.config.callAiTransaction(prompt, p.CallAI, func(response *aicommon.AIResponse) error {
		responseReader := response.GetOutputStreamReader("freedom-plan-review", false, p.config.Emitter)
		taskResponse, err := io.ReadAll(responseReader)
		if err != nil {
			return utils.Errorf("error reading AI response: %v", err)
		}

		task, err = ExtractTaskFromRawResponse(p.config, string(taskResponse))
		if err != nil {
			return utils.Errorf("error extracting task from raw response: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, utils.Error(err.Error())
	}
	return p.config.newPlanResponse(task), nil
}

// generateNewPlan 生成新的计划
func (p *planRequest) generateNewPlan(suggestion string, extraPrompt string, rsp *PlanResponse) (*PlanResponse, error) {
	tmpl, err := template.New("plan-review").Parse(planReviewPrompts)
	if err != nil {
		return nil, utils.Errorf("error parsing plan review prompt: %v", err)
	}

	nonce := utils.RandStringBytes(6)
	data := map[string]any{
		"Memory":         p.config.memory,
		"CurrentPlan":    rsp.RootTask,
		"UserSuggestion": suggestion,
		"ExtraPrompt":    extraPrompt,
		"NONCE":          nonce,
	}

	var planPrompt bytes.Buffer
	err = tmpl.Execute(&planPrompt, data)
	if err != nil {
		return nil, utils.Errorf("error executing plan review prompt: %v", err)
	}

	p.config.EmitInfo("re-plan review prompt: %s", planPrompt.String())
	// 调用 AI 生成新的任务计划
	prompt := planPrompt.String()

	var task *AiTask
	err = p.config.callAiTransaction(prompt, p.CallAI, func(response *aicommon.AIResponse) error {
		// 读取 AI 的响应
		responseReader := response.GetOutputStreamReader("dynamic-plan", false, p.config.Emitter)
		taskResponse, err := io.ReadAll(responseReader)
		if err != nil {
			return utils.Errorf("error reading AI response: %v", err)
		}

		task, err = ExtractTaskFromRawResponse(p.config, string(taskResponse))
		if err != nil {
			return utils.Errorf("error extracting task from raw response: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, utils.Error(err.Error())
	}
	//request := NewAIRequest(planPrompt.String())
	//response, err := p.callAI(request)
	//if err != nil {
	//	return nil, utils.Errorf("error calling AI: %v", err)
	//}
	return p.config.newPlanResponse(task), nil
}

// mergePlans 合并新旧计划
func (p *planRequest) mergePlans(newPlan *planRequest) {
	// 合并新的计划内容
	p.rawInput = newPlan.rawInput
}

// adjustTaskDifficulty 调整任务难度
func (p *planRequest) adjustTaskDifficulty(rsp *PlanResponse) error {
	// 从当前计划中提取任务
	task, err := ExtractTaskFromRawResponse(p.config, p.rawInput)
	if err != nil {
		return utils.Errorf("error extracting task: %v", err)
	}

	// 调整未完成任务的难度
	for _, subtask := range task.Subtasks {
		subtask.Goal = p.simplifyTaskGoal(subtask.Goal)
	}

	// 更新计划内容
	taskBytes, err := json.Marshal(task)
	if err != nil {
		return utils.Errorf("error marshaling task: %v", err)
	}
	p.rawInput = string(taskBytes)
	return nil
}

// simplifyTaskGoal 简化任务目标
func (p *planRequest) simplifyTaskGoal(goal string) string {
	// 这里可以实现具体的任务简化逻辑
	// 例如：移除复杂条件、减少依赖等
	return goal
}
