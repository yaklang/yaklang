package rag

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// type DocumentOption = vectorstore.DocumentOption

// Vector store related functions and types
// var ImportRAGFromFile = vectorstore.ImportRAGFromFile
var DeleteCollection = vectorstore.DeleteCollection
var GetCollection = vectorstore.GetCollection

func BuildVectorIndexForKnowledgeBaseEntry(db *gorm.DB, knowledgeBaseId int64, id string, opts ...RAGSystemConfigOption) (*vectorstore.SQLiteVectorStoreHNSW, error) {
	colOpts := NewRAGSystemConfig(opts...).ConvertToVectorStoreOptions()
	return vectorstore.BuildVectorIndexForKnowledgeBaseEntry(db, knowledgeBaseId, id, colOpts...)
}

func BuildVectorIndexForKnowledgeBase(db *gorm.DB, id int64, opts ...RAGSystemConfigOption) (*vectorstore.SQLiteVectorStoreHNSW, error) {
	colOpts := NewRAGSystemConfig(opts...).ConvertToVectorStoreOptions()
	return vectorstore.BuildVectorIndexForKnowledgeBase(db, id, colOpts...)
}

// DeleteRAG 完整删除一个RAG系统，包括集合、知识库、实体仓库
func DeleteRAG(db *gorm.DB, name string) error {
	var collection schema.VectorStoreCollection
	var knowledgeBases []schema.KnowledgeBaseInfo
	var entityRepositories []schema.EntityRepository
	hasCollection := false

	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		collectionQuery := tx.Model(&schema.VectorStoreCollection{}).
			Select("id, name, uuid, rag_id").Where("name = ?", name).First(&collection)
		if collectionQuery.Error == nil {
			hasCollection = true
		} else if !gorm.IsRecordNotFoundError(collectionQuery.Error) {
			return utils.Wrap(collectionQuery.Error, "query vector collection for deletion")
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

		// Legacy/corrupted RAG records may have no shared RAG ID. Preserve the
		// historical name fallback so the delete endpoint can repair them.
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
		vectorstore.GraphWrapperManager.RemoveCollectionFromCache(db, &collection)
	}
	return nil
}

func ListRAGSystemNames(db *gorm.DB) []string {
	nameSet := make(map[string]struct{})

	// 获取所有向量库（collections）的名字
	collectionNames := vectorstore.ListCollections(db)
	for _, name := range collectionNames {
		nameSet[name] = struct{}{}
	}

	// 获取所有知识库的名字
	knowledgeBaseNames, err := yakit.GetKnowledgeBaseNameList(db)
	if err == nil {
		for _, name := range knowledgeBaseNames {
			nameSet[name] = struct{}{}
		}
	}

	// 获取所有实体库的名字
	var entityRepos []*schema.EntityRepository
	err = db.Model(&schema.EntityRepository{}).Select("entity_base_name").Find(&entityRepos).Error
	if err == nil {
		for _, repo := range entityRepos {
			if repo.EntityBaseName != "" {
				nameSet[repo.EntityBaseName] = struct{}{}
			}
		}
	}

	// 将 map 转换为 slice
	result := make([]string, 0, len(nameSet))
	for name := range nameSet {
		result = append(result, name)
	}

	return result
}

// DeleteAllRAG deletes all RAG systems, including collections, knowledge bases, and entity repositories
func DeleteAllRAG(db *gorm.DB) error {
	names := ListRAGSystemNames(db)
	var lastErr error
	for _, name := range names {
		if err := DeleteRAG(db, name); err != nil {
			log.Errorf("failed to delete RAG system: %v, error: %v", name, err)
			lastErr = err
		}
	}
	return lastErr
}
