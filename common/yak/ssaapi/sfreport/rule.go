package sfreport

import (
	"slices"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type Rule struct {
	RuleName string             `json:"rule_name"`
	Language ssaconfig.Language `json:"language"`

	Title       string `json:"title"`
	TitleZh     string `json:"title_zh"`
	Description string `json:"description"`
	Solution    string `json:"solution"`
	Severity    string `json:"severity"`

	Content string `json:"content"`

	Risks []string `json:"risks"` // risk hash list
}

func NewRule(rule *schema.SyntaxFlowRule) *Rule {
	return &Rule{
		RuleName:    rule.RuleName,
		Language:    rule.Language,
		Title:       rule.Title,
		TitleZh:     rule.TitleZh,
		Severity:    string(rule.Severity),
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
