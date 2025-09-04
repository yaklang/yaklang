package schema

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type EntityBaseInfo struct {
	gorm.Model
	EntityBaseName string
	Description    string
	HiddenIndex    string
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

	EntityBaseID    uint // 外键，指向实体基础信息表
	EntityBaseIndex string

	EntityName string // 实体名称

	Description string // 对该实体的简要描述
	Rationale   string // 该实体存在的理由或依据
	EntityType  string // 实体的类型或类别

	Attributes MetadataMap `gorm:"type:text" json:"attributes"`

	HiddenIndex string
}

func (e *ERModelEntity) BeforeSave() error {
	if e.HiddenIndex == "" {
		e.HiddenIndex = uuid.NewString()
	}
	return nil
}

func (e *ERModelEntity) ToGRPC() *ypb.Entity {
	return &ypb.Entity{
		ID:          uint64(e.ID),
		BaseID:      uint64(e.EntityBaseID),
		Name:        e.EntityName,
		Type:        e.EntityType,
		Description: e.Description,
		Rationale:   e.Rationale,
		HiddenIndex: e.HiddenIndex,
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

type ERModelRelationship struct {
	gorm.Model

	EntityBaseID    uint `gorm:"index;not null"`
	EntityBaseIndex string

	SourceEntityID uint
	TargetEntityID uint

	SourceEntityIndex string
	TargetEntityIndex string
	RelationshipType  string

	DecisionRationale string      // 该关系存在的理由或依据
	Hash              string      `gorm:"unique_index"`
	Attributes        MetadataMap `gorm:"type:text" json:"attributes"`
}

func (r *ERModelRelationship) ToGRPC() *ypb.Relationship {
	return &ypb.Relationship{
		ID:                uint64(r.ID),
		Type:              r.RelationshipType,
		SourceEntityID:    uint64(r.SourceEntityID),
		TargetEntityID:    uint64(r.TargetEntityID),
		SourceEntityIndex: r.SourceEntityIndex,
		TargetEntityIndex: r.TargetEntityIndex,
		Rationale:         r.DecisionRationale,
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
		r.SourceEntityIndex,
		r.TargetEntityIndex,
	)
}

func (r *ERModelRelationship) BeforeSave() error {
	r.Hash = r.CalcHash()
	return nil
}

func ERMPatch(db *gorm.DB) {
	if err := db.Model(&EntityBaseInfo{}).AddUniqueIndex("base_index", "hidden_index").Error; err != nil {
		log.Errorf("failed to add unique index on entity_base_info.hidden_index: %v", err)
	}

	if err := db.Model(&ERModelEntity{}).AddUniqueIndex("entity_index", "hidden_index").Error; err != nil {
		log.Errorf("failed to add unique index on er_model_entity.hidden_index: %v", err)
	}

}

func init() {
	// 注册数据库表结构到系统中
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE,
		&EntityBaseInfo{},
		&ERModelEntity{},
		//&ERModelAttribute{},
		&ERModelRelationship{},
	)

	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE)

}
