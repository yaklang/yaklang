package ssaconfig

import (
	_ "embed"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v3"
)

//go:embed scan_policies.yaml
var scanPoliciesYAML []byte

// PolicyDefinition 策略定义（从YAML加载）
type PolicyDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Icon        string   `yaml:"icon"`
	// RuleGroups deprecated — prefer Tags / Severity / Purpose
	RuleGroups []string `yaml:"rule_groups"`
	Tags       []string `yaml:"tags"`
	Severity   []string `yaml:"severity"`
	Purpose    []string `yaml:"purpose"`
}

// ScanPoliciesConfig 策略配置文件结构
type ScanPoliciesConfig struct {
	Version         string                      `yaml:"version"`
	Policies        map[string]PolicyDefinition `yaml:"policies"`
	Categories      []PolicyCategory            `yaml:"categories"`
	CustomRuleTags  map[string][]RuleTagOption  `yaml:"custom_rule_tags"`
	// CustomRuleGroups kept for old YAML; unused in v2
	CustomRuleGroups CustomRuleGroupsConfig `yaml:"custom_rule_groups"`
}

// PolicyCategory 策略分类
type PolicyCategory struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Policies []string `yaml:"policies" json:"policies"`
}

// RuleTagOption is a selectable standard tag for custom policy UI.
type RuleTagOption struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"display_name" json:"display_name"`
}

// CustomRuleGroupsConfig 自定义规则组配置（deprecated）
type CustomRuleGroupsConfig struct {
	ComplianceRules []RuleGroupCategory `yaml:"compliance_rules" json:"compliance_rules"`
	TechStackRules  []RuleGroupCategory `yaml:"tech_stack_rules" json:"tech_stack_rules"`
	SpecialRules    []RuleGroupCategory `yaml:"special_rules" json:"special_rules"`
}

// RuleGroupCategory 规则组分类（deprecated）
type RuleGroupCategory struct {
	Category string      `yaml:"category" json:"category"`
	Groups   []RuleGroup `yaml:"groups" json:"groups"`
}

// RuleGroup 规则组（deprecated）
type RuleGroup struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"display_name" json:"display_name"`
}

// ScanPolicyConfig 扫描策略配置
type ScanPolicyConfig struct {
	PolicyType  string             `json:"policy_type"`
	CustomRules *CustomRulesConfig `json:"custom_rules"`
}

// CustomRulesConfig 自定义策略条件（Tag + 列字段）
type CustomRulesConfig struct {
	Tags     []string `json:"tags"`
	Severity []string `json:"severity"`
	Purpose  []string `json:"purpose"`
	// Deprecated aliases (treated as Tags if Tags empty)
	ComplianceRules []string `json:"compliance_rules"`
	TechStackRules  []string `json:"tech_stack_rules"`
	SpecialRules    []string `json:"special_rules"`
}

var (
	scanPoliciesConfig     *ScanPoliciesConfig
	scanPoliciesConfigOnce sync.Once
)

// GetScanPoliciesConfig 获取完整的扫描策略配置
func GetScanPoliciesConfig() *ScanPoliciesConfig {
	scanPoliciesConfigOnce.Do(func() {
		var config ScanPoliciesConfig
		if err := yaml.Unmarshal(scanPoliciesYAML, &config); err != nil {
			log.Errorf("Failed to load scan_policies.yaml: %v", err)
			return
		}
		scanPoliciesConfig = &config
	})
	return scanPoliciesConfig
}

// GetAllStandardGroupNames deprecated: returns standard tag names for legacy callers.
func GetAllStandardGroupNames() []string {
	return GetAllStandardTagNames()
}

// GetAllStandardTagNames returns selectable standard tags from YAML.
func GetAllStandardTagNames() []string {
	config := GetScanPoliciesConfig()
	if config == nil {
		return nil
	}
	var tags []string
	for _, opts := range config.CustomRuleTags {
		for _, opt := range opts {
			if opt.Name != "" {
				tags = append(tags, opt.Name)
			}
		}
	}
	for _, p := range config.Policies {
		tags = append(tags, p.Tags...)
	}
	return tags
}

var (
	standardGroupNamesMap     map[string]bool
	standardGroupNamesMapOnce sync.Once
)

// IsStandardGroupName deprecated alias for IsStandardTagName.
func IsStandardGroupName(groupName string) bool {
	return IsStandardTagName(groupName)
}

// IsStandardTagName reports whether name is a known standard tag.
func IsStandardTagName(name string) bool {
	standardGroupNamesMapOnce.Do(func() {
		standardGroupNamesMap = make(map[string]bool)
		for _, n := range GetAllStandardTagNames() {
			standardGroupNamesMap[n] = true
		}
	})
	return standardGroupNamesMap[name]
}

const (
	PolicyTypeOWASPWeb     = "owasp-web"
	PolicyTypeCriticalHigh = "critical-high"
	PolicyTypeFullStack    = "fullstack"
	PolicyTypeCustom       = "custom"
)

// PolicyFilter is the expanded filter for a scan policy.
type PolicyFilter struct {
	Tags     []string
	Severity []string
	Purpose  []string
}

// MapToFilter expands the policy into Tag + column filters.
func (p *ScanPolicyConfig) MapToFilter() *PolicyFilter {
	if p == nil {
		return nil
	}
	out := &PolicyFilter{}
	if p.PolicyType == PolicyTypeCustom {
		if p.CustomRules != nil {
			out.Tags = append(out.Tags, p.CustomRules.Tags...)
			if len(out.Tags) == 0 {
				out.Tags = append(out.Tags, p.CustomRules.ComplianceRules...)
				out.Tags = append(out.Tags, p.CustomRules.TechStackRules...)
				out.Tags = append(out.Tags, p.CustomRules.SpecialRules...)
			}
			out.Severity = append(out.Severity, p.CustomRules.Severity...)
			out.Purpose = append(out.Purpose, p.CustomRules.Purpose...)
		}
		return out
	}
	config := GetScanPoliciesConfig()
	if config == nil {
		return out
	}
	def, ok := config.Policies[p.PolicyType]
	if !ok {
		log.Warnf("Policy type '%s' not found in scan_policies.yaml", p.PolicyType)
		out.Severity = []string{"critical", "high"}
		return out
	}
	out.Tags = append(out.Tags, def.Tags...)
	out.Severity = append(out.Severity, def.Severity...)
	out.Purpose = append(out.Purpose, def.Purpose...)
	// legacy rule_groups that look like severity levels
	for _, g := range def.RuleGroups {
		switch g {
		case "critical", "high", "middle", "low", "info":
			out.Severity = append(out.Severity, g)
		default:
			out.Tags = append(out.Tags, g)
		}
	}
	return out
}

// MapToGroups deprecated: returns tags only (no longer group names).
func (p *ScanPolicyConfig) MapToGroups() []string {
	f := p.MapToFilter()
	if f == nil {
		return nil
	}
	return f.Tags
}

func (c *Config) GetScanPolicy() *ScanPolicyConfig {
	if c == nil {
		return nil
	}
	return c.ScanPolicy
}

// SetScanPolicy applies policy as RuleFilter Tags + Severity/Purpose (not GroupNames).
func (c *Config) SetScanPolicy(policy *ScanPolicyConfig) error {
	if c == nil {
		return nil
	}
	c.ScanPolicy = policy
	if policy == nil {
		return nil
	}
	f := policy.MapToFilter()
	if f == nil {
		return nil
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = defaultSyntaxFlowRuleConfig()
	}
	if c.SyntaxFlowRule.RuleFilter == nil {
		c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
	}
	rf := c.SyntaxFlowRule.RuleFilter
	if len(f.Tags) > 0 {
		rf.Tag = f.Tags
	}
	if len(f.Severity) > 0 {
		rf.Severity = f.Severity
	}
	if len(f.Purpose) > 0 {
		rf.Purpose = f.Purpose
	}
	// clear group-based selection
	rf.GroupNames = nil
	return nil
}

func WithScanPolicy(policyType string, customRules *CustomRulesConfig) Option {
	return func(c *Config) error {
		policy := &ScanPolicyConfig{
			PolicyType:  policyType,
			CustomRules: customRules,
		}
		return c.SetScanPolicy(policy)
	}
}

func WithOWASPWebPolicy() Option {
	return WithScanPolicy(PolicyTypeOWASPWeb, nil)
}

func WithCriticalHighPolicy() Option {
	return WithScanPolicy(PolicyTypeCriticalHigh, nil)
}

func WithFullStackPolicy() Option {
	return WithScanPolicy(PolicyTypeFullStack, nil)
}

func WithCustomPolicy(customRules *CustomRulesConfig) Option {
	return WithScanPolicy(PolicyTypeCustom, customRules)
}
