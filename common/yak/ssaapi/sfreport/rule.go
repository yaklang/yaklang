package sfreport

import (
	"slices"

	"github.com/yaklang/yaklang/common/schema"
)

type Rule struct {
	RuleName string `json:"rule_name"`
	Language string `json:"language"`

	Description string `json:"description"`
	Solution    string `json:"solution"`

	Content string `json:"content"`

	Risks []string `json:"risks"` // risk hash list
}

func NewRule(rule *schema.SyntaxFlowRule) *Rule {
	return &Rule{
		RuleName:    rule.RuleName,
		Language:    rule.Language,
		Description: rule.Description,
		Solution:    rule.Solution,
		Content:     rule.Content,
	}
}

func (r *Rule) AddRisk(risk *Risk) {
	if slices.Contains(r.Risks, risk.GetHash()) {
		return
	}
	r.Risks = append(r.Risks, risk.GetHash())
}
