//go:build hids

package builtin

import (
	"fmt"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	RuleSetCoverageActive   = "active"
	RuleSetCoveragePartial  = "partial"
	RuleSetCoverageInactive = "inactive"
)

type RuleCoverage struct {
	RuleID         string `json:"rule_id"`
	RuleSet        string `json:"rule_set"`
	MatchEventType string `json:"match_event_type"`
	Severity       string `json:"severity,omitempty"`
	Title          string `json:"title,omitempty"`
}

type InactiveRuleCoverage struct {
	RuleCoverage
	Reason string `json:"reason"`
}

type RuleSetCoverage struct {
	RuleSet       string                 `json:"rule_set"`
	Status        string                 `json:"status"`
	ActiveRules   []RuleCoverage         `json:"active_rules,omitempty"`
	InactiveRules []InactiveRuleCoverage `json:"inactive_rules,omitempty"`
}

func DescribeCoverage(
	ruleSets []string,
	canEmitEvent func(string) bool,
) ([]RuleSetCoverage, error) {
	if len(ruleSets) == 0 {
		return nil, nil
	}

	coverage := make([]RuleSetCoverage, 0, len(ruleSets))
	for index, ruleSet := range ruleSets {
		rules, ok := loadRuleSet(ruleSet)
		if !ok {
			return nil, &model.ValidationError{
				Field:  fmt.Sprintf("builtin_rule_sets[%d]", index),
				Reason: fmt.Sprintf("unknown builtin rule set %q", ruleSet),
			}
		}

		item := RuleSetCoverage{
			RuleSet: ruleSet,
			Status:  RuleSetCoverageActive,
		}
		for _, rule := range rules {
			ruleCoverage := describeRule(rule)
			if rule.MatchEventType == "" || canEmitEvent == nil || canEmitEvent(rule.MatchEventType) {
				item.ActiveRules = append(item.ActiveRules, ruleCoverage)
				continue
			}
			item.InactiveRules = append(item.InactiveRules, InactiveRuleCoverage{
				RuleCoverage: ruleCoverage,
				Reason:       fmt.Sprintf("enabled collectors cannot produce %s events", rule.MatchEventType),
			})
		}

		switch {
		case len(item.InactiveRules) == 0:
			item.Status = RuleSetCoverageActive
		case len(item.ActiveRules) == 0:
			item.Status = RuleSetCoverageInactive
		default:
			item.Status = RuleSetCoveragePartial
		}
		coverage = append(coverage, item)
	}
	return coverage, nil
}

func describeRule(rule Rule) RuleCoverage {
	return RuleCoverage{
		RuleID:         rule.ID,
		RuleSet:        rule.RuleSet,
		MatchEventType: rule.MatchEventType,
		Severity:       rule.Severity,
		Title:          rule.Title,
	}
}
