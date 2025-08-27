package knowledgegraph

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag"
)

// NewEntity 创建新实体
func NewEntity(id, name string, entityType EntityType, description string) *Entity {
	return &Entity{
		ID:          id,
		Name:        name,
		Type:        entityType,
		Description: description,
		Aliases:     []string{},
		Properties:  make(map[string]interface{}),
		Tags:        []string{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// AddAlias 添加别名
func (e *Entity) AddAlias(alias string) {
	e.Aliases = append(e.Aliases, alias)
	e.UpdatedAt = time.Now()
}

// AddTag 添加标签
func (e *Entity) AddTag(tag string) {
	e.Tags = append(e.Tags, tag)
	e.UpdatedAt = time.Now()
}

// SetProperty 设置属性
func (e *Entity) SetProperty(key string, value interface{}) {
	e.Properties[key] = value
	e.UpdatedAt = time.Now()
}

// ToRAGDocument 转换为RAG文档
func (e *Entity) ToRAGDocument() *rag.Document {
	// 构建实体的文本描述
	var content strings.Builder
	content.WriteString(fmt.Sprintf("实体名称: %s\n", e.Name))
	content.WriteString(fmt.Sprintf("实体类型: %s\n", e.Type))
	content.WriteString(fmt.Sprintf("描述: %s\n", e.Description))

	if len(e.Aliases) > 0 {
		content.WriteString(fmt.Sprintf("别名: %s\n", strings.Join(e.Aliases, ", ")))
	}

	if len(e.Tags) > 0 {
		content.WriteString(fmt.Sprintf("标签: %s\n", strings.Join(e.Tags, ", ")))
	}

	// 添加属性信息
	if len(e.Properties) > 0 {
		content.WriteString("属性:\n")
		for key, value := range e.Properties {
			content.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	// 构建元数据
	metadata := map[string]any{
		"entity_id":   e.ID,
		"entity_name": e.Name,
		"entity_type": string(e.Type),
		"aliases":     e.Aliases,
		"tags":        e.Tags,
		"properties":  e.Properties,
		"created_at":  e.CreatedAt.Unix(),
		"updated_at":  e.UpdatedAt.Unix(),
	}

	return &rag.Document{
		ID:       e.ID,
		Content:  content.String(),
		Metadata: metadata,
	}
}
