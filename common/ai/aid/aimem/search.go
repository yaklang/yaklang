package aimem

import (
	"github.com/yaklang/yaklang/common/ai/aid/aimem/memory_type"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SearchBySemantics 通过语义搜索记忆
func (r *AIMemoryTriage) SearchBySemantics(query string, limit int) ([]*memory_type.SearchResult, error) {
	// 检查 embedding 服务是否可用
	if !r.embeddingAvailable {
		log.Debugf("embedding service not available for session %s, returning empty semantic search results", r.sessionID)
		return []*memory_type.SearchResult{}, nil
	}

	// 检查 RAG 系统是否已初始化
	if r.rag == nil {
		log.Debugf("RAG system not initialized for session %s, returning empty semantic search results", r.sessionID)
		return []*memory_type.SearchResult{}, nil
	}

	// 使用 RAG 搜索相关问题
	ragResults, err := r.rag.QueryWithPage(query, 1, limit)
	if err != nil {
		log.Warnf("RAG search failed for session %s: %v, returning empty results", r.sessionID, err)
		return []*memory_type.SearchResult{}, nil
	}

	db := r.GetDB()
	if db == nil {
		log.Debugf("database connection is nil for session %s, returning empty semantic search results", r.sessionID)
		return []*memory_type.SearchResult{}, nil
	}

	var results []*memory_type.SearchResult
	processedMemoryIDs := make(map[string]bool)

	for _, ragResult := range ragResults {
		memoryID, ok := ragResult.Document.Metadata["memory_id"].(string)
		if !ok || memoryID == "" {
			continue
		}

		// 避免重复
		if processedMemoryIDs[memoryID] {
			continue
		}
		processedMemoryIDs[memoryID] = true

		// 从数据库获取完整记忆条目
		var dbEntity schema.AIMemoryEntity
		if err := db.Where("memory_id = ? AND session_id = ?", memoryID, r.sessionID).First(&dbEntity).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				log.Warnf("memory entity not found in database: %s", memoryID)
				continue
			}
			log.Errorf("query memory entity failed: %v", err)
			continue
		}

		entity := &memory_type.MemoryEntity{
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
		}

		results = append(results, &memory_type.SearchResult{
			Entity: entity,
			Score:  ragResult.Score,
		})
	}

	return results, nil
}

// SearchByScores 按照C.O.R.E. P.A.C.T.评分搜索
func (r *AIMemoryTriage) SearchByScores(filter *memory_type.ScoreFilter, limit int) ([]*memory_type.MemoryEntity, error) {
	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	query := db.Where("session_id = ?", r.sessionID)

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

	var results []*memory_type.MemoryEntity
	for _, dbEntity := range dbEntities {
		entity := &memory_type.MemoryEntity{
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
		}
		results = append(results, entity)
	}

	return results, nil
}

// SearchByScoreVector 通过分数向量搜索相似的记忆（基于HNSW）
func (r *AIMemoryTriage) SearchByScoreVector(targetScores *memory_type.MemoryEntity, limit int) ([]*memory_type.SearchResult, error) {
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
	var results []*memory_type.SearchResult
	for _, sr := range searchResults {
		results = append(results, &memory_type.SearchResult{
			Entity: sr.Entity,
			Score:  sr.Score,
		})
	}

	return results, nil
}

// SearchByTags 按照标签搜索
func (r *AIMemoryTriage) SearchByTags(tags []string, matchAll bool, limit int) ([]*memory_type.MemoryEntity, error) {
	if len(tags) == 0 {
		return nil, utils.Errorf("at least one tag is required")
	}

	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var dbEntities []schema.AIMemoryEntity
	if err := db.Where("session_id = ?", r.sessionID).Find(&dbEntities).Error; err != nil {
		return nil, utils.Errorf("query memory entities failed: %v", err)
	}

	var results []*memory_type.MemoryEntity
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

		entity := &memory_type.MemoryEntity{
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
		}
		results = append(results, entity)

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results, nil
}
