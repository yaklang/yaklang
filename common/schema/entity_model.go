package schema

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type EntityBaseInfo struct {
	gorm.Model
	EntityBaseName string
	Description    string
}

func (e *EntityBaseInfo) ToGRPC() *ypb.EntityRepository {
	return &ypb.EntityRepository{
		ID:          int64(e.ID),
		Name:        e.EntityBaseName,
		Description: e.Description,
	}
}

// ERModelEntity 是知识库中所有事物的基本单元
type ERModelEntity struct {
	gorm.Model

	EntityBaseID uint // 外键，指向实体基础信息表

	EntityName string // 实体名称

	Description string // 对该实体的简要描述
	Rationale   string // 该实体存在的理由或依据
	EntityType  string // 实体的类型或类别

	Attributes MetadataMap `gorm:"type:text" json:"attributes"`

	// ExtendAttributes []*ERModelAttribute `gorm:"foreignkey:OwnerID"`
	// OutgoingRelationship []*ERModelRelationship `gorm:"foreignkey:SourceEntityID"`
	// IncomingRelationship []*ERModelRelationship `gorm:"foreignkey:TargetEntityID"`
}

func (e *ERModelEntity) ToGRPC() *ypb.Entity {
	return &ypb.Entity{
		ID:          uint64(e.ID),
		BaseID:      uint64(e.EntityBaseID),
		Name:        e.EntityName,
		Type:        e.EntityType,
		Description: e.Description,
		Rationale:   e.Rationale,
		Attributes: lo.MapToSlice(e.Attributes, func(key string, value any) *ypb.KVPair {
			return &ypb.KVPair{
				Key:   key,
				Value: utils.InterfaceToString(value),
			}
		}),
	}
}

func (e *ERModelEntity) String() string {
	return e.Dump()
}

func (e *ERModelEntity) Dump() string {
	if e == nil {
		return "<nil ERModelEntity>"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- ERModelEntity ---\n"))
	sb.WriteString(fmt.Sprintf("  EntityName:   %s\n", e.EntityName))
	sb.WriteString(fmt.Sprintf("  EntityType:   %s\n", e.EntityType))
	sb.WriteString(fmt.Sprintf("  Description:  %s\n", e.Description))
	sb.WriteString("\n") // 添加空行以分隔
	sb.WriteString(fmt.Sprintf("  Attributes (%d):\n", len(e.Attributes)))
	if len(e.Attributes) == 0 {
		sb.WriteString("    (No attributes)\n")
	} else {
		for name, attr := range e.Attributes {
			sb.WriteString(fmt.Sprintf("    AttributeName:    %s\n", name))
			sb.WriteString(fmt.Sprintf("    sAttributeValue:    %s\n", attr))
		}
	}
	sb.WriteString("--------------------------------\n")
	return sb.String()
}

// ERModelAttribute 记录了实体属性随时间的变化
//type ERModelAttribute struct {
//	ID             uint   `gorm:"primarykey"`
//	OwnerID        uint   `gorm:"index;not null"` // 外键
//	AttributeName  string // 属性名称
//	AttributeValue string // 属性值
//
//	UniqueIdentifier bool // 是否该属性是唯一标识符（如身份证号、社保号等）
//
//	Hash string
//}
//
//func (a *ERModelAttribute) Dump(prefix string) string {
//	if a == nil {
//		return prefix + "<nil ERModelAttribute>\n"
//	}
//	var sb strings.Builder
//	sb.WriteString(fmt.Sprintf("%sAttributeName:    %s\n", prefix, a.AttributeName))
//	sb.WriteString(fmt.Sprintf("%sAttributeValue:   %s\n", prefix, a.AttributeValue))
//	sb.WriteString(fmt.Sprintf("%sUniqueIdentifier: %t\n", prefix, a.UniqueIdentifier))
//	return sb.String()
//}
//
//func (a *ERModelAttribute) CalcHash() string {
//	return utils.CalcSha1(a.OwnerID, a.AttributeName, a.AttributeValue)
//}
//
//func (a *ERModelAttribute) BeforeSave() error {
//	a.Hash = a.CalcHash()
//	return nil
//}

// ERModelRelationship 记录了实体间关系随时间的变化
type ERModelRelationship struct {
	gorm.Model

	EntityBaseID      uint        `gorm:"index;not null"`
	SourceEntityID    uint        `gorm:"index"`
	RelationshipType  string      `gorm:"index"`
	TargetEntityID    uint        `gorm:"index"`
	DecisionRationale string      // 该关系存在的理由或依据
	Hash              string      `gorm:"unique_index"`
	Attributes        MetadataMap `gorm:"type:text" json:"attributes"`
}

func (r *ERModelRelationship) ToGRPC() *ypb.Relationship {
	return &ypb.Relationship{
		ID:             uint64(r.ID),
		Type:           r.RelationshipType,
		SourceEntityID: uint64(r.SourceEntityID),
		TargetEntityID: uint64(r.TargetEntityID),
		Rationale:      r.DecisionRationale,
		Attributes: lo.MapToSlice(r.Attributes, func(key string, value any) *ypb.KVPair {
			return &ypb.KVPair{
				Key:   key,
				Value: utils.InterfaceToString(value),
			}
		}),
	}
}

func (r *ERModelRelationship) CalcHash() string {
	return utils.CalcSha1(
		r.EntityBaseID,
		r.SourceEntityID,
		r.RelationshipType,
		r.TargetEntityID,
		r.Attributes,
	)
}

func (r *ERModelRelationship) BeforeSave() error {
	r.Hash = r.CalcHash()
	return nil
}

func init() {
	// 注册数据库表结构到系统中
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE,
		&EntityBaseInfo{},
		&ERModelEntity{},
		//&ERModelAttribute{},
		&ERModelRelationship{},
	)
}
