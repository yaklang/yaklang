package aicommon

import "strings"

// This file provides shared prompt-matcher helpers used by test mock AI
// callbacks across packages (common/ai/aid/test, common/ai/aid/aireact, and
// reactloopstests) to classify which role-prompt the AI received so the mock
// can return an appropriate canned response.
//
// Design contract: each schema keyword literal (e.g. "directly_answer" /
// "require_tool" / "request_plan_and_execution" / "tool_compose" /
// "require_ai_blueprint" / "verify-satisfaction" / etc.) only appears in the
// schema block of the role that actually exposes that enum. Static system-level
// prose must never contain concrete action literals (snake_case or
// kebab-case); use semantic Chinese placeholders instead.
//
// When static prompts are edited, the散文 (prose) MUST NOT contain any concrete
// action literal. See common/ai/aid/aicache/LESSONS_LEARNED.md §6.

// --- 参数生成场景识别 (R2 / R3 / R5) ---

// IsToolParamGenerationPrompt detects a parameter-generation prompt (R2/R3/R5).
// Generic entry; does not distinguish tool vs blueprint vs regeneration.
func IsToolParamGenerationPrompt(prompt, toolName string) bool {
	return IsToolParamGenPrompt(prompt) && (toolName == "" || strings.Contains(prompt, toolName))
}

// IsToolParamGenPrompt detects a parameter-generation prompt (R2/R3/R5)
// without checking the tool name.
func IsToolParamGenPrompt(prompt string) bool {
	// The R2/R3/R5 dynamic section (tool-params/dynamic.txt) emits the
	// heading "# Parameter Generation Task" — this exact line never appears
	// in the R1 instruction or output example, making it the most reliable
	// discriminator between R1 (decision) and R2/R3/R5 (param generation).
	if strings.Contains(prompt, "# Parameter Generation Task") {
		return true
	}
	// Old standalone path: tool-params/instruction.txt (used when R2 does
	// not reuse the R1 instruction). These phrases only appear in that file.
	if strings.Contains(prompt, "Generate appropriate parameters based on the context above and the schema") {
		return true
	}
	return false
}

// IsToolParamGenPromptForTool detects a *tool* (non-blueprint) parameter
// generation prompt.
func IsToolParamGenPromptForTool(prompt, toolName string) bool {
	if !IsToolParamGenPrompt(prompt) {
		return false
	}
	// Blueprint param gen uses "You need to generate parameters for the AI Blueprint".
	if strings.Contains(prompt, "You need to generate parameters for the AI Blueprint") {
		return false
	}
	return toolName == "" || strings.Contains(prompt, toolName)
}

// IsToolParamGenPromptForBlueprint detects a *blueprint* parameter generation
// prompt.
func IsToolParamGenPromptForBlueprint(prompt, forgeName string) bool {
	// New path: dynamic section has "You need to generate parameters for the AI Blueprint".
	if strings.Contains(prompt, "You need to generate parameters for the AI Blueprint") {
		return forgeName == "" || strings.Contains(prompt, forgeName)
	}
	// Old fallback: "Blueprint Schema:" + "Blueprint Description:".
	if strings.Contains(prompt, "Blueprint Schema:") && strings.Contains(prompt, "Blueprint Description:") {
		return forgeName == "" || strings.Contains(prompt, forgeName)
	}
	return false
}

// IsChangeBlueprintPrompt detects the "change-ai-blueprint" prompt (R6).
// This prompt has a unique heading "# Change AI Blueprint Task" and the
// "change-ai-blueprint" action const that never appear in R1.
func IsChangeBlueprintPrompt(prompt string) bool {
	return strings.Contains(prompt, "# Change AI Blueprint Task") ||
		strings.Contains(prompt, "change-ai-blueprint")
}

// IsToolParamGenPromptWithOldParams detects a *regeneration* prompt (has old
// params).
func IsToolParamGenPromptWithOldParams(prompt string) bool {
	// The dynamic section wraps old params with a unique tag that never
	// appears in the R1 instruction or output example:
	return strings.Contains(prompt, "Parameter Regeneration Task")
}

// IsToolParamGenPromptForToolWithOldParams detects tool-parameter regeneration.
func IsToolParamGenPromptForToolWithOldParams(prompt, toolName string) bool {
	return IsToolParamGenPromptForTool(prompt, toolName) && IsToolParamGenPromptWithOldParams(prompt)
}

// IsToolParamGenPromptForBlueprintWithOldParams detects blueprint-parameter
// regeneration.
func IsToolParamGenPromptForBlueprintWithOldParams(prompt, forgeName string) bool {
	return IsToolParamGenPromptForBlueprint(prompt, forgeName) && IsToolParamGenPromptWithOldParams(prompt)
}

// IsIntentEnrichmentPrompt detects the intent enrichment loop prompt.
// The intent loop has a unique persistent-instruction heading that never
// appears in the main loop. We must NOT match on action names like
// "finalize_enrichment" or "query_capabilities" alone, because those
// strings can leak into the main loop via timeline or EXTRA_CAPABILITIES.
func IsIntentEnrichmentPrompt(prompt string) bool {
	return strings.Contains(prompt, "意图识别与上下文增强系统") ||
		strings.Contains(prompt, "意图识别与上下文增强") ||
		(strings.Contains(prompt, "Intent Recognition") && strings.Contains(prompt, "Context Enrichment"))
}

// --- 其他角色识别 ---

// IsDirectAnswerPrompt detects the directly_answer prompt.
func IsDirectAnswerPrompt(prompt string) bool {
	if strings.Contains(prompt, "FINAL_ANSWER") {
		return true
	}
	return strings.Contains(prompt, "directly_answer") && strings.Contains(prompt, "answer_payload")
}

// IsPrimaryDecisionPrompt detects the main-loop decision prompt (R1).
// R2 reuses the R1 instruction, so we must exclude R2 first.
func IsPrimaryDecisionPrompt(prompt string) bool {
	if IsToolParamGenPrompt(prompt) {
		return false
	}
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

// IsVerifySatisfactionPrompt detects the verify-satisfaction prompt.
func IsVerifySatisfactionPrompt(prompt string) bool {
	if strings.Contains(prompt, "verify-satisfaction") && strings.Contains(prompt, "user_satisfied") {
		return true
	}
	if !strings.Contains(prompt, "# Instructions") {
		return false
	}
	return strings.Contains(prompt, "任务策略师") ||
		(strings.Contains(prompt, "当前子任务") && strings.Contains(prompt, "completed_task_index"))
}

// IsToolCallReasonLiteForgePrompt detects the LiteForge prompt (forge action
// name "tool-call-reason").
func IsToolCallReasonLiteForgePrompt(prompt string) bool {
	return strings.Contains(prompt, `"tool-call-reason"`)
}

// MockedToolCallReasonActionJSON is a canned valid action for the
// tool-call-reason lite forge so the forge finishes immediately.
const MockedToolCallReasonActionJSON = `{"@action": "tool-call-reason", "reason": "mocked tool-call reason"}`
