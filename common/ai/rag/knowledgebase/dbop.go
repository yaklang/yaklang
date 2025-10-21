package knowledgebase

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func DeleteKnowledgeBase(db *gorm.DB, name string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		tx = tx.Model(&schema.KnowledgeBaseInfo{})

		var info schema.KnowledgeBaseInfo
		err := tx.Where("knowledge_base_name = ?", name).First(&info).Error
		if err != nil {
			return utils.Errorf("get KnowledgeBaseInfo failed: %s", err)
		}

		err = tx.Where("id = ?", info.ID).Unscoped().Delete(&schema.KnowledgeBaseInfo{}).Error
		if err != nil {
			return utils.Errorf("delete KnowledgeBaseInfo failed: %s", err)
		}
		err = tx.Where("knowledge_base_id = ?", info.ID).Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error
		if err != nil {
			return utils.Errorf("delete KnowledgeBaseEntry failed: %s", err)
		}

		// 删除 RAG Collection
		var collection schema.VectorStoreCollection
		err = tx.Where("name = ?", name).First(&collection).Error
		if err != nil {
			return utils.Errorf("get VectorStoreCollection failed: %s", err)
		}
		err = tx.Where("id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreCollection{}).Error
		if err != nil {
			return utils.Errorf("delete VectorStoreCollection failed: %s", err)
		}

		// 删除所有文档
		err = tx.Where("collection_id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreDocument{}).Error
		if err != nil {
			return utils.Errorf("delete VectorStoreDocument failed: %s", err)
		}

		return nil
	})
}

func DeleteKnowledgeBaseByID(db *gorm.DB, id int64) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		tx = tx.Model(&schema.KnowledgeBaseInfo{})

		var info schema.KnowledgeBaseInfo
		err := tx.Where("id = ?", id).First(&info).Error
		if err != nil {
			return utils.Errorf("get KnowledgeBaseInfo failed: %s", err)
		}

		err = tx.Where("id = ?", info.ID).Unscoped().Delete(&schema.KnowledgeBaseInfo{}).Error
		if err != nil {
			return utils.Errorf("delete KnowledgeBaseInfo failed: %s", err)
		}
		err = tx.Where("knowledge_base_id = ?", info.ID).Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error
		if err != nil {
			return utils.Errorf("delete KnowledgeBaseEntry failed: %s", err)
		}

		// 删除 RAG Collection
		var collection schema.VectorStoreCollection
		err = tx.Where("name = ?", info.KnowledgeBaseName).First(&collection).Error
		if err != nil {
			return utils.Errorf("get VectorStoreCollection failed: %s", err)
		}
		err = tx.Where("id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreCollection{}).Error
		if err != nil {
			return utils.Errorf("delete VectorStoreCollection failed: %s", err)
		}

		// 删除所有文档
		err = tx.Where("collection_id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreDocument{}).Error
		if err != nil {
			return utils.Errorf("delete VectorStoreDocument failed: %s", err)
		}

		return nil
	})
}
