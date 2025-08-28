package schema

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
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

	Attributes        []*ERModelAttribute `gorm:"foreignkey:EntityID"`
	OutgoingRelations []*ERModelRelation  `gorm:"foreignkey:SourceEntityID"`
	IncomingRelations []*ERModelRelation  `gorm:"foreignkey:TargetEntityID"`
}

func (e *ERModelEntity) String() string {
	content := e.EntityName
	if e.Description != "" {
		content += "\n\n" + e.Description
	}
	return content
}

func (e *ERModelEntity) Dump() string {
	// 防御性编程：检查接收者是否为 nil
	if e == nil {
		return "<nil ERModelEntity>"
	}
	var sb strings.Builder
	// 打印实体自身的信息
	sb.WriteString(fmt.Sprintf("--- ERModelEntity ---\n"))
	sb.WriteString(fmt.Sprintf("  EntityName:   %s\n", e.EntityName))
	sb.WriteString(fmt.Sprintf("  EntityType:   %s\n", e.EntityType))
	sb.WriteString(fmt.Sprintf("  Description:  %s\n", e.Description))
	sb.WriteString("\n") // 添加空行以分隔
	sb.WriteString(fmt.Sprintf("  Attributes (%d):\n", len(e.Attributes)))
	if len(e.Attributes) == 0 {
		sb.WriteString("    (No attributes)\n")
	} else {
		for i, attr := range e.Attributes {
			sb.WriteString(fmt.Sprintf("    [%d]:\n", i))
			sb.WriteString(attr.Dump("      "))
		}
	}
	sb.WriteString("--------------------------------\n")
	return sb.String()
}

// ERModelAttribute 记录了实体属性随时间的变化
type ERModelAttribute struct {
	ID             uint   `gorm:"primarykey"`
	EntityID       uint   `gorm:"index;not null"` // 外键
	AttributeName  string // 属性名称
	AttributeValue string // 属性值

	UniqueIdentifier bool // 是否该属性是唯一标识符（如身份证号、社保号等）

	Hash string
}

func (a *ERModelAttribute) Dump(prefix string) string {
	// 防御性编程：检查接收者是否为 nil
	if a == nil {
		return prefix + "<nil ERModelAttribute>\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sAttributeName:    %s\n", prefix, a.AttributeName))
	sb.WriteString(fmt.Sprintf("%sAttributeValue:   %s\n", prefix, a.AttributeValue))
	sb.WriteString(fmt.Sprintf("%sUniqueIdentifier: %t\n", prefix, a.UniqueIdentifier))
	return sb.String()
}

func (a *ERModelAttribute) CalcHash() string {
	return utils.CalcSha1(a.EntityID, a.AttributeName, a.AttributeValue)
}

func (a *ERModelAttribute) BeforeSave() error {
	a.Hash = a.CalcHash()
	return nil
}

// ERModelRelation 记录了实体间关系随时间的变化
type ERModelRelation struct {
	ID                uint `gorm:"primarykey"`
	EntityBaseID      uint
	SourceEntityID    uint   // source 实体的id
	RelationType      string // 关系的类型或类别
	TargetEntityID    uint   // target 实体的id
	DecisionRationale string // 该关系存在的理由或依据
	Hash              string
}

func (r *ERModelRelation) CalcHash() string {
	return utils.CalcSha1(r.SourceEntityID, r.RelationType, r.TargetEntityID)
}

func (r *ERModelRelation) BeforeSave() error {
	r.Hash = r.CalcHash()
	return nil
}

func init() {
	// 注册数据库表结构到系统中
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, &EntityBaseInfo{}, &ERModelEntity{}, &ERModelAttribute{}, &ERModelRelation{})
}
