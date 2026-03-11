package ssaproject

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSAProject struct {
	Config *ssaconfig.Config
	*schema.SSAProject
}

func (s *SSAProject) setConfig(config *ssaconfig.Config) {
	s.Config = config
}

func (s *SSAProject) fillSchemaProjectFromConfig(project *schema.SSAProject) error {
	if s == nil || s.Config == nil {
		return utils.Errorf("fill schema SSA project failed: project or config is nil")
	}
	if project == nil {
		return utils.Errorf("fill schema SSA project failed: schema project is nil")
	}
	project.ProjectName = s.Config.GetProjectName()
	project.Description = s.Config.GetProjectDescription()
	project.Language = s.Config.GetLanguage()
	project.URL = s.Config.GetCodeSourceLocalFileOrURL()
	project.SetTagsList(s.Config.GetTags())
	if err := project.SetConfig(s.Config); err != nil {
		return utils.Errorf("fill schema SSA project failed: %s", err)
	}
	return nil
}

func (s *SSAProject) loadSchemaProjectByID(db *gorm.DB, id uint) (*schema.SSAProject, error) {
	if db == nil {
		return nil, utils.Errorf("load SSA project failed: db is nil")
	}
	if id == 0 {
		return nil, utils.Errorf("load SSA project failed: id is required")
	}
	var project schema.SSAProject
	if err := db.First(&project, id).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return &project, nil
}

func (s *SSAProject) UpdateConfig(options ...ssaconfig.Option) (err error) {
	if s == nil {
		return utils.Errorf("update SSA project config failed: ssa project builder is nil")
	}
	config, err := s.GetConfig()
	if err != nil {
		return err
	}
	for _, opt := range options {
		err := opt(config)
		if err != nil {
			return err
		}
	}
	s.setConfig(config)
	return nil
}

func (s *SSAProject) SaveToDB(dbs ...*gorm.DB) (err error) {
	var db *gorm.DB
	if len(dbs) > 0 {
		db = dbs[0]
	} else {
		db = consts.GetGormProfileDatabase()
	}

	if s == nil {
		return utils.Errorf("save SSA project failed: ssa project builder is nil")
	}
	if s.Config == nil {
		return utils.Errorf("save SSA project failed: config is nil")
	}

	// Resolve schema project from DB; schema.ID is the source of truth for project_id.
	if s.SSAProject == nil || s.SSAProject.ID == 0 {
		if configProjectID := s.Config.GetProjectID(); configProjectID > 0 {
			project, err := s.loadSchemaProjectByID(db, uint(configProjectID))
			if err != nil {
				return err
			}
			s.SSAProject = project
		}
	} else {
		project, err := s.loadSchemaProjectByID(db, s.SSAProject.ID)
		if err != nil {
			return err
		}
		s.SSAProject = project
	}

	// Existing project: force config.project_id from schema.ID.
	if s.SSAProject != nil && s.SSAProject.ID > 0 {
		if err := s.Config.Update(ssaconfig.WithProjectID(uint64(s.SSAProject.ID))); err != nil {
			return utils.Errorf("update SSA project config failed: %s", err)
		}
	}

	// Create
	if s.SSAProject == nil || s.SSAProject.ID == 0 {
		s.SSAProject = &schema.SSAProject{}
		if err := s.fillSchemaProjectFromConfig(s.SSAProject); err != nil {
			return err
		}
		if err := db.Create(s.SSAProject).Error; err != nil {
			return err
		}

		// Persist authoritative project_id into stored config.
		if err := s.Config.Update(ssaconfig.WithProjectID(uint64(s.SSAProject.ID))); err != nil {
			return utils.Errorf("update SSA project config failed: %s", err)
		}
		if err := s.SSAProject.SetConfig(s.Config); err != nil {
			return utils.Errorf("update SSA project config failed: %s", err)
		}
		if err := db.Model(s.SSAProject).Update("config", s.SSAProject.Config).Error; err != nil {
			return utils.Errorf("update SSA project config failed: %s", err)
		}
		return nil
	}

	// Update
	if err := s.fillSchemaProjectFromConfig(s.SSAProject); err != nil {
		return err
	}
	if err := db.Save(s.SSAProject).Error; err != nil {
		return utils.Errorf("update SSA project failed: %s", err)
	}
	return nil
}

func (s *SSAProject) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	if s == nil || s.Config == nil {
		return nil
	}
	return s.Config.GetRuleFilter()
}

func (s *SSAProject) GetTags() []string {
	if s == nil || s.Config == nil {
		return nil
	}
	return s.Config.GetTags()
}

func (s *SSAProject) Validate() error {
	if s == nil {
		return utils.Errorf("validate SSA project failed: ssa project builder is nil")
	}
	if s.Config == nil {
		return utils.Errorf("validate SSA project failed: config is nil")
	}
	if s.Config.GetProjectName() == "" {
		return utils.Errorf("validate SSA project failed: project name is required")
	}
	if s.Config.GetLanguage() == "" {
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
	config, err := project.GetConfig()
	if err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return &SSAProject{
		Config:     config,
		SSAProject: project,
	}, nil
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
