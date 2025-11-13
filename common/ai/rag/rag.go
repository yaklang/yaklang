package rag

import (
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	// 获取集合信息
	collectionInfo, err := loadCollectionInfoByConfig(NewRAGSystemConfig(WithDB(db), WithName(name)))
	if err != nil {
		return utils.Errorf("failed to load collection info: %v", err)
	}
	err = DeleteCollection(db, collectionInfo.Name)
	if err != nil {
		return err
	}

	// 生成配置，用于加载知识库和实体仓库信息
	ragConfig := NewRAGSystemConfig(WithDB(db), WithName(collectionInfo.Name), WithRAGID(collectionInfo.RAGID))

	// 删除知识库
	knowledgeBaseInfo, err := loadKnowledgeBaseInfoByConfig(ragConfig)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorf("failed to load knowledge base info: %v", err)
		}
	} else {
		err = knowledgebase.DeleteKnowledgeBase(db, knowledgeBaseInfo.KnowledgeBaseName)
		if err != nil {
			log.Errorf("failed to delete knowledge base: %v, error: %v", knowledgeBaseInfo.KnowledgeBaseName, err)
		}
	}

	// 删除实体仓库
	entityRepositoryInfo, err := loadEntityRepositoryInfoByConfig(ragConfig)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorf("failed to load entity repository info: %v", err)
		}
	} else {
		err = entityrepos.DeleteEntityRepository(db, entityRepositoryInfo.EntityBaseName)
		if err != nil {
			log.Errorf("failed to delete entity repository: %v, error: %v", entityRepositoryInfo.EntityBaseName, err)
		}
	}
	return nil
}
