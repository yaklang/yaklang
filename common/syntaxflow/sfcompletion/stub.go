//go:build no_syntaxflow
// +build no_syntaxflow

package sfcompletion

import "github.com/yaklang/yaklang/common/ai/aispec"

// Stub package when SyntaxFlow support is excluded
// SyntaxFlow 支持被排除时的桩包

func CompleteRuleDesc(fileName, ruleContent string, aiConfig ...aispec.AIConfigOption) (string, error) {
	return ruleContent, nil
}

func CompleteTestCases(fileName, ruleContent string, aiConfig ...aispec.AIConfigOption) (string, error) {
	return "", nil
}
