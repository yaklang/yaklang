package ssaconfig

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 基础信息配置
type BaseInfo struct {
	ProgramNames       []string `json:"program_names"`
	ProjectName        string   `json:"project_name"`
	ProjectDescription string   `json:"project_description"`
	Language           string   `json:"language"`
	Tags               []string `json:"tags"`
}

// SSACompileConfig 编译配置
type SSACompileConfig struct {
	StrictMode    bool     `json:"strict_mode"`
	PeepholeSize  int      `json:"peephole_size"`
	ExcludeFiles  []string `json:"exclude_files"`
	ReCompile     bool     `json:"re_compile"`
	MemoryCompile bool     `json:"memory_compile"`
	Concurrency   uint32   `json:"compile_concurrency"`
}

// SyntaxFlowConfig 扫描配置
type SyntaxFlowConfig struct {
	Concurrency     uint32                 `json:"concurrency"`
	Memory          bool                   `json:"memory"`
	IgnoreLanguage  bool                   `json:"ignore_language"`
	Language        []string               `json:"language"`
	ProcessCallback func(progress float64) `json:"-"`
}

type SyntaxFlowRuleConfig struct {
	RuleNames  []string                  `json:"rule_names"`
	RuleInput  *ypb.SyntaxFlowRuleInput  `json:"rule_input"`
	RuleFilter *ypb.SyntaxFlowRuleFilter `json:"rule_filter"`
}

type Config struct {
	Mode           Mode
	BaseInfo       *BaseInfo
	CodeSource     *CodeSourceInfo
	SSACompile     *SSACompileConfig
	SyntaxFlow     *SyntaxFlowConfig
	SyntaxFlowRule *SyntaxFlowRuleConfig
}

type Mode int

const (
	ModeProjectBase    Mode = 1 << iota // 0 - 基础模式
	ModeSSACompile     Mode = 1 << iota // 1 - 编译模式
	ModeSyntaxFlow     Mode = 1 << iota // 2 - SyntaxFlow模式
	ModeSyntaxFlowRule Mode = 1 << iota // 4 - 规则模式
	ModeCodeSource     Mode = 1 << iota // 5 - 源码配置模式
	ModeSyntaxFlowScan Mode = ModeProjectBase | ModeSyntaxFlow | ModeSyntaxFlowRule
	// all
	ModeAll = ModeProjectBase | ModeSSACompile | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeCodeSource
)

func New(mode Mode, opts ...Option) (*Config, error) {
	cfg := &Config{}
	cfg.Mode = mode
	if mode&ModeCodeSource != 0 {
		cfg.CodeSource = &CodeSourceInfo{
			Auth:  &AuthConfigInfo{},
			Proxy: &ProxyConfigInfo{},
		}
	}
	if mode&ModeSSACompile != 0 {
		cfg.SSACompile = &SSACompileConfig{
			StrictMode:    false,
			PeepholeSize:  0,
			ExcludeFiles:  []string{},
			ReCompile:     false,
			MemoryCompile: false,
			Concurrency:   1,
		}
	}
	if mode&ModeSyntaxFlow != 0 {
		cfg.SyntaxFlow = &SyntaxFlowConfig{
			Concurrency:    1,
			Memory:         false,
			IgnoreLanguage: false,
		}
	}
	if mode&ModeSyntaxFlowRule != 0 {
		cfg.SyntaxFlowRule = &SyntaxFlowRuleConfig{
			RuleFilter: &ypb.SyntaxFlowRuleFilter{},
			RuleInput:  &ypb.SyntaxFlowRuleInput{},
		}
	}
	if mode&ModeProjectBase != 0 {
		cfg.BaseInfo = &BaseInfo{}
	}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func NewSyntaxFlowScanConfig(opts ...Option) (*Config, error) {
	return New(ModeSyntaxFlowScan, opts...)
}

func (c *Config) IsSyntaxFlowScanConfig() bool {
	return c.Mode == ModeSyntaxFlowScan
}

// GetRuleFilter 获取规则过滤器
func (c *Config) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	if c == nil || c.SyntaxFlowRule == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleFilter
}

func (c *Config) GetRuleNames() []string {
	if c == nil || c.SyntaxFlowRule == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleNames
}

func (c *Config) SetRuleNames(names []string) {
	if c == nil {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = &SyntaxFlowRuleConfig{}
	}
	c.SyntaxFlowRule.RuleNames = names
}

// SetRuleFilter 设置规则过滤器
func (c *Config) SetRuleFilter(filter *ypb.SyntaxFlowRuleFilter) {
	if c == nil {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = &SyntaxFlowRuleConfig{}
	}
	c.SyntaxFlowRule.RuleFilter = filter
}

func (c *Config) GetRuleInput() *ypb.SyntaxFlowRuleInput {
	if c == nil || c.SyntaxFlowRule == nil {
		return nil
	}
	return c.SyntaxFlowRule.RuleInput
}

func (c *Config) SetRuleInput(input *ypb.SyntaxFlowRuleInput) {
	if c == nil {
		return
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = &SyntaxFlowRuleConfig{}
	}
	c.SyntaxFlowRule.RuleInput = input
}

func (c *Config) GetScanConcurrency() uint32 {
	if c == nil || c.SyntaxFlow == nil {
		return 0
	}
	return c.SyntaxFlow.Concurrency
}

func (c *Config) GetScanMemory() bool {
	if c == nil || c.SyntaxFlow == nil {
		return false
	}
	return c.SyntaxFlow.Memory
}

func (c *Config) GetScanIgnoreLanguage() bool {
	if c == nil || c.SyntaxFlow == nil {
		return false
	}
	return c.SyntaxFlow.IgnoreLanguage
}

func (c *Config) GetScanProcessCallback() func(progress float64) {
	if c == nil || c.SyntaxFlow == nil {
		return nil
	}
	return c.SyntaxFlow.ProcessCallback
}

func (c *Config) GetCompileStrictMode() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.StrictMode
}

func (c *Config) GetCompilePeepholeSize() int {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.PeepholeSize
}

func (c *Config) GetCompileExcludeFiles() []string {
	if c == nil || c.SSACompile == nil {
		return nil
	}
	return c.SSACompile.ExcludeFiles
}

func (c *Config) GetCompileReCompile() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.ReCompile
}

func (c *Config) GetCompileMemory() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.MemoryCompile
}

func (c *Config) GetCompileConcurrency() uint32 {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.Concurrency
}

func (c *Config) GetProgramNames() []string {
	if c == nil || c.BaseInfo == nil {
		return nil
	}
	return c.BaseInfo.ProgramNames
}

func (c *Config) GetProjectName() string {
	if c == nil || c.BaseInfo == nil {
		return ""
	}
	return c.BaseInfo.ProjectName
}

func (c *Config) GetProjectDescription() string {
	if c == nil || c.BaseInfo == nil {
		return ""
	}
	return c.BaseInfo.ProjectDescription
}

func (c *Config) GetLanguage() string {
	if c == nil || c.BaseInfo == nil {
		return ""
	}
	return c.BaseInfo.Language
}

func (c *Config) GetTags() []string {
	if c == nil || c.BaseInfo == nil {
		return nil
	}
	return c.BaseInfo.Tags
}

// 代码源配置
func (c *Config) GetCodeSourceKind() CodeSourceKind {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Kind
}

func (c *Config) GetCodeSourceLocalFile() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.LocalFile
}

func (c *Config) GetCodeSourceURL() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.URL
}

func (c *Config) GetCodeSourceBranch() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Branch
}

func (c *Config) GetCodeSourcePath() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Path
}

func (c *Config) GetCodeSourceAuthKind() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Auth.Kind
}

func (c *Config) GetCodeSourceAuthUserName() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Auth.UserName
}

func (c *Config) GetCodeSourceAuthPassword() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Auth.Password
}

func (c *Config) GetCodeSourceProxyURL() string {
	if c == nil || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Proxy.URL
}

func (c *Config) GetCodeSourceProxyAuth() (string, string) {
	if c == nil || c.CodeSource == nil {
		return "", ""
	}
	return c.CodeSource.Proxy.User, c.CodeSource.Proxy.Password
}

func (c *Config) GetCodeSourceAuth() *AuthConfigInfo {
	if c == nil || c.CodeSource == nil {
		return nil
	}
	return c.CodeSource.Auth
}
