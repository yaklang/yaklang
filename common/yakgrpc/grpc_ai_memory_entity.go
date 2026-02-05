package yakgrpc

import (
	"context"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateAIMemoryEntity(ctx context.Context, req *ypb.CreateAIMemoryEntityRequest) (*ypb.Empty, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	memory, err := aimem.NewAIMemory(req.GetSessionID(), aimem.WithAutoReActInvoker(aicommon.WithContext(ctx)))
	if err != nil {
		return nil, err
	}
	err = memory.HandleMemory(req.GetFreeInput())
	if err != nil {
		return nil, err
	}

	return &ypb.Empty{}, nil
}

func (s *Server) UpdateAIMemoryEntity(ctx context.Context, req *ypb.AIMemoryEntity) (*ypb.DbOperateMessage, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	if req == nil {
		return nil, utils.Errorf("request is nil")
	}

	next := schema.GRPC2AIMemoryEntity(req)

	var prev schema.AIMemoryEntity
	if err := db.Where("session_id = ? AND memory_id = ?", next.SessionID, next.MemoryID).First(&prev).Error; err != nil {
		return nil, err
	}
	old := prev

	prev.Content = next.Content
	prev.Tags = next.Tags
	prev.PotentialQuestions = next.PotentialQuestions
	prev.C_Score = next.C_Score
	prev.O_Score = next.O_Score
	prev.R_Score = next.R_Score
	prev.E_Score = next.E_Score
	prev.P_Score = next.P_Score
	prev.A_Score = next.A_Score
	prev.T_Score = next.T_Score
	prev.CorePactVector = next.CorePactVector

	if err := db.Save(&prev).Error; err != nil {
		return nil, err
	}

	_ = syncAIMemoryVectors(ctx, db, &prev, &old)

	return &ypb.DbOperateMessage{
		TableName:  prev.TableName(),
		Operation:  "update",
		EffectRows: 1,
	}, nil
}

func (s *Server) DeleteAIMemoryEntity(ctx context.Context, req *ypb.DeleteAIMemoryEntityRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}

	if req == nil {
		return nil, utils.Errorf("request is nil")
	}

	vecSingleton := s.getAIMemoryVectorSingleton()
	count, err := yakit.DeleteAIMemoryEntityBatched(ctx, db, req.GetFilter(), 200, func(ctx context.Context, _ *gorm.DB, entities []schema.AIMemoryEntity) error {
		return deleteAIMemoryVectorsBatch(ctx, vecSingleton, entities)
	})
	if err != nil {
		return nil, err
	}

	return &ypb.DbOperateMessage{
		TableName:  (&schema.AIMemoryEntity{}).TableName(),
		Operation:  "delete",
		EffectRows: count,
	}, nil
}

// aiMemoryVectorSessionSingleton 保证再批量删除的时候同一会话ID的HNSW和RAG实例不会重复创建，减少性能消耗
type aiMemoryVectorSessionSingleton struct {
	db *gorm.DB

	mu sync.Mutex

	hnswBackends map[string]*aimem.AIMemoryHNSWBackend
	ragStores    map[string]*vectorstore.SQLiteVectorStoreHNSW
	ragExists    map[string]bool
}

func newAIMemoryVectorSessionSingleton(db *gorm.DB) *aiMemoryVectorSessionSingleton {
	return &aiMemoryVectorSessionSingleton{
		db:           db,
		hnswBackends: make(map[string]*aimem.AIMemoryHNSWBackend, 8),
		ragStores:    make(map[string]*vectorstore.SQLiteVectorStoreHNSW, 8),
		ragExists:    make(map[string]bool, 8),
	}
}

func (s *aiMemoryVectorSessionSingleton) GetHNSWBackend(sessionID string) (*aimem.AIMemoryHNSWBackend, error) {
	s.mu.Lock()
	if backend := s.hnswBackends[sessionID]; backend != nil {
		s.mu.Unlock()
		return backend, nil
	}
	s.mu.Unlock()

	backend, err := aimem.NewAIMemoryHNSWBackend(
		aimem.WithHNSWSessionID(sessionID),
		aimem.WithHNSWDatabase(s.db),
		aimem.WithHNSWAutoSave(false),
	)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if existed := s.hnswBackends[sessionID]; existed != nil {
		s.mu.Unlock()
		return existed, nil
	}
	s.hnswBackends[sessionID] = backend
	s.mu.Unlock()
	return backend, nil
}

func (s *aiMemoryVectorSessionSingleton) GetRAGStore(sessionID string) (*vectorstore.SQLiteVectorStoreHNSW, bool, error) {
	collectionName := aimem.Session2MemoryName(sessionID)

	s.mu.Lock()
	if store := s.ragStores[collectionName]; store != nil {
		s.mu.Unlock()
		return store, true, nil
	}
	if ok, exists := s.ragExists[collectionName]; exists && !ok {
		s.mu.Unlock()
		return nil, false, nil
	}
	s.mu.Unlock()

	if !vectorstore.HasCollection(s.db, collectionName) {
		s.mu.Lock()
		s.ragExists[collectionName] = false
		s.mu.Unlock()
		return nil, false, nil
	}

	store, err := vectorstore.LoadCollection(s.db, collectionName, vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()))
	if err != nil {
		return nil, false, err
	}

	s.mu.Lock()
	if existed := s.ragStores[collectionName]; existed != nil {
		s.mu.Unlock()
		return existed, true, nil
	}
	s.ragStores[collectionName] = store
	s.ragExists[collectionName] = true
	s.mu.Unlock()
	return store, true, nil
}

func deleteAIMemoryVectorsBatch(ctx context.Context, singleton *aiMemoryVectorSessionSingleton, entities []schema.AIMemoryEntity) error {
	if len(entities) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	uniqueStrings := func(in []string) []string {
		if len(in) <= 1 {
			return in
		}
		seen := make(map[string]struct{}, len(in))
		out := make([]string, 0, len(in))
		for _, v := range in {
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
		return out
	}

	type sessionPayload struct {
		memoryIDs []string
		docIDs    []string
	}
	bySession := make(map[string]*sessionPayload, 8)
	for i := range entities {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sessionID := strings.TrimSpace(entities[i].SessionID)
		if sessionID == "" {
			continue
		}
		p, ok := bySession[sessionID]
		if !ok {
			p = &sessionPayload{}
			bySession[sessionID] = p
		}
		if entities[i].MemoryID != "" {
			p.memoryIDs = append(p.memoryIDs, entities[i].MemoryID)
		}
		if ids := entities[i].DocumentQuestionHashIDs(); len(ids) > 0 {
			p.docIDs = append(p.docIDs, ids...)
		}
	}

	for sessionID, payload := range bySession {
		payload.memoryIDs = uniqueStrings(payload.memoryIDs)
		payload.docIDs = uniqueStrings(payload.docIDs)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		backend, err := singleton.GetHNSWBackend(sessionID)
		if err == nil {
			for _, memoryID := range payload.memoryIDs {
				_ = backend.Delete(memoryID)
			}
			if len(payload.memoryIDs) > 0 {
				if err := backend.SaveGraph(); err != nil {
					log.Warnf("AIMemory HNSW save skipped: %v", err)
				}
			}
		} else {
			log.Warnf("AIMemory HNSW delete skipped: %v", err)
		}

		if len(payload.docIDs) == 0 {
			continue
		}

		store, ok, err := singleton.GetRAGStore(sessionID)
		if err != nil {
			log.Warnf("AIMemory RAG delete skipped: %v", err)
			continue
		}
		if !ok {
			continue
		}
		if err := store.Delete(payload.docIDs...); err != nil {
			log.Warnf("AIMemory RAG delete docs skipped: %v", err)
		}
	}
	return nil
}

func (s *Server) GetAIMemoryEntity(ctx context.Context, req *ypb.GetAIMemoryEntityRequest) (*ypb.AIMemoryEntity, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}
	if req == nil {
		return nil, utils.Errorf("request is nil")
	}

	entity, err := yakit.GetAIMemoryEntity(db, strings.TrimSpace(req.GetSessionID()), strings.TrimSpace(req.GetMemoryID()))
	if err != nil {
		return nil, err
	}
	return entity.ToGRPC(), nil
}

func (s *Server) QueryAIMemoryEntity(ctx context.Context, req *ypb.QueryAIMemoryEntityRequest) (*ypb.QueryAIMemoryEntityResponse, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}
	if req == nil {
		return nil, utils.Errorf("request is nil")
	}

	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 10, OrderBy: "created_at", Order: "desc"}
	}
	if paging.GetPage() <= 0 {
		paging.Page = 1
	}
	if paging.GetLimit() == 0 {
		paging.Limit = 10
	}

	filter := req.GetFilter()
	if filter != nil {
		filter.SessionID = strings.TrimSpace(filter.GetSessionID())
	}

	// Vector semantic query (embedding based)
	if filter != nil && strings.TrimSpace(filter.GetSemanticQuery()) != "" {
		return queryAIMemoryBySemantic(db, paging, filter)
	}

	// Score-vector query (HNSW based)
	if filter != nil && len(filter.GetCorePactQueryVector()) > 0 {
		return queryAIMemoryByScoreVector(db, paging, filter)
	}

	pag, entities, err := yakit.QueryAIMemoryEntityPaging(db, filter, paging)
	if err != nil {
		return nil, err
	}

	results := make([]*ypb.AIMemoryEntity, 0, len(entities))
	for _, e := range entities {
		results = append(results, e.ToGRPC())
	}

	return &ypb.QueryAIMemoryEntityResponse{
		Pagination: &ypb.Paging{
			Page:    int64(pag.Page),
			Limit:   int64(pag.Limit),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		},
		Total: int64(pag.TotalRecord),
		Data:  results,
	}, nil
}

func (s *Server) CountAIMemoryEntityTags(ctx context.Context, req *ypb.CountAIMemoryEntityTagsRequest) (*ypb.CountAIMemoryEntityTagsResponse, error) {
	db := s.GetProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("database not initialized")
	}
	if req == nil {
		return nil, utils.Errorf("request is nil")
	}

	tagsCount, err := yakit.CountAIMemoryEntityTags(ctx, db, req.GetSessionID())
	if err != nil {
		return nil, err
	}

	return &ypb.CountAIMemoryEntityTagsResponse{
		TagsCount: tagsCount,
	}, nil
}

func queryAIMemoryBySemantic(db *gorm.DB, paging *ypb.Paging, filter *ypb.AIMemoryEntityFilter) (*ypb.QueryAIMemoryEntityResponse, error) {
	sessionID := strings.TrimSpace(filter.GetSessionID())
	if sessionID == "" {
		return nil, utils.Errorf("session_id is required for semantic query")
	}

	semanticQuery := strings.TrimSpace(filter.GetSemanticQuery())
	if semanticQuery == "" {
		return nil, utils.Errorf("semantic_query is required")
	}

	// NOTE: Semantic search results are not stable enough to support strict pagination (page+offset).
	// We only use paging to simulate an increasing "limit" (page*limit) and always return top-N.
	topK := int(paging.GetPage() * paging.GetLimit())
	if filter.GetVectorTopK() > int64(topK) {
		topK = int(filter.GetVectorTopK())
	}
	if topK <= 0 {
		topK = 10
	}

	triage, err := aimem.NewAIMemoryForQuery(sessionID, aimem.WithDatabase(db))
	if err != nil {
		return nil, err
	}
	idResults, err := triage.SearchBySemanticsMemoryIDs(semanticQuery, topK)
	if err != nil {
		return nil, err
	}

	scoreByMemoryID := make(map[string]float64)
	orderedMemoryIDs := make([]string, 0, len(idResults))
	for _, sr := range idResults {
		if sr == nil || sr.Entity == nil {
			continue
		}
		memoryID := strings.TrimSpace(sr.Entity.Id)
		if memoryID == "" {
			continue
		}
		scoreByMemoryID[memoryID] = sr.Score
		orderedMemoryIDs = append(orderedMemoryIDs, memoryID)
	}

	if len(orderedMemoryIDs) == 0 {
		return &ypb.QueryAIMemoryEntityResponse{
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   int64(topK),
				OrderBy: paging.GetOrderBy(),
				Order:   paging.GetOrder(),
			},
			Total: 0,
			Data:  []*ypb.AIMemoryEntity{},
		}, nil
	}

	q := yakit.FilterAIMemoryEntity(db, filter).Where("memory_id IN (?)", orderedMemoryIDs)
	var entities []*schema.AIMemoryEntity
	if err := q.Find(&entities).Error; err != nil {
		return nil, err
	}
	entityByMemoryID := make(map[string]*schema.AIMemoryEntity, len(entities))
	for _, e := range entities {
		entityByMemoryID[e.MemoryID] = e
	}

	allResults := make([]*ypb.AIMemoryEntity, 0, len(orderedMemoryIDs))
	for _, memoryID := range orderedMemoryIDs {
		entity := entityByMemoryID[memoryID]
		if entity == nil {
			continue
		}
		allResults = append(allResults, entity.ToGRPC())
	}

	return &ypb.QueryAIMemoryEntityResponse{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   int64(topK),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		},
		Total: int64(len(allResults)),
		Data:  allResults,
	}, nil
}

func queryAIMemoryByScoreVector(db *gorm.DB, paging *ypb.Paging, filter *ypb.AIMemoryEntityFilter) (*ypb.QueryAIMemoryEntityResponse, error) {
	sessionID := strings.TrimSpace(filter.GetSessionID())
	if sessionID == "" {
		return nil, utils.Errorf("session_id is required for score-vector query")
	}

	queryVector := filter.GetCorePactQueryVector()
	if len(queryVector) != 7 {
		return nil, utils.Errorf("core_pact_query_vector must be 7 dimensions, got %d", len(queryVector))
	}

	topK := int(paging.GetPage() * paging.GetLimit())
	if filter.GetVectorTopK() > int64(topK) {
		topK = int(filter.GetVectorTopK())
	}
	if topK <= 0 {
		topK = 10
	}

	triage, err := aimem.NewAIMemoryForQuery(sessionID, aimem.WithDatabase(db))
	if err != nil {
		return nil, err
	}

	searchResults, err := triage.SearchByScoreVectorMemoryIDs(queryVector, topK)
	if err != nil {
		return nil, err
	}

	scoreByMemoryID := make(map[string]float64, len(searchResults))
	orderedMemoryIDs := make([]string, 0, len(searchResults))
	for _, sr := range searchResults {
		if sr == nil || sr.Entity == nil {
			continue
		}
		memoryID := strings.TrimSpace(sr.Entity.Id)
		if memoryID == "" {
			continue
		}
		scoreByMemoryID[memoryID] = sr.Score
		orderedMemoryIDs = append(orderedMemoryIDs, memoryID)
	}

	if len(orderedMemoryIDs) == 0 {
		return &ypb.QueryAIMemoryEntityResponse{
			Pagination: paging,
			Total:      0,
			Data:       []*ypb.AIMemoryEntity{},
		}, nil
	}

	q := yakit.FilterAIMemoryEntity(db, filter).Where("memory_id IN (?)", orderedMemoryIDs)
	var entities []*schema.AIMemoryEntity
	if err := q.Find(&entities).Error; err != nil {
		return nil, err
	}
	entityByMemoryID := make(map[string]*schema.AIMemoryEntity, len(entities))
	for _, e := range entities {
		entityByMemoryID[e.MemoryID] = e
	}

	allResults := make([]*ypb.AIMemoryEntity, 0, len(orderedMemoryIDs))
	for _, memoryID := range orderedMemoryIDs {
		entity := entityByMemoryID[memoryID]
		if entity == nil {
			continue
		}
		allResults = append(allResults, entity.ToGRPC())
	}

	page := int(paging.GetPage())
	limit := int(paging.GetLimit())
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	start := (page - 1) * limit
	if start >= len(allResults) {
		return &ypb.QueryAIMemoryEntityResponse{
			Pagination: paging,
			Total:      int64(len(allResults)),
			Data:       []*ypb.AIMemoryEntity{},
		}, nil
	}
	end := start + limit
	if end > len(allResults) {
		end = len(allResults)
	}

	return &ypb.QueryAIMemoryEntityResponse{
		Pagination: paging,
		Total:      int64(len(allResults)),
		Data:       allResults[start:end],
	}, nil
}

func syncAIMemoryVectors(ctx context.Context, db *gorm.DB, entity *schema.AIMemoryEntity, prev *schema.AIMemoryEntity) error {
	if entity == nil {
		return nil
	}

	hnswBackend, err := aimem.NewAIMemoryHNSWBackend(aimem.WithHNSWSessionID(entity.SessionID), aimem.WithHNSWDatabase(db))
	if err == nil {
		_ = hnswBackend.Update(toAIMemoryEntity(entity))
	} else {
		log.Warnf("AIMemory HNSW update skipped: %v", err)
	}

	if err := syncAIMemorySemanticIndex(ctx, db, entity, prev); err != nil {
		log.Warnf("AIMemory RAG index update skipped: %v", err)
	}
	return nil
}

func deleteAIMemoryVectors(ctx context.Context, db *gorm.DB, entity *schema.AIMemoryEntity) error {
	if entity == nil {
		return nil
	}

	hnswBackend, err := aimem.NewAIMemoryHNSWBackend(aimem.WithHNSWSessionID(entity.SessionID), aimem.WithHNSWDatabase(db))
	if err == nil {
		_ = hnswBackend.Delete(entity.MemoryID)
	} else {
		log.Warnf("AIMemory HNSW delete skipped: %v", err)
	}

	return deleteAIMemorySemanticDocs(ctx, db, entity)
}

func syncAIMemorySemanticIndex(ctx context.Context, db *gorm.DB, entity *schema.AIMemoryEntity, prev *schema.AIMemoryEntity) error {
	sessionID := entity.SessionID
	collectionName := aimem.Session2MemoryName(sessionID)

	embeddingAvailable := rag.CheckConfigEmbeddingAvailable(rag.WithDB(db))
	if !embeddingAvailable && !vectorstore.HasCollection(db, collectionName) {
		return nil
	}

	store, err := vectorstore.LoadCollection(db, collectionName, vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()))
	if err != nil {
		if !embeddingAvailable {
			return nil
		}
		// If embedding is available, allow create via rag system below.
		store = nil
	}

	// Delete old docs (best-effort)
	if prev != nil && len(prev.PotentialQuestions) > 0 && store != nil {
		ids := prev.DocumentQuestionHashIDs()
		if len(ids) > 0 {
			_ = store.Delete(ids...)
		}
	}

	if !embeddingAvailable {
		return nil
	}

	system, err := rag.GetRagSystem(collectionName, rag.WithDB(db))
	if err != nil {
		return err
	}

	// Add current docs
	for _, q := range entity.PotentialQuestions {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		docID := entity.QuestionHashID(q)
		if err := system.Add(docID, q,
			rag.WithDocumentMetadataKeyValue("memory_id", entity.MemoryID),
			rag.WithDocumentMetadataKeyValue("question", q),
			rag.WithDocumentMetadataKeyValue("session_id", entity.SessionID),
		); err != nil {
			log.Warnf("AIMemory RAG add doc failed: %v", err)
		}
	}
	return nil
}

func deleteAIMemorySemanticDocs(_ context.Context, db *gorm.DB, entity *schema.AIMemoryEntity) error {
	collectionName := aimem.Session2MemoryName(entity.SessionID)
	if !vectorstore.HasCollection(db, collectionName) {
		return nil
	}

	store, err := vectorstore.LoadCollection(db, collectionName, vectorstore.WithEmbeddingClient(rag.NewEmptyMockEmbedding()))
	if err != nil {
		return err
	}

	var ids = entity.DocumentQuestionHashIDs()
	if len(ids) == 0 {
		return nil
	}
	return store.Delete(ids...)
}

func toAIMemoryEntity(entity *schema.AIMemoryEntity) *aicommon.MemoryEntity {
	if entity == nil {
		return nil
	}

	return &aicommon.MemoryEntity{
		Id:                 entity.MemoryID,
		CreatedAt:          entity.CreatedAt,
		Content:            entity.Content,
		Tags:               []string(entity.Tags),
		PotentialQuestions: []string(entity.PotentialQuestions),
		C_Score:            entity.C_Score,
		O_Score:            entity.O_Score,
		R_Score:            entity.R_Score,
		E_Score:            entity.E_Score,
		P_Score:            entity.P_Score,
		A_Score:            entity.A_Score,
		T_Score:            entity.T_Score,
		CorePactVector:     []float32(entity.CorePactVector),
	}
}
