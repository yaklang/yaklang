package test

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func isIntentEnrichmentPrompt(prompt string) bool {
	return strings.Contains(prompt, "意图识别与上下文增强系统") ||
		strings.Contains(prompt, "意图识别与上下文增强") ||
		(strings.Contains(prompt, "Intent Recognition") && strings.Contains(prompt, "Context Enrichment")) ||
		utils.MatchAllOfSubString(prompt, "query_capabilities", "finalize_enrichment", "intent_summary")
}

func isMemorySummaryPrompt(prompt string) bool {
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

func isPlanPrompt(prompt string) bool {
	if isPlanFactsHookPrompt(prompt) {
		return false
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

func isWrongToolReviewPrompt(prompt string) bool {
	return strings.Contains(prompt, "abandon") &&
		(strings.Contains(prompt, "require-tool") || strings.Contains(prompt, "require_tool"))
}

func unexpectedPromptError(prompt string) error {
	return utils.Errorf("unexpected prompt: %s", utils.ShrinkString(prompt, 400))
}
