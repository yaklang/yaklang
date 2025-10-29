package ssaproject

import (
	"encoding/json"
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSAProjectBuilder struct {
	ID          uint
	ProjectName string `json:"project_name"`
	Description string `json:"description"`
	Tags        []string
	Language    ssaconfig.Language `json:"language"`
	Config      *ssaconfig.Config
	Info        *ssaconfig.CodeSourceInfo `json:"info"`
}

func NewSSAProjectBuilderByRawData(rawData string) (*SSAProjectBuilder, error) {
	builder := &SSAProjectBuilder{}
	if rawData == "" {
		return nil, utils.Errorf("failed to new SSA project builder: raw data is empty")
	}
	err := json.Unmarshal([]byte(rawData), builder)
	if err != nil {
		return nil, utils.Errorf("failed to unmarshal SSA project raw data: %s", err)
	}
	return builder, nil
}

func NewSSAProjectBuilderByProto(proto *ypb.SSAProject) (*SSAProjectBuilder, error) {
	if proto == nil {
		return nil, utils.Errorf("failed to new SSA project builder: proto is nil")
	}
	builder := &SSAProjectBuilder{
		ID:          uint(proto.ID),
		ProjectName: proto.ProjectName,
		Description: proto.Description,
		Tags:        proto.Tags,
		// Language:     proto.Language,
	}
	if language, err := ssaconfig.ValidateLanguage(proto.Language); err != nil {
		return nil, utils.Errorf("failed to new SSA project builder: invalid language %s", proto.Language)
	} else {
		builder.Language = language
	}
	var err error
	builder.Config, err = ssaconfig.New(ssaconfig.ModeProjectBase)
	if err != nil {
		return nil, err
	}

	if proto.CodeSourceConfig != "" {
		json.Unmarshal([]byte(proto.CodeSourceConfig), builder.Config.CodeSource)
	}

	var opts []ssaconfig.Option
	if cc := proto.CompileConfig; cc != nil {
		opts = append(opts, ssaconfig.WithCompileStrictMode(cc.StrictMode))
		opts = append(opts, ssaconfig.WithCompilePeepholeSize(int(cc.PeepholeSize)))
		opts = append(opts, ssaconfig.WithCompileExcludeFiles(cc.ExcludeFiles))
		opts = append(opts, ssaconfig.WithCompileReCompile(cc.ReCompile))
		opts = append(opts, ssaconfig.WithCompileMemoryCompile(cc.Memory))
		opts = append(opts, ssaconfig.WithCompileConcurrency(int(cc.Concurrency)))
	}
	if sc := proto.ScanConfig; sc != nil {
		opts = append(opts, ssaconfig.WithScanConcurrency(sc.Concurrency))
		opts = append(opts, ssaconfig.WithSyntaxFlowMemory(sc.Memory))
		opts = append(opts, ssaconfig.WithScanIgnoreLanguage(sc.IgnoreLanguage))
	}
	if rc := proto.RuleConfig; rc != nil && rc.RuleFilter != nil {
		opts = append(opts, ssaconfig.WithRuleFilter(rc.RuleFilter))
	}
	if proto.CodeSourceConfig != "" {
		opts = append(opts, ssaconfig.WithCodeSourceJson(proto.CodeSourceConfig))
	}
	config, err := ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return nil, utils.Errorf("failed to new SSA project config: %s", err)
	}
	builder.Config = config
	if err := builder.Validate(); err != nil {
		return nil, utils.Errorf("failed to validate SSA project builder: %s", err)
	}
	return builder, nil
}

func (s *SSAProjectBuilder) ToSchemaSSAProject() (*schema.SSAProject, error) {
	if s == nil {
		return nil, utils.Errorf("to schema SSA project failed: ssa project builder is nil")
	}
	var result schema.SSAProject
	result.ID = s.ID
	result.ProjectName = s.ProjectName
	result.Description = s.Description
	result.Language = s.Language
	result.SetTagsList(s.Tags)
	err := result.SetConfig(s.Config)
	if err != nil {
		return nil, utils.Errorf("to schema SSA project failed: %s", err)
	}
	return &result, nil
}

func (s *SSAProjectBuilder) Save() error {
	if s == nil {
		return utils.Errorf("save SSA project failed: ssa project builder is nil")
	}
	schemaProject, err := s.ToSchemaSSAProject()
	if err != nil {
		return err
	}

	db := consts.GetGormProfileDatabase()
	var existingProject schema.SSAProject
	err = db.Where("project_name = ?", schemaProject.ProjectName).First(&existingProject).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return db.Create(schemaProject).Error
		} else {
			return utils.Errorf("check project existence failed: %s", err)
		}
	}
	err = db.Model(&existingProject).Updates(schemaProject).Error
	if err != nil {
		return utils.Errorf("update SSA project failed: %s", err)
	}
	return nil
}

func (s *SSAProjectBuilder) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	return s.Config.GetRuleFilter()
}

func (s *SSAProjectBuilder) Validate() error {
	if s == nil {
		return utils.Errorf("validate SSA project failed: ssa project builder is nil")
	}
	if s.ProjectName == "" {
		return utils.Errorf("validate SSA project failed: project name is required")
	}
	if s.Language == "" {
		return utils.Errorf("validate SSA project failed: language is required")
	}
	if s.Config.CodeSource == nil {
		return utils.Errorf("validate SSA project failed: code source config is required")
	}
	return nil
}

func (s *SSAProjectBuilder) GetCompileConfig() *ssaconfig.SSACompileConfig {
	if s == nil {
		return nil
	}
	if s.Config == nil {
		return nil
	}
	return s.Config.SSACompile
}

func (s *SSAProjectBuilder) GetScanConfig() *ssaconfig.SyntaxFlowConfig {
	if s == nil {
		return nil
	}
	if s.Config == nil {
		return nil
	}
	return s.Config.SyntaxFlow
}

func NewSSAProjectBuilder(opts ...ssaconfig.Option) (*SSAProjectBuilder, error) {
	config, err := ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return nil, utils.Errorf("failed to new SSA project builder: %s", err)
	}
	builder := &SSAProjectBuilder{
		ProjectName: config.GetProjectName(),
		Description: config.GetProjectDescription(),
		Tags:        config.GetTags(),
		Language:    config.GetLanguage(),
		Config:      config,
	}
	if err := builder.Validate(); err != nil {
		return nil, utils.Errorf("failed to validate SSA project builder: %s", err)
	}
	return builder, nil
}

func loadSSAProjectBySchema(project *schema.SSAProject) (*SSAProjectBuilder, error) {
	builder := &SSAProjectBuilder{
		ID:          project.ID,
		ProjectName: project.ProjectName,
		Description: project.Description,
		Tags:        project.GetTagsList(),
		Language:    project.Language,
	}
	config, err := project.GetConfig()
	if err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	builder.Config = config
	return builder, nil
}

func LoadSSAProjectBuilderByName(projectName string) (*SSAProjectBuilder, error) {
	db := consts.GetGormProfileDatabase()
	var project schema.SSAProject
	if err := db.Where("project_name = ?", projectName).First(&project).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}

func LoadSSAProjectBuilderByID(id uint) (*SSAProjectBuilder, error) {
	db := consts.GetGormProfileDatabase()
	var project schema.SSAProject
	if err := db.Where("id = ?", id).First(&project).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}
