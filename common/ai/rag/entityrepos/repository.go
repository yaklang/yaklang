package entityrepos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/asynchelper"
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
	ctx                 context.Context
	disableBulkProcess  bool

	entityRagQueryCache *utils.CacheEx[*schema.ERModelEntity]

	vectorStoreOptions []vectorstore.CollectionConfigFunc
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

func WithRuntimeID(runtimeID string) RuntimeConfigOption {
	return func(config *EntityRepositoryRuntimeConfig) {
		config.runtimeID = runtimeID
	}
}

func WithDisableBulkProcess() RuntimeConfigOption {
	return func(config *EntityRepositoryRuntimeConfig) {
		config.disableBulkProcess = true
	}
}

func WithContext(ctx context.Context) RuntimeConfigOption {
	return func(config *EntityRepositoryRuntimeConfig) {
		config.ctx = ctx
	}
}

func WithVectorStoreOptions(opts ...vectorstore.CollectionConfigFunc) RuntimeConfigOption {
	return func(config *EntityRepositoryRuntimeConfig) {
		config.vectorStoreOptions = opts
	}
}

func NewRuntimeConfig(opts ...any) *EntityRepositoryRuntimeConfig {
	config := &EntityRepositoryRuntimeConfig{
		similarityThreshold: 0.6, // 降低相似度阈值，减少low_score问题
		queryTop:            10,  // 增加查询数量，提高匹配概率
		runtimeID:           uuid.NewString(),
		ctx:                 context.Background(),
	}
	for _, opt := range opts {
		switch opt := opt.(type) {
		case RuntimeConfigOption:
			opt(config)
		case vectorstore.CollectionConfigFunc:
			config.vectorStoreOptions = append(config.vectorStoreOptions, opt)
		}
	}
	config.entityRagQueryCache = utils.NewCacheEx[*schema.ERModelEntity]()

	return config
}

type EntityRepository struct {
	db            *gorm.DB
	info          *schema.EntityRepository
	collectionMg  *vectorstore.SQLiteVectorStoreHNSW
	wg            sync.WaitGroup
	bulkProcessor *bulkProcessor
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

func (r *EntityRepository) GetCollectionMg() *vectorstore.SQLiteVectorStoreHNSW {
	return r.collectionMg
}

func (r *EntityRepository) AddVectorIndex(docId string, content string, opts ...vectorstore.DocumentOption) error {
	if r.bulkProcessor != nil {
		r.bulkProcessor.addRequest(docId, content, opts...)
		return nil
	}

	var finishCh = make(chan struct{})
	var err error
	// 实现锁超时机制防止永久死锁
	go func() {
		defer func() {
			if recover() != nil {
				log.Errorf("PANIC in AddVectorIndex lock acquire for doc [%s]", docId[:min(50, len(docId))])
			}
		}()

		ragAddStart := time.Now()
		err = r.GetCollectionMg().AddWithOptions(docId, content, opts...)
		ragAddDuration := time.Since(ragAddStart)
		close(finishCh)

		// 记录总体性能和分解
		if ragAddDuration > 10*time.Second {
			log.Errorf("CRITICAL AddVectorIndex: doc [%s] (rag_add=%v)",
				docId[:min(50, len(docId))], ragAddDuration)
		} else if ragAddDuration > 3*time.Second {
			log.Warnf("SLOW AddVectorIndex: doc [%s] (rag_add=%v)",
				docId[:min(50, len(docId))], ragAddDuration)
		}
	}()

	// 实现30秒超时机制
	select {
	case <-r.runtimeConfig.ctx.Done():
		return utils.Errorf("context cacel")
	case <-finishCh:
		return err
	case <-time.After(30 * time.Second):
		// 超时，强制返回错误，避免永久阻塞
		log.Errorf("DEADLOCK RECOVERY: AddVectorIndex lock timeout after 30s for doc [%s] - forcing abort", docId[:min(50, len(docId))])
		utils.PrintCurrentGoroutineRuntimeStack()
		return utils.Errorf("vector index operation timeout: possible deadlock detected")
	}
}

func (r *EntityRepository) QueryVector(query string, top int) ([]vectorstore.SearchResult, error) {
	queryStartTime := time.Now()

	// 实现读锁超时机制防止死锁
	resultCh := make(chan struct {
		results []vectorstore.SearchResult
		err     error
	}, 1)

	go func() {
		defer func() {
			if recover() != nil {
				log.Errorf("PANIC in QueryVector for query [%s]", query[:min(50, len(query))])
			}
		}()

		actualQueryStart := time.Now()
		results, err := r.GetCollectionMg().Query(query, top)
		queryDuration := time.Since(actualQueryStart)

		totalDuration := time.Since(queryStartTime)

		if err != nil {
			log.Errorf("RAG Query failed: query='%s', top=%d, error=%v", query[:min(50, len(query))], top, err)
		}

		// 性能监控 - 增强死锁检测
		if totalDuration > 15*time.Second {
			log.Errorf("CRITICAL RAG QUERY DEADLOCK SUSPECTED: query='%s' took %v ( actual_query: %v), returned %d results",
				query[:min(50, len(query))], totalDuration, queryDuration, len(results))
			utils.PrintCurrentGoroutineRuntimeStack()
		} else if totalDuration > 10*time.Second {
			log.Errorf("CRITICAL RAG QUERY: query='%s' took %v (query: %v), returned %d results",
				query[:min(50, len(query))], totalDuration, queryDuration, len(results))
		} else if totalDuration > 3*time.Second {
			log.Warnf("SLOW RAG QUERY: query='%s' took %v (query: %v), returned %d results",
				query[:min(50, len(query))], totalDuration, queryDuration, len(results))
		}

		// 记录低效查询（耗时长但结果少）
		if totalDuration > 5*time.Second && len(results) < 3 {
			log.Warnf("INEFFICIENT RAG QUERY: query='%s' took %v but only returned %d results",
				query[:min(100, len(query))], totalDuration, len(results))
		}

		select {
		case resultCh <- struct {
			results []vectorstore.SearchResult
			err     error
		}{results, err}:
		default:
			// 主goroutine已经超时，不发送结果
		}
	}()

	// 实现15秒超时机制
	select {
	case <-r.runtimeConfig.ctx.Done():
		return nil, utils.Errorf("context cacel")
	case result := <-resultCh:
		return result.results, result.err
	case <-time.After(15 * time.Second):
		log.Errorf("DEADLOCK RECOVERY: QueryVector timeout after 15s for query [%s] - forcing abort", query[:min(50, len(query))])
		return nil, utils.Errorf("vector query timeout: possible deadlock detected")
	}
}

//--- Entity Operations ---

func (r *EntityRepository) getEntityCache(cacheKey string) *schema.ERModelEntity {
	if r.runtimeConfig.entityRagQueryCache != nil {
		cacheData, ok := r.runtimeConfig.entityRagQueryCache.Get(cacheKey)
		if ok {
			return cacheData
		}
	}
	return nil
}

func (r *EntityRepository) cacheEntity(cacheKey string, entity *schema.ERModelEntity) {
	if entity == nil || entity.Uuid == "" {
		log.Errorf("entity is nil or empty for entity")
		return
	}
	if r.runtimeConfig.entityRagQueryCache != nil {
		r.runtimeConfig.entityRagQueryCache.Set(cacheKey, entity)
	}
}

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

	if r.collectionMg == nil {
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

	if r.collectionMg == nil {
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
		// 记录详细的分数信息用于调试
		if res.Score < r.runtimeConfig.similarityThreshold {
			lowScoreCount++
			// 当分数很接近阈值时记录详细信息
			if res.Score > r.runtimeConfig.similarityThreshold-0.1 {
				log.Debugf("Near-threshold entity: [%s] score=%.3f (threshold=%.3f, diff=%.3f)",
					entity.EntityName, res.Score, r.runtimeConfig.similarityThreshold,
					r.runtimeConfig.similarityThreshold-res.Score)
			}
			continue
		}
		if res.Document.Type == schema.RAGDocumentType_Entity {
			index, ok := res.Document.Metadata.GetDataUUID()
			if ok {
				entityIndex = append(entityIndex, utils.InterfaceToString(index))
				filteredResultsCount++
				// 记录成功匹配的实体分数
				log.Debugf("Matched entity: [%s] score=%.3f", entity.EntityName, res.Score)
			}
		} else {
			wrongTypeCount++
		}
	}

	filterDuration := time.Since(filterStartTime)

	if len(entityIndex) == 0 {
		// 增强低分数问题的诊断信息
		maxScore := 0.0
		for _, res := range results {
			if res.Score > maxScore {
				maxScore = res.Score
			}
		}

		log.Warnf("NO VALID ENTITIES: entity [%s] vector search found %d results but 0 valid entities (low_score: %d, wrong_type: %d, max_score: %.3f, threshold: %.3f)",
			entity.EntityName, vectorResultsCount, lowScoreCount, wrongTypeCount, maxScore, r.runtimeConfig.similarityThreshold)

		// 如果最高分数非常接近阈值，建议调整阈值
		if maxScore > 0 && maxScore < r.runtimeConfig.similarityThreshold && (r.runtimeConfig.similarityThreshold-maxScore) < 0.2 {
			log.Warnf("SIMILARITY THRESHOLD TOO HIGH: entity [%s] max_score=%.3f close to threshold=%.3f, consider lowering threshold",
				entity.EntityName, maxScore, r.runtimeConfig.similarityThreshold)
		}

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
func (r *EntityRepository) VectorYieldEntity(ctx context.Context, query string) (<-chan *vectorstore.RAGSearchResult, error) {
	return vectorstore.Query(r.db, query,
		vectorstore.WithRAGLimit(r.runtimeConfig.queryTop),
		vectorstore.WithRAGCtx(ctx),
		vectorstore.WithRAGCollectionName(r.info.EntityBaseName),
		vectorstore.WithRAGCollectionScoreLimit(r.runtimeConfig.similarityThreshold),
	)
}

func (r *EntityRepository) queryEntities(filter *ypb.EntityFilter) ([]*schema.ERModelEntity, error) {
	filter.BaseIndex = r.info.Uuid
	filter.RuntimeID = []string{r.runtimeConfig.runtimeID}
	return yakit.QueryEntities(r.db, filter)
}

func (r *EntityRepository) addEntityToVectorIndex(entry *schema.ERModelEntity) error {
	addEntityStartTime := time.Now()
	defer func() {
		duration := time.Since(addEntityStartTime)
		if duration > 3*time.Second {
			log.Warnf("SLOW addEntityToVectorIndex: entity [%s] took %v", entry.EntityName, duration)
		}
		if duration > 10*time.Second {
			log.Errorf("CRITICAL addEntityToVectorIndex: entity [%s] took %v - possible deadlock", entry.EntityName, duration)
		}
	}()

	metadataStartTime := time.Now()
	metadata := map[string]any{
		schema.META_Data_UUID:  entry.Uuid,
		schema.META_Data_Title: entry.EntityName,
		schema.META_Repos_UUID: entry.RepositoryUUID,
		META_EntityType:        entry.EntityType,
	}

	var opts []vectorstore.DocumentOption

	opts = append(opts, vectorstore.WithDocumentRawMetadata(metadata),
		vectorstore.WithDocumentType(schema.RAGDocumentType_Entity),
		vectorstore.WithDocumentEntityID(entry.Uuid), // let RAG system generate embedding
		vectorstore.WithDocumentRuntimeID(entry.RuntimeID),
	)
	documentID := fmt.Sprintf("%v_entity", entry.Uuid)
	content := entry.ToRAGContent()
	metadataDuration := time.Since(metadataStartTime)

	vectorIndexStartTime := time.Now()
	err := r.AddVectorIndex(documentID, content, opts...)
	vectorIndexDuration := time.Since(vectorIndexStartTime)

	totalDuration := time.Since(addEntityStartTime)
	if totalDuration > 5*time.Second {
		log.Warnf("addEntityToVectorIndex PERFORMANCE: entity [%s] total=%v, metadata=%v, vectorIndex=%v",
			entry.EntityName, totalDuration, metadataDuration, vectorIndexDuration)
	}

	return err
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
		vectorstore.WithDocumentType(schema.RAGDocumentType_Relationship),
		vectorstore.WithDocumentRelatedEntities(relationship.SourceEntityIndex, relationship.TargetEntityIndex),
		vectorstore.WithDocumentRuntimeID(relationship.RuntimeID),
		vectorstore.WithDocumentRawMetadata(metadata))
}

func (r *EntityRepository) MergeAndSaveEntity(entity *schema.ERModelEntity) (*schema.ERModelEntity, error) {
	helper := asynchelper.NewAsyncPerformanceHelper("merge_and_save_entity")
	defer helper.Close()
	cacheKey := entity.EntityName
	if cacheData := r.getEntityCache(cacheKey); cacheData != nil {
		cacheData.Attributes = utils.MergeGeneralMap(cacheData.Attributes, entity.Attributes)
		err := r.UpdateEntity(cacheData.ID, cacheData)
		if err != nil {
			log.Errorf("failed to update entity [%s]: %v", cacheData.EntityName, err)
		}
		return cacheData, nil
	}

	helper.MarkNow()
	matchedEntity, isExactMatch, err := r.MatchEntities(entity)
	if err != nil { // not critical error
		log.Errorf("failed to match entity [%s]: %v", entity.EntityName, err)
	}
	matchDuration := helper.CheckLastMarkAndLog(3*time.Second, "match_entities")

	if len(matchedEntity) <= 0 {
		// 记录实体创建的原因和统计信息
		log.Infof("Creating new entity: %s (no matches found after %v)", entity.EntityName, matchDuration)

		helper.MarkNow()
		err = r.CreateEntity(entity)
		helper.CheckLastMarkAndLog(2*time.Second, "create_entity")

		if err != nil {
			return nil, utils.Errorf("failed to create entity [%s]: %v", entity.EntityName, err)
		}
		r.cacheEntity(cacheKey, entity)
		return entity, nil
	}

	// 记录成功匹配的情况
	log.Debugf("Entity matched: %s found %d matches (exact: %v)", entity.EntityName, len(matchedEntity), isExactMatch)

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
	r.cacheEntity(cacheKey, firstEntity)
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
		goroutineStartTime := time.Now()
		defer func() {
			goroutineDuration := time.Since(goroutineStartTime)
			// 增强异步操作死锁检测
			if goroutineDuration > 30*time.Second {
				log.Errorf("CRITICAL ASYNC DEADLOCK: UpdateEntity vector index goroutine for [%s] took %v - possible deadlock", e.EntityName, goroutineDuration)
				utils.PrintCurrentGoroutineRuntimeStack()
			} else if goroutineDuration > 10*time.Second {
				log.Errorf("CRITICAL ASYNC SLOW: UpdateEntity vector index goroutine for [%s] took %v", e.EntityName, goroutineDuration)
			} else if goroutineDuration > 5*time.Second {
				log.Warnf("SLOW entity vector index goroutine: entity [%s] took %v", e.EntityName, goroutineDuration)
			}
		}()

		// 在独立goroutine中实现超时控制
		done := make(chan error, 1)
		go func() {
			done <- r.addEntityToVectorIndex(e)
		}()

		select {
		case err := <-done:
			if err != nil {
				log.Errorf("failed to add entity [%s] to vector index: %v", e.EntityName, err)
			}
		case <-time.After(35 * time.Second):
			log.Errorf("ASYNC DEADLOCK ABORT: UpdateEntity vector index for [%s] timeout after 35s - abandoning operation", e.EntityName)
			// 不等待结果，直接返回避免goroutine泄漏加剧死锁
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
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		goroutineStartTime := time.Now()
		defer func() {
			goroutineDuration := time.Since(goroutineStartTime)
			// 增强异步操作死锁检测
			if goroutineDuration > 30*time.Second {
				log.Errorf("CRITICAL ASYNC DEADLOCK: CreateEntity vector index goroutine for [%s] took %v - possible deadlock", entity.EntityName, goroutineDuration)
				utils.PrintCurrentGoroutineRuntimeStack()
			} else if goroutineDuration > 10*time.Second {
				log.Errorf("CRITICAL ASYNC SLOW: CreateEntity vector index goroutine for [%s] took %v", entity.EntityName, goroutineDuration)
			} else if goroutineDuration > 5*time.Second {
				log.Warnf("SLOW entity vector index goroutine: entity [%s] took %v", entity.EntityName, goroutineDuration)
			}
		}()

		// 在独立goroutine中实现超时控制
		done := make(chan error, 1)
		go func() {
			done <- r.addEntityToVectorIndex(entity)
		}()

		select {
		case err := <-done:
			if err != nil {
				log.Errorf("failed to add entity [%s] to vector index: %v", entity.EntityName, err)
			}
		case <-time.After(35 * time.Second):
			log.Errorf("ASYNC DEADLOCK ABORT: CreateEntity vector index for [%s] timeout after 35s - abandoning operation", entity.EntityName)
			// 不等待结果，直接返回避免goroutine泄漏加剧死锁
		}
	}()
	return nil
}

func (r *EntityRepository) Wait() {
	r.wg.Wait()
}

//--- Relationship Operations ---

func (r *EntityRepository) MergeAndSaveRelationship(newRelationship *schema.ERModelRelationship) error {
	mergeStartTime := time.Now()
	defer func() {
		duration := time.Since(mergeStartTime)
		if duration > 5*time.Second {
			log.Warnf("SLOW MergeAndSaveRelationship: relationship [%s->%s] took %v",
				newRelationship.SourceEntityIndex[:min(20, len(newRelationship.SourceEntityIndex))],
				newRelationship.TargetEntityIndex[:min(20, len(newRelationship.TargetEntityIndex))], duration)
		}
	}()

	queryStartTime := time.Now()
	oldRelationships, err := r.queryRelationship(&ypb.RelationshipFilter{
		SourceEntityIndex: []string{newRelationship.SourceEntityIndex},
		TargetEntityIndex: []string{newRelationship.TargetEntityIndex},
		Types:             []string{newRelationship.RelationshipType},
	})
	queryDuration := time.Since(queryStartTime)

	if err != nil {
		return utils.Errorf("failed to query relationship: %v", err)
	}

	if queryDuration > time.Second {
		log.Warnf("SLOW relationship query: took %v for %d results", queryDuration, len(oldRelationships))
	}

	similarCheck := func(old string, new string) bool {
		if old == new {
			return true
		}
		// 避免在关系合并时调用VectorSimilarity，这可能导致死锁
		// VectorSimilarity需要RLock，而当前可能已经持有其他锁
		log.Debugf("relationship type similarity check avoided: %s vs %s", old, new)
		return false // 简化逻辑，避免潜在死锁
	}

	similarityStartTime := time.Now()
	var similarityChecks int
	for _, relationship := range oldRelationships { // 关系相对于实体来说相对明确，可以简单地通过语义相似度做合并
		similarityChecks++
		if similarCheck(relationship.RelationshipType, newRelationship.RelationshipType) {
			relationship.Attributes = utils.MergeGeneralMap(newRelationship.Attributes, relationship.Attributes)
			updateStartTime := time.Now()
			err := r.UpdateRelationship(relationship.Uuid, relationship)
			updateDuration := time.Since(updateStartTime)

			if updateDuration > time.Second {
				log.Warnf("SLOW relationship update: took %v", updateDuration)
			}

			return err
		}
	}
	similarityDuration := time.Since(similarityStartTime)

	if similarityDuration > 2*time.Second {
		log.Warnf("SLOW similarity checks: %d checks took %v", similarityChecks, similarityDuration)
	}

	addStartTime := time.Now()
	err = r.AddRelationship(newRelationship.SourceEntityIndex, newRelationship.TargetEntityIndex, newRelationship.RelationshipType, newRelationship.RelationshipTypeVerbose, newRelationship.Attributes)
	addDuration := time.Since(addStartTime)

	if addDuration > time.Second {
		log.Warnf("SLOW relationship add: took %v", addDuration)
	}

	return err
}

func (r *EntityRepository) UpdateRelationship(uuid string, relationship *schema.ERModelRelationship) error {
	err := yakit.UpdateRelationshipByUUID(r.db, uuid, relationship)
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
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
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

func (r *EntityRepository) StartBulkProcessor() error {
	bp := startBulkProcessor(r.runtimeConfig.ctx, r.collectionMg, 10, 3*time.Second)
	r.bulkProcessor = bp
	return nil
}

func GetEntityRepositoryByName(db *gorm.DB, name string, opts ...any) (*EntityRepository, error) {
	runtimeConfig := NewRuntimeConfig(opts...)
	var entityBaseInfo schema.EntityRepository
	err := db.Model(&schema.EntityRepository{}).Where("entity_base_name = ?", name).First(&entityBaseInfo).Error
	if err != nil {
		return nil, err
	}

	collectionExists := vectorstore.HasCollection(db, name)

	var collectionMg *vectorstore.SQLiteVectorStoreHNSW
	if !collectionExists {
		collectionMg, err = vectorstore.CreateCollection(db, name, entityBaseInfo.Description, runtimeConfig.vectorStoreOptions...)
		if err != nil {
			_ = utils.GormTransaction(db, func(tx *gorm.DB) error {
				return yakit.DeleteEntityBaseInfo(tx, int64(entityBaseInfo.ID))
			})
			return nil, utils.Errorf("create entity repository & rag collection err: %v", err)
		}
	} else {
		collectionMg, err = vectorstore.LoadCollection(db, name, runtimeConfig.vectorStoreOptions...)
		if err != nil {
			return nil, utils.Errorf("加载RAG集合失败: %v", err)
		}
	}
	var repos = &EntityRepository{
		db:            db,
		info:          &entityBaseInfo,
		collectionMg:  collectionMg,
		runtimeConfig: NewRuntimeConfig(opts...),
	}
	return repos, nil
}

func GetEntityRepositoryWithVectorStore(db *gorm.DB, name, description string, vectorStore *vectorstore.SQLiteVectorStoreHNSW, opts ...any) (*EntityRepository, error) {
	runtimeConfig := NewRuntimeConfig(opts...)
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
	var repos = &EntityRepository{
		db:            db,
		info:          &entityBaseInfo,
		collectionMg:  vectorStore,
		runtimeConfig: runtimeConfig,
	}

	return repos, nil
}

func GetOrCreateEntityRepository(db *gorm.DB, name, description string, opts ...any) (*EntityRepository, error) {
	runtimeConfig := NewRuntimeConfig(opts...)
	var err error
	collectionExists := vectorstore.HasCollection(db, name)

	var collectionMg *vectorstore.SQLiteVectorStoreHNSW
	if !collectionExists {
		collectionMg, err = vectorstore.CreateCollection(db, name, description, runtimeConfig.vectorStoreOptions...)
		if err != nil {
			return nil, utils.Errorf("create entity repository & rag collection err: %v", err)
		}
	} else {
		collectionMg, err = vectorstore.LoadCollection(db, name, runtimeConfig.vectorStoreOptions...)
		if err != nil {
			return nil, utils.Errorf("加载RAG集合失败: %v", err)
		}
	}

	return GetEntityRepositoryWithVectorStore(db, name, description, collectionMg, opts...)
}

// Export 导出实体仓库
func (r *EntityRepository) Export(ctx context.Context, opts *ExportEntityRepositoryOptions) (io.Reader, error) {
	if opts == nil {
		opts = &ExportEntityRepositoryOptions{}
	}
	opts.RepositoryID = r.GetID()
	return ExportEntityRepository(ctx, r.db, opts)
}

// DeleteEntityRepository 删除实体仓库
func DeleteEntityRepository(db *gorm.DB, name string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		var info schema.EntityRepository
		err := tx.Model(&schema.EntityRepository{}).Where("entity_base_name = ?", name).First(&info).Error
		if err != nil {
			return utils.Errorf("get EntityRepository failed: %s", err)
		}

		// 删除所有实体
		err = tx.Where("repository_uuid = ?", info.Uuid).Unscoped().Delete(&schema.ERModelEntity{}).Error
		if err != nil {
			return utils.Errorf("delete entities failed: %s", err)
		}

		// 删除所有关系
		err = tx.Where("repository_uuid = ?", info.Uuid).Unscoped().Delete(&schema.ERModelRelationship{}).Error
		if err != nil {
			return utils.Errorf("delete relationships failed: %s", err)
		}

		// 删除实体仓库信息
		err = tx.Where("id = ?", info.ID).Unscoped().Delete(&schema.EntityRepository{}).Error
		if err != nil {
			return utils.Errorf("delete EntityRepository failed: %s", err)
		}

		return nil
	})
}
