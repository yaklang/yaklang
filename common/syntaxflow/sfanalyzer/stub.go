//go:build no_syntaxflow
// +build no_syntaxflow

package sfanalyzer

import "github.com/yaklang/yaklang/common/yakgrpc/ypb"

// Stub when SyntaxFlow support is excluded

const (
	Error   = "error"
	Warning = "warning"
	Info    = "info"
)

type SyntaxFlowAnalyzer struct {
}

type SyntaxFlowRuleProblem struct {
	Type        string
	Severity    string
	Description string
	Suggestion  string
	Range       *ypb.Range
}

type SyntaxFlowRuleAnalyzeResult struct {
	Score    int
	MaxScore int
	Problems []SyntaxFlowRuleProblem
}

func NewSyntaxFlowAnalyzer(ruleContent string) *SyntaxFlowAnalyzer {
	return &SyntaxFlowAnalyzer{}
}

func (s *SyntaxFlowAnalyzer) Analyze() *SyntaxFlowRuleAnalyzeResult {
	return &SyntaxFlowRuleAnalyzeResult{
		Score:    0,
		MaxScore: 100,
		Problems: []SyntaxFlowRuleProblem{},
	}
}

func (s *SyntaxFlowRuleAnalyzeResult) GetResponse() *ypb.SmokingEvaluatePluginResponse {
	return nil
}

func GetGrade(score int) string {
	return "F"
}

func GetGradeDescription(grade string) string {
	return ""
}
