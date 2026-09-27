package aisessioncleanup

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

// SessionCleanupResult counts deletion of ordinary AI memory and its vectors.
type SessionCleanupResult struct {
	DeletedMemoryEntities    int64
	DeletedMemoryCollections int64
	DeletedRAGCollections    int64
	DeletedRAGDocuments      int64
}

// DeleteSessionArtifacts deletes only the exact session's memory and vectors.
// Session IDs are opaque, including SQL wildcard characters. No prefix matching.
func DeleteSessionArtifacts(db *gorm.DB, sessionID string) (*SessionCleanupResult, error) {
	result := &SessionCleanupResult{}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return result, utils.Error("sessionID is empty")
	}
	if db == nil {
		return result, utils.Error("database is nil")
	}
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		if err := deleteRAGCollectionsByName(tx, []string{"ai-memory-" + sessionID}, result); err != nil {
			return err
		}
		var err error
		result.DeletedMemoryEntities, err = hardDeleteWhere(tx, &schema.AIMemoryEntity{}, "session_id = ?", sessionID)
		if err != nil {
			return err
		}
		result.DeletedMemoryCollections, err = hardDeleteWhere(tx, &schema.AIMemoryCollection{}, "session_id = ?", sessionID)
		return err
	})
	if err != nil {
		return &SessionCleanupResult{}, err
	}
	return result, nil
}

func deleteAllMemoryRAGCollections(db *gorm.DB, result *SessionCleanupResult) error {
	var names []string
	if err := db.Model(&schema.VectorStoreCollection{}).Where("name LIKE ?", "ai-memory-%").Pluck("name", &names).Error; err != nil {
		if isMissingTableErr(err) {
			return nil
		}
		return err
	}
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		return deleteRAGCollectionsByName(tx, names, result)
	})
}

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

	docRes := db.Model(&schema.VectorStoreDocument{}).
		Where("collection_id IN (?)", collectionIDs).
		Unscoped().
		Delete(&schema.VectorStoreDocument{})
	if docRes.Error != nil && !isMissingTableErr(docRes.Error) {
		return docRes.Error
	}
	result.DeletedRAGDocuments += docRes.RowsAffected

	colRes := db.Model(&schema.VectorStoreCollection{}).
		Where("id IN (?)", collectionIDs).
		Unscoped().
		Delete(&schema.VectorStoreCollection{})
	if colRes.Error != nil && !isMissingTableErr(colRes.Error) {
		return colRes.Error
	}
	result.DeletedRAGCollections += colRes.RowsAffected
	return nil
}

func hardDeleteWhere(db *gorm.DB, model interface{}, query string, args ...interface{}) (int64, error) {
	res := db.Model(model).Where(query, args...).Unscoped().Delete(model)
	if res.Error != nil && !isMissingTableErr(res.Error) {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func dropAllMemoryTables(db *gorm.DB, result *SessionCleanupResult) error {
	memoryTables := []struct {
		model   interface{}
		counter *int64
	}{
		{&schema.AIMemoryEntity{}, &result.DeletedMemoryEntities},
		{&schema.AIMemoryCollection{}, &result.DeletedMemoryCollections},
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

func DeleteAllAIMemoryArtifacts(db *gorm.DB) (*SessionCleanupResult, error) {
	result := &SessionCleanupResult{}
	if db == nil {
		return result, utils.Errorf("database is nil")
	}

	// 1. 删除所有 ai-memory-% RAG 集合（向量文档 + collection 行）
	if err := deleteAllMemoryRAGCollections(db, result); err != nil {
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
