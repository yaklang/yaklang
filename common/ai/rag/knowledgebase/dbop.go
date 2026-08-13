package knowledgebase

import (
	"errors"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func DeleteKnowledgeBase(db *gorm.DB, name string) error {
	return deleteKnowledgeBase(db, "knowledge_base_name = ?", name)
}

func DeleteKnowledgeBaseByID(db *gorm.DB, id int64) error {
	return deleteKnowledgeBase(db, "id = ?", id)
}

func deleteKnowledgeBase(db *gorm.DB, query string, arg any) error {
	var collection schema.VectorStoreCollection
	hasCollection := false
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		var info schema.KnowledgeBaseInfo
		if err := tx.Model(&schema.KnowledgeBaseInfo{}).Where(query, arg).First(&info).Error; err != nil {
			return utils.Errorf("get KnowledgeBaseInfo failed: %s", err)
		}

		err := tx.Model(&schema.VectorStoreCollection{}).
			Select("id, uuid").Where("name = ?", info.KnowledgeBaseName).First(&collection).Error
		if err == nil {
			hasCollection = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Errorf("get VectorStoreCollection failed: %s", err)
		}

		// Delete high-cardinality child rows before their parents. Both foreign-key
		// lookups are indexed by schema migration and the entire operation rolls
		// back if any table fails.
		if err := tx.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", info.ID).
			Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error; err != nil {
			return utils.Errorf("delete KnowledgeBaseEntry failed: %s", err)
		}
		if hasCollection {
			if err := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).
				Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
				return utils.Errorf("delete VectorStoreDocument failed: %s", err)
			}
			if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).
				Unscoped().Delete(&schema.VectorStoreCollection{}).Error; err != nil {
				return utils.Errorf("delete VectorStoreCollection failed: %s", err)
			}
		}
		if err := tx.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", info.ID).
			Unscoped().Delete(&schema.KnowledgeBaseInfo{}).Error; err != nil {
			return utils.Errorf("delete KnowledgeBaseInfo failed: %s", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if hasCollection {
		vectorstore.GraphWrapperManager.RemoveCollectionFromCache(db, &collection)
	}
	return nil
}
