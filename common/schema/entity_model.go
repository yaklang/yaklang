package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type EntityBaseInfo struct {
	gorm.Model
	EntityBaseName string
	Description    string
}

// ERModelEntity 是知识库中所有事物的基本单元
type ERModelEntity struct {
	ID uint `gorm:"primarykey"`

	EntityBaseID uint // 外键，指向实体基础信息表

	EntityName string // 实体名称

	Description string // 对该实体的简要描述
	Rationale   string // 该实体存在的理由或依据
	EntityType  string // 实体的类型或类别

	Attributes        []*ERModelAttribute
	OutgoingRelations []*ERModelRelation
	IncomingRelations []*ERModelRelation
}

// ERModelAttribute 记录了实体属性随时间的变化
type ERModelAttribute struct {
	ID             uint `gorm:"primarykey"`
	EntityBaseID   uint
	EntityID       uint   `gorm:"index;not null"` // 外键
	AttributeName  string // 属性名称
	AttributeValue string // 属性值

	UniqueIdentifier bool // 是否该属性是唯一标识符（如身份证号、社保号等）

	Entity *ERModelEntity
}

// ERModelRelation 记录了实体间关系随时间的变化
type ERModelRelation struct {
	ID                uint `gorm:"primarykey"`
	EntityBaseID      uint
	SourceEntityID    uint   // source 实体的id
	RelationType      string // 关系的类型或类别
	TargetEntityID    uint   // target 实体的id
	DecisionRationale string // 该关系存在的理由或依据
	Hash              string // 用于唯一标识关系的哈希值 避免在长的分析过程中大量重复的关系被创建

	SourceEntity *ERModelEntity
	TargetEntity *ERModelEntity
}

func (r *ERModelRelation) CalcHash() string {
	return utils.CalcSha1(r.SourceEntityID, r.RelationType, r.TargetEntityID)
}

func (r *ERModelRelation) BeforeSave() error {
	r.Hash = r.CalcHash()
	return nil
}
