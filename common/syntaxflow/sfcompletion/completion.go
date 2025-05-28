package sfcompletion

import "github.com/yaklang/yaklang/common/ai/aispec"

type RuleCompletion struct {
	FileUrl     string
	RuleContent string
	AIConfig    []aispec.AIConfigOption
}

func NewRuleCompletion(fileUrl, ruleContent string, aiConfig ...aispec.AIConfigOption) *RuleCompletion {
	return &RuleCompletion{
		FileUrl:     fileUrl,
		RuleContent: ruleContent,
		AIConfig:    aiConfig,
	}
}
