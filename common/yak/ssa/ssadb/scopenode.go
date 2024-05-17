package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type IrScopeNode struct {
	gorm.Model

	ProgramName   string     `json:"program_name" gorm:"index"`
	ParentNodeId  int64      `json:"parent_node_id" gorm:"index"`
	ChildrenNodes Int64Slice `json:"children" gorm:"type:text"`
	ExtraInfo     string     `json:"extraInfo"`
}

func RequireScopeNode() (int64, *IrScopeNode) {
	db := GetDB()
	treeNode := &IrScopeNode{}
	db.Create(&treeNode)
	return int64(treeNode.ID), treeNode
}

func GetIrScope(id int64) (*IrScopeNode, error) {
	db := GetDB()
	treeNode := &IrScopeNode{}
	if db := db.First(treeNode, id); db.Error != nil {
		return nil, utils.Errorf("failed to get tree node: %v", db.Error)
	}
	return treeNode, nil
}
