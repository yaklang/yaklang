package engine

import (
	"fmt"

	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type CompiledRulePlan struct {
	Index     uint64
	Rule      *rule.Rule
	Matcher   *match.Matcher
	Flow      *rule.FlowRule
	Options   []rule.StateOption
	Threshold *rule.ThresholdingConfig
	NoAlert   bool
}

func (p *CompiledRulePlan) streamOverlap() int {
	maxLength := 0
	if p.Rule == nil || p.Rule.ContentRuleConfig == nil {
		return 0
	}
	for _, content := range p.Rule.ContentRuleConfig.ContentRules {
		if len(content.Content) > maxLength {
			maxLength = len(content.Content)
		}
	}
	if maxLength > 0 {
		return maxLength - 1
	}
	return 0
}

func CompileStatefulRule(r *rule.Rule) (*CompiledRulePlan, error) {
	matcher, err := match.CompileStatefulRule(r)
	if err != nil {
		return nil, err
	}
	if r.Protocol != "tcp" {
		return nil, fmt.Errorf("stateful M1 supports TCP rules, got %q", r.Protocol)
	}
	plan := &CompiledRulePlan{Rule: r, Matcher: matcher}
	if r.ContentRuleConfig != nil {
		plan.Flow = r.ContentRuleConfig.Flow
		plan.Threshold = r.ContentRuleConfig.Thresholding
	}
	plan.Options = append([]rule.StateOption(nil), r.StateOptions...)
	for _, option := range plan.Options {
		if option.NoAlert || option.FlowBits != nil && option.FlowBits.Operation == rule.FlowBitsNoAlert {
			plan.NoAlert = true
		}
		if option.FlowInt != nil || option.XBits != nil || option.Metric != nil {
			return nil, fmt.Errorf("state option %s is parsed but not executable in M1", option.Keyword)
		}
	}
	return plan, nil
}
