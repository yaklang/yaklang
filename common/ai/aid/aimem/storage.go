package aimem

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SaveMemoryEntities 保存记忆条目到数据库并索引到RAG系统
func (r *AIMemoryTriage) SaveMemoryEntities(entities ...*MemoryEntity) error {
	db := consts.GetGormProjectDatabase()
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
	db := consts.GetGormProjectDatabase()
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
