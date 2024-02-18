package yakit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	INIT_DATABASE_RECORD_NAME = "[default]"
	FolderID                  = 0
	ChildFolderID             = 0
	TypeProject               = "project"
	TypeFile                  = "file"
	TEMPORARY_PROJECT_NAME    = "[temporary]"
	MIGRATE_DATABASE_KEY      = "__migrate_database__"
)

func InitializingProjectDatabase() error {
	profileDB := consts.GetGormProfileDatabase()
	profileDB.Model(&Project{}).RemoveIndex("uix_projects_project_name")
	defaultProj, _ := GetDefaultProject(profileDB)

	defaultYakitPath := consts.GetDefaultYakitBaseDir()
	log.Debugf("Yakit base directory: %s", defaultYakitPath)
	homeYakitPath := filepath.Join(utils.GetHomeDirDefault("."), "yakit-projects")
	defaultDBPath := consts.GetDefaultYakitProjectDatabase(defaultYakitPath)
	// 需要迁移所有yakit-projects/projects
	if defaultYakitPath != homeYakitPath && GetKey(profileDB, MIGRATE_DATABASE_KEY) == "" {
		log.Debugf("migrate project database path from %s to %s", homeYakitPath, defaultYakitPath)
		SetKey(profileDB, MIGRATE_DATABASE_KEY, true)
		projCh := YieldProject(profileDB, context.Background())
		for proj := range projCh {
			if proj.ProjectName == "[default]" || !utils.IsSubPath(proj.DatabasePath, homeYakitPath) {
				continue
			}
			filename := filepath.Base(proj.DatabasePath)
			err := UpdateProjectDatabasePath(profileDB, int64(proj.ID), filepath.Join(defaultYakitPath, "projects", filename))
			if err != nil {
				log.Errorf("migrate project %s failed: %s", proj.ProjectName, err)
			}
		}
	}

	// 迁移默认数据库
	if defaultProj == nil || defaultProj.DatabasePath != defaultDBPath {
		if defaultProj != nil {
			log.Debugf("migrate default database path from %s to %s", defaultProj.DatabasePath, defaultDBPath)
		}
		projectData := &Project{
			ProjectName:   INIT_DATABASE_RECORD_NAME,
			Description:   "默认数据库(~/yakit-projects/***.db): Default Database!",
			DatabasePath:  defaultDBPath,
			FolderID:      FolderID,
			ChildFolderID: ChildFolderID,
			Type:          TypeProject,
		}
		err := CreateOrUpdateProject(profileDB, INIT_DATABASE_RECORD_NAME, FolderID, ChildFolderID, TypeProject, projectData)
		if err != nil {
			log.Errorf("create default database file failed: %s", err)
		}
	}
	return nil
}

func init() {
	// 一开始应该创建一个最基础的数据库
	RegisterPostInitDatabaseFunction(func() error {
		return InitializingProjectDatabase()
	})
}

// Project 描述一个 Yakit 项目
// 一般项目数据都是应该用 ProjectDatabase 作为连接的
// 但是项目本身的元数据应该存在 ProfileDatabase 中
type Project struct {
	gorm.Model

	ProjectName  string
	Description  string
	DatabasePath string

	IsCurrentProject bool
	FolderID         int64
	ChildFolderID    int64
	Type             string
	// Hash string `gorm:"unique_index"`
}

type BackProject struct {
	Project
	FolderName      string
	ChildFolderName string
}

func (p *Project) ToGRPCModel() *ypb.ProjectDescription {
	db := consts.GetGormProfileDatabase()
	var folderName, childFolderName string
	if p.FolderID > 0 {
		folder, _ := GetProjectById(db, p.FolderID, TypeFile)
		if folder != nil {
			folderName = folder.ProjectName
		}
	}
	if p.ChildFolderID > 0 {
		childFolder, _ := GetProjectById(db, p.ChildFolderID, TypeFile)
		if childFolder != nil {
			childFolderName = childFolder.ProjectName
		}
	}
	var fileSize string
	fileInfo, _ := os.Stat(p.DatabasePath)
	if fileInfo == nil {
		fileSize = formatFileSize(0)
	} else {
		fileSize = formatFileSize(fileInfo.Size())
	}
	return &ypb.ProjectDescription{
		ProjectName:     p.ProjectName,
		Description:     p.Description,
		Id:              int64(p.ID),
		DatabasePath:    p.DatabasePath,
		CreatedAt:       p.CreatedAt.Unix(),
		FolderId:        p.FolderID,
		ChildFolderId:   p.ChildFolderID,
		Type:            p.Type,
		UpdateAt:        p.UpdatedAt.Unix(),
		FolderName:      folderName,
		ChildFolderName: childFolderName,
		FileSize:        fileSize,
	}
}

func formatFileSize(size int64) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
		tb = 1 << 40
	)
	switch {
	case size < kb:
		return fmt.Sprintf("%d B", size)
	case size < mb:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(kb))
	case size < gb:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(mb))
	case size < tb:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(gb))
	default:
		return fmt.Sprintf("%.2f TB", float64(size)/float64(tb))
	}
}

func (p *BackProject) BackGRPCModel() *ypb.ProjectDescription {
	return &ypb.ProjectDescription{
		ProjectName:     utils.EscapeInvalidUTF8Byte([]byte(p.ProjectName)),
		Description:     utils.EscapeInvalidUTF8Byte([]byte(p.Description)),
		Id:              int64(p.ID),
		DatabasePath:    utils.EscapeInvalidUTF8Byte([]byte(p.DatabasePath)),
		CreatedAt:       p.CreatedAt.Unix(),
		FolderId:        p.FolderID,
		ChildFolderId:   p.ChildFolderID,
		Type:            p.Type,
		UpdateAt:        p.UpdatedAt.Unix(),
		FolderName:      p.FolderName,
		ChildFolderName: p.ChildFolderName,
	}
}

func (p *Project) CalcHash() string {
	return utils.CalcSha1(p.ProjectName, p.FolderID, p.ChildFolderID, p.Type)
}

func CreateOrUpdateProject(db *gorm.DB, name string, folderID, childFolderID int64, Type string, i interface{}) error {
	db = db.Model(&Project{})

	db = db.Where("project_name = ? and (folder_id = ? or folder_id IS NULL) and (child_folder_id = ? or child_folder_id IS NULL )", name, folderID, childFolderID)
	if Type == TypeFile {
		db = db.Where("type = ?", Type)
	} else {
		db = db.Where("type IS NULL or type = ?", Type)
	}
	db = db.Assign(i).FirstOrCreate(&Project{})
	if db.Error != nil {
		return utils.Errorf("create/update Project failed: %s", db.Error)
	}

	return nil
}

func GetProjectByID(db *gorm.DB, id int64) (*Project, error) {
	var req Project
	if db := db.Model(&Project{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
}

func GetProjectByName(db *gorm.DB, name string) (*Project, error) {
	var req Project
	if db := db.Model(&Project{}).Where("project_name = ?", name).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteProjectByProjectName(db *gorm.DB, name string) error {
	if db := db.Model(&Project{}).Where(
		"project_name = ?", name,
	).Unscoped().Delete(&Project{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteProjectByUid(db *gorm.DB, id string) error {
	if db := db.Model(&Project{}).Where(
		"uid = ?", id,
	).Unscoped().Delete(&Project{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryProject(db *gorm.DB, params *ypb.GetProjectsRequest) (*bizhelper.Paginator, []*Project, error) {
	db = db.Model(&Project{})
	db = db.Where("deleted_at = '' or deleted_at IS NULL ")
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination
	if p.GetOrder() == "" {
		p.Order = "desc"
	}
	if p.GetOrderBy() == "" {
		p.OrderBy = "updated_at"
	}
	if params.GetProjectName() != "" {
		db = bizhelper.FuzzQueryLike(db, "project_name", params.GetProjectName())
	} else {
		if params.FolderId > 0 {
			db = db.Where("folder_id = ? ", params.FolderId)
		} else {
			db = db.Where("folder_id IS NULL or folder_id = false")
		}
		if params.ChildFolderId > 0 {
			db = db.Where("child_folder_id = ?", params.ChildFolderId)
		} else {
			db = db.Where("child_folder_id IS NULL or child_folder_id = false")
		}
	}
	switch params.Type {
	case TypeFile:
		db = db.Where("type = ?", params.Type)
	case TypeProject:
		db = db.Where("type IS NULL or type = ?", params.Type)
	}
	db = db.Where(" NOT (project_name = ? AND folder_id = false AND child_folder_id = false AND type = 'project' )", TEMPORARY_PROJECT_NAME)
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)
	db = db.Unscoped()
	var ret []*Project
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func GetCurrentProject(db *gorm.DB) (*Project, error) {
	var proj Project
	if db1 := db.Model(&Project{}).Where("is_current_project = true").First(&proj); db1.Error != nil {
		var defaultProj Project
		if db2 := db.Model(&Project{}).Where("project_name = ?", INIT_DATABASE_RECORD_NAME).First(&defaultProj); db2.Error != nil {
			return nil, utils.Errorf("cannot found current project or default database: %s", db2.Error)
		}

		db.Model(&Project{}).Where("true").Update(map[string]interface{}{"is_current_project": false})
		db.Model(&Project{}).Where("project_name = ?", INIT_DATABASE_RECORD_NAME).Update(map[string]interface{}{
			"is_current_project": true,
		})

		return &defaultProj, nil
	}
	return &proj, nil
}

func SetCurrentProject(db *gorm.DB, name string) error {
	if db1 := db.Model(&Project{}).Where("true").Update(map[string]interface{}{
		"is_current_project": false,
	}); db1.Error != nil {
		log.Errorf("unset all projects current status: %s", db1.Error)
	}

	if db := db.Model(&Project{}).Where("project_name = ?", name).Update(map[string]interface{}{
		"is_current_project": true,
	}); db.Error != nil {
		db.Model(&Project{}).Where("project_name = ?", name).Update(map[string]interface{}{
			"is_current_project": false,
		})
		return utils.Errorf("cannot set current project: %s", db.Error)
	}
	return nil
}

func GetProject(db *gorm.DB, params *ypb.IsProjectNameValidRequest) (*Project, error) {
	var req Project
	db = db.Model(&Project{}).Where("project_name = ? ", params.ProjectName)
	db = db.Where("folder_id = ? or folder_id IS NULL", params.FolderId)
	db = db.Where("child_folder_id = ? or child_folder_id IS NULL", params.ChildFolderId)
	if params.Type == TypeFile {
		db = db.Where("type = ?", params.Type)
	} else {
		db = db.Where("type IS NULL or type = ?", params.Type)
	}
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
}

func QueryProjectTotal(db *gorm.DB, req *ypb.GetProjectsRequest) (*bizhelper.Paginator, error) {
	db = db.Model(&Project{})
	db = db.Where("deleted_at = '' or deleted_at IS NULL ")
	db = db.Unscoped()
	if req.Pagination == nil {
		req.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	params := req.Pagination
	db = db.Where("type IS NULL or type = ? ", TypeProject)
	db = db.Where(" NOT (project_name = ? AND folder_id = false AND child_folder_id = false AND type = 'project' )", TEMPORARY_PROJECT_NAME)
	var ret []*Project
	paging, db := bizhelper.Paging(db, int(params.Page), int(params.Limit), &ret)
	if db.Error != nil {
		return nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, nil
}

func GetProjectById(db *gorm.DB, id int64, Type string) (*Project, error) {
	var req Project
	db = db.Model(&Project{}).Where("id = ?", id)
	if Type == TypeFile {
		db = db.Where("type = ?", Type)
	} else {
		db = db.Where("type IS NULL or type = ?", Type)
	}
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
}

func YieldProject(db *gorm.DB, ctx context.Context) chan *Project {
	outC := make(chan *Project)
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*Project
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func SetCurrentProjectById(db *gorm.DB, id int64) error {
	if db1 := db.Model(&Project{}).Where("is_current_project = true").Update(map[string]interface{}{
		"is_current_project": false,
	}); db1.Error != nil {
		log.Errorf("unset all projects current status: %s", db1.Error)
	}

	if db := db.Model(&Project{}).Where("id = ?", id).Update(map[string]interface{}{
		"is_current_project": true,
	}); db.Error != nil {
		db.Model(&Project{}).Where("id = ?", id).Update(map[string]interface{}{
			"is_current_project": false,
		})
		return utils.Errorf("cannot set current project: %s", db.Error)
	}
	return nil
}

func DeleteProjectById(db *gorm.DB, id int64) error {
	db = db.Model(&Project{})
	db = db.Where("id = ?", id).Unscoped().Delete(&Project{})
	if db.Error != nil {
		return db.Error
	}
	return nil
}

func GetDefaultProject(db *gorm.DB) (*Project, error) {
	var req Project
	db = db.Model(&Project{})
	db = db.Where("folder_id = ? or folder_id IS NULL", 0)
	db = db.Where("child_folder_id = ? or child_folder_id IS NULL", 0)
	db = db.Where("type IS NULL or type = ?", TypeProject).Where("project_name = ?", INIT_DATABASE_RECORD_NAME)
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
}

func GetProjectDetail(db *gorm.DB, id int64) (*BackProject, error) {
	var req BackProject
	db = db.Model(&Project{})
	db = db.Select("projects.*, F.project_name as folder_name, C.project_name as child_folder_name")
	db = db.Where("projects.id = ? and (projects.type = ? or projects.type IS NULL)", id, TypeProject)
	db = db.Joins("left join projects F on projects.folder_id = F.id ")
	db = db.Joins("left join projects C on projects.child_folder_id = C.id ")
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
}

func GetProjectByWhere(db *gorm.DB, name string, folderID, childFolderID int64, Type string, id int64) (*Project, error) {
	var req Project
	db = db.Model(&Project{})
	db = db.Where("project_name = ? and (folder_id = ? or folder_id IS NULL) and (child_folder_id = ? or child_folder_id IS NULL )", name, folderID, childFolderID)
	if Type == TypeFile {
		db = db.Where("type = ?", Type)
	} else {
		db = db.Where("type IS NULL or type = ?", Type)
	}
	if id > 0 {
		db = db.Where("id <> ?", id)
	}
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}
	return &req, nil
}

func UpdateProject(db *gorm.DB, id int64, i Project) error {
	db = db.Model(&Project{}).Where("id = ?", id).Update(i)
	if db.Error != nil {
		return utils.Errorf("update project: %s", db.Error)
	}
	return nil
}

func UpdateProjectDatabasePath(db *gorm.DB, id int64, databasePath string) error {
	db = db.Model(&Project{}).Where("id = ?", id).Update("database_path", databasePath)
	if db.Error != nil {
		return utils.Errorf("update project: %s", db.Error)
	}
	return nil
}

func GetTemporaryProject(db *gorm.DB) (*Project, error) {
	var req Project
	db = db.Model(&Project{})
	db = db.Where("folder_id = ? or folder_id IS NULL", 0)
	db = db.Where("child_folder_id = ? or child_folder_id IS NULL", 0)
	db = db.Where("type IS NULL or type = ?", TypeProject).Where("project_name = ?", TEMPORARY_PROJECT_NAME)
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get temporary Project failed: %s", db.Error)
	}

	return &req, nil
}
