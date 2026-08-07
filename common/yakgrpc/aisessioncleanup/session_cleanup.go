package aisessioncleanup

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 中期 timeline 记忆在同一 persistent session id 下使用两套独立命名：
//
//  1. RAG 侧 — 向量/实体/知识库表中的 name 类字段
//     格式：ai-memory-timeline-midterm:{persistentSessionId}[@fork:{taskIndex}]
//     表：rag_vector_collection_v1、rag_entity_repository_v1、rag_knowledge_base_v1
//
//  2. Memory 侧 — 记忆表中的 session_id 字段
//     格式：timeline-midterm:{persistentSessionId}[@fork:{taskIndex}]
//     表：ai_midterm_archive_entities_v1、ai_midterm_archive_collections_v1
//     （历史数据可能仍存于 ai_memory_entities_v1、ai_memory_collections_v1）
//
// 删除策略：RAG 侧三张共享表的 name 类字段均有索引（unique_index / index），
// LIKE 'prefix%' 走索引 range scan 而非全表扫描，且 "name 以该前缀开头" 本身就是
// RAG 数据的归属判定（能顺带清掉从未写入记忆的孤儿 collection），因此 RAG 侧
// 一律按前缀 LIKE 过滤删除；Memory 侧整表可 drop，无需过滤。
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

// ragMidtermTableNameLike 匹配 RAG 侧基础名及 fork 变体（前缀 LIKE，走索引 range scan）。
func ragMidtermTableNameLike(persistentSessionID string) string {
	return ragMidtermTableName(persistentSessionID) + "%"
}

// memoryMidtermSessionID 返回 Memory 侧写入记忆表的 session_id。
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
//
// 中期记忆在 Memory 侧已物理分表（ai_midterm_archive_*），deleteAll 语义下整表
// 都是待删数据，因此统计行数后直接 drop + recreate，远快于 DELETE FROM。
// RAG 侧的向量/实体/知识库表与其它用途（如知识库）共享，按
// ai-memory-timeline-midterm:* 前缀 LIKE 过滤删除（name 字段有索引，走 range scan）。
func DeleteAllSessionArtifacts(db *gorm.DB) (*SessionCleanupResult, error) {
	result := &SessionCleanupResult{}
	if db == nil {
		return result, utils.Errorf("database is nil")
	}

	ragLike := ragMidtermTableNamePrefix + "%"
	if err := deleteRAGCollectionsForSession(db, ragLike, result); err != nil {
		return result, err
	}
	if err := deleteEntityRepositoriesForSession(db, ragLike, result); err != nil {
		return result, err
	}
	if err := deleteKnowledgeBasesForSession(db, ragLike, result); err != nil {
		return result, err
	}

	if err := dropAllMemoryTables(db, result); err != nil {
		return result, err
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

// pluckRAGArtifactNames 按前缀 LIKE 从 RAG 共享表中取出目标 name 列表。
// name 类字段均有索引，LIKE 'prefix%' 走 range scan，且能覆盖从未写入记忆的孤儿 collection。
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
	n1, err := hardDeleteWhere(db, &schema.AIMemoryEntity{},
		"session_id = ? OR session_id LIKE ?",
		persistentSessionID, memoryMidtermSessionIDLike(persistentSessionID),
	)
	if err != nil {
		return 0, err
	}
	// Also delete from the independent midterm archive table
	n2, err := hardDeleteWhere(db, &schema.AIMidtermArchiveEntity{},
		"session_id = ? OR session_id LIKE ?",
		persistentSessionID, memoryMidtermSessionIDLike(persistentSessionID),
	)
	if err != nil {
		return n1, err
	}
	return n1 + n2, nil
}

func deleteMemoryCollectionsForSession(db *gorm.DB, persistentSessionID string) (int64, error) {
	n1, err := hardDeleteWhere(db, &schema.AIMemoryCollection{},
		"session_id = ? OR session_id LIKE ?",
		persistentSessionID, memoryMidtermSessionIDLike(persistentSessionID),
	)
	if err != nil {
		return 0, err
	}
	// Also delete from the independent midterm archive table
	n2, err := hardDeleteWhere(db, &schema.AIMidtermArchiveCollection{},
		"session_id = ? OR session_id LIKE ?",
		persistentSessionID, memoryMidtermSessionIDLike(persistentSessionID),
	)
	if err != nil {
		return n1, err
	}
	return n1 + n2, nil
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

// dropAllMemoryTables 统计并清空全部记忆表（普通 + 中期归档）。
// 先 COUNT 记录删除行数到 result，再 drop + recreate 整表。
func dropAllMemoryTables(db *gorm.DB, result *SessionCleanupResult) error {
	memoryTables := []struct {
		model   interface{}
		counter *int64
	}{
		{&schema.AIMemoryEntity{}, &result.DeletedMemoryEntities},
		{&schema.AIMemoryCollection{}, &result.DeletedMemoryCollections},
		{&schema.AIMidtermArchiveEntity{}, &result.DeletedMemoryEntities},
		{&schema.AIMidtermArchiveCollection{}, &result.DeletedMemoryCollections},
	}
	for _, item := range memoryTables {
		var count int64
		if err := db.Model(item.model).Count(&count).Error; err != nil {
			if !isMissingTableErr(err) {
				return err
			}
		}
		*item.counter += count
		if err := schema.DropRecreateTable(db, item.model); err != nil {
			return err
		}
	}
	return nil
}

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}

// DeleteAllAIMemoryArtifacts 清空全部 AI 记忆数据（不含会话/runtime/event 等会话元数据）。
// 供 DeleteAIMemoryEntity 在 filter 全空时调用，快速清空所有记忆相关表。
//
// 与 DeleteAllSessionArtifacts 不同，本函数只处理 AI Memory 侧的表和 RAG 集合：
//   - 用 schema.DropRecreateTable 快速清空 4 张记忆表（entity + collection × 普通/中期归档）
//   - 用 SQL 批量删除所有 ai-memory-% RAG 集合（向量文档 + collection 行）
//
// 不加载 HNSW 图、不走 RAG vectorstore 运行时，性能远优于逐条删除。
func DeleteAllAIMemoryArtifacts(db *gorm.DB) (*SessionCleanupResult, error) {
	result := &SessionCleanupResult{}
	if db == nil {
		return result, utils.Errorf("database is nil")
	}

	// 1. 删除所有 ai-memory-% RAG 集合（向量文档 + collection 行）
	if err := deleteRAGCollectionsForSession(db, "ai-memory-%", result); err != nil {
		return result, err
	}

	// 2. Drop + Recreate 所有记忆表，比 DELETE FROM 快得多
	if err := dropAllMemoryTables(db, result); err != nil {
		return result, err
	}

	log.Infof(
		"deleted all AI memory artifacts: memory_entities=%d memory_collections=%d rag_collections=%d rag_documents=%d (memory tables dropped & recreated)",
		result.DeletedMemoryEntities,
		result.DeletedMemoryCollections,
		result.DeletedRAGCollections,
		result.DeletedRAGDocuments,
	)
	return result, nil
}
