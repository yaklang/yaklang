package yakit

import (
	"github.com/yaklang/yaklang/common/yak/ssaproject"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func SSAProjectToSchemaData(s *ssaproject.SSAProject) (*schema.SSAProject, error) {
	if s == nil {
		return nil, utils.Errorf("to schema SSA project failed: SSA project is nil")
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

func CreateSSAProject(db *gorm.DB, req *ypb.CreateSSAProjectRequest) (*schema.SSAProject, error) {
	if req == nil {
		return nil, utils.Errorf("create SSA project failed: project is nil")
	}

	var projectBuilder *ssaproject.SSAProject
	var err error
	if req.Project != nil {
		projectBuilder, err = ssaproject.NewSSAProjectByProto(req.Project)
	} else if req.ProjectRawData != "" {
		projectBuilder, err = ssaproject.NewSSAProjectByRawData(req.ProjectRawData)
	} else {
		return nil, utils.Errorf("create SSA project failed: project data is missing")
	}
	if projectBuilder == nil {
		return nil, utils.Errorf("create SSA project failed: project builder is nil")
	}

	err = projectBuilder.Save(db)
	if err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	schemaProject, err := SSAProjectToSchemaData(projectBuilder)
	if err != nil {
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

	projectBuilder, err := ssaproject.NewSSAProjectByProto(project)
	if err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	if projectBuilder == nil {
		return nil, utils.Errorf("update SSA project failed: project builder is nil")
	}

	err = projectBuilder.Save(db)
	if err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	schemaProject, err := SSAProjectToSchemaData(projectBuilder)
	if err != nil {
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

	db = FilterSSAProject(db, req.GetFilter())
	projects := make([]*schema.SSAProject, 0)
	paging, db := bizhelper.YakitPagingQuery(db, p, &projects)
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
