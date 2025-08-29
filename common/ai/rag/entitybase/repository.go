package entitybase

import (
	"errors"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	META_EntityID     = "entity_id"
	META_EntityBaseID = "entity_base_id"
	META_EntityName   = "entity_name"
	META_EntityType   = "entity_type"
)

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
	entities, err := eb.queryEntities(&ypb.EntityFilter{
		Names: []string{entity.EntityName},
		Types: []string{entity.EntityType},
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

	var entityIds []uint64
	for _, res := range results {
		if res.Score <= 0.8 {
			continue
		}
		id, ok := res.Document.Metadata[META_EntityID]
		if ok {
			entityIds = append(entityIds, uint64(utils.InterfaceToInt(id)))
		}
	}

	if len(entityIds) == 0 {
		return nil, nil
	}

	return eb.queryEntities(&ypb.EntityFilter{
		IDs: entityIds,
	})
}

func (eb *EntityRepository) queryEntities(filter *ypb.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.BaseID = uint64(eb.baseInfo.ID)
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
func (eb *EntityRepository) GetSubERModel(keyword string, maxDepths ...int) (*yakit.ERModel, error) {
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
	return yakit.EntityRelationshipFind(eb.db, []*schema.ERModelEntity{startEntity}, maxDepth)
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
