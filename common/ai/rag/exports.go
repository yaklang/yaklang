package rag

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/utils"
)

// CollectionIsExists 检查知识库是否存在，别名
var CollectionIsExists = vectorstore.HasCollection

var IsMockMode = false

var ListCollections = vectorstore.ListCollections

// AddDocument 添加文档
func AddDocument(db *gorm.DB, knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...RAGSystemConfigOption) error {
	defaultOpts := []RAGSystemConfigOption{
		WithDB(db),
	}
	opts = append(defaultOpts, opts...)
	ragSystem, err := GetRagSystem(knowledgeBaseName, opts...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.AddDocuments(&vectorstore.Document{
		ID:        documentName,
		Content:   document,
		Metadata:  metadata,
		Embedding: nil,
	})
}

// DeleteDocument 删除文档
func DeleteDocument(db *gorm.DB, knowledgeBaseName, documentName string, opts ...RAGSystemConfigOption) error {
	defaultOpts := []RAGSystemConfigOption{
		WithDB(db),
	}
	opts = append(defaultOpts, opts...)
	ragSystem, err := GetRagSystem(knowledgeBaseName, opts...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.DeleteDocuments(documentName)
}

// QueryDocuments 查询文档
func QueryDocuments(db *gorm.DB, knowledgeBaseName, query string, limit int, opts ...RAGSystemConfigOption) ([]vectorstore.SearchResult, error) {
	defaultOpts := []RAGSystemConfigOption{
		WithDB(db),
	}
	opts = append(defaultOpts, opts...)
	ragSystem, err := GetRagSystem(knowledgeBaseName, opts...)
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.QueryWithPage(query, 1, limit)
}

// Get 获取或创建 RAG 集合
// 如果集合不存在，会自动创建一个新的集合
// Example:
// ```
//
//	ragSystem = rag.GetCollection("my_collection")~
//	ragSystem = rag.GetCollection("my_collection", rag.ragForceNew(true))~
//
// ```
var Get = GetRagSystem

// func Get(name string, i ...RAGSystemConfigOption) (*RAGSystem, error) {
// 	GetRagSystem(name, i...)
// 	log.Infof("getting RAG collection '%s' with local embedding service", name)
// 	config := NewRAGSystemConfig(i...)
// 	if config.ForceNew {
// 		log.Infof("force creating new RAG collection for name: %s", name)
// 		return CreateCollection(config.DB, name, config.Description, i...)
// 	}

// 	// load existed first
// 	log.Infof("attempting to load existing RAG collection '%s'", name)
// 	ragSystem, err := LoadCollection(config.DB, name, i...)
// 	if errors.Is(err, gorm.ErrRecordNotFound) {
// 		log.Errorf("failed to load existing RAG collection '%s': %v, creating new one", name, err)
// 		return CreateCollection(config.DB, name, config.Description, i...)
// 	} else {
// 		return ragSystem, err
// 	}
// }
