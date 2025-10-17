package aimem

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SaveMemoryEntities 保存记忆条目到数据库并索引到RAG系统和HNSW
func (r *AIMemoryTriage) SaveMemoryEntities(entities ...*MemoryEntity) error {
	db := r.SafeGetDB()
	if db == nil {
		return utils.Errorf("database connection is nil")
	}

	for _, entity := range entities {
		if entity == nil {
			continue
		}

		// 保存到数据库
		dbEntity := &schema.AIMemoryEntity{
			MemoryID:           entity.Id,
			SessionID:          r.sessionID,
			Content:            entity.Content,
			Tags:               schema.StringArray(entity.Tags),
			PotentialQuestions: schema.StringArray(entity.PotentialQuestions),
			C_Score:            entity.C_Score,
			O_Score:            entity.O_Score,
			R_Score:            entity.R_Score,
			E_Score:            entity.E_Score,
			P_Score:            entity.P_Score,
			A_Score:            entity.A_Score,
			T_Score:            entity.T_Score,
			CorePactVector:     schema.FloatArray(entity.CorePactVector),
		}

		if err := db.Create(dbEntity).Error; err != nil {
			log.Errorf("save memory entity to database failed: %v", err)
			return utils.Errorf("save memory entity to database failed: %v", err)
		}

		log.Infof("saved memory entity to database: %s", entity.Id)

		// 添加到HNSW索引
		if r.hnswBackend != nil {
			if err := r.hnswBackend.Add(entity); err != nil {
				log.Errorf("add to HNSW index failed: %v", err)
				return utils.Errorf("add to HNSW index failed: %v", err)
			}
			log.Infof("added memory entity to HNSW index: %s", entity.Id)
		}

		// 索引 potential_questions 到 RAG 系统
		// 每个问题作为一个文档，关联到同一个 memory_id
		for _, question := range entity.PotentialQuestions {
			if strings.TrimSpace(question) == "" {
				continue
			}

			// 使用 question + memory_id 作为文档 ID，确保唯一性
			docID := fmt.Sprintf("%s-%s", entity.Id, utils.CalcMd5(question))

			err := r.rag.Add(docID, question,
				rag.WithDocumentMetadataKeyValue("memory_id", entity.Id),
				rag.WithDocumentMetadataKeyValue("question", question),
				rag.WithDocumentMetadataKeyValue("session_id", r.sessionID),
			)
			if err != nil {
				log.Errorf("index question to RAG failed: %v", err)
				return utils.Errorf("index question to RAG failed: %v", err)
			}
		}

		log.Infof("indexed %d questions for memory entity: %s", len(entity.PotentialQuestions), entity.Id)
	}

	return nil
}

// GetAllTags 获取当前会话的所有标签
func (r *AIMemoryTriage) GetAllTags() ([]string, error) {
	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var dbEntities []schema.AIMemoryEntity
	if err := db.Where("session_id = ?", r.sessionID).Find(&dbEntities).Error; err != nil {
		return nil, utils.Errorf("query memory entities failed: %v", err)
	}

	tagSet := make(map[string]bool)
	for _, dbEntity := range dbEntities {
		for _, tag := range dbEntity.Tags {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				tagSet[trimmed] = true
			}
		}
	}

	var tags []string
	for tag := range tagSet {
		tags = append(tags, tag)
	}

	return tags, nil
}

// GetDynamicContextWithTags 获取包含已有标签的动态上下文
func (r *AIMemoryTriage) GetDynamicContextWithTags() (string, error) {
	tags, err := r.GetAllTags()
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "当前没有已存储的记忆标签。", nil
	}

	var builder strings.Builder
	builder.WriteString("已存储的记忆领域标签（请优先使用这些标签）：\n")
	for i, tag := range tags {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, tag))
	}

	return builder.String(), nil
}

// DeleteMemoryEntity 删除记忆条目
func (r *AIMemoryTriage) DeleteMemoryEntity(memoryID string) error {
	db := r.GetDB()
	if db == nil {
		return utils.Errorf("database connection is nil")
	}

	// 从数据库删除
	if err := db.Where("memory_id = ? AND session_id = ?", memoryID, r.sessionID).
		Delete(&schema.AIMemoryEntity{}).Error; err != nil {
		return utils.Errorf("delete memory entity from database failed: %v", err)
	}

	// 从HNSW索引删除
	if r.hnswBackend != nil {
		if err := r.hnswBackend.Delete(memoryID); err != nil {
			log.Errorf("delete from HNSW index failed: %v", err)
		}
	}

	// 从RAG系统删除相关问题
	// 注意：这里需要删除所有以 memoryID 开头的文档
	// 由于RAG系统的限制，我们暂时不实现这个功能
	log.Warnf("RAG documents for memory %s not deleted (not implemented)", memoryID)

	return nil
}

// UpdateMemoryEntity 更新记忆条目
func (r *AIMemoryTriage) UpdateMemoryEntity(entity *MemoryEntity) error {
	db := r.GetDB()
	if db == nil {
		return utils.Errorf("database connection is nil")
	}

	// 先查询现有实体
	var existingEntity schema.AIMemoryEntity
	if err := db.Where("memory_id = ? AND session_id = ?", entity.Id, r.sessionID).
		First(&existingEntity).Error; err != nil {
		return utils.Errorf("find existing memory entity failed: %v", err)
	}

	// 更新字段
	existingEntity.Content = entity.Content
	existingEntity.Tags = schema.StringArray(entity.Tags)
	existingEntity.PotentialQuestions = schema.StringArray(entity.PotentialQuestions)
	existingEntity.C_Score = entity.C_Score
	existingEntity.O_Score = entity.O_Score
	existingEntity.R_Score = entity.R_Score
	existingEntity.E_Score = entity.E_Score
	existingEntity.P_Score = entity.P_Score
	existingEntity.A_Score = entity.A_Score
	existingEntity.T_Score = entity.T_Score
	existingEntity.CorePactVector = schema.FloatArray(entity.CorePactVector)

	// 保存更新
	if err := db.Save(&existingEntity).Error; err != nil {
		return utils.Errorf("update memory entity in database failed: %v", err)
	}

	// 更新HNSW索引
	if r.hnswBackend != nil {
		if err := r.hnswBackend.Update(entity); err != nil {
			log.Errorf("update HNSW index failed: %v", err)
			return utils.Errorf("update HNSW index failed: %v", err)
		}
	}

	log.Infof("updated memory entity: %s", entity.Id)

	return nil
}

// GetMemoryEntity 获取单个记忆条目
func (r *AIMemoryTriage) GetMemoryEntity(memoryID string) (*MemoryEntity, error) {
	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	var dbEntity schema.AIMemoryEntity
	if err := db.Where("memory_id = ? AND session_id = ?", memoryID, r.sessionID).
		First(&dbEntity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, utils.Errorf("memory entity not found: %s", memoryID)
		}
		return nil, utils.Errorf("query memory entity failed: %v", err)
	}

	entity := &MemoryEntity{
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

	return entity, nil
}

// ListAllMemories 列出所有记忆条目
func (r *AIMemoryTriage) ListAllMemories(limit int) ([]*MemoryEntity, error) {
	db := r.GetDB()
	if db == nil {
		return nil, utils.Errorf("database connection is nil")
	}

	query := db.Where("session_id = ?", r.sessionID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var dbEntities []schema.AIMemoryEntity
	if err := query.Find(&dbEntities).Error; err != nil {
		return nil, utils.Errorf("query memory entities failed: %v", err)
	}

	var results []*MemoryEntity
	for _, dbEntity := range dbEntities {
		entity := &MemoryEntity{
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
