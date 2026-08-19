package ssaconfig

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowRuleConfig struct {
	RuleNames  []string                   `json:"rule_names"`
	RuleInput  []*ypb.SyntaxFlowRuleInput `json:"rule_input"`
	RuleFilter *ypb.SyntaxFlowRuleFilter  `json:"rule_filter"`
	// TaskLocal marks an immutable, dispatch-scoped rule input. It is consumed
	// only by syntaxflow_scan and keeps ordinary inline/debug rule behavior
	// backward compatible.
	TaskLocal            bool   `json:"task_local,omitempty"`
	TaskLocalInputFile   string `json:"task_local_input_file,omitempty"`
	TaskLocalInputSHA256 string `json:"task_local_input_sha256,omitempty"`
	TaskLocalInputCount  int    `json:"task_local_input_count,omitempty"`
}

const TaskLocalRuleInputFileVersionV1 = "syntaxflow_rule_input.v1"

type TaskLocalRuleInputFile struct {
	Version  string                           `json:"version"`
	Rules    []*ypb.SyntaxFlowRuleInput       `json:"rules"`
	Metadata map[string]TaskLocalRuleMetadata `json:"metadata"`
}

type TaskLocalRuleMetadata struct {
	AssetID       string          `json:"asset_id"`
	SourceRuleID  string          `json:"source_rule_id,omitempty"`
	Title         string          `json:"title,omitempty"`
	TitleZh       string          `json:"title_zh,omitempty"`
	Language      string          `json:"language,omitempty"`
	Purpose       string          `json:"purpose,omitempty"`
	Tag           string          `json:"tag,omitempty"`
	CWE           []string        `json:"cwe,omitempty"`
	CVE           string          `json:"cve,omitempty"`
	RiskType      string          `json:"risk_type,omitempty"`
	Type          string          `json:"type,omitempty"`
	Severity      string          `json:"severity,omitempty"`
	Description   string          `json:"description,omitempty"`
	Solution      string          `json:"solution,omitempty"`
	Version       string          `json:"version,omitempty"`
	ContentHash   string          `json:"content_hash,omitempty"`
	IsBuiltin     bool            `json:"is_builtin"`
	Verified      bool            `json:"verified"`
	AllowIncluded bool            `json:"allow_included"`
	IncludedName  string          `json:"included_name,omitempty"`
	Groups        []string        `json:"groups,omitempty"`
	AlertDesc     json.RawMessage `json:"alert_desc,omitempty"`
}

func (c *Config) IsTaskLocalRuleInput() bool {
	return c != nil && c.Mode&ModeSyntaxFlowRule != 0 && c.SyntaxFlowRule != nil && c.SyntaxFlowRule.TaskLocal
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

// SetRuleGroups 设置规则组（便捷方法）
func (c *Config) SetRuleGroups(groupNames ...string) {
	if c == nil {
		return
	}
	if c.Mode&ModeSyntaxFlowRule == 0 {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = defaultSyntaxFlowRuleConfig()
	}
	if c.SyntaxFlowRule.RuleFilter == nil {
		c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
	}
	c.SyntaxFlowRule.RuleFilter.GroupNames = groupNames
}

// GetRuleGroups 获取规则组（便捷方法）
func (c *Config) GetRuleGroups() []string {
	if c == nil || c.Mode&ModeSyntaxFlowRule == 0 || c.SyntaxFlowRule == nil {
		return nil
	}
	if c.SyntaxFlowRule.RuleFilter == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleFilter.GroupNames
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

func (c *Config) GetRuleInput() []*ypb.SyntaxFlowRuleInput {
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
	c.SyntaxFlowRule.RuleInput = append(c.SyntaxFlowRule.RuleInput, input)
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

func WithRuleInputRaw(raw string) Option {
	return WithRuleInput(&ypb.SyntaxFlowRuleInput{Content: raw})
}

func WithRuleInput(input *ypb.SyntaxFlowRuleInput) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Input"); err != nil {
			return err
		}
		c.SyntaxFlowRule.RuleInput = append(c.SyntaxFlowRule.RuleInput, input)
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
// Deprecated: 此字段已废弃，请使用 WithRuleFilterLibRuleKind
func WithRuleFilterIncludeLibraryRule(includeLibraryRule bool) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Include Library Rule"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.IncludeLibraryRule = includeLibraryRule
		// 同时设置 FilterLibRuleKind 以确保兼容性
		if !includeLibraryRule {
			c.SyntaxFlowRule.RuleFilter.FilterLibRuleKind = "noLib"
		} else {
			c.SyntaxFlowRule.RuleFilter.FilterLibRuleKind = "lib"
		}
		return nil
	}
}

// WithRuleFilterLibRuleKind 设置规则过滤器库规则类型
// kind: "lib" 只包含库规则, "noLib" 不包含库规则, "" 所有规则
func WithRuleFilterLibRuleKind(kind string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowRule("Rule Filter Lib Rule Kind"); err != nil {
			return err
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.FilterLibRuleKind = kind
		return nil
	}
}
