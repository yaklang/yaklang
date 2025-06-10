package model

import (
	"fmt"
	"os"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	TypeFile    = "file"    // same as common/yakgrpc/yakit/projects.go
	TypeProject = "project" // same as common/yakgrpc/yakit/projects.go
)

func ToProjectGRPCModel(p *schema.Project, db *gorm.DB) *ypb.ProjectDescription {
	var folderName, childFolderName string
	if p.FolderID > 0 {
		folder, _ := GetProjectById(db, p.FolderID)
		if folder != nil {
			folderName = folder.ProjectName
		}
	}
	if p.ChildFolderID > 0 {
		childFolder, _ := GetProjectById(db, p.ChildFolderID)
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
		ProjectName:         p.ProjectName,
		Description:         p.Description,
		Id:                  int64(p.ID),
		DatabasePath:        p.DatabasePath,
		CreatedAt:           p.CreatedAt.Unix(),
		FolderId:            p.FolderID,
		ChildFolderId:       p.ChildFolderID,
		Type:                p.Type,
		UpdateAt:            p.UpdatedAt.Unix(),
		FolderName:          folderName,
		ChildFolderName:     childFolderName,
		FileSize:            fileSize,
		ExternalModule:      p.ExternalModule,
		ExternalProjectCode: p.ExternalProjectCode,
	}
}

func GetProjectById(db *gorm.DB, id int64) (*schema.Project, error) {
	var req schema.Project
	db = db.Model(&schema.Project{}).Where("id = ?", id)
	db = db.First(&req)
	if db.Error != nil {
		return nil, utils.Errorf("get Project failed: %s", db.Error)
	}

	return &req, nil
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
