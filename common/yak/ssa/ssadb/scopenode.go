package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type ScopeNode struct {
	gorm.Model

	ParentNodeId  int64      `json:"parent_node_id" gorm:"index"`
	ChildrenNodes Int64Slice `json:"children" gorm:"type:text"`
	ExtraInfo     string     `json:"extraInfo"`
}

var migrateTreeNodeOnce = new(sync.Once)

func migrateTreeNode(db *gorm.DB) {
	migrateTreeNodeOnce.Do(func() {
		db.AutoMigrate(&ScopeNode{})
	})
}

func RequireScopeNode() (int64, *ScopeNode) {
	db := consts.GetGormProjectDatabase()
	migrateTreeNode(db)
	treeNode := &ScopeNode{}
	db.Create(&treeNode)
	return int64(treeNode.ID), treeNode
}

func GetTreeNode(id int64) (*ScopeNode, error) {
	db := consts.GetGormProjectDatabase()
	migrateTreeNode(db)
	treeNode := &ScopeNode{}
	if db := db.First(treeNode, id); db.Error != nil {
		return nil, utils.Errorf("failed to get tree node: %v", db.Error)
	}
	return treeNode, nil
}
