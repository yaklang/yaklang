package ssaproject

import (
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSAProject struct {
	ID          uint64
	ProjectName string
	Description string
	Tags        []string
	URL         string
	Language    ssaconfig.Language
	Config      *ssaconfig.Config
}

func (s *SSAProject) setConfig(config *ssaconfig.Config) {
	s.Config = config
	s.coverBaseInfo()
}

func (s *SSAProject) toSchemaData() (*schema.SSAProject, error) {
	if s == nil {
		return nil, utils.Errorf("to schema SSA project failed: ssa project builder is nil")
	}
	var result schema.SSAProject
	result.ID = uint(s.ID)
	result.ProjectName = s.ProjectName
	result.Description = s.Description
	result.Language = s.Language
	result.SetTagsList(s.Tags)
	result.URL = s.URL
	err := result.SetConfig(s.Config)
	if err != nil {
		return nil, utils.Errorf("to schema SSA project failed: %s", err)
	}
	return &result, nil
}

func (s *SSAProject) Save(options ...ssaconfig.Option) (err error) {
	var schemaProject *schema.SSAProject
	defer func() {
		if err != nil {
			return
		}
		// 更新或者创建成功时，更新项目配置（因为ID可能会变化）
		config, err := schemaProject.GetConfig()
		if err != nil {
			return
		}
		s.setConfig(config)
	}()

	if s == nil {
		return utils.Errorf("save SSA project failed: ssa project builder is nil")
	}

	db := consts.GetGormProfileDatabase()
	config := s.Config
	for _, opt := range options {
		err := opt(config)
		if err != nil {
			return err
		}
	}
	s.setConfig(config)
	schemaProject, err = s.toSchemaData()
	if err != nil {
		return err
	}
	// just create
	if schemaProject.ID == 0 {
		err = db.Create(schemaProject).Error
		return err
	}

	// update
	var existingProject schema.SSAProject
	err = db.First(&existingProject, schemaProject.ID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = db.Create(schemaProject).Error
			return err
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

func (s *SSAProject) coverBaseInfo() {
	if s == nil || s.Config == nil {
		return
	}
	s.ProjectName = s.Config.GetProjectName()
	s.Description = s.Config.GetProjectDescription()
	s.Tags = s.Config.GetTags()
	s.Language = s.Config.GetLanguage()
	s.ID = s.Config.GetProjectID()

	if localFile := s.Config.GetCodeSourceLocalFile(); localFile != "" {
		s.URL = localFile
	}
	if url := s.Config.GetCodeSourceURL(); url != "" {
		s.URL = url
	}
}

func (s *SSAProject) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	return s.Config.GetRuleFilter()
}

func (s *SSAProject) Validate() error {
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

func (s *SSAProject) GetCompileConfig() *ssaconfig.SSACompileConfig {
	if s == nil {
		return nil
	}
	if s.Config == nil {
		return nil
	}
	return s.Config.SSACompile
}

func (s *SSAProject) GetScanConfig() *ssaconfig.SyntaxFlowConfig {
	if s == nil {
		return nil
	}
	if s.Config == nil {
		return nil
	}
	return s.Config.SyntaxFlow
}

func NewSSAProject(opts ...ssaconfig.Option) (*SSAProject, error) {
	config, err := ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return nil, utils.Errorf("failed to new SSA project builder: %s", err)
	}
	builder := &SSAProject{}
	builder.setConfig(config)
	//if err := builder.Validate(); err != nil {
	//	return nil, utils.Errorf("failed to validate SSA project builder: %s", err)
	//}
	return builder, nil
}

func loadSSAProjectBySchema(project *schema.SSAProject) (*SSAProject, error) {
	builder := &SSAProject{
		ID:          uint64(project.ID),
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

func LoadSSAProjectByName(projectName string) (*SSAProject, error) {
	db := consts.GetGormProfileDatabase()
	var project schema.SSAProject
	if err := db.Where("project_name = ?", projectName).First(&project).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}

func LoadSSAProjectByID(id uint) (*SSAProject, error) {
	db := consts.GetGormProfileDatabase()
	var project schema.SSAProject
	if err := db.Where("id = ?", id).First(&project).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}

func LoadSSAProjectByNameAndURL(projectName, url string) (*SSAProject, error) {
	db := consts.GetGormProfileDatabase()
	var project schema.SSAProject
	err := db.Where("project_name = ? AND url = ?", projectName, url).First(&project).Error
	if err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}

func (s *SSAProject) GetConfig() (*ssaconfig.Config, error) {
	if s == nil {
		return nil, utils.Errorf("get SSA project config failed: ssa project builder is nil")
	}
	if s.Config == nil {
		return nil, utils.Errorf("get SSA project config failed: config is nil")
	}
	return s.Config, nil
}
