package knowledgegraph

import (
	"time"
)

// EntityType 实体类型
type EntityType string

const (
	EntityTypePerson        EntityType = "person"        // 人物
	EntityTypeOrganization  EntityType = "organization"  // 组织/公司
	EntityTypeTechnology    EntityType = "technology"    // 技术/工具
	EntityTypeVulnerability EntityType = "vulnerability" // 漏洞
	EntityTypeConcept       EntityType = "concept"       // 概念
	EntityTypeProduct       EntityType = "product"       // 产品
	EntityTypeLocation      EntityType = "location"      // 地点
	EntityTypeEvent         EntityType = "event"         // 事件
)

// Entity 知识图谱实体
type Entity struct {
	ID          string                 `json:"id"`          // 实体唯一标识
	Name        string                 `json:"name"`        // 实体名称
	Type        EntityType             `json:"type"`        // 实体类型
	Description string                 `json:"description"` // 详细描述
	Aliases     []string               `json:"aliases"`     // 别名
	Properties  map[string]interface{} `json:"properties"`  // 扩展属性
	Tags        []string               `json:"tags"`        // 标签
	CreatedAt   time.Time              `json:"created_at"`  // 创建时间
	UpdatedAt   time.Time              `json:"updated_at"`  // 更新时间
}
