package aimem

import (
	"errors"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SearchBySemanticsMemoryIDs validates hits using only memory IDs, without
// decoding memory contents or score vectors.
func (r *AIMemoryTriage) SearchBySemanticsMemoryIDs(query string, limit int) ([]*aicommon.SearchResult, error) {
	MaybeCleanup(r.db)
	return r.searchBySemantics(query, limit, false)
}

// SearchBySemantics 通过语义搜索记忆
func (r *AIMemoryTriage) SearchBySemantics(query string, limit int) ([]*aicommon.SearchResult, error) {
	MaybeCleanup(r.db)
	return r.searchBySemantics(query, limit, true)
}

const semanticSearchBatchSize = 100

var errSemanticOrphanChanged = errors.New("semantic orphan changed before repair")

func (r *AIMemoryTriage) searchBySemantics(query string, limit int, loadEntities bool) ([]*aicommon.SearchResult, error) {
	if !r.embeddingAvailable || r.rag == nil || r.GetDB() == nil {
		return []*aicommon.SearchResult{}, nil
	}

	var results []*aicommon.SearchResult
	// Repair at most one batch and refill once. A large historical backlog must
	// not turn an interactive search into an unbounded maintenance sweep.
	for pass := 0; pass < 2; pass++ {
		ragResults, err := r.rag.QueryWithPage(query, 1, limit)
		if err != nil {
			log.Warnf("RAG search failed for session %s: %v", r.sessionID, err)
			return results, nil
		}

		scores := make(map[string]float64)
		var orderedIDs []string
		documents := make(map[string]string)
		for _, rr := range ragResults {
			if rr == nil || rr.Document == nil {
				continue
			}
			id, ok := r.semanticMemoryID(rr.Document.Metadata)
			if !ok {
				continue
			}
			old, exists := scores[id]
			if !exists {
				orderedIDs = append(orderedIDs, id)
			}
			if !exists || rr.Score > old {
				scores[id] = rr.Score
			}
			documents[rr.Document.ID] = id
		}

		entities, err := r.loadSemanticEntities(r.GetDB(), orderedIDs, loadEntities)
		if err != nil {
			// A failed query is not evidence of deletion. Do not prune any hits.
			return nil, utils.Wrap(err, "validate semantic memory hits")
		}
		results = make([]*aicommon.SearchResult, 0, len(entities))
		for _, id := range orderedIDs {
			if entity, exists := entities[id]; exists {
				results = append(results, &aicommon.SearchResult{Entity: entity, Score: scores[id]})
			}
		}
		if pass > 0 {
			return results, nil
		}
		orphans := make(map[string]string)
		for docID, memoryID := range documents {
			if _, exists := entities[memoryID]; !exists {
				orphans[docID] = memoryID
				if len(orphans) == semanticSearchBatchSize {
					break
				}
			}
		}
		if len(orphans) == 0 {
			return results, nil
		}
		if err := r.deleteOrphanSemanticDocuments(orphans); err != nil {
			if !errors.Is(err, errSemanticOrphanChanged) {
				log.Warnf("repair of %d orphan semantic documents deferred for session %s: %v", len(orphans), r.sessionID, err)
			}
			// Maintenance failure must not discard healthy search results.
			return results, nil
		}
	}
	return results, nil
}

func (r *AIMemoryTriage) semanticMemoryID(metadata schema.MetadataMap) (string, bool) {
	id, ok := metadata["memory_id"].(string)
	if !ok || strings.TrimSpace(id) == "" {
		return "", false
	}
	if sid, ok := metadata["session_id"].(string); ok && sid != "" && sid != r.sessionID {
		return "", false
	}
	return id, true
}

func (r *AIMemoryTriage) loadSemanticEntities(db *gorm.DB, ids []string, full bool) (map[string]*aicommon.MemoryEntity, error) {
	entities := make(map[string]*aicommon.MemoryEntity, len(ids))
	for start := 0; start < len(ids); start += semanticSearchBatchSize {
		end := min(start+semanticSearchBatchSize, len(ids))
		query := db.Table(r.entityTableName()).
			Where("session_id = ? AND memory_id IN (?) AND deleted_at IS NULL", r.sessionID, ids[start:end])
		if !full {
			query = query.Select("memory_id")
		}
		var batch []schema.AIMemoryEntity
		if err := query.Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, entity := range batch {
			if full {
				entities[entity.MemoryID] = memoryEntityFromDBEntity(entity)
			} else {
				entities[entity.MemoryID] = &aicommon.MemoryEntity{Id: entity.MemoryID}
			}
		}
	}
	return entities, nil
}

func (r *AIMemoryTriage) deleteOrphanSemanticDocuments(orphans map[string]string) error {
	ids := make([]string, 0, len(orphans))
	memoryIDs := make([]string, 0, len(orphans))
	for docID, memoryID := range orphans {
		ids = append(ids, docID)
		memoryIDs = append(memoryIDs, memoryID)
	}
	store := r.rag.VectorStore
	collectionID := store.GetCollectionInfo().ID
	return store.DeleteWithTransactionCheck(func(tx *gorm.DB) error {
		// A memory may have been restored after the search. Recheck in the
		// deletion transaction so concurrent writes cannot lose live documents.
		live, err := r.loadSemanticEntities(tx, memoryIDs, false)
		if err != nil {
			return err
		}
		if len(live) > 0 {
			return errSemanticOrphanChanged
		}
		var documents []schema.VectorStoreDocument
		if err := tx.Select("document_id, metadata").
			Where("collection_id = ? AND document_id IN (?)", collectionID, ids).Find(&documents).Error; err != nil {
			return err
		}
		if len(documents) == 0 {
			return errSemanticOrphanChanged
		}
		for _, doc := range documents {
			id, ok := r.semanticMemoryID(doc.Metadata)
			if !ok || id != orphans[doc.DocumentID] {
				return errSemanticOrphanChanged
			}
		}
		return nil
	}, ids...)
}

// SearchByScores 按照C.O.R.E. P.A.C.T.评分搜索
func (r *AIMemoryTriage) SearchByScores(filter *aicommon.ScoreFilter, limit int) ([]*aicommon.MemoryEntity, error) {
	MaybeCleanup(r.db)
	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	query := db.Table(r.entityTableName()).Where("session_id = ?", r.sessionID)

	if filter != nil {
		if filter.C_Min > 0 || filter.C_Max > 0 {
			if filter.C_Max == 0 {
				filter.C_Max = 1.0
			}
			query = query.Where("c_score BETWEEN ? AND ?", filter.C_Min, filter.C_Max)
		}
		if filter.O_Min > 0 || filter.O_Max > 0 {
			if filter.O_Max == 0 {
				filter.O_Max = 1.0
			}
			query = query.Where("o_score BETWEEN ? AND ?", filter.O_Min, filter.O_Max)
		}
		if filter.R_Min > 0 || filter.R_Max > 0 {
			if filter.R_Max == 0 {
				filter.R_Max = 1.0
			}
			query = query.Where("r_score BETWEEN ? AND ?", filter.R_Min, filter.R_Max)
		}
		if filter.E_Min > 0 || filter.E_Max > 0 {
			if filter.E_Max == 0 {
				filter.E_Max = 1.0
			}
			query = query.Where("e_score BETWEEN ? AND ?", filter.E_Min, filter.E_Max)
		}
		if filter.P_Min > 0 || filter.P_Max > 0 {
			if filter.P_Max == 0 {
				filter.P_Max = 1.0
			}
			query = query.Where("p_score BETWEEN ? AND ?", filter.P_Min, filter.P_Max)
		}
		if filter.A_Min > 0 || filter.A_Max > 0 {
			if filter.A_Max == 0 {
				filter.A_Max = 1.0
			}
			query = query.Where("a_score BETWEEN ? AND ?", filter.A_Min, filter.A_Max)
		}
		if filter.T_Min > 0 || filter.T_Max > 0 {
			if filter.T_Max == 0 {
				filter.T_Max = 1.0
			}
			query = query.Where("t_score BETWEEN ? AND ?", filter.T_Min, filter.T_Max)
		}
	}

	var dbEntities []schema.AIMemoryEntity
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("created_at DESC").Find(&dbEntities).Error; err != nil {
		return nil, utils.Errorf("query memory entities failed: %v", err)
	}

	var results []*aicommon.MemoryEntity
	for _, dbEntity := range dbEntities {
		entity := &aicommon.MemoryEntity{
			Id:                 dbEntity.MemoryID,
			CreatedAt:          dbEntity.CreatedAt,
			Content:            dbEntity.Content,
			Tags:               []string(dbEntity.Tags),
			PotentialQuestions: []string(dbEntity.PotentialQuestions),
			C_Score:            dbEntity.C_Score,
			O_Score:            dbEntity.O_Score,
			R_Score:            dbEntity.R_Score,
			E_Score:            dbEntity.E_Score,
			P_Score:            dbEntity.P_Score,
			A_Score:            dbEntity.A_Score,
			T_Score:            dbEntity.T_Score,
			CorePactVector:     []float32(dbEntity.CorePactVector),
			ExpiresAt:          dbEntity.ExpiresAt,
		}
		results = append(results, entity)
	}

	return results, nil
}

// SearchByScoreVectorMemoryIDs 仅执行 HNSW 检索并返回命中的 memory_id（不加载数据库实体）
func (r *AIMemoryTriage) SearchByScoreVectorMemoryIDs(queryVector []float32, limit int) ([]*aicommon.SearchResult, error) {
	MaybeCleanup(r.db)
	if r.hnswBackend == nil {
		return nil, utils.Errorf("HNSW backend is not initialized")
	}
	searchResults, err := r.hnswBackend.searchKeysWithDistance(queryVector, limit)
	if err != nil {
		return nil, utils.Errorf("HNSW search failed: %v", err)
	}
	if len(searchResults) == 0 {
		return []*aicommon.SearchResult{}, nil
	}

	results := make([]*aicommon.SearchResult, 0, len(searchResults))
	for _, sr := range searchResults {
		memoryID := strings.TrimSpace(sr.Key)
		if memoryID == "" {
			continue
		}
		results = append(results, &aicommon.SearchResult{
			Entity: &aicommon.MemoryEntity{Id: memoryID},
			Score:  1 - sr.Distance,
		})
	}
	return results, nil
}

// SearchByScoreVector 通过分数向量搜索相似的记忆（基于HNSW）
func (r *AIMemoryTriage) SearchByScoreVector(targetScores *aicommon.MemoryEntity, limit int) ([]*aicommon.SearchResult, error) {
	MaybeCleanup(r.db)
	// 构建目标向量
	queryVector := []float32{
		float32(targetScores.C_Score),
		float32(targetScores.O_Score),
		float32(targetScores.R_Score),
		float32(targetScores.E_Score),
		float32(targetScores.P_Score),
		float32(targetScores.A_Score),
		float32(targetScores.T_Score),
	}

	// 使用HNSW后端搜索
	if r.hnswBackend == nil {
		return nil, utils.Errorf("HNSW backend is not initialized")
	}

	searchResults, err := r.hnswBackend.Search(queryVector, limit)
	if err != nil {
		return nil, utils.Errorf("HNSW search failed: %v", err)
	}

	// 转换结果格式
	var results []*aicommon.SearchResult
	for _, sr := range searchResults {
		results = append(results, &aicommon.SearchResult{
			Entity: sr.Entity,
			Score:  sr.Score,
		})
	}

	return results, nil
}

// SearchByTags 按照标签搜索
func (r *AIMemoryTriage) SearchByTags(tags []string, matchAll bool, limit int) ([]*aicommon.MemoryEntity, error) {
	MaybeCleanup(r.db)
	if len(tags) == 0 {
		return nil, utils.Errorf("at least one tag is required")
	}

	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var dbEntities []schema.AIMemoryEntity
	if err := db.Table(r.entityTableName()).Where("session_id = ?", r.sessionID).Find(&dbEntities).Error; err != nil {
		return nil, utils.Errorf("query memory entities failed: %v", err)
	}

	var results []*aicommon.MemoryEntity
	for _, dbEntity := range dbEntities {
		entityTags := []string(dbEntity.Tags)

		if matchAll {
			// 必须包含所有标签
			allMatch := true
			for _, tag := range tags {
				found := false
				for _, entityTag := range entityTags {
					if strings.EqualFold(strings.TrimSpace(tag), strings.TrimSpace(entityTag)) {
						found = true
						break
					}
				}
				if !found {
					allMatch = false
					break
				}
			}
			if !allMatch {
				continue
			}
		} else {
			// 至少包含一个标签
			hasMatch := false
			for _, tag := range tags {
				for _, entityTag := range entityTags {
					if strings.EqualFold(strings.TrimSpace(tag), strings.TrimSpace(entityTag)) {
						hasMatch = true
						break
					}
				}
				if hasMatch {
					break
				}
			}
			if !hasMatch {
				continue
			}
		}

		entity := &aicommon.MemoryEntity{
			Id:                 dbEntity.MemoryID,
			CreatedAt:          dbEntity.CreatedAt,
			Content:            dbEntity.Content,
			Tags:               entityTags,
			PotentialQuestions: []string(dbEntity.PotentialQuestions),
			C_Score:            dbEntity.C_Score,
			O_Score:            dbEntity.O_Score,
			R_Score:            dbEntity.R_Score,
			E_Score:            dbEntity.E_Score,
			P_Score:            dbEntity.P_Score,
			A_Score:            dbEntity.A_Score,
			T_Score:            dbEntity.T_Score,
			CorePactVector:     []float32(dbEntity.CorePactVector),
			ExpiresAt:          dbEntity.ExpiresAt,
		}
		results = append(results, entity)

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results, nil
}
