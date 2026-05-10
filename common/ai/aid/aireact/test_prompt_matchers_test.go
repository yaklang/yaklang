package aireact

// 本文件是 aireact 包内 mock AI 回调里"按 prompt 类型分流"的判定函数集.
// 与 common/ai/aid/test/prompt_matchers_test.go 共享同一套契约假设:
// 每个 schema 关键字面量 (`directly_answer` / `require_tool` /
// `tool_compose` / `request_plan_and_execution` / `finish_exploration`
// 等) **只**出现在该轮真正暴露该 enum 的 schema 块里, 不出现在被每轮
// 共享的 high-static / base 等"系统级静态段"散文里. 一旦在静态段散文里
// 写出这些字面量, mock 分流会全线错位.
//
// 修改静态系统级 prompt 时, **散文里禁止出现任何具体动作字面**, 无论
// snake_case 还是 kebab-case. 一律改用中文语义类别指代. 详见
// common/ai/aid/aicache/LESSONS_LEARNED.md 第 6 节 "反例与教训" 以及
// common/ai/aid/aireact/prompts/loop/README.md "修改 high-static 段时的
// 硬约束" 第 2 条.
//
// 历史教训 (2026-05): wrong-tool re-select 的 schema enum 字面就是
// kebab-case (`require-tool` / `abandon`), 散文里写 `require-tool` 会让
// fallback 匹配 `MatchAllOfSubString(prompt, "require-tool", "abandon")`
// 在 timeline 含 abandon 字样时误命中.
//
// 关键词: prompt-mock 分流, high-static 散文污染, schema 字面量解耦,
//        kebab-case 同样不安全

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
