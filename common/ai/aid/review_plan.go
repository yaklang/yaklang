package aid

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type PlanReviewSuggestion struct {
	Value             string `json:"value"`
	Suggestion        string `json:"prompt"`
	SuggestionEnglish string `json:"prompt_english"`
	AllowExtraPrompt  bool   `json:"allow_extra_prompt"`

	PromptBuilder    func(plan *planRequest, rt *runtime) `json:"-"`
	ResponseCallback func(reader io.Reader)               `json:"-"`
	ParamSchema      string                               `json:"param_schema"`
}

// PlanReviewSuggestions 是计划审查时的建议(内置一些常见选项)
var PlanReviewSuggestions = []*PlanReviewSuggestion{
	{
		Value:             "incomplete",
		Suggestion:        "计划不够完整，需要补充更多细节",
		SuggestionEnglish: "The plan is not complete enough, more details need to be added",
		AllowExtraPrompt:  true,
	},
	{
		Value:             "continue",
		Suggestion:        "计划合理，继续执行",
		SuggestionEnglish: "The plan is reasonable, continue execution",
	},
	{
		Value:             "replan",
		Suggestion:        "需要重新规划整个任务或者局部任务",
		SuggestionEnglish: "Need to replan the entire task or partial task",
	},
}

func (p *planRequest) handleReviewPlanResponse(rsp *planResponse, param aitool.InvokeParams) (*planResponse, error) {
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
	case "incomplete":
		p.config.EmitInfo("plan is incomplete")
		// 重新生成计划，但保留现有任务
		extraPrompt := param.GetString("extra_prompt", "prompt")
		newPlan, err := p.generateNewPlan(suggestion, extraPrompt, rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return nil, utils.Errorf("generate new plan failed: %v", err)
		}

		ep := p.config.epm.createEndpoint()
		ep.SetDefaultSuggestionContinue()

		p.config.EmitRequireReviewForPlan(newPlan, ep.id)
		p.config.doWaitAgree(nil, ep)
		params := ep.GetParams()
		p.config.memory.StoreInteractiveUserInput(ep.id, params)
		if params == nil {
			p.config.EmitError("user review params is nil, plan failed")
			return newPlan, nil
		}
		return p.handleReviewPlanResponse(newPlan, params)
	case "replan":
		return nil, utils.Errorf("replan is not supported yet")
	default:
		p.config.EmitError("unknown review suggestion: %s", suggestion)
		return rsp, nil
	}
}

// generateNewPlan 生成新的计划
func (p *planRequest) generateNewPlan(suggestion string, extraPrompt string, rsp *planResponse) (*planResponse, error) {
	tmpl, err := template.New("plan-review").Parse(planReviewPrompts)
	if err != nil {
		return nil, utils.Errorf("error parsing plan review prompt: %v", err)
	}

	data := map[string]any{
		"Memory":         p.config.memory,
		"CurrentPlan":    rsp.RootTask,
		"UserSuggestion": suggestion,
		"ExtraPrompt":    extraPrompt,
	}

	var planPrompt bytes.Buffer
	err = tmpl.Execute(&planPrompt, data)
	if err != nil {
		return nil, utils.Errorf("error executing plan review prompt: %v", err)
	}

	p.config.EmitInfo("re-plan review prompt: %s", planPrompt.String())
	// 调用 AI 生成新的任务计划
	request := NewAIRequest(planPrompt.String())
	response, err := p.callAI(request)
	if err != nil {
		return nil, utils.Errorf("error calling AI: %v", err)
	}

	// 读取 AI 的响应
	responseReader := response.GetOutputStreamReader("dynamic-plan", false, p.config)
	taskResponse, err := io.ReadAll(responseReader)
	if err != nil {
		return nil, utils.Errorf("error reading AI response: %v", err)
	}

	task, err := p.extractTaskFromRawResponse(string(taskResponse))
	if err != nil {
		return nil, utils.Errorf("error extracting task from raw response: %v", err)
	}
	return p.config.newPlanResponse(task), nil
}

// mergePlans 合并新旧计划
func (p *planRequest) mergePlans(newPlan *planRequest) {
	// 合并新的计划内容
	p.rawInput = newPlan.rawInput
}

// adjustTaskDifficulty 调整任务难度
func (p *planRequest) adjustTaskDifficulty(rsp *planResponse) error {
	// 从当前计划中提取任务
	task, err := p.extractTaskFromRawResponse(p.rawInput)
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
