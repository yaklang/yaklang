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

// SyntaxFlow结果保存类型
type SFResultSaveKind string

const (
	SFResultSaveNone     SFResultSaveKind = "none"     // no save
	SFResultSaveMemory   SFResultSaveKind = "memory"   // in cache
	SFResultSaveDatabase SFResultSaveKind = "database" // in database
)

type SyntaxFlowConfig struct {
	Memory          bool                  `json:"memory"`
	ResultSaveKind  SFResultSaveKind      `json:"result_save_kind"`
	ProcessCallback func(float64, string) `json:"-"`
}

type SyntaxFlowScanManagerConfig struct {
	IgnoreLanguage  bool                   `json:"ignore_language"`
	Language        []string               `json:"language"`
	Concurrency     uint32                 `json:"concurrency"`
	ProcessCallback func(progress float64) `json:"-"`
}

type SyntaxFlowRuleConfig struct {
	RuleNames  []string                  `json:"rule_names"`
	RuleInput  *ypb.SyntaxFlowRuleInput  `json:"rule_input"`
	RuleFilter *ypb.SyntaxFlowRuleFilter `json:"rule_filter"`
}

type Config struct {
	Mode                  Mode
	BaseInfo              *BaseInfo
	CodeSource            *CodeSourceInfo
	SSACompile            *SSACompileConfig
	SyntaxFlow            *SyntaxFlowConfig
	SyntaxFlowScanManager *SyntaxFlowScanManagerConfig
	SyntaxFlowRule        *SyntaxFlowRuleConfig
}

type Mode int

const (
	ModeProjectBase           Mode = 1 << iota // 0 - 基础模式
	ModeSSACompile            Mode = 1 << iota // 1 - 编译模式
	ModeSyntaxFlowScanManager Mode = 1 << iota // 2 - 扫描管理器模式
	ModeSyntaxFlow            Mode = 1 << iota // 3 - SyntaxFlow模式
	ModeSyntaxFlowRule        Mode = 1 << iota // 4 - 规则模式
	ModeCodeSource            Mode = 1 << iota // 5 - 源码配置模式
	ModeSyntaxFlowScan        Mode = ModeProjectBase | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeSyntaxFlowScanManager
	// all
	ModeAll = ModeProjectBase | ModeSSACompile | ModeSyntaxFlow | ModeSyntaxFlowRule | ModeCodeSource | ModeSyntaxFlowScanManager
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
			Memory:         false,
			ResultSaveKind: SFResultSaveNone,
		}
	}
	if mode&ModeSyntaxFlowScanManager != 0 {
		cfg.SyntaxFlowScanManager = &SyntaxFlowScanManagerConfig{
			IgnoreLanguage: false,
			Language:       []string{},
			Concurrency:    5,
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

// syntaxflow scan manager

func (c *Config) GetScanConcurrency() uint32 {
	if c == nil || c.SyntaxFlowScanManager == nil {
		return 0
	}
	return c.SyntaxFlowScanManager.Concurrency
}

func (c *Config) GetScanMemory() bool {
	if c == nil || c.SyntaxFlow == nil {
		return false
	}
	return c.SyntaxFlow.Memory
}

func (c *Config) GetScanIgnoreLanguage() bool {
	if c == nil || c.SyntaxFlowScanManager == nil {
		return false
	}
	return c.SyntaxFlowScanManager.IgnoreLanguage
}

func (c *Config) GetScanProcessCallback() func(progress float64) {
	if c == nil || c.SyntaxFlow == nil {
		return nil
	}
	return c.SyntaxFlowScanManager.ProcessCallback
}

func (c *Config) GetCompileStrictMode() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.StrictMode
}

func (c *Config) SetCompileStrictMode(strictMode bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.StrictMode = strictMode
}

func (c *Config) GetCompilePeepholeSize() int {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.PeepholeSize
}

func (c *Config) SetCompilePeepholeSize(peepholeSize int) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.PeepholeSize = peepholeSize
}

func (c *Config) GetCompileExcludeFiles() []string {
	if c == nil || c.SSACompile == nil {
		return nil
	}
	return c.SSACompile.ExcludeFiles
}

func (c *Config) SetCompileExcludeFiles(excludeFiles []string) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.ExcludeFiles = excludeFiles
}

func (c *Config) GetCompileReCompile() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.ReCompile
}

func (c *Config) SetCompileReCompile(reCompile bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.ReCompile = reCompile
}

func (c *Config) GetCompileMemory() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.MemoryCompile
}

func (c *Config) SetCompileMemory(memory bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.MemoryCompile = memory
}

func (c *Config) GetCompileConcurrency() uint32 {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.Concurrency
}

func (c *Config) SetCompileConcurrency(concurrency uint32) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.Concurrency = concurrency
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

// syntaxflow

func (c *Config) GetSyntaxFlowResultKind() SFResultSaveKind {
	if c == nil || c.SyntaxFlow == nil {
		return SFResultSaveNone
	}
	return c.SyntaxFlow.ResultSaveKind
}

func (c *Config) SetSyntaxFlowResultKind(resultKind SFResultSaveKind) {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		return
	}
	c.SyntaxFlow.ResultSaveKind = resultKind
}

func (c *Config) SetSyntaxFlowResultSaveDataBase() {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		return
	}
	c.SyntaxFlow.ResultSaveKind = SFResultSaveDatabase
}

func (c *Config) SetSyntaxFlowResultSaveMemory() {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		return
	}
	c.SyntaxFlow.ResultSaveKind = SFResultSaveMemory
}

func (c *Config) GetSyntaxFlowProcessCallback() func(float64, string) {
	if c == nil || c.SyntaxFlow == nil {
		return nil
	}
	return c.SyntaxFlow.ProcessCallback
}

func (c *Config) SetSyntaxFlowProcessCallback(processCallback func(float64, string)) {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		return
	}
	c.SyntaxFlow.ProcessCallback = processCallback
}
