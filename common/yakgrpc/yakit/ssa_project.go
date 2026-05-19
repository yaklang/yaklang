package yakit

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateSSAProject(db *gorm.DB, req *ypb.CreateSSAProjectRequest) (*schema.SSAProject, error) {
	if req == nil {
		return nil, utils.Errorf("create SSA project failed: project is nil")
	}

	var projectBuilder *ssaproject.SSAProject
	var err error
	if req.Project != nil {
		projectBuilder, err = NewSSAProjectByProto(req.Project)
	} else if req.JSONStringConfig != "" {
		projectBuilder, err = ssaproject.NewSSAProject(ssaconfig.WithJsonRawConfig([]byte(req.JSONStringConfig)))
	} else {
		err = utils.Errorf("create SSA project failed: request project and JSONStringConfig are both empty")
	}
	if err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	if projectBuilder == nil {
		return nil, utils.Errorf("create SSA project failed: project builder is nil")
	}

	if existing, err := loadExistingSSAProjectForCreate(db, projectBuilder); err != nil {
		return nil, err
	} else if existing != nil {
		return finalizeSSAProjectAfterCreate(db, existing)
	}

	err = projectBuilder.SaveToDB(db)
	if err != nil {
		if isSSAProjectDuplicateDBError(err) {
			if existing, loadErr := loadExistingSSAProjectForCreate(db, projectBuilder); loadErr != nil {
				return nil, loadErr
			} else if existing != nil {
				return finalizeSSAProjectAfterCreate(db, existing)
			}
		}
		return nil, utils.Errorf("save SSA project failed: %s", err)
	}
	if projectBuilder.SSAProject == nil {
		return nil, utils.Errorf("create SSA project failed: schema project is nil")
	}
	return finalizeSSAProjectAfterCreate(db, projectBuilder.SSAProject)
}

func loadExistingSSAProjectForCreate(profileDB *gorm.DB, builder *ssaproject.SSAProject) (*schema.SSAProject, error) {
	if builder == nil || builder.Config == nil {
		return nil, nil
	}
	if profileDB == nil {
		profileDB = consts.GetGormProfileDatabase()
	}

	if id := builder.Config.GetProjectID(); id > 0 {
		project, err := GetSSAProjectById(id)
		if err == nil {
			return project, nil
		}
	}

	projectName := builder.Config.GetProjectName()
	codeURL := builder.Config.GetCodeSourceLocalFileOrURL()
	if projectName != "" && codeURL != "" {
		if existing, err := ssaproject.LoadSSAProjectByNameAndURL(projectName, codeURL); err == nil && existing != nil {
			return existing.SSAProject, nil
		}
	}
	if projectName != "" {
		if existing, err := ssaproject.LoadSSAProjectByName(projectName); err == nil && existing != nil {
			return existing.SSAProject, nil
		}
	}

	hash := utils.CalcMd5(codeURL, projectName)
	if hash == "" {
		return nil, nil
	}
	var project schema.SSAProject
	if err := profileDB.Where("hash = ?", hash).First(&project).Error; err != nil {
		return nil, nil
	}
	return &project, nil
}

func isSSAProjectDuplicateDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "constraint failed")
}

func finalizeSSAProjectAfterCreate(profileDB *gorm.DB, project *schema.SSAProject) (*schema.SSAProject, error) {
	if project == nil || project.ID == 0 {
		return nil, utils.Errorf("create SSA project failed: project is nil or has no id")
	}
	if err := ensureSSAProjectDatabaseBound(profileDB, project); err != nil {
		return nil, err
	}
	SetCurrentSSAProjectID(profileDB, uint64(project.ID))
	return project, nil
}

func ensureSSAProjectDatabaseBound(profileDB *gorm.DB, project *schema.SSAProject) error {
	if project == nil || project.ID == 0 {
		return utils.Errorf("bind SSA project database failed: project is nil or has no id")
	}
	if project.DatabasePath == "" {
		return BindSSAProjectDatabase(profileDB, project)
	}
	return OpenSSAProjectDatabase(project)
}

func UpdateSSAProject(db *gorm.DB, project *ypb.SSAProject) (*schema.SSAProject, error) {
	if project == nil {
		return nil, utils.Errorf("update SSA project failed: project is nil")
	}

	if project.ID <= 0 {
		return nil, utils.Errorf("update SSA project failed: project ID is required")
	}

	projectBuilder, err := NewSSAProjectByProto(project)
	if err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	if projectBuilder == nil {
		return nil, utils.Errorf("update SSA project failed: project builder is nil")
	}

	err = projectBuilder.SaveToDB(db)
	if err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	if projectBuilder.SSAProject == nil {
		return nil, utils.Errorf("update SSA project failed: schema project is nil")
	}
	return projectBuilder.SSAProject, nil
}

type DeleteSSAProjectMode string

const (
	SSAProjectClearCompileHistory DeleteSSAProjectMode = "clear_compile_history"
	SSAProjectDeleteAll           DeleteSSAProjectMode = "delete_all"
)

func DeleteSSAProject(db *gorm.DB, req *ypb.DeleteSSAProjectRequest) (int64, error) {
	if req == nil {
		return 0, utils.Errorf("delete SSA project failed: request is nil")
	}

	deleteAll := req.GetDeleteAllProject()

	if !deleteAll && req.Filter == nil {
		return 0, utils.Errorf("delete SSA project failed: filter is nil")
	}

	var query *gorm.DB
	if deleteAll {
		query = db.Model(&schema.SSAProject{})
	} else {
		query = FilterSSAProject(db, req.Filter)
	}

	var projects []*schema.SSAProject
	if err := query.Find(&projects).Error; err != nil {
		return 0, utils.Errorf("query SSA projects failed: %s", err)
	}

	if len(projects) == 0 {
		return 0, nil
	}

	deleteMode := req.GetDeleteMode()
	if deleteMode == "" {
		deleteMode = string(SSAProjectDeleteAll)
	}

	var totalDeleted int64
	var failReasons []string

	for _, project := range projects {
		if err := EnsureSSAProjectDatabaseOpen(uint64(project.ID)); err != nil {
			failReasons = append(failReasons, fmt.Sprintf("%s(%d): %s", project.ProjectName, project.ID, err))
			continue
		}

		var err error
		switch deleteMode {
		case string(SSAProjectClearCompileHistory):
			err = resetDedicatedSSAProjectDatabase(db, project)
		default:
			err = deleteSSAProjectFully(db, project)
		}
		if err != nil {
			log.Errorf("delete SSA project %d failed: %s", project.ID, err)
			failReasons = append(failReasons, fmt.Sprintf("%s(%d): %s", project.ProjectName, project.ID, err))
			continue
		}
		totalDeleted++
	}

	if len(failReasons) > 0 {
		msg := strings.Join(failReasons, "; ")
		if totalDeleted == 0 {
			return 0, utils.Errorf("delete SSA project failed: %s", msg)
		}
		return totalDeleted, utils.Errorf("delete SSA project partially failed: %s", msg)
	}
	return totalDeleted, nil
}

func deleteSSAProjectFully(profileDB *gorm.DB, project *schema.SSAProject) error {
	if project == nil {
		return utils.Errorf("delete SSA project failed: project is nil")
	}
	dbPath := ResolveSSAProjectDatabasePath(project)
	dedicated := project.DatabasePath != "" && !IsDefaultSSADatabasePath(dbPath)
	if !dedicated {
		programFilter := &ypb.SSAProgramFilter{
			ProjectIds: []uint64{uint64(project.ID)},
		}
		if _, err := DeleteSSAProgram(consts.GetGormSSAProjectDataBase(), programFilter); err != nil {
			return utils.Errorf("delete SSA programs failed: %s", err)
		}
	}
	if err := removeDedicatedSSAProjectDatabaseFile(profileDB, project, true); err != nil {
		return err
	}
	return deleteSSAProjectRecord(profileDB, project)
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

func NewSSAProjectByProto(proto *ypb.SSAProject) (*ssaproject.SSAProject, error) {
	if proto == nil {
		return nil, utils.Errorf("failed to new SSA project builder: proto is nil")
	}

	var language ssaconfig.Language
	language, err := ssaconfig.ValidateLanguage(proto.Language)
	if err != nil {
		return nil, err
	}

	var opts []ssaconfig.Option
	if cc := proto.CompileConfig; cc != nil {
		opts = append(opts, ssaconfig.WithCompileStrictMode(cc.StrictMode))
		opts = append(opts, ssaconfig.WithCompilePeepholeSize(int(cc.PeepholeSize)))
		opts = append(opts, ssaconfig.WithCompileExcludeFiles(cc.ExcludeFiles...))
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

	opts = append(opts, []ssaconfig.Option{
		ssaconfig.WithProjectID(uint64(proto.ID)),
		ssaconfig.WithProjectName(proto.ProjectName),
		ssaconfig.WithProjectLanguage(language),
		ssaconfig.WithProgramDescription(proto.Description),
		ssaconfig.WithProjectTags(proto.Tags),
	}...)

	return ssaproject.NewSSAProject(opts...)
}
