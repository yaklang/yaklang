package schema

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SSAProject 用于配置SSA的项目信息，包括项目名称、源码获取方式以及编译、扫描选项等
type SSAProject struct {
	gorm.Model
	// 项目基础信息
	ProjectName string             `json:"project_name" gorm:"unique_index;not null;comment:项目名称"`
	Description string             `json:"description,omitempty" gorm:"comment:项目描述"`
	Tags        string             `json:"tags,omitempty" gorm:"comment:项目标签"`
	Language    ssaconfig.Language `json:"language" gorm:"comment:项目语言"`
	// 配置选项
	Config []byte `json:"config"`
}

func (p *SSAProject) GetTagsList() []string {
	return strings.Split(p.Tags, ",")
}

func (p *SSAProject) SetTagsList(tags []string) {
	p.Tags = strings.Join(tags, ",")
}

func (p *SSAProject) GetConfig() (*ssaconfig.Config, error) {
	var config ssaconfig.Config
	err := json.Unmarshal(p.Config, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (p *SSAProject) SetConfig(config *ssaconfig.Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	p.Config = data
	return nil
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
		Language:         string(p.Language),
		CodeSourceConfig: config.CodeSource.JsonString(),
		Description:      p.Description,
		Tags:             p.GetTagsList(),
	}

	result.CompileConfig = &ypb.SSAProjectCompileConfig{
		StrictMode:   config.GetCompileStrictMode(),
		PeepholeSize: int64(config.GetCompilePeepholeSize()),
		ExcludeFiles: config.GetCompileExcludeFiles(),
		ReCompile:    config.GetCompileReCompile(),
		Memory:       config.GetCompileMemory(),
	}

	result.ScanConfig = &ypb.SSAProjectScanConfig{
		Concurrency:    uint32(int64(config.GetScanConcurrency())),
		Memory:         config.GetSyntaxFlowMemory(),
		IgnoreLanguage: config.GetScanIgnoreLanguage(),
	}

	result.RuleConfig = &ypb.SSAProjectScanRuleConfig{
		RuleFilter: config.GetRuleFilter(),
	}
	return result
}
