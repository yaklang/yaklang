package entitybase

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	META_EntityID     = "entity_id"
	META_EntityBaseID = "entity_base_id"
	META_EntityName   = "entity_name"
	META_EntityType   = "entity_type"
)

type ERModel struct {
	Entities      []*schema.ERModelEntity
	Relationships []*schema.ERModelRelationship
}

func (model *ERModel) Dump() string {
	var sb strings.Builder
	sb.WriteString("Entities:\n")
	for _, entity := range model.Entities {
		sb.WriteString(fmt.Sprintf("- ID: %d\n", entity.ID))
		sb.WriteString(fmt.Sprintf("  EntityName: %s\n", entity.EntityName))
		sb.WriteString(fmt.Sprintf("  EntityType: %s\n", entity.EntityType))
		if entity.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", utils.ShrinkString(entity.Description, 100)))
		}
		if entity.Rationale != "" {
			sb.WriteString(fmt.Sprintf("  Rationale: %s\n", utils.ShrinkString(entity.Rationale, 100)))
		}
		if len(entity.Attributes) > 0 {
			sb.WriteString("  Attributes:\n")
			for key, value := range entity.Attributes {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", key, utils.ShrinkString(value, 100)))
			}
		}
	}
	sb.WriteString("Relationships:\n")
	for _, relationship := range model.Relationships {
		sb.WriteString(fmt.Sprintf("- Source: %d\n", relationship.SourceEntityID))
		sb.WriteString(fmt.Sprintf("  Target: %d\n", relationship.TargetEntityID))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", relationship.RelationshipType))
		if relationship.DecisionRationale != "" {
			sb.WriteString(fmt.Sprintf("  Rationale: %s\n", utils.ShrinkString(relationship.DecisionRationale, 100)))
		}
		if len(relationship.Attributes) > 0 {
			sb.WriteString("  Attributes:\n")
			for key, value := range relationship.Attributes {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", key, utils.ShrinkString(value, 100)))
			}
		}
	}
	return sb.String()
}

func (model *ERModel) Dot() *dot.Graph {
	G := dot.New()
	G.MakeDirected()

	rMap := make(map[uint]int)

	for _, entity := range model.Entities {
		n := G.AddNode(entity.EntityName)
		for key, value := range entity.Attributes {
			G.NodeAttribute(n, key, utils.InterfaceToString(value))
		}
		rMap[entity.ID] = n
	}

	for _, Relationship := range model.Relationships {
		sid, ok := rMap[Relationship.SourceEntityID]
		tid, ok2 := rMap[Relationship.TargetEntityID]
		if !ok || !ok2 {
			continue
		}
		G.AddEdge(sid, tid, Relationship.RelationshipType)
	}
	return G
}

type EntityRepository struct {
	db        *gorm.DB
	baseInfo  *schema.EntityBaseInfo
	ragSystem *rag.RAGSystem
}

func (eb *EntityRepository) GetInfo() (*schema.EntityBaseInfo, error) {
	if eb.baseInfo == nil {
		return nil, utils.Errorf("entity base info is nil")
	}
	return eb.baseInfo, nil
}

func (eb *EntityRepository) GetRAGSystem() *rag.RAGSystem {
	return eb.ragSystem
}

//--- Entity Operations ---

func (eb *EntityRepository) MatchEntities(entity *schema.ERModelEntity) (matchEntity *schema.ERModelEntity, accurate bool, err error) {
	var results []*schema.ERModelEntity
	results, err = eb.IdentifierSearchEntity(entity)
	if err != nil {
		return
	}
	if len(results) > 0 {
		matchEntity = results[0]
		accurate = true
		return
	}

	results, err = eb.VectorSearchEntity(entity)
	if err != nil {
		return
	}
	if len(results) > 0 {
		matchEntity = results[0]
		return
	}
	return
}

func (eb *EntityRepository) IdentifierSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	// name and type query
	entities, err := eb.queryEntities(&yakit.EntityFilter{
		Name: []string{entity.EntityName},
		Type: []string{entity.EntityType},
	})
	if err != nil {
		return nil, err
	}

	if len(entities) > 0 {
		return entities, nil
	}

	return nil, nil

	// unique attribute query
	//var attributeIndexedId []uint
	//for _, attribute := range entity.ExtendAttributes {
	//	if attribute.UniqueIdentifier {
	//		id, ok := yakit.UniqueAttributesIndexEntity(eb.db, attribute.AttributeName, attribute.AttributeValue)
	//		if ok {
	//			attributeIndexedId = append(attributeIndexedId, id...)
	//		}
	//	}
	//}

	//if len(attributeIndexedId) == 0 {
	//	return nil, nil
	//}
	//
	//return eb.queryEntities(&yakit.EntityFilter{
	//	ID: attributeIndexedId,
	//})
}

func (eb *EntityRepository) VectorSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	if eb.ragSystem == nil {
		return nil, utils.Errorf("RAG system is not initialized")
	}

	results, err := eb.GetRAGSystem().Query(entity.String(), 5)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	var entityIds []uint
	for _, res := range results {
		if res.Score <= 0.8 {
			continue
		}
		id, ok := res.Document.Metadata[META_EntityID]
		if ok {
			entityIds = append(entityIds, uint(utils.InterfaceToInt(id)))
		}
	}

	if len(entityIds) == 0 {
		return nil, nil
	}

	return eb.queryEntities(&yakit.EntityFilter{
		ID: entityIds,
	})
}

func (eb *EntityRepository) queryEntities(filter *yakit.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.EntityBaseID = []uint{eb.baseInfo.ID}
	return yakit.QueryEntities(eb.db, filter)
}

func (eb *EntityRepository) addEntityToVectorIndex(entry *schema.ERModelEntity) error {
	metadata := map[string]any{
		META_EntityID:     entry.ID,
		META_EntityBaseID: entry.EntityBaseID,
		META_EntityName:   entry.EntityName,
		META_EntityType:   entry.EntityType,
	}

	documentID := fmt.Sprintf("base_%d_entity_%d[%s]", eb.baseInfo.ID, entry.ID, entry.EntityName)
	err := eb.GetRAGSystem().Add(documentID, entry.EntityName, rag.WithDocumentRawMetadata(metadata))
	if err != nil {
		return err
	}
	return eb.GetRAGSystem().Add(documentID+"_detail", entry.String(), rag.WithDocumentRawMetadata(metadata))
}

func (eb *EntityRepository) SaveEntity(entity *schema.ERModelEntity) error {
	if entity.ID == 0 {
		return eb.CreateEntity(entity)
	}
	return eb.UpdateEntity(entity.ID, entity)
}

func (eb *EntityRepository) UpdateEntity(id uint, e *schema.ERModelEntity) error {
	err := yakit.UpdateEntity(eb.db, id, e)
	if err != nil {
		return err
	}
	return eb.addEntityToVectorIndex(e)
}

func (eb *EntityRepository) CreateEntity(entity *schema.ERModelEntity) error {
	entity.EntityBaseID = eb.baseInfo.ID
	err := yakit.CreateEntity(eb.db, entity)
	if err != nil {
		return err
	}
	return eb.addEntityToVectorIndex(entity)
}

//--- Relationship Operations ---

func (eb *EntityRepository) AddRelationship(sourceID uint, targetID uint, RelationshipType string, decisionRationale string, attr map[string]any) error {
	return yakit.AddRelationship(eb.db, sourceID, targetID, RelationshipType, decisionRationale, attr)
}

func (eb *EntityRepository) QueryOutgoingRelationships(entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := eb.db.Model(&schema.ERModelRelationship{}).Where("source_entity_id = ?", entity.ID).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func (eb *EntityRepository) QueryIncomingRelationships(entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := eb.db.Model(&schema.ERModelRelationship{}).Where("target_entity_id = ?", entity.ID).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

// --- ER Model Operations ---
func (eb *EntityRepository) GetSubERModel(keyword string, maxDepths ...int) (*ERModel, error) {
	allEntities := make([]*schema.ERModelEntity, 0)
	allRelationships := make([]*schema.ERModelRelationship, 0)

	appendEntity := func(entityList ...*schema.ERModelEntity) {
		allEntities = append(allEntities, entityList...)
	}

	appendRelationship := func(RelationshipList ...*schema.ERModelRelationship) {
		allRelationships = append(allRelationships, RelationshipList...)
	}

	maxDepth := 2
	if len(maxDepths) > 0 {
		maxDepth = maxDepths[0]
	}

	startEntity, _, err := eb.MatchEntities(&schema.ERModelEntity{
		EntityName: keyword,
	})
	if err != nil {
		return nil, err
	}
	if startEntity == nil {
		return nil, utils.Errorf("实体 %s 不存在", keyword)
	}

	type queueItem struct {
		entityID uint
		depth    int
		e        *schema.ERModelEntity
	}

	queue := []queueItem{
		{
			entityID: startEntity.ID,
			depth:    0,
			e:        startEntity,
		},
	}
	visited := map[uint]bool{startEntity.ID: true}
	visitedRelationships := map[uint]bool{}

	head := 0
	for head < len(queue) {
		currentItem := queue[head]
		head++
		currentEntity := currentItem.e
		if maxDepth > 0 && currentItem.depth >= maxDepth {
			continue
		}
		// 准备要遍历的关系列表
		RelationshipsToExplore := make([]*schema.ERModelRelationship, 0)

		// query outgoing and incoming relationships
		if outgoings, err := eb.QueryOutgoingRelationships(currentEntity); err == nil {
			RelationshipsToExplore = append(RelationshipsToExplore, outgoings...)
		} else {
			log.Errorf("query outgoing failed: %v", err)
		}

		if incomings, err := eb.QueryIncomingRelationships(currentEntity); err == nil {
			RelationshipsToExplore = append(RelationshipsToExplore, incomings...)
		} else {
			log.Errorf("query incoming failed: %v", err)
		}

		for _, Relationship := range RelationshipsToExplore {
			if visitedRelationships[Relationship.ID] {
				continue
			}
			visitedRelationships[Relationship.ID] = true
			appendRelationship(Relationship)
			var neighborID uint
			if Relationship.SourceEntityID == currentItem.entityID {
				neighborID = Relationship.TargetEntityID
			} else {
				neighborID = Relationship.SourceEntityID
			}
			if !visited[neighborID] {
				neighbor, err := yakit.GetEntityByID(eb.db, neighborID)
				if err != nil {
					return nil, err
				}
				visited[neighborID] = true
				appendEntity(neighbor)
				queue = append(queue, queueItem{entityID: neighborID, depth: currentItem.depth + 1, e: neighbor})
			}
		}
	}

	return &ERModel{
		Entities:      allEntities,
		Relationships: allRelationships,
	}, nil
}

func NewEntityRepository(db *gorm.DB, name, description string, opts ...any) (*EntityRepository, error) {
	var entityBaseInfo schema.EntityBaseInfo
	err := db.Model(&schema.EntityBaseInfo{}).Where("entity_base_name = ?", name).First(&entityBaseInfo).Error

	var needCreateInfo bool
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			needCreateInfo = true
		} else {
			return nil, utils.Errorf("查询实体库信息失败: %v", err)
		}
	}

	collectionExists := rag.CollectionIsExists(db, name)

	if needCreateInfo && !collectionExists {
		err = utils.GormTransaction(db, func(tx *gorm.DB) error {
			entityBaseInfo = schema.EntityBaseInfo{
				EntityBaseName: name,
				Description:    description,
			}
			return yakit.CreateEntityBaseInfo(tx, &entityBaseInfo)
		})
		if err != nil {
			return nil, utils.Errorf("创建实体库信息失败: %v", err)
		}

		ragSystem, err := rag.CreateCollection(db, name, description, opts...)
		if err != nil {
			_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
				return yakit.DeleteEntityBaseInfo(tx, int64(entityBaseInfo.ID))
			})
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &EntityRepository{
			db:        db,
			baseInfo:  &entityBaseInfo,
			ragSystem: ragSystem,
		}, nil
	}

	// 如果实体库信息存在但 RAG Collection 不存在，创建 RAG Collection
	if !needCreateInfo && !collectionExists {
		ragSystem, err := rag.CreateCollection(db, name, entityBaseInfo.Description, opts...)
		if err != nil {
			return nil, utils.Errorf("创建RAG集合失败: %v", err)
		}

		return &EntityRepository{
			db:        db,
			baseInfo:  &entityBaseInfo,
			ragSystem: ragSystem,
		}, nil
	}

	// 如果实体库信息不存在但 RAG Collection 存在，创建实体库信息
	if needCreateInfo && collectionExists {
		err = utils.GormTransaction(db, func(tx *gorm.DB) error {
			entityBaseInfo = schema.EntityBaseInfo{
				EntityBaseName: name,
				Description:    description,
			}
			return yakit.CreateEntityBaseInfo(tx, &entityBaseInfo)
		})
		if err != nil {
			return nil, utils.Errorf("创建实体库信息失败: %v", err)
		}
	}

	// 如果都存在，直接加载
	ragSystem, err := rag.LoadCollection(db, name)
	if err != nil {
		return nil, utils.Errorf("加载RAG集合失败: %v", err)
	}

	return &EntityRepository{
		db:        db,
		baseInfo:  &entityBaseInfo,
		ragSystem: ragSystem,
	}, nil
}
