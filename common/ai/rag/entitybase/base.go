package entitybase

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strconv"
)

type ERModelGraph struct {
	Entities  []*schema.ERModelEntity
	Relations []*schema.ERModelRelation
}

func (e *ERModelGraph) Dot() *dot.Graph {
	g := dot.New()

	dbID2GraphID := make(map[uint]int)
	for _, entity := range e.Entities {
		label := entity.EntityName
		if entity.EntityType != "" {
			label += " (" + entity.EntityType + ")"
		}
		dbID2GraphID[entity.ID] = g.AddNode(label)
	}

	for _, relation := range e.Relations {
		sID, ok1 := dbID2GraphID[relation.SourceEntityID]
		tID, ok2 := dbID2GraphID[relation.TargetEntityID]
		if ok1 && ok2 {
			g.AddEdge(sID, tID, relation.RelationType)
		}
	}
	return g
}

type EntityBase struct {
	db        *gorm.DB
	baseInfo  *schema.EntityBaseInfo
	ragSystem *rag.RAGSystem
}

func (eb *EntityBase) GetInfo() (*schema.EntityBaseInfo, error) {
	if eb.baseInfo == nil {
		return nil, utils.Errorf("entity base info is nil")
	}
	return eb.baseInfo, nil
}

func (eb *EntityBase) GetRAGSystem() *rag.RAGSystem {
	return eb.ragSystem
}

//--- Entity Operations ---

func (eb *EntityBase) MatchEntities(entity *schema.ERModelEntity) (matchEntity *schema.ERModelEntity, accurate bool, err error) {
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

func (eb *EntityBase) IdentifierSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
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

	// unique attribute query
	var attributeIndexedId []uint
	for _, attribute := range entity.Attributes {
		if attribute.UniqueIdentifier {
			id, ok := yakit.UniqueAttributesIndexEntity(eb.db, eb.baseInfo.ID, attribute.AttributeName, attribute.AttributeValue)
			if ok {
				attributeIndexedId = append(attributeIndexedId, id)
			}
		}
	}

	if attributeIndexedId == nil || len(attributeIndexedId) == 0 {
		return nil, nil
	}

	return eb.queryEntities(&yakit.EntityFilter{
		ID: attributeIndexedId,
	})
}

func (eb *EntityBase) VectorSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
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
		if res.Score <= 0.85 {
			continue
		}
		id, err := strconv.Atoi(res.Document.ID)
		if err != nil {
			continue
		}
		entityIds = append(entityIds, uint(id))
	}

	if len(entityIds) == 0 {
		return nil, nil
	}

	return eb.queryEntities(&yakit.EntityFilter{
		ID: entityIds,
	})
}

func (eb *EntityBase) queryEntities(filter *yakit.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.EntityBaseID = []uint{eb.baseInfo.ID}
	return yakit.QueryEntities(eb.db, filter)
}

func (eb *EntityBase) addEntityToVectorIndex(entry *schema.ERModelEntity) error {

	metadata := map[string]any{
		"entity_base_id": entry.EntityBaseID,
		"entity_name":    entry.EntityName,
		"entity_type":    entry.EntityType,
	}

	documentID := utils.InterfaceToString(entry.ID)

	return eb.GetRAGSystem().Add(documentID, entry.String(), rag.WithDocumentRawMetadata(metadata))
}

func (eb *EntityBase) UpdateEntity(id uint, e *schema.ERModelEntity) error {
	err := yakit.UpdateEntity(eb.db, id, e)
	if err != nil {
		return err
	}
	err = eb.AppendAttrs(id, e.Attributes)
	if err != nil {
		return err
	}
	err = eb.GetRAGSystem().DeleteDocuments(utils.InterfaceToString(e.ID))
	if err != nil {
		return err
	}
	return eb.addEntityToVectorIndex(e)
}

func (eb *EntityBase) CreateEntity(entity *schema.ERModelEntity) error {
	entity.EntityBaseID = eb.baseInfo.ID
	err := yakit.CreateEntity(eb.db, entity)
	if err != nil {
		return err
	}
	return eb.addEntityToVectorIndex(entity)
}

//--- Attribute Operations ---

func (eb *EntityBase) AppendAttrs(entityId uint, attrs []*schema.ERModelAttribute) error {
	if len(attrs) == 0 {
		return nil
	}

	return utils.GormTransaction(eb.db, func(tx *gorm.DB) error {
		for _, attr := range attrs {
			attr.ID = 0
			attr.EntityID = entityId
			if err := eb.db.Where("hash = ?", attr.CalcHash()).FirstOrCreate(attrs).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

//--- Relation Operations ---

func (eb *EntityBase) AddRelation(sourceID uint, targetID uint, relationType string, decisionRationale string) error {
	return yakit.AddRelation(eb.db, sourceID, targetID, relationType, decisionRationale)
}

// --- ER Model Operations ---

func (eb *EntityBase) GetSubERModel(entityName string, deeps ...int) (*ERModelGraph, error) {
	allEntities := make([]*schema.ERModelEntity, 0)
	allRelations := make([]*schema.ERModelRelation, 0)
	entityFilterMap := make(map[uint]struct{})
	relationFilterMap := make(map[uint]struct{})

	appendEntity := func(entityList ...*schema.ERModelEntity) {
		for _, e := range entityList {
			if _, exists := entityFilterMap[e.ID]; !exists {
				entityFilterMap[e.ID] = struct{}{}
				allEntities = append(allEntities, e)
			}
		}
	}

	appendRelation := func(relationList ...*schema.ERModelRelation) {
		for _, relation := range relationList {
			if _, exists := relationFilterMap[relation.ID]; !exists {
				relationFilterMap[relation.ID] = struct{}{}
				allRelations = append(allRelations, relation)
			}
		}
	}

	deep := 2
	if len(deeps) > 0 {
		deep = deeps[0]
	}

	centerEntity, _, err := eb.MatchEntities(&schema.ERModelEntity{
		EntityName: entityName,
	})
	if err != nil {
		return nil, err
	}
	if centerEntity == nil {
		return nil, utils.Errorf("实体 %s 不存在", entityName)
	}

	appendEntity(centerEntity)
	appendRelation(centerEntity.IncomingRelations...)
	appendRelation(centerEntity.OutgoingRelations...)

	for i := 0; i < deep; i++ {
		var newEntities []*schema.ERModelEntity
		for _, relation := range allRelations {
			if relation.SourceEntityID != 0 {
				sourceEntity, err := yakit.GetEntity(eb.db, relation.SourceEntityID)
				if err != nil {
					return nil, nil, err
				}
				if sourceEntity != nil {
					newEntities = append(newEntities, sourceEntity)
				}
			}
			if relation.TargetEntityID != 0 {
				targetEntity, err := yakit.GetEntity(eb.db, relation.TargetEntityID)
				if err != nil {
					return nil, nil, err
				}
				if targetEntity != nil {
					newEntities = append(newEntities, targetEntity)
				}
			}
		}

		appendEntity(newEntities...)

		var newRelations []*schema.ERModelRelation
		for _, entity := range newEntities {
			newRelations = append(newRelations, entity.IncomingRelations...)
			newRelations = append(newRelations, entity.OutgoingRelations...)
		}

		appendRelation(newRelations...)
	}

}

func NewEntityBase(db *gorm.DB, name, description string, opts ...any) (*EntityBase, error) {
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

		return &EntityBase{
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

		return &EntityBase{
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

	return &EntityBase{
		db:        db,
		baseInfo:  &entityBaseInfo,
		ragSystem: ragSystem,
	}, nil
}
