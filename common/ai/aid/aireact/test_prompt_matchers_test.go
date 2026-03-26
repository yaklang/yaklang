package aireact

import "strings"

func isToolParamGenerationPrompt(prompt, toolName string) bool {
	if strings.Contains(prompt, "You need to generate parameters for the tool") {
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
