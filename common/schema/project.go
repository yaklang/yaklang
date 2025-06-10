package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Project 描述一个 Yakit 项目
// 一般项目数据都是应该用 ProjectDatabase 作为连接的
// 但是项目本身的元数据应该存在 ProfileDatabase 中
type Project struct {
	gorm.Model

	ProjectName string
	Description string

	DatabasePath string

	IsCurrentProject bool
	FolderID         int64
	ChildFolderID    int64
	Type             string
	// Hash string `gorm:"unique_index"`
	// 企业版 项目模块及项目编号
	ExternalModule      string
	ExternalProjectCode string
}

type BackProject struct {
	Project
	FolderName      string `json:"folder_name"`
	ChildFolderName string `json:"child_folder_name"`
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
