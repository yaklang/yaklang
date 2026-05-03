package aireact

import "strings"

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

	return false
}

func isDirectAnswerPrompt(prompt string) bool {
	if strings.Contains(prompt, "FINAL_ANSWER") {
		return true
	}

	return strings.Contains(prompt, "directly_answer") && strings.Contains(prompt, "answer_payload")
}

func isPrimaryDecisionPrompt(prompt string) bool {
	if strings.Contains(prompt, "# Background") && strings.Contains(prompt, "Current Time:") && strings.Contains(prompt, "# 工具调用系统") {
		return true
	}

	// 兼容新老两种 high-static 标签：AI_CACHE_SYSTEM_high-static（新形态）与
	// PROMPT_SECTION_high-static（老形态）任意命中即视为 primary decision prompt。
	// 关键词: AI_CACHE_SYSTEM_high-static, PROMPT_SECTION_high-static 双标签兼容
	hasHighStatic := strings.Contains(prompt, "<|AI_CACHE_SYSTEM_high-static|>") ||
		strings.Contains(prompt, "<|PROMPT_SECTION_high-static|>")
	if hasHighStatic &&
		strings.Contains(prompt, "<|PROMPT_SECTION_dynamic_") &&
		strings.Contains(prompt, "<|TRAITS|>") &&
		strings.Contains(prompt, `"require_tool"`) &&
		strings.Contains(prompt, `"tool_require_payload"`) {
		return true
	}

	return false
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
