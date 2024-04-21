package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type TreeNode struct {
	gorm.Model

	ParentNodeId  int64      `json:"parent_node_id" gorm:"index"`
	ChildrenNodes Int64Slice `json:"children"`
	ExtraInfo     string     `json:"extraInfo"`
}

var migrateTreeNodeOnce = new(sync.Once)

func migrateTreeNode(db *gorm.DB) {
	migrateTreeNodeOnce.Do(func() {
		db.AutoMigrate(&TreeNode{})
	})
}

func RequireTreeNode() (int64, *TreeNode) {
	db := consts.GetGormProjectDatabase()
	migrateTreeNode(db)
	treeNode := &TreeNode{}
	db.Create(&treeNode)
	return int64(treeNode.ID), treeNode
}

func GetTreeNode(id int64) (*TreeNode, error) {
	db := consts.GetGormProjectDatabase()
	migrateTreeNode(db)
	treeNode := &TreeNode{}
	if db := db.First(treeNode, id); db.Error != nil {
		return nil, utils.Errorf("failed to get tree node: %v", db.Error)
	}
	return treeNode, nil
}
