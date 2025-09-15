package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateSSAProject(db *gorm.DB, project *ypb.SSAProject) (*schema.SSAProject, error) {
	if project == nil {
		return nil, utils.Errorf("create SSA project failed: project is nil")
	}

	schemaProject := ProtoToSchemaSSAProject(project)
	if err := schemaProject.Validate(); err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	if err := db.Create(schemaProject).Error; err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	return schemaProject, nil
}

func UpdateSSAProject(db *gorm.DB, project *ypb.SSAProject) (*schema.SSAProject, error) {
	if project == nil {
		return nil, utils.Errorf("update SSA project failed: project is nil")
	}

	if project.ID <= 0 {
		return nil, utils.Errorf("update SSA project failed: project ID is required")
	}

	schemaProject := ProtoToSchemaSSAProject(project)

	if err := schemaProject.Validate(); err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}

	var existingProject schema.SSAProject
	if err := db.First(&existingProject, schemaProject.ID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, utils.Errorf("project with ID %d not found", schemaProject.ID)
		}
		return nil, utils.Errorf("check project existence failed: %s", err)
	}

	if err := db.Save(schemaProject).Error; err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}

	return schemaProject, nil
}

func DeleteSSAProject(db *gorm.DB, req *ypb.DeleteSSAProjectRequest) (int64, error) {
	if req == nil || req.Filter == nil {
		return 0, utils.Errorf("delete SSA project failed: filter is nil")
	}

	db = db.Model(&schema.SSAProject{})
	query := FilterSSAProject(db, req.Filter)

	result := query.Unscoped().Delete(&schema.SSAProject{})
	if result.Error != nil {
		return 0, utils.Errorf("delete SSA projects failed: %s", result.Error)
	}
	return result.RowsAffected, nil
}

func QuerySSAProject(db *gorm.DB, req *ypb.QuerySSAProjectRequest) (*bizhelper.Paginator, []*schema.SSAProject, error) {
	if req == nil {
		req = &ypb.QuerySSAProjectRequest{}
	}
	db = db.Model(&schema.SSAProject{})
	p := req.Pagination
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	db = bizhelper.OrderByPaging(db, p)
	db = FilterSSAProject(db, req.GetFilter())
	var projects []*schema.SSAProject
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &projects)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, projects, nil
}

func FilterSSAProject(db *gorm.DB, filter *ypb.SSAProjectFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	db = db.Model(&schema.SSAProject{})

	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetIDs())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "project_name", filter.GetProjectNames())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "language", filter.GetLanguages())

	if filter.GetSearchKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"project_name", "description", "tags",
		}, []string{filter.GetSearchKeyword()}, false)
	}
	return db
}

func QuerySSAProjectById(id uint64) (*schema.SSAProject, error) {
	if id == 0 {
		return nil, utils.Errorf("get SSA project failed: id is required")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SSAProject{})
	var project schema.SSAProject
	db = db.Where("id = ?", id).First(&project)
	if db.Error != nil {
		return nil, utils.Errorf("get SSA project failed: %s", db.Error)
	}
	return &project, nil
}

func ProtoToSchemaSSAProject(proto *ypb.SSAProject) *schema.SSAProject {
	if proto == nil {
		return nil
	}
	project := &schema.SSAProject{
		ProjectName:      proto.ProjectName,
		Language:         proto.Language,
		CodeSourceConfig: proto.CodeSourceConfig,
		Description:      proto.Description,
	}
	project.SetTagsList(proto.Tags)
	if proto.CompileConfig != nil {
		project.StrictMode = proto.CompileConfig.StrictMode
		project.PeepholeSize = int(proto.CompileConfig.PeepholeSize)
		project.ReCompile = proto.CompileConfig.ReCompile
		project.SetExcludeFilesList(proto.CompileConfig.ExcludeFiles)
	}
	if proto.ScanConfig != nil {
		project.ScanConcurrency = proto.ScanConfig.Concurrency
		project.MemoryScan = proto.ScanConfig.Memory
		project.IgnoreLanguage = proto.ScanConfig.IgnoreLanguage
	}
	if proto.RuleConfig != nil {
		project.SetRuleFilter(proto.RuleConfig.RuleFilter)
	}
	if proto.ID > 0 {
		project.ID = uint(proto.ID)
	}
	return project
}
