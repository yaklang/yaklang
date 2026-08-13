package vectorstore

import (
	"errors"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ErrCorruptedRAGDeleted reports that loading failed because the persisted
// graph was corrupted, its one rebuild attempt failed, and the RAG was removed.
var ErrCorruptedRAGDeleted = errors.New("corrupted RAG was deleted")

// DeleteRAGDataByName removes the complete RAG identified by name. All child
// rows and the collection are deleted in one transaction using exact indexed
// predicates; the graph binary is never loaded or traversed.
func DeleteRAGDataByName(db *gorm.DB, name string) error {
	return deleteRAGData(db, name, nil)
}

// DeleteCorruptedRAG removes exactly the supplied collection and the KB/entity
// data linked through its RAG ID. The collection ID, not its potentially reused
// name, is the authoritative vector deletion key.
func DeleteCorruptedRAG(db *gorm.DB, collection *schema.VectorStoreCollection) error {
	if collection == nil || collection.ID == 0 {
		return utils.Error("delete corrupted RAG: invalid collection")
	}
	return deleteRAGData(db, collection.Name, collection)
}

func deleteRAGData(db *gorm.DB, name string, exactCollection *schema.VectorStoreCollection) error {
	var collection schema.VectorStoreCollection
	var knowledgeBases []schema.KnowledgeBaseInfo
	var entityRepositories []schema.EntityRepository
	hasCollection := false

	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		collectionQuery := tx.Model(&schema.VectorStoreCollection{}).
			Select("id, name, uuid, rag_id")
		if exactCollection != nil {
			collectionQuery = collectionQuery.Where("id = ?", exactCollection.ID)
		} else {
			collectionQuery = collectionQuery.Where("name = ?", name)
		}
		collectionQuery = collectionQuery.First(&collection)
		if collectionQuery.Error == nil {
			hasCollection = true
			name = collection.Name
		} else if !gorm.IsRecordNotFoundError(collectionQuery.Error) {
			return utils.Wrap(collectionQuery.Error, "query vector collection for deletion")
		} else if exactCollection != nil {
			// An exact deletion must never fall through to another collection that
			// happens to have the same name.
			return nil
		}

		ragID := collection.RAGID
		if ragID != "" {
			if err := tx.Model(&schema.KnowledgeBaseInfo{}).
				Select("id, knowledge_base_name, rag_id").Where("rag_id = ?", ragID).Find(&knowledgeBases).Error; err != nil {
				return utils.Wrap(err, "query knowledge base by rag id for deletion")
			}
			if err := tx.Model(&schema.EntityRepository{}).
				Select("id, uuid, entity_base_name, rag_id").Where("rag_id = ?", ragID).Find(&entityRepositories).Error; err != nil {
				return utils.Wrap(err, "query entity repository by rag id for deletion")
			}
		}

		// Legacy records can lack a shared RAG ID. Equality by the exact RAG
		// name preserves the repair behavior without introducing a LIKE scan.
		if len(knowledgeBases) == 0 {
			if err := tx.Model(&schema.KnowledgeBaseInfo{}).
				Select("id, knowledge_base_name, rag_id").Where("knowledge_base_name = ?", name).Find(&knowledgeBases).Error; err != nil {
				return utils.Wrap(err, "query knowledge base by name for deletion")
			}
			if ragID == "" && len(knowledgeBases) > 0 {
				ragID = knowledgeBases[0].RAGID
			}
		}
		if len(entityRepositories) == 0 && ragID != "" {
			if err := tx.Model(&schema.EntityRepository{}).
				Select("id, uuid, entity_base_name, rag_id").Where("rag_id = ?", ragID).Find(&entityRepositories).Error; err != nil {
				return utils.Wrap(err, "query entity repository by rag id for deletion")
			}
		}
		if len(entityRepositories) == 0 {
			if err := tx.Model(&schema.EntityRepository{}).
				Select("id, uuid, entity_base_name, rag_id").Where("entity_base_name = ?", name).Find(&entityRepositories).Error; err != nil {
				return utils.Wrap(err, "query entity repository by name for deletion")
			}
			if ragID == "" && len(entityRepositories) > 0 {
				ragID = entityRepositories[0].RAGID
			}
		}
		if len(knowledgeBases) == 0 && ragID != "" {
			if err := tx.Model(&schema.KnowledgeBaseInfo{}).
				Select("id, knowledge_base_name, rag_id").Where("rag_id = ?", ragID).Find(&knowledgeBases).Error; err != nil {
				return utils.Wrap(err, "query knowledge base by rag id for deletion")
			}
		}

		if len(knowledgeBases) > 0 {
			knowledgeBaseIDs := make([]uint, 0, len(knowledgeBases))
			for _, knowledgeBase := range knowledgeBases {
				knowledgeBaseIDs = append(knowledgeBaseIDs, knowledgeBase.ID)
			}
			if err := tx.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id IN (?)", knowledgeBaseIDs).
				Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error; err != nil {
				return utils.Wrap(err, "delete knowledge base entries")
			}
			if err := tx.Model(&schema.KnowledgeBaseInfo{}).Where("id IN (?)", knowledgeBaseIDs).
				Unscoped().Delete(&schema.KnowledgeBaseInfo{}).Error; err != nil {
				return utils.Wrap(err, "delete knowledge base")
			}
		}

		if len(entityRepositories) > 0 {
			repositoryIDs := make([]uint, 0, len(entityRepositories))
			repositoryUUIDs := make([]string, 0, len(entityRepositories))
			for _, entityRepository := range entityRepositories {
				repositoryIDs = append(repositoryIDs, entityRepository.ID)
				repositoryUUIDs = append(repositoryUUIDs, entityRepository.Uuid)
			}
			if err := tx.Model(&schema.ERModelRelationship{}).Where("repository_uuid IN (?)", repositoryUUIDs).
				Unscoped().Delete(&schema.ERModelRelationship{}).Error; err != nil {
				return utils.Wrap(err, "delete entity relationships")
			}
			if err := tx.Model(&schema.ERModelEntity{}).Where("repository_uuid IN (?)", repositoryUUIDs).
				Unscoped().Delete(&schema.ERModelEntity{}).Error; err != nil {
				return utils.Wrap(err, "delete entities")
			}
			if err := tx.Model(&schema.EntityRepository{}).Where("id IN (?)", repositoryIDs).
				Unscoped().Delete(&schema.EntityRepository{}).Error; err != nil {
				return utils.Wrap(err, "delete entity repository")
			}
		}

		if hasCollection {
			if err := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).
				Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
				return utils.Wrap(err, "delete vector documents")
			}
			if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).
				Unscoped().Delete(&schema.VectorStoreCollection{}).Error; err != nil {
				return utils.Wrap(err, "delete vector collection")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if hasCollection {
		GraphWrapperManager.RemoveCollectionFromCache(db, &collection)
	}
	return nil
}
