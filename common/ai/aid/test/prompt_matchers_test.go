package test

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func isIntentEnrichmentPrompt(prompt string) bool {
	return strings.Contains(prompt, "意图识别与上下文增强系统") ||
		strings.Contains(prompt, "意图识别与上下文增强") ||
		(strings.Contains(prompt, "Intent Recognition") && strings.Contains(prompt, "Context Enrichment")) ||
		utils.MatchAllOfSubString(prompt, "query_capabilities", "finalize_enrichment", "intent_summary")
}

func isMemorySummaryPrompt(prompt string) bool {
	if isPlanGuidanceDocLiteForge(prompt) || isPlanFromDocLiteForge(prompt) {
		return false
	}
	return strings.Contains(prompt, "数据处理和总结提示小助手") ||
		strings.Contains(prompt, "tag-selection") ||
		strings.Contains(prompt, "memory-triage") ||
		utils.MatchAllOfSubString(prompt, `"const": "tag-selection"`, `"tags"`) ||
		utils.MatchAllOfSubString(prompt, `"const": "memory-triage"`, `"memory_entities"`)
}

func isPlanFactsHookPrompt(prompt string) bool {
	return strings.Contains(prompt, `"const": "plan_facts_hook"`) ||
		utils.MatchAllOfSubString(prompt, `"@action"`, `plan_facts_hook`, `"facts"`)
}

func isCapabilityCatalogMatchPrompt(prompt string) bool {
	return utils.MatchAllOfSubString(prompt, `"const": "capability-catalog-match"`, "matched_identifiers")
}

// isPlanExplorationPrompt detects the plan exploration ReAct loop prompt.
// In the new plan flow, this is the first phase where the AI explores and
// gathers facts before calling finish_exploration.
func isPlanExplorationPrompt(prompt string) bool {
	return strings.Contains(prompt, "任务规划使命") &&
		strings.Contains(prompt, "finish_exploration")
}

// isPlanGuidanceDocLiteForge detects the guidance document generation LiteForge prompt.
// This is the second phase where a guidance document is generated from collected facts.
func isPlanGuidanceDocLiteForge(prompt string) bool {
	return strings.Contains(prompt, "数据处理和总结提示小助手") &&
		strings.Contains(prompt, `"const": "plan_guidance_document"`)
}

// isPlanFromDocLiteForge detects the plan-from-document LiteForge prompt.
// This is the third phase where a structured plan is generated from the guidance document.
func isPlanFromDocLiteForge(prompt string) bool {
	return strings.Contains(prompt, "数据处理和总结提示小助手") &&
		strings.Contains(prompt, `"const": "plan_from_document"`)
}

// isPlanPrompt returns true for ANY plan-related prompt (exploration, guidance doc, or plan-from-doc).
func isPlanPrompt(prompt string) bool {
	if isPlanFactsHookPrompt(prompt) {
		return false
	}

	if isPlanExplorationPrompt(prompt) || isPlanGuidanceDocLiteForge(prompt) || isPlanFromDocLiteForge(prompt) {
		return true
	}

	if strings.Contains(prompt, "plan: when user needs to create or refine a plan for a specific task") {
		return true
	}

	if strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具") {
		return strings.Contains(prompt, "main_task_goal") &&
			strings.Contains(prompt, "subtask_goal") &&
			(strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema") || strings.Contains(prompt, "PERSISTENT_"))
	}

	return utils.MatchAllOfSubString(prompt, "main_task", "main_task_goal", "subtask_name", "subtask_goal")
}

// defaultTestPlanFromDocJSON is the default plan JSON response for plan-from-document LiteForge prompts.
var defaultTestPlanFromDocJSON = `{
    "@action": "plan_from_document",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {"subtask_name": "遍历目标目录", "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"},
        {"subtask_name": "筛选最大文件", "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"},
        {"subtask_name": "输出结果", "subtask_goal": "将最大文件的路径和大小以可读格式输出"}
    ]
}`

// isPlanFactsHookLiteForge detects the plan facts hook LiteForge prompt.
// This is called during post-iteration to extract incremental facts.
func isPlanFactsHookLiteForge(prompt string) bool {
	return strings.Contains(prompt, "数据处理和总结提示小助手") &&
		strings.Contains(prompt, `"const": "plan_facts_hook"`)
}

// tryHandleNewPlanFlowPrompt handles prompts from the new 3-phase plan generation flow:
//   - Phase 1 (exploration): respond with finish_exploration to skip exploration
//   - Phase 1.5 (facts hook): respond with empty facts
//   - Phase 2 (guidance doc): respond with a mock guidance document
//   - Phase 3 (plan from doc): respond with the planJSON parameter
//
// Returns (response, nil) if handled, (nil, nil) if not a plan flow prompt.
func tryHandleNewPlanFlowPrompt(config aicommon.AICallerConfigIf, prompt string, planJSON string) (*aicommon.AIResponse, error) {
	if isPlanExplorationPrompt(prompt) {
		rsp := config.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader(
			`{"@action": "finish_exploration", "human_readable_thought": "Ready to generate plan"}`))
		rsp.Close()
		return rsp, nil
	}

	if isPlanFactsHookLiteForge(prompt) {
		rsp := config.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader(
			`{"@action": "plan_facts_hook", "facts": ""}`))
		rsp.Close()
		return rsp, nil
	}

	if isPlanGuidanceDocLiteForge(prompt) {
		rsp := config.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader(
			`{"@action": "plan_guidance_document", "document": "Mock guidance document for testing."}`))
		rsp.Close()
		return rsp, nil
	}

	if isPlanFromDocLiteForge(prompt) {
		rsp := config.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader(planJSON))
		rsp.Close()
		return rsp, nil
	}

	return nil, nil
}

func isNextActionDecisionPrompt(prompt string) bool {
	if strings.Contains(prompt, "FINAL_ANSWER") {
		return true
	}

	if strings.Contains(prompt, "# Background") && strings.Contains(prompt, "Current Time:") && strings.Contains(prompt, "# 工具调用系统") {
		return true
	}

	return strings.Contains(prompt, "directly_answer") &&
		(strings.Contains(prompt, "require_tool") || strings.Contains(prompt, "ask_for_clarification") || strings.Contains(prompt, "answer_payload"))
}

func isToolParamGenerationPrompt(prompt, toolName string) bool {
	if strings.Contains(prompt, "Generate appropriate parameters for this tool call based on the context above") {
		return toolName == "" || strings.Contains(prompt, toolName)
	}

	if strings.Contains(prompt, "Tool Parameter Generation") || strings.Contains(prompt, "需要为 '") {
		if toolName == "" {
			return true
		}
		return strings.Contains(prompt, "'"+toolName+"'") ||
			strings.Contains(prompt, "\""+toolName+"\"") ||
			strings.Contains(prompt, "`"+toolName+"`")
	}

	if strings.Contains(prompt, "重新生成一套参数") || strings.Contains(prompt, "参数名不匹配") {
		return true
	}

	return strings.Contains(prompt, "call-tool") && strings.Contains(prompt, "params")
}

func isVerifySatisfactionPrompt(prompt string) bool {
	if strings.Contains(prompt, "verify-satisfaction") && strings.Contains(prompt, "user_satisfied") {
		return true
	}

	if !strings.Contains(prompt, "# Instructions") {
		return false
	}

	return strings.Contains(prompt, "任务策略师") ||
		(strings.Contains(prompt, "当前子任务") && strings.Contains(prompt, "completed_task_index"))
}

func isSummaryPrompt(prompt string) bool {
	if strings.Contains(prompt, "short_summary") {
		return true
	}

	return utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary")
}

func isPlanReviewLiteForgePrompt(prompt string) bool {
	return strings.Contains(prompt, "Plan Review AI") &&
		strings.Contains(prompt, `"const": "plan_review"`)
}

func isWrongToolReviewPrompt(prompt string) bool {
	return strings.Contains(prompt, "abandon") &&
		(strings.Contains(prompt, "require-tool") || strings.Contains(prompt, "require_tool"))
}

func isPerceptionPrompt(prompt string) bool {
	return strings.Contains(prompt, "感知模块") &&
		utils.MatchAllOfSubString(prompt, "summary", "topics", "keywords", "changed", "confidence")
}

func tryHandlePerceptionPrompt(config aicommon.AICallerConfigIf, prompt string) (*aicommon.AIResponse, error) {
	if !isPerceptionPrompt(prompt) {
		return nil, nil
	}
	rsp := config.NewAIResponse()
	rsp.EmitOutputStream(strings.NewReader(`{
		"@action": "perception",
		"summary": "User is testing perception functionality",
		"topics": ["perception testing", "AI loop"],
		"keywords": ["perception", "test", "loop"],
		"changed": true,
		"confidence": 0.85
	}`))
	rsp.Close()
	return rsp, nil
}

func unexpectedPromptError(prompt string) error {
	return utils.Errorf("unexpected prompt: %s", utils.ShrinkString(prompt, 400))
}
