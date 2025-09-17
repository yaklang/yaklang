package schema

import (
	"encoding/json"
	"fmt"
	"strings"

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
	// 源码获取方式配置
	CodeSourceConfig string `json:"code_source_config" gorm:"type:text;not null;comment:源码获取配置"`
	// 编译配置选项
	Language           string `json:"language" gorm:"comment:项目语言"`
	StrictMode         bool   `json:"strict_mode" gorm:"comment:是否启用严格模式"`
	PeepholeSize       int    `json:"peephole_size" gorm:"comment:窥孔编译大小"`
	ExcludeFiles       string `json:"exclude_files,omitempty" gorm:"comment:排除文件列表,逗号分隔"`
	ReCompile          bool   `json:"re_compile" gorm:"comment:是否重新编译"`
	MemoryCompile      bool   `json:"memory_compile" gorm:"comment:是否使用内存编译"`
	CompileConcurrency uint32 `json:"compile_concurrency" gorm:"comment:编译并发数"`
	// 扫描配置选项
	ScanConcurrency uint32 `json:"scan_concurrency" gorm:"comment:扫描并发数"`
	MemoryScan      bool   `json:"memory_scan" gorm:"comment:是否使用内存扫描"`
	IgnoreLanguage  bool   `json:"ignore_language" gorm:"comment:是否忽略语言检查"`
	// 扫描策略配置
	RuleFilter []byte `json:"rule_filter,omitempty" gorm:"comment:扫描规则过滤配置"`
}

func (p *SSAProject) GetSourceCodeInfo() (*CodeSourceInfo, error) {
	if p.CodeSourceConfig == "" {
		return nil, fmt.Errorf("source config is required")
	}

	var configInfo CodeSourceInfo
	if err := json.Unmarshal([]byte(p.CodeSourceConfig), &configInfo); err != nil {
		return nil, fmt.Errorf("failed to parse source config JSON: %v", err)
	}
	return &configInfo, nil
}

func (p *SSAProject) SetSourceConfig(config *CodeSourceInfo) error {
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal source config: %v", err)
	}
	p.CodeSourceConfig = string(configBytes)
	return nil
}

func (p *SSAProject) GetSourceConfig() (*CodeSourceInfo, error) {
	if p.CodeSourceConfig == "" {
		return nil, fmt.Errorf("source config is required")
	}
	var config CodeSourceInfo
	if err := json.Unmarshal([]byte(p.CodeSourceConfig), &config); err != nil {
		return nil, fmt.Errorf("failed to parse source config JSON: %v", err)
	}
	return &config, nil
}

func (p *SSAProject) GetSourceConfigInfo() (string, error) {
	config, err := p.GetSourceConfig()
	if err != nil {
		return "", err
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal source config: %v", err)
	}
	return string(configBytes), nil
}

func (p *SSAProject) GetSourceConfigInfoMap() (map[string]any, error) {
	config, err := p.GetSourceConfig()
	if err != nil {
		return nil, err
	}
	configMap := make(map[string]any)
	configMap["kind"] = config.Kind
	configMap["url"] = config.URL
	configMap["path"] = config.Path
	configMap["branch"] = config.Branch
	configMap["auth"] = config.Auth
	configMap["proxy"] = config.Proxy
	return configMap, nil
}

// Validate 验证SSAProject配置的有效性
func (p *SSAProject) Validate() error {
	if p.ProjectName == "" {
		return fmt.Errorf("project name is required")
	}
	config, err := p.GetSourceConfig()
	if err != nil {
		return fmt.Errorf("invalid source config: %v", err)
	}
	if !isValidCodeSourceKind(config.Kind) {
		return fmt.Errorf("invalid source kind: %s", config.Kind)
	}
	return config.ValidateSourceConfig()
}

func (p *SSAProject) GetCompileConfigInfo() (string, error) {
	if err := p.Validate(); err != nil {
		return "", fmt.Errorf("project validation failed: %v", err)
	}

	compileConfig := &SSACompileConfig{
		Language:           p.Language,
		StrictMode:         p.StrictMode,
		PeepholeSize:       p.PeepholeSize,
		ExcludeFiles:       p.GetExcludeFilesList(),
		ReCompile:          p.ReCompile,
		MemoryCompile:      p.MemoryCompile,
		CompileConcurrency: p.CompileConcurrency,
	}

	configBytes, err := json.Marshal(compileConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal compile config: %v", err)
	}
	return string(configBytes), nil
}

func (p *SSAProject) GetScanConfigInfo() (string, error) {
	if err := p.Validate(); err != nil {
		return "", fmt.Errorf("project validation failed: %v", err)
	}

	scanOptions := &SSAScanConfig{
		Concurrency:    p.ScanConcurrency,
		Memory:         p.MemoryScan,
		IgnoreLanguage: p.IgnoreLanguage,
	}

	configBytes, err := json.Marshal(scanOptions)
	if err != nil {
		return "", fmt.Errorf("failed to marshal scan config: %v", err)
	}
	return string(configBytes), nil
}

func (p *SSAProject) GetExcludeFilesList() []string {
	return strings.Split(p.ExcludeFiles, ",")
}

func (p *SSAProject) GetTagsList() []string {
	return strings.Split(p.Tags, ",")
}

func (p *SSAProject) SetTagsList(tags []string) {
	p.Tags = strings.Join(tags, ",")
}

func (p *SSAProject) SetExcludeFilesList(files []string) {
	p.ExcludeFiles = strings.Join(files, ",")
}

func (p *SSAProject) GetRuleFilter() (*ypb.SyntaxFlowRuleFilter, error) {
	if p.RuleFilter == nil {
		return nil, fmt.Errorf("rule filter is required")
	}
	var filter *ypb.SyntaxFlowRuleFilter
	err := json.Unmarshal(p.RuleFilter, &filter)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal rule filter: %v", err)
	}
	return filter, nil
}

func (p *SSAProject) SetRuleFilter(filter *ypb.SyntaxFlowRuleFilter) error {
	filterBytes, err := json.Marshal(filter)
	if err != nil {
		return fmt.Errorf("failed to marshal rule filter: %v", err)
	}
	p.RuleFilter = filterBytes
	return nil
}

type SSACompileConfig struct {
	Language           string   `json:"language"`
	ConfigInfo         string   `json:"config_info"`
	StrictMode         bool     `json:"strict_mode"`
	PeepholeSize       int      `json:"peephole_size"`
	ExcludeFiles       []string `json:"exclude_files"`
	ReCompile          bool     `json:"re_compile"`
	MemoryCompile      bool     `json:"memory_compile"`
	CompileConcurrency uint32   `json:"compile_concurrency"`
}

type SSAScanConfig struct {
	Concurrency    uint32 `json:"concurrency"`
	Memory         bool   `json:"memory"`
	IgnoreLanguage bool   `json:"ignore_language"`
}

func (p *SSAProject) ToGRPCModel() *ypb.SSAProject {
	result := &ypb.SSAProject{
		ID:               int64(p.ID),
		CreatedAt:        p.CreatedAt.Unix(),
		UpdatedAt:        p.UpdatedAt.Unix(),
		ProjectName:      p.ProjectName,
		Language:         p.Language,
		CodeSourceConfig: p.CodeSourceConfig,
		Description:      p.Description,
		CompileConfig: &ypb.SSAProjectCompileConfig{
			StrictMode:   p.StrictMode,
			PeepholeSize: int64(p.PeepholeSize),
			ExcludeFiles: p.GetExcludeFilesList(),
			ReCompile:    p.ReCompile,
		},
		ScanConfig: &ypb.SSAProjectScanConfig{
			Concurrency:    p.ScanConcurrency,
			Memory:         p.MemoryScan,
			IgnoreLanguage: p.IgnoreLanguage,
		},
		Tags: p.GetTagsList(),
	}
	filter, err := p.GetRuleFilter()
	if err != nil {
		return nil
	}
	if filter != nil {
		result.RuleConfig = &ypb.SSAProjectScanRuleConfig{
			RuleFilter: filter,
		}
	}
	return result
}
