package aisessioncleanup

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 中期 timeline 记忆在同一 persistent session id 下使用两套独立命名：
//
//  1. RAG 侧 — 向量/实体/知识库表中的 name 类字段
//     格式：ai-memory-timeline-midterm:{persistentSessionId}[@fork...]
//     表：rag_vector_collection_v1、rag_entity_repository_v1、rag_knowledge_base_v1
//
//  2. Memory 侧 — ai_memory_* 表中的 session_id 字段
//     格式：timeline-midterm:{persistentSessionId}[@fork...]
//     表：ai_memory_entities_v1、ai_memory_collections_v1
const (
	ragMidtermTableNamePrefix    = "ai-memory-timeline-midterm:"
	memoryMidtermSessionIDPrefix = "timeline-midterm:"
)

// SessionCleanupResult 汇总单次 session 清理删除的行数（纯 SQL）。
type SessionCleanupResult struct {
	DeletedMemoryEntities      int64
	DeletedMemoryCollections   int64
	DeletedRAGCollections      int64
	DeletedRAGDocuments        int64
	DeletedEntityRepositories  int64
	DeletedEntityRelationships int64
	DeletedERModelEntities     int64
	DeletedKnowledgeBases      int64
	DeletedKnowledgeEntries    int64
}

// ragMidtermTableName 返回 RAG 侧的基础表名（向量 collection / 实体仓库 / 知识库）。
func ragMidtermTableName(persistentSessionID string) string {
	return ragMidtermTableNamePrefix + persistentSessionID
}

// ragMidtermTableNameLike 匹配 RAG 侧基础名及 fork 变体。
func ragMidtermTableNameLike(persistentSessionID string) string {
	return ragMidtermTableName(persistentSessionID) + "%"
}

// memoryMidtermSessionID 返回 Memory 侧写入 ai_memory_* 的 session_id。
func memoryMidtermSessionID(persistentSessionID string) string {
	return memoryMidtermSessionIDPrefix + persistentSessionID
}

// memoryMidtermSessionIDLike 匹配 Memory 侧基础 session_id 及 fork 变体。
func memoryMidtermSessionIDLike(persistentSessionID string) string {
	return memoryMidtermSessionID(persistentSessionID) + "%"
}

// DeleteSessionArtifacts 删除指定 persistent session 关联的中期 Memory + RAG 数据。
// 纯 SQL，不加载 HNSW 图、不走 RAG vectorstore 运行时。
// 工作目录清理由调用方负责（yakit.CleanupAISpaceWorkDirsForSessions）。
func DeleteSessionArtifacts(db *gorm.DB, sessionID string) (*SessionCleanupResult, error) {
	result := &SessionCleanupResult{}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return result, utils.Errorf("sessionID is empty")
	}
	if db == nil {
		return result, utils.Errorf("database is nil")
	}

	ragLike := ragMidtermTableNameLike(sessionID)

	if err := deleteRAGCollectionsForSession(db, ragLike, result); err != nil {
		return result, err
	}
	if err := deleteEntityRepositoriesForSession(db, ragLike, result); err != nil {
		return result, err
	}
	if err := deleteKnowledgeBasesForSession(db, ragLike, result); err != nil {
		return result, err
	}
	if n, err := deleteMemoryEntitiesForSession(db, sessionID); err != nil {
		return result, err
	} else {
		result.DeletedMemoryEntities = n
	}
	if n, err := deleteMemoryCollectionsForSession(db, sessionID); err != nil {
		return result, err
	} else {
		result.DeletedMemoryCollections = n
	}

	log.Infof(
		"deleted session artifacts: session_id=%s memory_entities=%d memory_collections=%d rag_collections=%d rag_documents=%d entity_repositories=%d entity_relationships=%d er_model_entities=%d knowledge_bases=%d knowledge_entries=%d",
		sessionID,
		result.DeletedMemoryEntities,
		result.DeletedMemoryCollections,
		result.DeletedRAGCollections,
		result.DeletedRAGDocuments,
		result.DeletedEntityRepositories,
		result.DeletedEntityRelationships,
		result.DeletedERModelEntities,
		result.DeletedKnowledgeBases,
		result.DeletedKnowledgeEntries,
	)
	return result, nil
}

// DeleteAllSessionArtifacts 删除全部中期 Memory + 所有 ai-memory-timeline-midterm:* RAG 数据。
// 供 DeleteAISession 的 deleteAll 分支使用。纯 SQL。
func DeleteAllSessionArtifacts(db *gorm.DB) (*SessionCleanupResult, error) {
	result := &SessionCleanupResult{}
	if db == nil {
		return result, utils.Errorf("database is nil")
	}

	var collectionNames []string
	if err := db.Model(&schema.VectorStoreCollection{}).
		Where("name LIKE ?", ragMidtermTableNamePrefix+"%").
		Pluck("name", &collectionNames).Error; err != nil {
		if !isMissingTableErr(err) {
			return result, err
		}
	}
	if err := deleteRAGCollectionsByName(db, collectionNames, result); err != nil {
		return result, err
	}

	var entityRepoNames []string
	if err := db.Model(&schema.EntityRepository{}).
		Where("entity_base_name LIKE ?", ragMidtermTableNamePrefix+"%").
		Pluck("entity_base_name", &entityRepoNames).Error; err != nil {
		if !isMissingTableErr(err) {
			return result, err
		}
	}
	if err := deleteEntityRepositoriesByName(db, entityRepoNames, result); err != nil {
		return result, err
	}

	var knowledgeBaseNames []string
	if err := db.Model(&schema.KnowledgeBaseInfo{}).
		Where("knowledge_base_name LIKE ?", ragMidtermTableNamePrefix+"%").
		Pluck("knowledge_base_name", &knowledgeBaseNames).Error; err != nil {
		if !isMissingTableErr(err) {
			return result, err
		}
	}
	if err := deleteKnowledgeBasesByName(db, knowledgeBaseNames, result); err != nil {
		return result, err
	}

	if n, err := hardDeleteAll(db, &schema.AIMemoryEntity{}); err != nil {
		return result, err
	} else {
		result.DeletedMemoryEntities = n
	}
	if n, err := hardDeleteAll(db, &schema.AIMemoryCollection{}); err != nil {
		return result, err
	} else {
		result.DeletedMemoryCollections = n
	}

	log.Infof(
		"deleted all session artifacts: memory_entities=%d memory_collections=%d rag_collections=%d rag_documents=%d entity_repositories=%d entity_relationships=%d er_model_entities=%d knowledge_bases=%d knowledge_entries=%d",
		result.DeletedMemoryEntities,
		result.DeletedMemoryCollections,
		result.DeletedRAGCollections,
		result.DeletedRAGDocuments,
		result.DeletedEntityRepositories,
		result.DeletedEntityRelationships,
		result.DeletedERModelEntities,
		result.DeletedKnowledgeBases,
		result.DeletedKnowledgeEntries,
	)
	return result, nil
}

func deleteRAGCollectionsForSession(db *gorm.DB, likePattern string, result *SessionCleanupResult) error {
	names, err := pluckRAGArtifactNames(db, &schema.VectorStoreCollection{}, "name", likePattern)
	if err != nil {
		return err
	}
	return deleteRAGCollectionsByName(db, names, result)
}

func deleteEntityRepositoriesForSession(db *gorm.DB, likePattern string, result *SessionCleanupResult) error {
	names, err := pluckRAGArtifactNames(db, &schema.EntityRepository{}, "entity_base_name", likePattern)
	if err != nil {
		return err
	}
	return deleteEntityRepositoriesByName(db, names, result)
}

func deleteKnowledgeBasesForSession(db *gorm.DB, likePattern string, result *SessionCleanupResult) error {
	names, err := pluckRAGArtifactNames(db, &schema.KnowledgeBaseInfo{}, "knowledge_base_name", likePattern)
	if err != nil {
		return err
	}
	return deleteKnowledgeBasesByName(db, names, result)
}

func pluckRAGArtifactNames(db *gorm.DB, model interface{}, column, likePattern string) ([]string, error) {
	var names []string
	q := db.Model(model).Where(column+" LIKE ?", likePattern)
	if err := q.Pluck(column, &names).Error; err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, err
	}
	return names, nil
}

func deleteMemoryEntitiesForSession(db *gorm.DB, persistentSessionID string) (int64, error) {
	return hardDeleteWhere(db, &schema.AIMemoryEntity{},
		"session_id = ? OR session_id LIKE ?",
		persistentSessionID, memoryMidtermSessionIDLike(persistentSessionID),
	)
}

func deleteMemoryCollectionsForSession(db *gorm.DB, persistentSessionID string) (int64, error) {
	return hardDeleteWhere(db, &schema.AIMemoryCollection{},
		"session_id = ? OR session_id LIKE ?",
		persistentSessionID, memoryMidtermSessionIDLike(persistentSessionID),
	)
}

// deleteRAGCollectionsByName 按 collection 名删除向量文档与 collection 行。
func deleteRAGCollectionsByName(db *gorm.DB, names []string, result *SessionCleanupResult) error {
	if len(names) == 0 {
		return nil
	}
	var collectionIDs []uint
	if err := db.Model(&schema.VectorStoreCollection{}).
		Where("name IN (?)", names).
		Pluck("id", &collectionIDs).Error; err != nil {
		if !isMissingTableErr(err) {
			return err
		}
	}
	if len(collectionIDs) == 0 {
		return nil
	}

	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		docRes := tx.Model(&schema.VectorStoreDocument{}).
			Where("collection_id IN (?)", collectionIDs).
			Unscoped().
			Delete(&schema.VectorStoreDocument{})
		if docRes.Error != nil && !isMissingTableErr(docRes.Error) {
			return docRes.Error
		}
		result.DeletedRAGDocuments += docRes.RowsAffected

		colRes := tx.Model(&schema.VectorStoreCollection{}).
			Where("id IN (?)", collectionIDs).
			Unscoped().
			Delete(&schema.VectorStoreCollection{})
		if colRes.Error != nil && !isMissingTableErr(colRes.Error) {
			return colRes.Error
		}
		result.DeletedRAGCollections += colRes.RowsAffected
		return nil
	})
}

func deleteEntityRepositoriesByName(db *gorm.DB, names []string, result *SessionCleanupResult) error {
	if len(names) == 0 {
		return nil
	}
	var repoUUIDs []string
	if err := db.Model(&schema.EntityRepository{}).
		Where("entity_base_name IN (?)", names).
		Pluck("uuid", &repoUUIDs).Error; err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return err
	}
	if len(repoUUIDs) == 0 {
		return nil
	}

	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		relRes := tx.Model(&schema.ERModelRelationship{}).
			Where("repository_uuid IN (?)", repoUUIDs).
			Unscoped().
			Delete(&schema.ERModelRelationship{})
		if relRes.Error != nil && !isMissingTableErr(relRes.Error) {
			return relRes.Error
		}
		result.DeletedEntityRelationships += relRes.RowsAffected

		entRes := tx.Model(&schema.ERModelEntity{}).
			Where("repository_uuid IN (?)", repoUUIDs).
			Unscoped().
			Delete(&schema.ERModelEntity{})
		if entRes.Error != nil && !isMissingTableErr(entRes.Error) {
			return entRes.Error
		}
		result.DeletedERModelEntities += entRes.RowsAffected

		repoRes := tx.Model(&schema.EntityRepository{}).
			Where("uuid IN (?)", repoUUIDs).
			Unscoped().
			Delete(&schema.EntityRepository{})
		if repoRes.Error != nil && !isMissingTableErr(repoRes.Error) {
			return repoRes.Error
		}
		result.DeletedEntityRepositories += repoRes.RowsAffected
		return nil
	})
}

func deleteKnowledgeBasesByName(db *gorm.DB, names []string, result *SessionCleanupResult) error {
	if len(names) == 0 {
		return nil
	}
	var knowledgeBaseIDs []uint
	if err := db.Model(&schema.KnowledgeBaseInfo{}).
		Where("knowledge_base_name IN (?)", names).
		Pluck("id", &knowledgeBaseIDs).Error; err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return err
	}
	if len(knowledgeBaseIDs) == 0 {
		return nil
	}

	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		entryRes := tx.Model(&schema.KnowledgeBaseEntry{}).
			Where("knowledge_base_id IN (?)", knowledgeBaseIDs).
			Unscoped().
			Delete(&schema.KnowledgeBaseEntry{})
		if entryRes.Error != nil && !isMissingTableErr(entryRes.Error) {
			return entryRes.Error
		}
		result.DeletedKnowledgeEntries += entryRes.RowsAffected

		kbRes := tx.Model(&schema.KnowledgeBaseInfo{}).
			Where("id IN (?)", knowledgeBaseIDs).
			Unscoped().
			Delete(&schema.KnowledgeBaseInfo{})
		if kbRes.Error != nil && !isMissingTableErr(kbRes.Error) {
			return kbRes.Error
		}
		result.DeletedKnowledgeBases += kbRes.RowsAffected
		return nil
	})
}

func hardDeleteWhere(db *gorm.DB, model interface{}, query string, args ...interface{}) (int64, error) {
	res := db.Model(model).Where(query, args...).Unscoped().Delete(model)
	if res.Error != nil && !isMissingTableErr(res.Error) {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func hardDeleteByColumn(db *gorm.DB, model interface{}, query string, args ...interface{}) (int64, error) {
	return hardDeleteWhere(db, model, query, args...)
}

func hardDeleteAll(db *gorm.DB, model interface{}) (int64, error) {
	res := db.Model(model).Unscoped().Delete(model)
	if res.Error != nil && !isMissingTableErr(res.Error) {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}
