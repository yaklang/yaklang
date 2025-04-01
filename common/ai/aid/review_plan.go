package aid

import (
	"encoding/json"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type PlanReviewSuggestion struct {
	Value             string `json:"value"`
	Suggestion        string `json:"prompt"`
	SuggestionEnglish string `json:"prompt_english"`

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
	},
	{
		Value:             "unrealistic",
		Suggestion:        "计划不够现实，需要调整任务难度和范围",
		SuggestionEnglish: "The plan is not realistic enough, task difficulty and scope need to be adjusted",
	},
	{
		Value:             "continue",
		Suggestion:        "计划合理，继续执行",
		SuggestionEnglish: "The plan is reasonable, continue execution",
	},
	{
		Value:             "replan",
		Suggestion:        "需要重新规划整个任务",
		SuggestionEnglish: "Need to replan the entire task",
	},
}

func (p *planRequest) handleReviewPlanResponse(rsp *planResponse, param aitool.InvokeParams) error {
	// 1. 获取审查建议
	suggestion := param.GetString("suggestion")
	if suggestion == "" {
		return utils.Error("suggestion is empty")
	}

	// 2. 根据审查建议处理
	switch suggestion {
	case "incomplete":
		p.config.EmitInfo("plan is incomplete")
		// 重新生成计划，但保留现有任务
		newPlan, err := p.generateNewPlan(rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return utils.Errorf("generate new plan failed: %v", err)
		}
		// 合并新旧计划
		p.mergePlans(newPlan)

	case "unrealistic":
		p.config.EmitInfo("plan is unrealistic")
		// 调整现有任务的难度和范围
		if err := p.adjustTaskDifficulty(rsp); err != nil {
			p.config.EmitError("adjust task difficulty failed: %v", err)
			return utils.Errorf("adjust task difficulty failed: %v", err)
		}

	case "continue":
		p.config.EmitInfo("plan is reasonable, continue")
		// 继续执行现有计划
		return nil

	case "replan":
		p.config.EmitInfo("replanning entire task")
		// 完全重新规划
		newPlan, err := p.generateNewPlan(rsp)
		if err != nil {
			p.config.EmitError("generate new plan failed: %v", err)
			return utils.Errorf("generate new plan failed: %v", err)
		}
		// 替换现有计划
		p.rawInput = newPlan.rawInput

	default:
		p.config.EmitError("unknown review suggestion: %s", suggestion)
		return utils.Errorf("unknown review suggestion: %s", suggestion)
	}
	return nil
}

// generateNewPlan 生成新的计划
func (p *planRequest) generateNewPlan(rsp *planResponse) (*planRequest, error) {
	planPrompt, err := p.GeneratePrompt()
	if err != nil {
		return nil, utils.Errorf("error generating dynamic plan prompt: %v", err)
	}

	// 调用 AI 生成新的任务计划
	request := NewAIRequest(planPrompt)
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

	// 解析响应并创建新计划
	newPlan := &planRequest{
		config:   p.config,
		rawInput: p.rawInput,
	}

	// 更新计划
	newPlan.rawInput = string(taskResponse)
	return newPlan, nil
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
