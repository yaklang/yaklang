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

var (
	policyMappings     map[string][]string
	policyMappingsOnce sync.Once
)

// PolicyDefinition 策略定义（从YAML加载）
type PolicyDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Icon        string   `yaml:"icon"`
	RuleGroups  []string `yaml:"rule_groups"`
}

// ScanPoliciesConfig 策略配置文件结构
type ScanPoliciesConfig struct {
	Version          string                      `yaml:"version"`
	Policies         map[string]PolicyDefinition `yaml:"policies"`
	Categories       []PolicyCategory            `yaml:"categories"`
	CustomRuleGroups CustomRuleGroupsConfig      `yaml:"custom_rule_groups"`
}

// PolicyCategory 策略分类
type PolicyCategory struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Policies []string `yaml:"policies" json:"policies"`
}

// CustomRuleGroupsConfig 自定义规则组配置
type CustomRuleGroupsConfig struct {
	ComplianceRules []RuleGroupCategory `yaml:"compliance_rules" json:"compliance_rules"`
	TechStackRules  []RuleGroupCategory `yaml:"tech_stack_rules" json:"tech_stack_rules"`
	SpecialRules    []RuleGroupCategory `yaml:"special_rules" json:"special_rules"`
}

// RuleGroupCategory 规则组分类
type RuleGroupCategory struct {
	Category string      `yaml:"category" json:"category"`
	Groups   []RuleGroup `yaml:"groups" json:"groups"`
}

// RuleGroup 规则组
type RuleGroup struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"display_name" json:"display_name"`
}

// ScanPolicyConfig 扫描策略配置
type ScanPolicyConfig struct {
	PolicyType  string            `json:"policy_type"`  // 策略类型: owasp-web, critical-high, fullstack, custom
	CustomRules *CustomRulesConfig `json:"custom_rules"` // 自定义规则组（当 PolicyType 为 custom 时使用）
}

// CustomRulesConfig 自定义规则配置（支持分类）
type CustomRulesConfig struct {
	ComplianceRules []string `json:"compliance_rules"` // 合规规则
	TechStackRules  []string `json:"tech_stack_rules"` // 技术栈规则
	SpecialRules    []string `json:"special_rules"`    // 特殊规则
}

var (
	scanPoliciesConfig     *ScanPoliciesConfig
	scanPoliciesConfigOnce sync.Once
)

// loadPolicyMappings 加载策略映射关系（懒加载，只加载一次）
func loadPolicyMappings() map[string][]string {
	policyMappingsOnce.Do(func() {
		policyMappings = make(map[string][]string)

		config := GetScanPoliciesConfig()
		if config == nil {
			return
		}

		for policyType, policy := range config.Policies {
			policyMappings[policyType] = policy.RuleGroups
		}
	})
	return policyMappings
}

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

// GetAllStandardGroupNames 获取所有标准规则组名称
func GetAllStandardGroupNames() []string {
	config := GetScanPoliciesConfig()
	if config == nil {
		return nil
	}
	
	var groupNames []string
	
	// 从 custom_rule_groups 中提取所有组名
	if config.CustomRuleGroups.ComplianceRules != nil {
		for _, category := range config.CustomRuleGroups.ComplianceRules {
			for _, group := range category.Groups {
				groupNames = append(groupNames, group.Name)
			}
		}
	}
	
	if config.CustomRuleGroups.TechStackRules != nil {
		for _, category := range config.CustomRuleGroups.TechStackRules {
			for _, group := range category.Groups {
				groupNames = append(groupNames, group.Name)
			}
		}
	}
	
	if config.CustomRuleGroups.SpecialRules != nil {
		for _, category := range config.CustomRuleGroups.SpecialRules {
			for _, group := range category.Groups {
				groupNames = append(groupNames, group.Name)
			}
		}
	}
	
	return groupNames
}

var (
	standardGroupNamesMap     map[string]bool
	standardGroupNamesMapOnce sync.Once
)

// IsStandardGroupName 判断是否是标准规则组名称
func IsStandardGroupName(groupName string) bool {
	standardGroupNamesMapOnce.Do(func() {
		standardGroupNamesMap = make(map[string]bool)
		for _, name := range GetAllStandardGroupNames() {
			standardGroupNamesMap[name] = true
		}
	})
	return standardGroupNamesMap[groupName]
}

// 预定义策略类型
const (
	PolicyTypeOWASPWeb     = "owasp-web"     // OWASP Top 10 合规扫描
	PolicyTypeCriticalHigh = "critical-high" // 严重+高危漏洞快速扫描
	PolicyTypeFullStack    = "fullstack"     // 全栈深度扫描
	PolicyTypeCustom       = "custom"        // 自定义规则组
)

// MapToGroups 将扫描策略映射为规则组列表（从YAML配置读取）
func (p *ScanPolicyConfig) MapToGroups() []string {
	if p == nil {
		return nil
	}

	// 对于自定义策略，展平并合并所有分类的规则组
	if p.PolicyType == PolicyTypeCustom {
		if p.CustomRules != nil {
			var allGroups []string
			allGroups = append(allGroups, p.CustomRules.ComplianceRules...)
			allGroups = append(allGroups, p.CustomRules.TechStackRules...)
			allGroups = append(allGroups, p.CustomRules.SpecialRules...)
			if len(allGroups) > 0 {
				return allGroups
			}
		}
		return nil
	}

	// 从YAML配置中加载映射关系
	mappings := loadPolicyMappings()
	if groups, ok := mappings[p.PolicyType]; ok {
		return groups
	}

	// 如果YAML中没有定义，返回默认值
	log.Warnf("Policy type '%s' not found in scan_policies.yaml, using default", p.PolicyType)
	return []string{"critical", "high"}
}

// --- 策略配置 Get/Set 方法 ---

// GetScanPolicy 获取扫描策略
func (c *Config) GetScanPolicy() *ScanPolicyConfig {
	if c == nil {
		return nil
	}
	return c.ScanPolicy
}

// SetScanPolicy 设置扫描策略并自动应用到RuleFilter
func (c *Config) SetScanPolicy(policy *ScanPolicyConfig) error {
	if c == nil {
		return nil
	}
	c.ScanPolicy = policy

	// 自动映射策略到规则组
	if policy != nil {
		groupNames := policy.MapToGroups()
		if len(groupNames) > 0 {
			// 确保 SyntaxFlowRule 存在
			if c.SyntaxFlowRule == nil {
				c.SyntaxFlowRule = defaultSyntaxFlowRuleConfig()
			}
			if c.SyntaxFlowRule.RuleFilter == nil {
				c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
			}
			c.SyntaxFlowRule.RuleFilter.GroupNames = groupNames
		}
	}

	return nil
}

// --- 策略配置 Options ---

// WithScanPolicy 设置扫描策略（支持分类结构）
func WithScanPolicy(policyType string, customRules *CustomRulesConfig) Option {
	return func(c *Config) error {
		policy := &ScanPolicyConfig{
			PolicyType:  policyType,
			CustomRules: customRules,
		}
		return c.SetScanPolicy(policy)
	}
}

// WithOWASPWebPolicy 快捷方法：OWASP Top 10 扫描
func WithOWASPWebPolicy() Option {
	return WithScanPolicy(PolicyTypeOWASPWeb, nil)
}

// WithCriticalHighPolicy 快捷方法：严重+高危扫描
func WithCriticalHighPolicy() Option {
	return WithScanPolicy(PolicyTypeCriticalHigh, nil)
}

// WithFullStackPolicy 快捷方法：全栈深度扫描
func WithFullStackPolicy() Option {
	return WithScanPolicy(PolicyTypeFullStack, nil)
}

// WithCustomPolicy 快捷方法：自定义规则组（支持分类）
func WithCustomPolicy(customRules *CustomRulesConfig) Option {
	return WithScanPolicy(PolicyTypeCustom, customRules)
}
