package yakit

import (
	"errors"

	"github.com/yaklang/yaklang/common/yak/ssaproject"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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
	} else if req.JSONStringConfig != "" {
		projectBuilder, err = ssaproject.NewSSAProjectByRawConfigData([]byte(req.JSONStringConfig))
	} else {
		err = utils.Errorf("create SSA project failed: request project and JSONStringConfig are both empty")
	}
	if err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	if projectBuilder == nil {
		return nil, utils.Errorf("create SSA project failed: project builder is nil")
	}
	schemaProject, err := SSAProjectToSchemaData(projectBuilder)
	err = db.Model(&schema.SSAProject{}).Create(schemaProject).Error
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
	schemaProject, err := SSAProjectToSchemaData(projectBuilder)

	var existingProject schema.SSAProject
	err = db.First(&existingProject, schemaProject.ID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.Create(schemaProject).Error
		} else {
			return nil, utils.Errorf("check project existence failed: %s", err)
		}
	}
	err = db.Model(&existingProject).Updates(schemaProject).Error
	if err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	return schemaProject, nil
}

type DeleteSSAProjectMode string

const (
	SSAProjectClearCompileHistory DeleteSSAProjectMode = "clear_compile_history"
	SSAProjectDeleteAll           DeleteSSAProjectMode = "delete_all"
)

func DeleteSSAProject(db *gorm.DB, req *ypb.DeleteSSAProjectRequest) (int64, error) {
	if req == nil || req.Filter == nil {
		return 0, utils.Errorf("delete SSA project failed: filter is nil")
	}

	query := FilterSSAProject(db, req.Filter)
	var projects []*schema.SSAProject
	if err := query.Find(&projects).Error; err != nil {
		return 0, utils.Errorf("query SSA projects failed: %s", err)
	}

	if len(projects) == 0 {
		return 0, nil
	}
	ssaDB := consts.GetGormDefaultSSADataBase()
	deleteMode := req.GetDeleteMode()
	var totalDeleted int64
	for _, project := range projects {
		programFilter := &ypb.SSAProgramFilter{
			ProjectIds: []int64{int64(project.ID)},
		}
		count, err := DeleteSSAProgram(ssaDB, programFilter)
		if err != nil {
			log.Errorf("delete SSA programs for project %d failed: %s", project.ID, err)
			continue
		}
		switch deleteMode {
		case string(SSAProjectClearCompileHistory):
			totalDeleted += int64(count)
		default:
			result := db.Model(&schema.SSAProject{}).Where("id = ?", project.ID).Unscoped().Delete(&schema.SSAProject{})
			if result.Error != nil {
				log.Errorf("delete SSA project %d failed: %s", project.ID, result.Error)
				continue
			}
			totalDeleted += result.RowsAffected
		}
	}
	return totalDeleted, nil
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

func GetSSAProjectById(id uint64) (*schema.SSAProject, error) {
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

func GetSSAProjectByName(name string) (*schema.SSAProject, error) {
	if name == "" {
		return nil, utils.Errorf("get SSA project failed: name is required")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SSAProject{})
	var project schema.SSAProject
	db = db.Where("project_name = ?", name).First(&project)
	if db.Error != nil {
		return nil, utils.Errorf("get SSA project failed: %s", db.Error)
	}
	return &project, nil
}

func GetSSAProjectIDByName(name string) (uint64, error) {
	if name == "" {
		return 0, utils.Errorf("get SSA project ID failed: name is required")
	}
	project, err := GetSSAProjectByName(name)
	if err != nil {
		return 0, utils.Errorf("get SSA project ID failed: %s", err)
	}
	return uint64(project.ID), nil
}
