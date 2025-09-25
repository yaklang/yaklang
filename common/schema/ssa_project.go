package schema

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SSAProject 用于配置SSA的项目信息，包括项目名称、源码获取方式以及编译、扫描选项等
type SSAProject struct {
	gorm.Model
	// 项目基础信息
	ProjectName string `json:"project_name" gorm:"unique_index;not null;comment:项目名称"`
	Description string `json:"description,omitempty" gorm:"comment:项目描述"`
	Tags        string `json:"tags,omitempty" gorm:"comment:项目标签"`
	Language    string `json:"language" gorm:"comment:项目语言"`
	// 源码获取方式配置
	CodeSourceConfig string `json:"code_source_config"`
	// 配置选项
	Config []byte `json:"config"`
}

func (p *SSAProject) GetSourceConfig() *CodeSourceInfo {
	var config CodeSourceInfo
	err := json.Unmarshal([]byte(p.CodeSourceConfig), &config)
	if err != nil {
		log.Errorf("failed to unmarshal code source config: %v", err)
		return &CodeSourceInfo{}
	}
	return &config
}

func (p *SSAProject) GetCodeSourceConfigRaw() string {
	return p.CodeSourceConfig
}

func (p *SSAProject) SetSourceConfig(raw string) {
	p.CodeSourceConfig = raw
}

func (p *SSAProject) GetTagsList() []string {
	return strings.Split(p.Tags, ",")
}

func (p *SSAProject) SetTagsList(tags []string) {
	p.Tags = strings.Join(tags, ",")
}

func (p *SSAProject) GetConfig() (*SSAProjectConfig, error) {
	var config SSAProjectConfig
	err := json.Unmarshal(p.Config, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (p *SSAProject) SetConfig(config *SSAProjectConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	p.Config = data
	return nil
}

type SSAProjectConfig struct {
	CompileConfig *SSACompileConfig
	ScanConfig    *SSAScanConfig
	RuleConfig    *SSARuleConfig
}
type SSACompileConfig struct {
	StrictMode    bool     `json:"strict_mode"`
	PeepholeSize  int      `json:"peephole_size"`
	ExcludeFiles  []string `json:"exclude_files"`
	ReCompile     bool     `json:"re_compile"`
	MemoryCompile bool     `json:"memory_compile"`
	Concurrency   uint32   `json:"compile_concurrency"`
}
type SSAScanConfig struct {
	Concurrency    uint32 `json:"concurrency"`
	Memory         bool   `json:"memory"`
	IgnoreLanguage bool   `json:"ignore_language"`
	// 运行时配置，不存数据库
	ProcessCallback func(progress float64) `json:"-"`
}

type SSARuleConfig struct {
	RuleFilter *ypb.SyntaxFlowRuleFilter
}

func NewSSAProjectConfig() *SSAProjectConfig {
	return &SSAProjectConfig{
		CompileConfig: &SSACompileConfig{},
		ScanConfig:    &SSAScanConfig{},
		RuleConfig: &SSARuleConfig{
			RuleFilter: &ypb.SyntaxFlowRuleFilter{},
		},
	}
}

func (s *SSAProjectConfig) SetRuleFilter(filter *ypb.SyntaxFlowRuleFilter) {
	if s == nil {
		return
	}
	if s.RuleConfig == nil {
		s.RuleConfig = &SSARuleConfig{}
	}
	s.RuleConfig.RuleFilter = filter
}

func (s *SSAProjectConfig) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	if s == nil || s.RuleConfig == nil {
		return nil
	}
	return s.RuleConfig.RuleFilter
}

func (p *SSAProject) ToGRPCModel() *ypb.SSAProject {
	config, err := p.GetConfig()
	if err != nil {
		log.Errorf("failed to marshal code source config: %v", err)
	}

	result := &ypb.SSAProject{
		ID:               int64(p.ID),
		CreatedAt:        p.CreatedAt.Unix(),
		UpdatedAt:        p.UpdatedAt.Unix(),
		ProjectName:      p.ProjectName,
		Language:         p.Language,
		CodeSourceConfig: p.CodeSourceConfig,
		Description:      p.Description,
		Tags:             p.GetTagsList(),
	}

	if cc := config.CompileConfig; cc != nil {
		result.CompileConfig = &ypb.SSAProjectCompileConfig{
			StrictMode:   cc.StrictMode,
			PeepholeSize: int64(cc.PeepholeSize),
			ExcludeFiles: cc.ExcludeFiles,
			ReCompile:    cc.ReCompile,
			Memory:       cc.MemoryCompile,
		}
	}

	if sc := config.ScanConfig; sc != nil {
		result.ScanConfig = &ypb.SSAProjectScanConfig{
			Concurrency:    uint32(int64(sc.Concurrency)),
			Memory:         sc.Memory,
			IgnoreLanguage: sc.IgnoreLanguage,
		}
	}

	if rc := config.RuleConfig; rc != nil {
		if filter := rc.RuleFilter; filter != nil {
			result.RuleConfig = &ypb.SSAProjectScanRuleConfig{
				RuleFilter: filter,
			}
		}
	}
	return result
}
