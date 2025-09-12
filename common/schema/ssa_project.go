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
	StrictMode    bool   `json:"strict_mode" gorm:"comment:是否启用严格模式"`
	PeepholeSize  int    `json:"peephole_size" gorm:"comment:窥孔编译大小"`
	ExcludeFiles  string `json:"exclude_files,omitempty" gorm:"comment:排除文件列表,逗号分隔"`
	ReCompile     bool   `json:"re_compile" gorm:"comment:是否重新编译"`
	MemoryCompile bool   `json:"memory_compile" gorm:"comment:是否使用内存编译"`
	// 扫描配置选项
	ScanConcurrency uint32 `json:"scan_concurrency" gorm:"comment:扫描并发数"`
	MemoryScan      bool   `json:"memory_scan" gorm:"comment:是否使用内存扫描"`
	ScanRuleGroups  string `json:"scan_rule_groups,omitempty" gorm:"comment:扫描规则组,逗号分隔"`
	ScanRuleNames   string `json:"scan_rule_names,omitempty" gorm:"comment:扫描规则名称,逗号分隔"`
	IgnoreLanguage  bool   `json:"ignore_language" gorm:"comment:是否忽略语言检查"`
}

// GetSourceCodeInfo 获取源码配置信息
func (p *SSAProject) GetSourceCodeInfo() (map[string]interface{}, error) {
	if p.CodeSourceConfig == "" {
		return nil, fmt.Errorf("source config is required")
	}

	var configInfo CodeSourceInfo
	if err := json.Unmarshal([]byte(p.CodeSourceConfig), &configInfo); err != nil {
		return nil, fmt.Errorf("failed to parse source config JSON: %v", err)
	}

	configBytes, err := json.Marshal(configInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config info: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(configBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %v", err)
	}

	return result, nil
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

func (p *SSAProject) GetCompileOptions() (map[string]interface{}, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("project validation failed: %v", err)
	}

	configInfo, err := p.GetSourceCodeInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to generate config info: %v", err)
	}

	optionsMap := map[string]interface{}{
		"configInfo":    configInfo,
		"programName":   p.ProjectName,
		"description":   p.Description,
		"strictMode":    p.StrictMode,
		"peepholeSize":  p.PeepholeSize,
		"reCompile":     p.ReCompile,
		"memoryCompile": p.MemoryCompile,
		"excludeFiles":  p.GetExcludeFilesList(),
	}
	return optionsMap, nil
}

func (p *SSAProject) GetScanOptions() (map[string]interface{}, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("project validation failed: %v", err)
	}
	// 扫描相关的选项
	scanOptions := map[string]interface{}{
		"programName":    []string{p.ProjectName},
		"concurrency":    p.ScanConcurrency,
		"memory":         p.MemoryScan,
		"ignoreLanguage": p.IgnoreLanguage,
		"ruleGroups":     p.GetScanRuleGroupsList(),
		"ruleNames":      p.GetScanRuleNamesList(),
	}
	return scanOptions, nil
}

func (p *SSAProject) GetExcludeFilesList() []string {
	return strings.Split(p.ExcludeFiles, ",")
}

func (p *SSAProject) GetScanRuleGroupsList() []string {
	return strings.Split(p.ScanRuleGroups, ",")
}

func (p *SSAProject) GetScanRuleNamesList() []string {
	return strings.Split(p.ScanRuleNames, ",")
}

func (p *SSAProject) GetTagsList() []string {
	return strings.Split(p.Tags, ",")
}

func (p *SSAProject) SetExcludeFilesList(files []string) {
	p.ExcludeFiles = strings.Join(files, ",")
}

func (p *SSAProject) SetScanRuleGroupsList(groups []string) {
	p.ScanRuleGroups = strings.Join(groups, ",")
}

func (p *SSAProject) SetScanRuleNamesList(names []string) {
	p.ScanRuleNames = strings.Join(names, ",")
}

func (p *SSAProject) SetTagsList(tags []string) {
	p.Tags = strings.Join(tags, ",")
}

func (p *SSAProject) ToGRPCModel() *ypb.SSAProject {
	return &ypb.SSAProject{
		ID:               int64(p.ID),
		CreatedAt:        p.CreatedAt.Unix(),
		UpdatedAt:        p.UpdatedAt.Unix(),
		ProjectName:      p.ProjectName,
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
			RuleGroups:     p.GetScanRuleGroupsList(),
			RuleNames:      p.GetScanRuleNamesList(),
		},
		Tags: p.GetTagsList(),
	}
}
