package entityrepos

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	META_EntityType = "entity_type"
)

type EntityRepository struct {
	db        *gorm.DB
	info      *schema.EntityRepository
	ragSystem *rag.RAGSystem
	ragMutex  sync.RWMutex

	mergeEntityFunc func(old, new *schema.ERModelEntity) (*schema.ERModelEntity, bool, error)
}

func (eb *EntityRepository) GetID() int64 {
	if eb.info == nil {
		return 0
	}
	return int64(eb.info.ID)
}

func (eb *EntityRepository) GetInfo() (*schema.EntityRepository, error) {
	if eb.info == nil {
		return nil, utils.Errorf("entity base info is nil")
	}
	return eb.info, nil
}

func (eb *EntityRepository) GetRAGSystem() *rag.RAGSystem {
	return eb.ragSystem
}

func (eb *EntityRepository) SetMergeEntityFunc(f func(new, old *schema.ERModelEntity) (*schema.ERModelEntity, bool, error)) {
	eb.mergeEntityFunc = f
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
}

func (eb *EntityRepository) VectorSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("error in vector search entity: %v ", panicErr)
		}
	}()

	if eb.ragSystem == nil {
		return nil, utils.Errorf("RAG system is not initialized")
	}

	eb.ragMutex.RLock()
	defer eb.ragMutex.RUnlock()

	results, err := eb.GetRAGSystem().Query(entity.String(), 5)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	var entityIndex []string
	for _, res := range results {
		if res.Score <= 0.8 {
			continue
		}
		index, ok := res.Document.Metadata.GetDocIndex()
		if ok {
			entityIndex = append(entityIndex, utils.InterfaceToString(index))
		}
	}

	if len(entityIndex) == 0 {
		return nil, nil
	}

	return eb.queryEntities(&ypb.EntityFilter{
		HiddenIndex: entityIndex,
	})
}

func (eb *EntityRepository) queryEntities(filter *ypb.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.BaseID = uint64(eb.info.ID)
	return yakit.QueryEntities(eb.db, filter)
}

func (eb *EntityRepository) addEntityToVectorIndex(entry *schema.ERModelEntity) error {
	eb.ragMutex.Lock()
	defer eb.ragMutex.Unlock()

	metadata := map[string]any{
		schema.META_Doc_Index:  entry.Uuid,
		schema.META_Doc_Name:   entry.EntityName,
		schema.META_Base_Index: entry.RepositoryUUID,
		META_EntityType:        entry.EntityType,
	}

	var opts []rag.DocumentOption

	opts = append(opts, rag.WithDocumentRawMetadata(metadata),
		rag.WithDocumentType(schema.RAGDocumentType_Entity),
		rag.WithDocumentEntityID(entry.Uuid), // let RAG system generate embedding
	)
	documentID := fmt.Sprintf("%v_entity", entry.Uuid)
	content := entry.ToRAGContent()
	return eb.GetRAGSystem().Add(documentID, content, opts...)
}

func (e *EntityRepository) addRelationshipToVectorIndex(entry *schema.ERModelRelationship) error {
	e.ragMutex.Lock()
	defer e.ragMutex.Unlock()

	src, err := e.GetEntityByUUID(entry.SourceEntityIndex)
	if err != nil {
		return utils.Errorf("failed to get source entity by uuid [%s]: %v", entry.SourceEntityIndex, err)
	}
	srcDoc := src.ToRAGContent()
	dst, err := e.GetEntityByUUID(entry.TargetEntityIndex)
	if err != nil {
		return utils.Errorf("failed to get target entity by uuid [%s]: %v", entry.TargetEntityIndex, err)
	}
	dstDoc := dst.ToRAGContent()
	content := entry.ToRAGContent(srcDoc, dstDoc)
	return e.GetRAGSystem().Add(
		entry.Uuid, content,
		rag.WithDocumentType(schema.RAGDocumentType_Relationship),
		rag.WithDocumentRelatedEntities(entry.SourceEntityIndex, entry.TargetEntityIndex),
	)
}

func (eb *EntityRepository) MergeAndSaveEntity(entity *schema.ERModelEntity) (*schema.ERModelEntity, error) {
	matchedEntity, accurate, err := eb.MatchEntities(entity)
	if err != nil { // not critical error
		log.Errorf("failed to match entity [%s]: %v", entity.EntityName, err)
	}
	if matchedEntity == nil {
		log.Infof("start to create entity: %s", entity.EntityName)
		err = eb.CreateEntity(entity)
		if err != nil {
			return nil, utils.Errorf("failed to create entity [%s]: %v", entity.EntityName, err)
		}
		return entity, nil
	}

	mergeEntity := matchedEntity
	if accurate { // if search is accurate, just use the matched entity
		for s, i := range entity.Attributes {
			matchedEntity.Attributes[s] = i
		}
	} else if eb.mergeEntityFunc != nil {
		resolvedEntity, isSame, err := eb.mergeEntityFunc(matchedEntity, entity)
		if err != nil {
			return nil, utils.Errorf("failed to merge entity [%s]: %v", entity.EntityName, err)
		}
		if isSame {
			mergeEntity = resolvedEntity
		}
	} else {
		mergeEntity = entity
	}
	err = eb.SaveEntity(mergeEntity) // create or update entity
	if err != nil {
		return nil, utils.Errorf("failed to save entity [%s]: %v", entity.EntityName, err)
	}
	return mergeEntity, nil
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
	go func() {
		err := eb.addEntityToVectorIndex(e)
		if err != nil {
			log.Errorf("failed to add entity [%s] to vector index: %v", e.EntityName, err)
		}
	}()
	return nil
}

func (eb *EntityRepository) CreateEntity(entity *schema.ERModelEntity) error {
	entity.RepositoryUUID = eb.info.Uuid
	err := yakit.CreateEntity(eb.db, entity)
	if err != nil {
		return err
	}
	go func() {
		err := eb.addEntityToVectorIndex(entity)
		if err != nil {
			log.Errorf("failed to add entity [%s] to vector index: %v", entity.EntityName, err)
		}
	}()
	return nil
}

//--- Relationship Operations ---

func (eb *EntityRepository) AddRelationship(sourceIndex, targetIndex string, relationType string, typeVerbose string, attr map[string]any) error {
	data, err := yakit.AddRelationship(eb.db, sourceIndex, targetIndex, eb.info.Uuid, relationType, typeVerbose, attr)
	if err != nil {
		log.Warnf("failed to add relation [%s] to vector [%s]: %v", relationType, sourceIndex, err)
		return utils.Wrapf(err, "failed to add relation [%s] to vector [%s]", relationType, sourceIndex)
	}
	err = eb.addRelationshipToVectorIndex(data)
	if err != nil {
		log.Warnf("failed to add relation [%s] to vector index: %v", relationType, err)
		return utils.Wrapf(err, "failed to add relation [%s] to vector index", relationType)
	}
	return nil
}

func (eb *EntityRepository) QueryOutgoingRelationships(entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := eb.db.Model(&schema.ERModelRelationship{}).Where("source_entity_index = ?", entity.Uuid).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func (eb *EntityRepository) QueryIncomingRelationships(entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := eb.db.Model(&schema.ERModelRelationship{}).Where("target_entity_index = ?", entity.Uuid).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func (eb *EntityRepository) NewSaveEndpoint(ctx context.Context) *SaveEndpoint {
	return &SaveEndpoint{
		ctx:          ctx,
		eb:           eb,
		nameToIndex:  omap.NewOrderedMap[string, string](make(map[string]string)),
		nameSig:      omap.NewOrderedMap[string, *endpointDataSignal](make(map[string]*endpointDataSignal)),
		entityFinish: make(chan struct{}),
		once:         sync.Once{},
	}
}

func GetOrCreateEntityRepository(db *gorm.DB, name, description string, opts ...any) (*EntityRepository, error) {
	var entityBaseInfo schema.EntityRepository
	err := db.Model(&schema.EntityRepository{}).Where("entity_base_name = ?", name).First(&entityBaseInfo).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.Errorf("query entity repository failed: %v", err)
		}
		if createErr := utils.GormTransaction(db, func(tx *gorm.DB) error {
			entityBaseInfo = schema.EntityRepository{
				EntityBaseName: name,
				Description:    description,
				Uuid:           entityBaseInfo.Uuid,
			}
			return yakit.CreateEntityBaseInfo(tx, &entityBaseInfo)
		}); createErr != nil {
			return nil, utils.Errorf("create entity repository err: %v", err)
		}
	}

	collectionExists := rag.CollectionIsExists(db, name)

	var ragSystem *rag.RAGSystem
	if !collectionExists {
		ragSystem, err = rag.CreateCollection(db, name, description, opts...)
		if err != nil {
			_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
				return yakit.DeleteEntityBaseInfo(tx, int64(entityBaseInfo.ID))
			})
			return nil, utils.Errorf("create entity repository & rag collection err: %v", err)
		}
	} else {
		// 如果都存在，直接加载
		ragSystem, err = rag.LoadCollection(db, name)
		if err != nil {
			return nil, utils.Errorf("加载RAG集合失败: %v", err)
		}
	}
	var repos = &EntityRepository{
		db:        db,
		info:      &entityBaseInfo,
		ragSystem: ragSystem,
	}

	return repos, nil
}
