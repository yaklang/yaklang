package ssaconfig

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowRuleConfig struct {
	RuleNames  []string                  `json:"rule_names"`
	RuleInput  *ypb.SyntaxFlowRuleInput  `json:"rule_input"`
	RuleFilter *ypb.SyntaxFlowRuleFilter `json:"rule_filter"`
}

// --- 规则配置 Get 方法 ---

// GetRuleFilter 获取规则过滤器
func (c *Config) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	if c == nil || c.Mode&ModeSyntaxFlowRule == 0 || c.SyntaxFlowRule == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleFilter
}

// SetRuleFilter 设置规则过滤器
func (c *Config) SetRuleFilter(filter *ypb.SyntaxFlowRuleFilter) {
	if c == nil {
		return
	}
	if c.Mode&ModeSyntaxFlowRule == 0 {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = defaultSyntaxFlowRuleConfig()
	}
	c.SyntaxFlowRule.RuleFilter = filter
}

func (c *Config) GetRuleNames() []string {
	if c == nil || c.Mode&ModeSyntaxFlowRule == 0 || c.SyntaxFlowRule == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleNames
}

func (c *Config) SetRuleNames(names []string) {
	if c == nil {
		return
	}
	if c.Mode&ModeSyntaxFlowRule == 0 {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = &SyntaxFlowRuleConfig{}
	}
	c.SyntaxFlowRule.RuleNames = names
}

func (c *Config) GetRuleInput() *ypb.SyntaxFlowRuleInput {
	if c == nil || c.Mode&ModeSyntaxFlowRule == 0 || c.SyntaxFlowRule == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleInput
}

func (c *Config) SetRuleInput(input *ypb.SyntaxFlowRuleInput) {
	if c == nil {
		return
	}
	if c.Mode&ModeSyntaxFlowRule == 0 {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = &SyntaxFlowRuleConfig{}
	}
	c.SyntaxFlowRule.RuleInput = input
}

// --- 规则配置 Options ---

// WithRuleFilter 设置规则过滤器
func WithRuleFilter(filter *ypb.SyntaxFlowRuleFilter) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter"); err != nil {
			return err
		}
		c.SyntaxFlowRule.RuleFilter = filter
		return nil
	}
}

func WithRuleInput(input *ypb.SyntaxFlowRuleInput) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Input"); err != nil {
			return err
		}
		c.SyntaxFlowRule.RuleInput = input
		return nil
	}
}

// WithRuleFilterLanguage 设置规则过滤器语言
func WithRuleFilterLanguage(language ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Language"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Language = language
		return nil
	}
}

// WithRuleFilterSeverity 设置规则过滤器严重程度
func WithRuleFilterSeverity(severity ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Severity"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Severity = severity
		return nil
	}
}

// WithRuleFilterKind 设置规则过滤器类型
func WithRuleFilterKind(kind string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Kind"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.FilterRuleKind = kind
		return nil
	}
}

// WithRuleFilterPurpose 设置规则过滤器用途
func WithRuleFilterPurpose(purpose ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Purpose"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Purpose = purpose
		return nil
	}
}

// WithRuleFilterKeyword 设置规则过滤器关键字
func WithRuleFilterKeyword(keyword string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Keyword"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Keyword = keyword
		return nil
	}
}

// WithRuleFilterGroupNames 设置规则过滤器组名
func WithRuleFilterGroupNames(groupNames ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Group Names"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.GroupNames = groupNames
		return nil
	}
}

// WithRuleFilterRuleNames 设置规则过滤器规则名
func WithRuleFilterRuleNames(ruleNames ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Rule Names"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.RuleNames = ruleNames
		return nil
	}
}

// WithRuleFilterTag 设置规则过滤器标签
func WithRuleFilterTag(tag ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Tag"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Tag = tag
		return nil
	}
}

// WithRuleFilterIncludeLibraryRule 设置规则过滤器包含库规则
func WithRuleFilterIncludeLibraryRule(includeLibraryRule bool) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Include Library Rule"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.IncludeLibraryRule = includeLibraryRule
		return nil
	}
}
