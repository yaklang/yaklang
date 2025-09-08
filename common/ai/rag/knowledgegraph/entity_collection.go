package knowledgegraph

import (
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// EntityCollection RAG实体集合管理器
type EntityCollection struct {
	ragSystem      *rag.RAGSystem
	collectionName string
	db             *gorm.DB
}

// NewEntityCollection 创建实体集合管理器
func NewEntityCollection(db *gorm.DB, collectionName string) (*EntityCollection, error) {
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	description := fmt.Sprintf("知识图谱实体集合：%s - 专门存储和检索知识图谱中的实体信息，支持基于实体名称、类型、描述和属性的语义搜索，为知识图谱应用提供高效的实体发现和关联分析能力。", collectionName)

	ragSystem, err := rag.CreateOrLoadCollection(db, collectionName, description)
	if err != nil {
		return nil, utils.Errorf("failed to create entity collection: %v", err)
	}

	return &EntityCollection{
		ragSystem:      ragSystem,
		collectionName: collectionName,
		db:             db,
	}, nil
}

// AddEntities 批量导入实体到RAG集合
func (ec *EntityCollection) AddEntities(entities ...*Entity) error {
	//log.Infof("importing %d entities to collection: %s", len(entities), ec.collectionName)

	for _, entity := range entities {
		// 转换为RAG文档
		doc := entity.ToRAGDocument()

		// 添加到RAG系统
		err := ec.ragSystem.Add(doc.ID, doc.Content, rag.WithDocumentRawMetadata(doc.Metadata))
		if err != nil {
			log.Errorf("failed to import entity %s: %v", entity.ID, err)
			return utils.Errorf("failed to import entity %s: %v", entity.ID, err)
		}
	}

	log.Infof("successfully imported %d entities", len(entities))
	return nil
}

// SearchEntities 在RAG集合中搜索实体
func (ec *EntityCollection) SearchEntities(query string, limit int) ([]*Entity, error) {
	log.Infof("searching entities in collection %s for: %s", ec.collectionName, query)

	// 使用RAG系统搜索
	results, err := ec.ragSystem.QueryTopN(query, limit)
	if err != nil {
		return nil, utils.Errorf("failed to search entities: %v", err)
	}

	var entities []*Entity
	for _, result := range results {
		entity, err := ec.documentToEntity(&result.Document)
		if err != nil {
			log.Warnf("failed to convert document to entity: %v", err)
			continue
		}
		entities = append(entities, entity)
	}

	log.Infof("found %d entities for query: %s", len(entities), query)
	return entities, nil
}

// SearchEntitiesByType 按类型搜索实体
func (ec *EntityCollection) SearchEntitiesByType(entityType EntityType, query string, limit int) ([]*Entity, error) {
	log.Infof("searching entities by type %s in collection %s for: %s", entityType, ec.collectionName, query)

	// 使用过滤器限制实体类型
	results, err := ec.ragSystem.QueryWithFilter(query, 1, limit, func(key string, getDoc func() *rag.Document) bool {
		doc := getDoc()
		if doc == nil {
			return false
		}

		// 检查实体类型
		if docType, ok := doc.Metadata["entity_type"].(string); ok {
			return docType == string(entityType)
		}
		return false
	})
	if err != nil {
		return nil, utils.Errorf("failed to search entities by type: %v", err)
	}

	var entities []*Entity
	for _, result := range results {
		entity, err := ec.documentToEntity(&result.Document)
		if err != nil {
			log.Warnf("failed to convert document to entity: %v", err)
			continue
		}
		entities = append(entities, entity)
	}

	log.Infof("found %d %s entities for query: %s", len(entities), entityType, query)
	return entities, nil
}

// GetEntity 根据ID获取实体
func (ec *EntityCollection) GetEntity(entityID string) (*Entity, error) {
	doc, exists, err := ec.ragSystem.GetDocument(entityID)
	if err != nil {
		return nil, utils.Errorf("failed to get entity %s: %v", entityID, err)
	}

	if !exists {
		return nil, utils.Errorf("entity %s not found", entityID)
	}

	return ec.documentToEntity(&doc)
}

// UpdateEntity 更新实体
func (ec *EntityCollection) UpdateEntity(entity *Entity) error {
	entity.UpdatedAt = time.Now()

	// 转换为RAG文档
	doc := entity.ToRAGDocument()

	// 删除旧文档
	err := ec.ragSystem.DeleteDocuments(entity.ID)
	if err != nil {
		log.Warnf("failed to delete old entity document %s: %v", entity.ID, err)
	}

	// 添加新文档
	err = ec.ragSystem.Add(doc.ID, doc.Content, rag.WithDocumentRawMetadata(doc.Metadata))
	if err != nil {
		return utils.Errorf("failed to update entity %s: %v", entity.ID, err)
	}

	log.Infof("successfully updated entity: %s", entity.ID)
	return nil
}

// DeleteEntity 删除实体
func (ec *EntityCollection) DeleteEntity(entityID string) error {
	err := ec.ragSystem.DeleteDocuments(entityID)
	if err != nil {
		return utils.Errorf("failed to delete entity %s: %v", entityID, err)
	}

	log.Infof("successfully deleted entity: %s", entityID)
	return nil
}

// CountEntities 获取实体总数
func (ec *EntityCollection) CountEntities() (int, error) {
	return ec.ragSystem.CountDocuments()
}

// ListAllEntities 列出所有实体
func (ec *EntityCollection) ListAllEntities() ([]*Entity, error) {
	docs, err := ec.ragSystem.ListDocuments()
	if err != nil {
		return nil, utils.Errorf("failed to list entities: %v", err)
	}

	var entities []*Entity
	for _, doc := range docs {
		entity, err := ec.documentToEntity(&doc)
		if err != nil {
			log.Warnf("failed to convert document to entity: %v", err)
			continue
		}
		entities = append(entities, entity)
	}

	return entities, nil
}

// documentToEntity 将RAG文档转换为实体
func (ec *EntityCollection) documentToEntity(doc *rag.Document) (*Entity, error) {
	entity := &Entity{
		ID:         doc.ID,
		Properties: make(map[string]interface{}),
	}

	// 从元数据中恢复实体信息
	if name, ok := doc.Metadata["entity_name"].(string); ok {
		entity.Name = name
	}

	if entityType, ok := doc.Metadata["entity_type"].(string); ok {
		entity.Type = EntityType(entityType)
	}

	if aliases, ok := doc.Metadata["aliases"].([]interface{}); ok {
		for _, alias := range aliases {
			if aliasStr, ok := alias.(string); ok {
				entity.Aliases = append(entity.Aliases, aliasStr)
			}
		}
	}

	if tags, ok := doc.Metadata["tags"].([]interface{}); ok {
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				entity.Tags = append(entity.Tags, tagStr)
			}
		}
	}

	if properties, ok := doc.Metadata["properties"].(map[string]interface{}); ok {
		entity.Properties = properties
	}

	if createdAt, ok := doc.Metadata["created_at"].(int64); ok {
		entity.CreatedAt = time.Unix(createdAt, 0)
	}

	if updatedAt, ok := doc.Metadata["updated_at"].(int64); ok {
		entity.UpdatedAt = time.Unix(updatedAt, 0)
	}

	// 从文档内容中提取描述
	content := doc.Content
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "描述: ") {
			entity.Description = strings.TrimPrefix(line, "描述: ")
			break
		}
	}

	return entity, nil
}
