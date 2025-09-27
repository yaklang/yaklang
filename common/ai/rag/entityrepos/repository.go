package entityrepos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

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

type EntityRepositoryRuntimeConfig struct {
	similarityThreshold float64
	runtimeID           string
	queryTop            int
}

type RuntimeConfigOption func(config *EntityRepositoryRuntimeConfig)

func WithSimilarityThreshold(threshold float64) RuntimeConfigOption {
	return func(config *EntityRepositoryRuntimeConfig) {
		config.similarityThreshold = threshold
	}
}

func WithQueryTop(top int) RuntimeConfigOption {
	return func(config *EntityRepositoryRuntimeConfig) {
		config.queryTop = top
	}
}

func NewRuntimeConfig(opts ...any) *EntityRepositoryRuntimeConfig {
	config := &EntityRepositoryRuntimeConfig{
		similarityThreshold: 0.8,
		queryTop:            5,
		runtimeID:           uuid.NewString(),
	}
	for _, opt := range opts {
		switch configOpt := opt.(type) {
		case RuntimeConfigOption:
			configOpt(config)
		}
	}
	return config
}

type EntityRepository struct {
	db                *gorm.DB
	info              *schema.EntityRepository
	ragSystem         *rag.RAGSystem
	entityVectorMutex sync.RWMutex

	runtimeConfig *EntityRepositoryRuntimeConfig
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

func (r *EntityRepository) AddVectorIndex(docId string, content string, opts ...rag.DocumentOption) error {
	r.entityVectorMutex.Lock()
	defer r.entityVectorMutex.Unlock()
	return r.GetRAGSystem().Add(docId, content, opts...)
}

func (r *EntityRepository) QueryVector(query string, top int) ([]rag.SearchResult, error) {
	queryStartTime := time.Now()

	r.entityVectorMutex.RLock()
	lockAcquireTime := time.Since(queryStartTime)
	defer r.entityVectorMutex.RUnlock()

	actualQueryStart := time.Now()
	results, err := r.GetRAGSystem().Query(query, top)
	queryDuration := time.Since(actualQueryStart)

	totalDuration := time.Since(queryStartTime)

	if err != nil {
		log.Errorf("RAG Query failed: query='%s', top=%d, error=%v", query[:min(50, len(query))], top, err)
		return results, err
	}

	// 性能监控
	if totalDuration > 10*time.Second {
		log.Errorf("CRITICAL RAG QUERY: query='%s' took %v (lock: %v, query: %v), returned %d results",
			query[:min(50, len(query))], totalDuration, lockAcquireTime, queryDuration, len(results))
	} else if totalDuration > 3*time.Second {
		log.Warnf("SLOW RAG QUERY: query='%s' took %v (lock: %v, query: %v), returned %d results",
			query[:min(50, len(query))], totalDuration, lockAcquireTime, queryDuration, len(results))
	}

	// 记录低效查询（耗时长但结果少）
	if totalDuration > 5*time.Second && len(results) < 3 {
		log.Warnf("INEFFICIENT RAG QUERY: query='%s' took %v but only returned %d results",
			query[:min(100, len(query))], totalDuration, len(results))
	}

	return results, err
}

//--- Entity Operations ---

func (r *EntityRepository) MatchEntities(entity *schema.ERModelEntity) ([]*schema.ERModelEntity, bool, error) {
	totalStartTime := time.Now()
	var results []*schema.ERModelEntity

	// 标识符搜索阶段
	dbSearchStart := time.Now()
	results, err := r.IdentifierSearchEntity(entity)
	dbSearchDuration := time.Since(dbSearchStart)

	if err != nil {
		log.Errorf("identifier search entity [%s] failed: %v", entity.EntityName, err)
		return nil, false, err
	}

	// 标识符搜索性能监控
	if dbSearchDuration > time.Second {
		log.Warnf("SLOW IDENTIFIER SEARCH: entity [%s] took %v and found %d results", entity.EntityName, dbSearchDuration, len(results))
	} else if dbSearchDuration > 100*time.Millisecond {
		log.Debugf("identifier search entity [%s] took %v and found %d results", entity.EntityName, dbSearchDuration, len(results))
	}

	if len(results) > 0 {
		totalDuration := time.Since(totalStartTime)
		if totalDuration > time.Second {
			log.Warnf("FAST MATCH: entity [%s] found via identifier search in %v", entity.EntityName, totalDuration)
		}
		return results, true, nil
	}

	// 向量搜索阶段 - 这是性能瓶颈的主要来源
	vectorSearchStart := time.Now()
	results, err = r.VectorSearchEntity(entity)
	vectorSearchDuration := time.Since(vectorSearchStart)

	totalDuration := time.Since(totalStartTime)

	// 详细的向量搜索性能分析
	if vectorSearchDuration > 30*time.Second || totalDuration > 30*time.Second {
		log.Errorf("CRITICAL VECTOR SEARCH PERFORMANCE: entity [%s] total %v, vector %v, found %d results",
			entity.EntityName, totalDuration, vectorSearchDuration, len(results))
		log.Errorf("VECTOR SEARCH DETAILS: entity_type=%s, similarity_threshold=%.2f, query_top=%d",
			entity.EntityType, r.runtimeConfig.similarityThreshold, r.runtimeConfig.queryTop)
	} else if vectorSearchDuration > 10*time.Second {
		log.Warnf("SLOW VECTOR SEARCH: entity [%s] took %v and found %d results (total: %v)",
			entity.EntityName, vectorSearchDuration, len(results), totalDuration)
	} else if vectorSearchDuration > 3*time.Second {
		log.Warnf("vector search entity [%s] took %v and found %d results", entity.EntityName, vectorSearchDuration, len(results))
	}

	// 统计没有找到结果的情况 - 这表明向量搜索效率低下
	if len(results) == 0 && vectorSearchDuration > time.Second {
		log.Warnf("INEFFECTIVE VECTOR SEARCH: entity [%s] searched %v but found 0 results - consider tuning similarity threshold (current: %.2f)",
			entity.EntityName, vectorSearchDuration, r.runtimeConfig.similarityThreshold)
	}

	return results, false, err
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

	needSocreLimit := 0.0
	if len(scoreLimit) > 0 {
		needSocreLimit = scoreLimit[0]
	}

	if top == 0 {
		top = r.runtimeConfig.queryTop
	}

	results, err := r.QueryVector(query, top)

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
			index, ok := res.Document.Metadata.GetDataUUID()
			if ok {
				entityIndex = append(entityIndex, utils.InterfaceToString(index))
			}
		case schema.RAGDocumentType_Relationship:
			index, ok := res.Document.Metadata.GetDataUUID()
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
	methodStartTime := time.Now()

	defer func() {
		panicErr := recover()
		if panicErr != nil {
			log.Errorf("error in vector search entity [%s]: %v", entity.EntityName, panicErr)
		}
	}()

	if r.ragSystem == nil {
		log.Errorf("RAG system not initialized for entity [%s]", entity.EntityName)
		return nil, utils.Errorf("RAG system is not initialized")
	}

	// 构建查询字符串
	queryString := entity.String()
	queryBuildTime := time.Since(methodStartTime)
	if queryBuildTime > 100*time.Millisecond {
		log.Warnf("SLOW QUERY BUILD: entity [%s] query string build took %v", entity.EntityName, queryBuildTime)
	}

	// 执行向量查询 - 这是最耗时的步骤
	vectorQueryStart := time.Now()
	results, err := r.QueryVector(queryString, r.runtimeConfig.queryTop)
	vectorQueryDuration := time.Since(vectorQueryStart)

	if err != nil {
		log.Errorf("vector query failed for entity [%s]: %v", entity.EntityName, err)
		return nil, err
	}

	// 分析向量查询结果
	vectorResultsCount := len(results)
	filteredResultsCount := 0

	if vectorResultsCount == 0 {
		totalDuration := time.Since(methodStartTime)
		if totalDuration > 5*time.Second {
			log.Warnf("EMPTY VECTOR SEARCH: entity [%s] query took %v but returned 0 results (query: %s)",
				entity.EntityName, totalDuration, queryString[:min(50, len(queryString))])
		}
		return nil, nil
	}

	// 过滤和处理结果
	filterStartTime := time.Now()
	var entityIndex []string
	lowScoreCount := 0
	wrongTypeCount := 0

	for _, res := range results {
		if res.Score < r.runtimeConfig.similarityThreshold {
			lowScoreCount++
			continue
		}
		if res.Document.Type == schema.RAGDocumentType_Entity {
			index, ok := res.Document.Metadata.GetDataUUID()
			if ok {
				entityIndex = append(entityIndex, utils.InterfaceToString(index))
				filteredResultsCount++
			}
		} else {
			wrongTypeCount++
		}
	}

	filterDuration := time.Since(filterStartTime)

	if len(entityIndex) == 0 {
		log.Warnf("NO VALID ENTITIES: entity [%s] vector search found %d results but 0 valid entities (low_score: %d, wrong_type: %d)",
			entity.EntityName, vectorResultsCount, lowScoreCount, wrongTypeCount)
		return nil, nil
	}

	// 数据库查询实体详情
	dbQueryStart := time.Now()
	finalResults, err := r.queryEntities(&ypb.EntityFilter{
		HiddenIndex: entityIndex,
	})
	dbQueryDuration := time.Since(dbQueryStart)

	if err != nil {
		log.Errorf("database query failed for entity [%s]: %v", entity.EntityName, err)
		return nil, err
	}

	totalDuration := time.Since(methodStartTime)

	// 详细性能分析
	if totalDuration > 30*time.Second {
		log.Errorf("CRITICAL VECTOR SEARCH PERFORMANCE: entity [%s] total %v", entity.EntityName, totalDuration)
		log.Errorf("  - Query build: %v", queryBuildTime)
		log.Errorf("  - Vector query: %v (%d raw results)", vectorQueryDuration, vectorResultsCount)
		log.Errorf("  - Result filter: %v (filtered: %d/%d, low_score: %d, wrong_type: %d)",
			filterDuration, filteredResultsCount, vectorResultsCount, lowScoreCount, wrongTypeCount)
		log.Errorf("  - DB query: %v (%d final results)", dbQueryDuration, len(finalResults))
		log.Errorf("  - Query: %s", queryString[:min(100, len(queryString))])
		log.Errorf("  - Config: threshold=%.2f, top=%d", r.runtimeConfig.similarityThreshold, r.runtimeConfig.queryTop)
	} else if totalDuration > 10*time.Second {
		log.Warnf("SLOW VECTOR SEARCH BREAKDOWN: entity [%s] total %v, vector %v, db %v, results %d/%d",
			entity.EntityName, totalDuration, vectorQueryDuration, dbQueryDuration, len(finalResults), vectorResultsCount)
	}

	return finalResults, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// VectorYieldEntity 使用向量搜索实体，注意这里使用增强查询，不能在实时性高的过程调用！
func (r *EntityRepository) VectorYieldEntity(ctx context.Context, query string) (<-chan *rag.RAGSearchResult, error) {
	return rag.Query(r.db, query,
		rag.WithRAGLimit(r.runtimeConfig.queryTop),
		rag.WithRAGCtx(ctx),
		rag.WithRAGCollectionName(r.info.EntityBaseName),
		rag.WithRAGCollectionScoreLimit(r.runtimeConfig.similarityThreshold),
	)
}

func (r *EntityRepository) queryEntities(filter *ypb.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.BaseIndex = r.info.Uuid
	filter.RuntimeID = []string{r.runtimeConfig.runtimeID}
	return yakit.QueryEntities(r.db, filter)
}

func (r *EntityRepository) addEntityToVectorIndex(entry *schema.ERModelEntity) error {
	metadata := map[string]any{
		schema.META_Data_UUID:  entry.Uuid,
		schema.META_Data_Title: entry.EntityName,
		schema.META_Repos_UUID: entry.RepositoryUUID,
		META_EntityType:        entry.EntityType,
	}

	var opts []rag.DocumentOption

	opts = append(opts, rag.WithDocumentRawMetadata(metadata),
		rag.WithDocumentType(schema.RAGDocumentType_Entity),
		rag.WithDocumentEntityID(entry.Uuid), // let RAG system generate embedding
		rag.WithDocumentRuntimeID(entry.RuntimeID),
	)
	documentID := fmt.Sprintf("%v_entity", entry.Uuid)
	content := entry.ToRAGContent()
	return r.AddVectorIndex(documentID, content, opts...)
}

func (r *EntityRepository) addRelationshipToVectorIndex(relationship *schema.ERModelRelationship) error {
	src, err := r.GetEntityByUUID(relationship.SourceEntityIndex)
	if err != nil {
		return utils.Errorf("failed to get source entity by uuid [%s]: %v", relationship.SourceEntityIndex, err)
	}
	srcDoc := src.ToRAGContent()
	dst, err := r.GetEntityByUUID(relationship.TargetEntityIndex)
	if err != nil {
		return utils.Errorf("failed to get target entity by uuid [%s]: %v", relationship.TargetEntityIndex, err)
	}
	dstDoc := dst.ToRAGContent()
	content := relationship.ToRAGContent(srcDoc, dstDoc)
	metadata := map[string]any{
		schema.META_Data_UUID:  relationship.Uuid,
		schema.META_Data_Title: fmt.Sprintf("关系[%s]", relationship.RelationshipTypeVerbose),
		schema.META_Repos_UUID: relationship.RepositoryUUID,
	}

	return r.AddVectorIndex(relationship.Uuid, content,
		rag.WithDocumentType(schema.RAGDocumentType_Relationship),
		rag.WithDocumentRelatedEntities(relationship.SourceEntityIndex, relationship.TargetEntityIndex),
		rag.WithDocumentRuntimeID(relationship.RuntimeID),
		rag.WithDocumentRawMetadata(metadata))
}

func (r *EntityRepository) MergeAndSaveEntity(entity *schema.ERModelEntity) (*schema.ERModelEntity, error) {
	matchedEntity, _, err := r.MatchEntities(entity)
	if err != nil { // not critical error
		log.Errorf("failed to match entity [%s]: %v", entity.EntityName, err)
	}
	if len(matchedEntity) <= 0 {
		log.Infof("start to create entity: %s", entity.EntityName)
		err = r.CreateEntity(entity)
		if err != nil {
			return nil, utils.Errorf("failed to create entity [%s]: %v", entity.EntityName, err)
		}
		return entity, nil
	}

	var firstEntity = matchedEntity[0]
	for _, m := range matchedEntity {
		if m.CreatedAt.Before(firstEntity.CreatedAt) {
			firstEntity = m
		}
		m.Attributes = utils.MergeGeneralMap(m.Attributes, entity.Attributes)
	}

	err = r.UpdateEntity(firstEntity.ID, firstEntity) // 只更新最早创建的实体 并为它生成冗余向量
	if err != nil {
		log.Errorf("failed to update entity [%s]: %v", firstEntity.EntityName, err)
	}

	return firstEntity, nil // 返回最早创建的实体，用于将关系集中联系在一个实体上，用于维护无目的质量中心
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
	entity.RuntimeID = r.runtimeConfig.runtimeID
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

func (r *EntityRepository) MergeAndSaveRelationship(newRelationship *schema.ERModelRelationship) error {
	oldRelationships, err := r.queryRelationship(&ypb.RelationshipFilter{
		SourceEntityIndex: []string{newRelationship.SourceEntityIndex},
		TargetEntityIndex: []string{newRelationship.TargetEntityIndex},
		Types:             []string{newRelationship.RelationshipType},
	})
	if err != nil {
		return utils.Errorf("failed to query relationship: %v", err)
	}
	similarCheck := func(old string, new string) bool {
		if old == new {
			return true
		}
		score, err := r.GetRAGSystem().VectorSimilarity(old, new)
		if err != nil {
			log.Errorf("failed to calculate relationship similarity: %v", err)
		}
		if score > r.runtimeConfig.similarityThreshold {
			return true
		}
		return false
	}
	for _, relationship := range oldRelationships { // 关系相对于实体来说相对明确，可以简单地通过语义相似度做合并
		if similarCheck(relationship.RelationshipType, relationship.RelationshipType) {
			relationship.Attributes = utils.MergeGeneralMap(newRelationship.Attributes, relationship.Attributes)
			return r.UpdateRelationship(relationship.Uuid, relationship)
		}
	}
	return r.AddRelationship(newRelationship.SourceEntityIndex, newRelationship.TargetEntityIndex, newRelationship.RelationshipType, newRelationship.RelationshipTypeVerbose, newRelationship.Attributes)
}

func (r *EntityRepository) UpdateRelationship(uuid string, relationship *schema.ERModelRelationship) error {
	err := yakit.UpdateRelationship(r.db, uuid, relationship)
	if err != nil {
		return err
	}

	go func() {
		err = r.addRelationshipToVectorIndex(relationship)
		if err != nil {
			log.Warnf("failed to add relation [%s] to vector index: %v", relationship.RelationshipType, err)
		}
	}()
	return nil
}

func (r *EntityRepository) AddRelationship(sourceIndex, targetIndex string, relationType string, typeVerbose string, attr map[string]any) error {
	data, err := yakit.AddRelationship(r.db, sourceIndex, targetIndex, r.info.Uuid, relationType, typeVerbose, attr, r.runtimeConfig.runtimeID)
	if err != nil {
		log.Warnf("failed to add relation [%s] to vector [%s]: %v", relationType, sourceIndex, err)
		return utils.Wrapf(err, "failed to add relation [%s] to vector [%s]", relationType, sourceIndex)
	}
	go func() {
		err = r.addRelationshipToVectorIndex(data)
		if err != nil {
			log.Warnf("failed to add relation [%s] to vector index: %v", relationType, err)
		}
	}()
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

func GetEntityRepositoryByName(db *gorm.DB, name string, opts ...any) (*EntityRepository, error) {
	var entityBaseInfo schema.EntityRepository
	err := db.Model(&schema.EntityRepository{}).Where("entity_base_name = ?", name).First(&entityBaseInfo).Error
	if err != nil {
		return nil, err
	}

	collectionExists := rag.CollectionIsExists(db, name)

	var ragSystem *rag.RAGSystem
	if !collectionExists {
		ragSystem, err = rag.CreateCollection(db, name, entityBaseInfo.Description, opts...)
		if err != nil {
			_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
				return yakit.DeleteEntityBaseInfo(tx, int64(entityBaseInfo.ID))
			})
			return nil, utils.Errorf("create entity repository & rag collection err: %v", err)
		}
	} else {
		ragSystem, err = rag.LoadCollectionEx(db, name)
		if err != nil {
			return nil, utils.Errorf("加载RAG集合失败: %v", err)
		}
	}
	var repos = &EntityRepository{
		db:            db,
		info:          &entityBaseInfo,
		ragSystem:     ragSystem,
		runtimeConfig: NewRuntimeConfig(opts...),
	}

	return repos, nil
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
		ragSystem, err = rag.LoadCollectionEx(db, name)
		if err != nil {
			return nil, utils.Errorf("加载RAG集合失败: %v", err)
		}
	}
	var repos = &EntityRepository{
		db:            db,
		info:          &entityBaseInfo,
		ragSystem:     ragSystem,
		runtimeConfig: NewRuntimeConfig(opts...),
	}

	return repos, nil
}
