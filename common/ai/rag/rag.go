package rag

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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
	return vectorstore.DeleteRAGDataByName(db, name)
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
