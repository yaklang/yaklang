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

func (r *EntityRepository) GetID() int64 {
	if r.info == nil {
		return 0
	}
	return int64(r.info.ID)
}

func (r *EntityRepository) GetInfo() (*schema.EntityRepository, error) {
	if r.info == nil {
		return nil, utils.Errorf("entity base info is nil")
	}
	return r.info, nil
}

func (r *EntityRepository) GetRAGSystem() *rag.RAGSystem {
	return r.ragSystem
}

func (r *EntityRepository) SetMergeEntityFunc(f func(new, old *schema.ERModelEntity) (*schema.ERModelEntity, bool, error)) {
	r.mergeEntityFunc = f
}

//--- Entity Operations ---

func (r *EntityRepository) MatchEntities(entity *schema.ERModelEntity) (matchEntity *schema.ERModelEntity, accurate bool, err error) {
	var results []*schema.ERModelEntity
	results, err = r.IdentifierSearchEntity(entity)
	if err != nil {
		return
	}
	if len(results) > 0 {
		matchEntity = results[0]
		accurate = true
		return
	}

	results, err = r.VectorSearchEntity(entity)
	if err != nil {
		return
	}
	if len(results) > 0 {
		matchEntity = results[0]
		return
	}
	return
}

func (r *EntityRepository) IdentifierSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	// name and type query
	entities, err := r.queryEntities(&ypb.EntityFilter{
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

func (r *EntityRepository) VectorSearch(query string, top int, scoreLimit ...float64) ([]*schema.ERModelEntity, []*schema.ERModelRelationship, error) {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("error in vector search entity: %v ", panicErr)
		}
	}()

	if r.ragSystem == nil {
		return nil, nil, utils.Errorf("RAG system is not initialized")
	}

	r.ragMutex.RLock()
	defer r.ragMutex.RUnlock()

	needSocreLimit := 0.0
	if len(scoreLimit) > 0 {
		needSocreLimit = scoreLimit[0]
	}

	results, err := r.GetRAGSystem().Query(query, top)
	if err != nil {
		return nil, nil, err
	}

	if len(results) == 0 {
		return nil, nil, nil
	}

	var entityIndex []string
	var relationshipIndex []string
	for _, res := range results {
		if res.Score < needSocreLimit {
			continue
		}
		switch res.Document.Type {
		case schema.RAGDocumentType_Entity:
			index, ok := res.Document.Metadata.GetDocIndex()
			if ok {
				entityIndex = append(entityIndex, utils.InterfaceToString(index))
			}
		case schema.RAGDocumentType_Relationship:
			index, ok := res.Document.Metadata.GetDocIndex()
			if ok {
				relationshipIndex = append(relationshipIndex, utils.InterfaceToString(index))
			}
		default:
		}
	}

	var entityResults []*schema.ERModelEntity
	var relationshipResults []*schema.ERModelRelationship
	if len(entityIndex) > 0 {
		entityResults, err = r.queryEntities(&ypb.EntityFilter{
			HiddenIndex: entityIndex,
		})
		if err != nil {
			return nil, nil, err
		}

	}

	if len(relationshipIndex) > 0 {
		relationshipResults, err = r.queryRelationship(&ypb.RelationshipFilter{
			UUIDS: relationshipIndex,
		})
		if err != nil {
			return nil, nil, err
		}
	}

	return entityResults, relationshipResults, nil

}

func (r *EntityRepository) VectorSearchEntity(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, error) {
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("error in vector search entity: %v ", panicErr)
		}
	}()

	if r.ragSystem == nil {
		return nil, utils.Errorf("RAG system is not initialized")
	}

	r.ragMutex.RLock()
	defer r.ragMutex.RUnlock()

	results, err := r.GetRAGSystem().Query(entity.String(), 5)
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
		if res.Document.Type == schema.RAGDocumentType_Entity {
			index, ok := res.Document.Metadata.GetDocIndex()
			if ok {
				entityIndex = append(entityIndex, utils.InterfaceToString(index))
			}
		}
	}

	if len(entityIndex) == 0 {
		return nil, nil
	}

	return r.queryEntities(&ypb.EntityFilter{
		HiddenIndex: entityIndex,
	})
}

func (r *EntityRepository) queryEntities(filter *ypb.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.BaseIndex = r.info.Uuid
	return yakit.QueryEntities(r.db, filter)
}

func (r *EntityRepository) addEntityToVectorIndex(entry *schema.ERModelEntity) error {
	r.ragMutex.Lock()
	defer r.ragMutex.Unlock()

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
	return r.GetRAGSystem().Add(documentID, content, opts...)
}

func (r *EntityRepository) addRelationshipToVectorIndex(entry *schema.ERModelRelationship) error {
	r.ragMutex.Lock()
	defer r.ragMutex.Unlock()

	src, err := r.GetEntityByUUID(entry.SourceEntityIndex)
	if err != nil {
		return utils.Errorf("failed to get source entity by uuid [%s]: %v", entry.SourceEntityIndex, err)
	}
	srcDoc := src.ToRAGContent()
	dst, err := r.GetEntityByUUID(entry.TargetEntityIndex)
	if err != nil {
		return utils.Errorf("failed to get target entity by uuid [%s]: %v", entry.TargetEntityIndex, err)
	}
	dstDoc := dst.ToRAGContent()
	content := entry.ToRAGContent(srcDoc, dstDoc)
	return r.GetRAGSystem().Add(
		entry.Uuid, content,
		rag.WithDocumentType(schema.RAGDocumentType_Relationship),
		rag.WithDocumentRelatedEntities(entry.SourceEntityIndex, entry.TargetEntityIndex),
	)
}

func (r *EntityRepository) MergeAndSaveEntity(entity *schema.ERModelEntity) (*schema.ERModelEntity, error) {
	matchedEntity, accurate, err := r.MatchEntities(entity)
	if err != nil { // not critical error
		log.Errorf("failed to match entity [%s]: %v", entity.EntityName, err)
	}
	if matchedEntity == nil {
		log.Infof("start to create entity: %s", entity.EntityName)
		err = r.CreateEntity(entity)
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
	} else if r.mergeEntityFunc != nil {
		resolvedEntity, isSame, err := r.mergeEntityFunc(matchedEntity, entity)
		if err != nil {
			return nil, utils.Errorf("failed to merge entity [%s]: %v", entity.EntityName, err)
		}
		if isSame {
			mergeEntity = resolvedEntity
		}
	} else {
		mergeEntity = entity
	}
	err = r.SaveEntity(mergeEntity) // create or update entity
	if err != nil {
		return nil, utils.Errorf("failed to save entity [%s]: %v", entity.EntityName, err)
	}
	return mergeEntity, nil
}

func (r *EntityRepository) SaveEntity(entity *schema.ERModelEntity) error {
	if entity.ID == 0 {
		return r.CreateEntity(entity)
	}
	return r.UpdateEntity(entity.ID, entity)
}

func (r *EntityRepository) UpdateEntity(id uint, e *schema.ERModelEntity) error {
	err := yakit.UpdateEntity(r.db, id, e)
	if err != nil {
		return err
	}
	go func() {
		err := r.addEntityToVectorIndex(e)
		if err != nil {
			log.Errorf("failed to add entity [%s] to vector index: %v", e.EntityName, err)
		}
	}()
	return nil
}

func (r *EntityRepository) CreateEntity(entity *schema.ERModelEntity) error {
	entity.RepositoryUUID = r.info.Uuid
	err := yakit.CreateEntity(r.db, entity)
	if err != nil {
		return err
	}
	go func() {
		err := r.addEntityToVectorIndex(entity)
		if err != nil {
			log.Errorf("failed to add entity [%s] to vector index: %v", entity.EntityName, err)
		}
	}()
	return nil
}

//--- Relationship Operations ---

func (r *EntityRepository) AddRelationship(sourceIndex, targetIndex string, relationType string, typeVerbose string, attr map[string]any) error {
	data, err := yakit.AddRelationship(r.db, sourceIndex, targetIndex, r.info.Uuid, relationType, typeVerbose, attr)
	if err != nil {
		log.Warnf("failed to add relation [%s] to vector [%s]: %v", relationType, sourceIndex, err)
		return utils.Wrapf(err, "failed to add relation [%s] to vector [%s]", relationType, sourceIndex)
	}
	err = r.addRelationshipToVectorIndex(data)
	if err != nil {
		log.Warnf("failed to add relation [%s] to vector index: %v", relationType, err)
		return utils.Wrapf(err, "failed to add relation [%s] to vector index", relationType)
	}
	return nil
}

func (r *EntityRepository) QueryOutgoingRelationships(entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := r.db.Model(&schema.ERModelRelationship{}).Where("source_entity_index = ?", entity.Uuid).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func (r *EntityRepository) QueryIncomingRelationships(entity *schema.ERModelEntity) ([]*schema.ERModelRelationship, error) {
	var relationships []*schema.ERModelRelationship
	if err := r.db.Model(&schema.ERModelRelationship{}).Where("target_entity_index = ?", entity.Uuid).Find(&relationships).Error; err != nil {
		return nil, err
	}
	return relationships, nil
}

func (r *EntityRepository) queryRelationship(filter *ypb.RelationshipFilter) ([]*schema.ERModelRelationship, error) {
	filter.BaseIndex = r.info.Uuid
	return yakit.QueryRelationships(r.db, filter)
}

func (r *EntityRepository) NewSaveEndpoint(ctx context.Context) *SaveEndpoint {
	return &SaveEndpoint{
		ctx:          ctx,
		eb:           r,
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
