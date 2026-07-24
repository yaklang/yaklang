package aireact

// 本文件是 aireact 包内 mock AI 回调里"按 prompt 类型分流"的判定函数集.
// 所有判定逻辑已统一迁移到 common/ai/aid/aicommon/prompt_matchers.go
// (导出为 aicommon.IsPrimaryDecisionPrompt / aicommon.IsToolParamGen* 等),
// 这里保留薄包装以维持本包测试文件调用风格不变.
//
// 修改静态系统级 prompt 时, **散文里禁止出现任何具体动作字面**, 无论
// snake_case 还是 kebab-case. 一律改用中文语义类别指代.
//
// 关键词: prompt-mock 分流, high-static 散文污染, schema 字面量解耦

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// --- 参数生成场景识别 ---

func isToolParamGenerationPrompt(prompt, toolName string) bool {
	return aicommon.IsToolParamGenerationPrompt(prompt, toolName)
}

func isToolParamGenPrompt(prompt string) bool {
	return aicommon.IsToolParamGenPrompt(prompt)
}

func isToolParamGenPromptForTool(prompt, toolName string) bool {
	return aicommon.IsToolParamGenPromptForTool(prompt, toolName)
}

func isToolParamGenPromptForBlueprint(prompt, forgeName string) bool {
	return aicommon.IsToolParamGenPromptForBlueprint(prompt, forgeName)
}

func isToolParamGenPromptWithOldParams(prompt string) bool {
	return aicommon.IsToolParamGenPromptWithOldParams(prompt)
}

func isToolParamGenPromptForToolWithOldParams(prompt, toolName string) bool {
	return aicommon.IsToolParamGenPromptForToolWithOldParams(prompt, toolName)
}

func isToolParamGenPromptForBlueprintWithOldParams(prompt, forgeName string) bool {
	return aicommon.IsToolParamGenPromptForBlueprintWithOldParams(prompt, forgeName)
}

func isChangeBlueprintPrompt(prompt string) bool {
	return aicommon.IsChangeBlueprintPrompt(prompt)
}

// --- 其他角色识别 ---

func isIntentEnrichmentPrompt(prompt string) bool {
	return aicommon.IsIntentEnrichmentPrompt(prompt)
}

func isDirectAnswerPrompt(prompt string) bool {
	return aicommon.IsDirectAnswerPrompt(prompt)
}

// isPrimaryDecisionPrompt 检测主循环决策 prompt (R1).
// R2 复用 R1 instruction, 需排除 R2.
func isPrimaryDecisionPrompt(prompt string) bool {
	return aicommon.IsPrimaryDecisionPrompt(prompt)
}

func isVerifySatisfactionPrompt(prompt string) bool {
	return aicommon.IsVerifySatisfactionPrompt(prompt)
}

func isToolCallReasonLiteForgePrompt(prompt string) bool {
	return aicommon.IsToolCallReasonLiteForgePrompt(prompt)
}

// ensure strings is referenced even if no direct call remains after refactor
var _ = strings.Contains

const mockedToolCallReasonActionJSON = aicommon.MockedToolCallReasonActionJSON
