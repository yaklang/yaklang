package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type IrScopeNode struct {
	gorm.Model

	ParentNodeId  int64      `json:"parent_node_id" gorm:"index"`
	ChildrenNodes Int64Slice `json:"children" gorm:"type:text"`
	ExtraInfo     string     `json:"extraInfo"`
}

var migrateTreeNodeOnce = new(sync.Once)

func migrateTreeNode(db *gorm.DB) {
	migrateTreeNodeOnce.Do(func() {
		db.AutoMigrate(&IrScopeNode{})
	})
}

func RequireScopeNode() (int64, *IrScopeNode) {
	db := consts.GetGormProjectDatabase()
	migrateTreeNode(db)
	treeNode := &IrScopeNode{}
	db.Create(&treeNode)
	return int64(treeNode.ID), treeNode
}

func GetTreeNode(id int64) (*IrScopeNode, error) {
	db := consts.GetGormProjectDatabase()
	migrateTreeNode(db)
	treeNode := &IrScopeNode{}
	if db := db.First(treeNode, id); db.Error != nil {
		return nil, utils.Errorf("failed to get tree node: %v", db.Error)
	}
	return treeNode, nil
}
