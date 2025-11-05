package rag

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// createTempTestDatabaseForRAGSystem 创建用于 RAG 系统测试的临时数据库
func createTempTestDatabaseForRAGSystem() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	// 迁移所有相关的表
	db.AutoMigrate(
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.EntityRepository{},
		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
	)
	return db, nil
}

// TestNewRAGSystem_Success 测试成功创建 RAG 系统
func TestNewRAGSystem_Success(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_rag_system_" + utils.RandStringBytes(8)

	// 预先创建实体仓库
	entityRepos, err := entityrepos.GetOrCreateEntityRepository(db, collectionName, "测试实体仓库",
		vectorstore.WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)
	assert.NoError(t, err)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithDescription("测试 RAG 系统"),
		WithEnableKnowledgeBase(true),
		WithEntityRepository(entityRepos),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)

	// 验证创建成功
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
	assert.NotNil(t, ragSystem.VectorStore)
	assert.NotNil(t, ragSystem.KnowledgeBase)
	assert.NotNil(t, ragSystem.EntityRepository)
	assert.NotEmpty(t, ragSystem.RAGID)

	// 验证集合信息
	collectionInfo := ragSystem.VectorStore.GetCollectionInfo()
	assert.Equal(t, collectionName, collectionInfo.Name)
	assert.NotEmpty(t, collectionInfo.RAGID)

	// 验证知识库
	kbInfo := ragSystem.KnowledgeBase.GetKnowledgeBaseInfo()
	assert.Equal(t, collectionName, kbInfo.KnowledgeBaseName)
	assert.NotEmpty(t, kbInfo.RAGID)

	// 验证实体仓库
	entityInfo, err := ragSystem.EntityRepository.GetInfo()
	assert.NoError(t, err)
	assert.Equal(t, collectionName, entityInfo.EntityBaseName)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestNewRAGSystem_WithVectorStore 测试使用现有 VectorStore 创建 RAG 系统
func TestNewRAGSystem_WithVectorStore(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_rag_with_store_" + utils.RandStringBytes(8)

	// 先创建 VectorStore
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	store, err := vectorstore.GetCollection(db, collectionName,
		vectorstore.WithEmbeddingClient(mockEmbedding),
		vectorstore.WithDescription("测试集合"),
	)
	assert.NoError(t, err)

	// 预先创建实体仓库
	entityRepos, err := entityrepos.GetOrCreateEntityRepository(db, collectionName, "测试实体仓库",
		vectorstore.WithEmbeddingClient(mockEmbedding),
	)
	assert.NoError(t, err)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithVectorStore(store),
		WithEnableKnowledgeBase(true),
		WithEntityRepository(entityRepos),
	)

	// 验证创建成功
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
	assert.Equal(t, store, ragSystem.VectorStore)
	assert.NotNil(t, ragSystem.KnowledgeBase)
	assert.NotNil(t, ragSystem.EntityRepository)
	assert.NotEmpty(t, ragSystem.RAGID)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestNewRAGSystem_OnlyVectorStore 测试只启用 VectorStore 的 RAG 系统
func TestNewRAGSystem_OnlyVectorStore(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_rag_vector_only_" + utils.RandStringBytes(8)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEnableKnowledgeBase(false),
		WithEnableEntityRepository(false),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)

	// 验证创建成功
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
	assert.NotNil(t, ragSystem.VectorStore)
	assert.Nil(t, ragSystem.KnowledgeBase)
	assert.Nil(t, ragSystem.EntityRepository)
	assert.Equal(t, collectionName, ragSystem.Name)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestNewRAGSystem_WithRAGID 测试使用现有 RAG ID 创建 RAG 系统
func TestNewRAGSystem_WithRAGID(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	ragID := utils.RandStringBytes(16)
	collectionName := "test_rag_with_ragid_" + utils.RandStringBytes(8)

	// 先创建带有特定 RAG ID 的集合
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	collection, err := vectorstore.GetCollection(db, collectionName,
		vectorstore.WithEmbeddingClient(mockEmbedding),
	)
	assert.NoError(t, err)

	// 设置 RAG ID
	collectionInfo := collection.GetCollectionInfo()
	err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionInfo.ID).Update("rag_id", ragID).Error
	assert.NoError(t, err)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithRAGID(ragID),
		WithEnableKnowledgeBase(true),
		WithEnableEntityRepository(true),
	)

	// 验证创建成功且使用了正确的 RAG ID
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
	assert.Equal(t, ragID, ragSystem.RAGID)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestHasRagSystem 测试 HasRagSystem 函数
func TestHasRagSystem(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_has_rag_" + utils.RandStringBytes(8)

	// 测试不存在的系统
	exists := HasRagSystem(db, collectionName)
	assert.False(t, exists)

	// 创建 RAG 系统
	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)
	assert.NoError(t, err)

	// 测试存在的系统
	exists = HasRagSystem(db, collectionName)
	assert.True(t, exists)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
	_ = ragSystem
}

// TestLoadRAGSystem 测试 LoadRAGSystem 函数
func TestLoadRAGSystem(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_load_rag_" + utils.RandStringBytes(8)

	// 创建 RAG 系统
	originalSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
		WithEnableKnowledgeBase(true),
		WithEnableEntityRepository(true),
	)
	assert.NoError(t, err)

	// 加载 RAG 系统
	loadedSystem, err := LoadRAGSystem(collectionName, WithDB(db))

	// 验证加载成功
	assert.NoError(t, err)
	assert.NotNil(t, loadedSystem)
	assert.Equal(t, originalSystem.RAGID, loadedSystem.RAGID)
	assert.Equal(t, collectionName, loadedSystem.Name)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestLoadRAGSystem_NotExists 测试加载不存在的 RAG 系统
func TestLoadRAGSystem_NotExists(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_load_nonexistent_" + utils.RandStringBytes(8)

	// 尝试加载不存在的系统
	loadedSystem, err := LoadRAGSystem(collectionName, WithDB(db))

	// 应该返回错误
	assert.Error(t, err)
	assert.Nil(t, loadedSystem)
	assert.Contains(t, err.Error(), "not existed")
}

// TestGetRagSystem_New 测试 GetRagSystem 创建新系统
func TestGetRagSystem_New(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_get_rag_new_" + utils.RandStringBytes(8)

	// 预先创建实体仓库
	entityRepos, err := entityrepos.GetOrCreateEntityRepository(db, collectionName, "测试实体仓库",
		vectorstore.WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)
	assert.NoError(t, err)

	// 获取不存在的系统（应该创建新的）
	ragSystem, err := GetRagSystem(collectionName,
		WithDB(db),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
		WithEnableKnowledgeBase(true),
		WithEntityRepository(entityRepos),
	)

	// 验证创建成功
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
	assert.Equal(t, collectionName, ragSystem.Name)
	assert.NotEmpty(t, ragSystem.RAGID)

	// 验证确实存在了
	exists := HasRagSystem(db, collectionName)
	assert.True(t, exists)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestGetRagSystem_Existing 测试 GetRagSystem 加载现有系统
func TestGetRagSystem_Existing(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_get_rag_existing_" + utils.RandStringBytes(8)

	// 先创建系统
	originalSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)
	assert.NoError(t, err)

	// 获取现有系统
	loadedSystem, err := GetRagSystem(collectionName, WithDB(db))

	// 验证加载成功
	assert.NoError(t, err)
	assert.NotNil(t, loadedSystem)
	assert.Equal(t, originalSystem.RAGID, loadedSystem.RAGID)
	assert.Equal(t, collectionName, loadedSystem.Name)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestRAGSystem_VectorSimilarity 测试向量相似度计算
func TestRAGSystem_VectorSimilarity(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_similarity_" + utils.RandStringBytes(8)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)
	assert.NoError(t, err)

	// 测试相似度计算
	text1 := "人工智能是计算机科学的一个分支"
	text2 := "AI是计算机科学的重要领域"
	similarity, err := ragSystem.VectorSimilarity(text1, text2)

	// 验证计算成功
	assert.NoError(t, err)
	assert.Greater(t, similarity, 0.0)
	assert.LessOrEqual(t, similarity, 1.0)

	// 测试相同文本的相似度
	sameSimilarity, err := ragSystem.VectorSimilarity(text1, text1)
	assert.NoError(t, err)
	assert.Greater(t, sameSimilarity, 0.9) // 相同文本相似度应该很高

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestRAGSystem_DocumentOperations 测试文档操作
func TestRAGSystem_DocumentOperations(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_doc_ops_" + utils.RandStringBytes(8)

	// 直接创建向量存储而不是完整的 RAG 系统
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	vectorStore, err := vectorstore.GetCollection(db, collectionName,
		vectorstore.WithEmbeddingClient(mockEmbedding),
	)
	assert.NoError(t, err)

	docID := "test_doc_1"
	content := "这是一个测试文档，包含了一些关于人工智能的内容。"

	// 测试添加文档
	err = vectorStore.AddWithOptions(docID, content)
	assert.NoError(t, err)

	// 测试检查文档存在
	exists := vectorStore.Has(docID)
	assert.True(t, exists)

	// 测试获取文档
	doc, found, err := vectorStore.Get(docID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.NotNil(t, doc)
	assert.Equal(t, docID, doc.ID)
	assert.Equal(t, content, doc.Content)

	// 测试查询
	results, err := vectorStore.QueryTopN("人工智能", 5)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, docID, results[0].Document.ID)

	// 测试分页查询
	pageResults, err := vectorStore.QueryWithPage("人工智能", 1, 10)
	assert.NoError(t, err)
	assert.Len(t, pageResults, 1)

	// 测试删除文档
	err = vectorStore.Delete(docID)
	assert.NoError(t, err)

	// 验证文档已被删除
	exists = vectorStore.Has(docID)
	assert.False(t, exists)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestRAGSystem_BulkOperations 测试批量操作
func TestRAGSystem_BulkOperations(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_bulk_ops_" + utils.RandStringBytes(8)

	// 直接创建向量存储
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	vectorStore, err := vectorstore.GetCollection(db, collectionName,
		vectorstore.WithEmbeddingClient(mockEmbedding),
	)
	assert.NoError(t, err)

	// 创建多个文档
	docs := []*vectorstore.Document{
		{ID: "doc1", Content: "第一个测试文档"},
		{ID: "doc2", Content: "第二个测试文档"},
		{ID: "doc3", Content: "第三个测试文档"},
	}

	// 测试批量添加
	err = vectorStore.Add(docs...)
	assert.NoError(t, err)

	// 验证文档数量
	count, err := vectorStore.Count()
	assert.NoError(t, err)
	assert.Equal(t, 3, count)

	// 验证所有文档都存在
	for _, doc := range docs {
		exists := vectorStore.Has(doc.ID)
		assert.True(t, exists, "Document %s should exist", doc.ID)
	}

	// 测试列出文档
	listedDocs, err := vectorStore.List()
	assert.NoError(t, err)
	assert.Len(t, listedDocs, 3)

	// 测试清空文档
	err = vectorStore.Clear()
	assert.NoError(t, err)

	// 验证文档已被清空
	count, err = vectorStore.Count()
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestRAGSystem_KnowledgeBaseOperations 测试知识库操作
func TestRAGSystem_KnowledgeBaseOperations(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_kb_ops_" + utils.RandStringBytes(8)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
		WithEnableKnowledgeBase(true),
	)
	assert.NoError(t, err)

	// 创建知识库条目
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  ragSystem.GetKnowledgeBaseID(),
		KnowledgeTitle:   "Go语言入门",
		KnowledgeType:    "Programming",
		KnowledgeDetails: "Go语言是由Google开发的开源编程语言。",
		Keywords:         schema.StringArray{"Go", "编程语言", "Google"},
		ImportanceScore:  8,
	}

	// 测试添加知识库条目
	err = ragSystem.AddKnowledgeEntry(entry)
	assert.NoError(t, err)

	// 验证知识库 ID
	kbID := ragSystem.GetKnowledgeBaseID()
	assert.Greater(t, kbID, int64(0))

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestRAGSystem_ArchivedOperations 测试归档操作
func TestRAGSystem_ArchivedOperations(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_archived_" + utils.RandStringBytes(8)

	ragSystem, err := NewRAGSystem(
		WithDB(db),
		WithName(collectionName),
		WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()),
	)
	assert.NoError(t, err)

	// 默认应该不是归档状态
	archived := ragSystem.GetArchived()
	assert.False(t, archived)

	// 设置为归档状态
	err = ragSystem.SetArchived(true)
	assert.NoError(t, err)

	// 验证归档状态
	archived = ragSystem.GetArchived()
	assert.True(t, archived)

	// 取消归档
	err = ragSystem.SetArchived(false)
	assert.NoError(t, err)

	// 验证非归档状态
	archived = ragSystem.GetArchived()
	assert.False(t, archived)

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestBuildDocument 测试文档构建函数
func TestBuildDocument(t *testing.T) {
	docID := "test_doc"
	content := "测试文档内容"

	doc := BuildDocument(docID, content,
		WithDocumentRawMetadata(map[string]any{
			"title":   "测试标题",
			"author":  "测试作者",
			"version": 1,
		}),
	)

	assert.NotNil(t, doc)
	assert.Equal(t, docID, doc.ID)
	assert.Equal(t, content, doc.Content)
	assert.NotNil(t, doc.Metadata)
	assert.Equal(t, "测试标题", doc.Metadata["title"])
	assert.Equal(t, "测试作者", doc.Metadata["author"])
	assert.Equal(t, 1, doc.Metadata["version"])
}

// TestRAGSystem_FuzzSearch 测试模糊搜索
func TestRAGSystem_FuzzSearch(t *testing.T) {
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_fuzz_search_" + utils.RandStringBytes(8)

	// 直接创建向量存储
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	vectorStore, err := vectorstore.GetCollection(db, collectionName,
		vectorstore.WithEmbeddingClient(mockEmbedding),
	)
	assert.NoError(t, err)

	// 添加测试文档
	testDocs := []struct {
		id      string
		content string
	}{
		{"doc1", "Go语言是一种高效的编程语言"},
		{"doc2", "Python是数据科学的重要工具"},
		{"doc3", "JavaScript用于前端开发"},
	}

	for _, doc := range testDocs {
		err = vectorStore.AddWithOptions(doc.id, doc.content)
		assert.NoError(t, err)
	}

	// 测试模糊搜索
	ctx := context.Background()
	resultsChan, err := vectorStore.FuzzSearch(ctx, "Go", 10)
	assert.NoError(t, err)

	// 收集结果
	var results []vectorstore.SearchResult
	for result := range resultsChan {
		results = append(results, result)
	}

	// 验证结果 - 应该至少找到一个包含"Go"的文档
	found := false
	for _, result := range results {
		if result.Document.ID == "doc1" && strings.Contains(result.Document.Content, "Go") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should find doc1 containing 'Go'")

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

func TestNewRAGSystem_WithImportFile(t *testing.T) {
	// 生成导出数据
	// db, err := createTempTestDatabaseForRAGSystem()
	// assert.NoError(t, err)
	db := consts.GetGormProfileDatabase()
	exportCollectionName := "test_export_import_" + utils.RandStringBytes(8)
	ragSystem, err := Get(exportCollectionName, WithDB(db))
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)

	ragSystem.VectorStore.AddWithOptions("test_doc", "test_content")
	// 执行导出
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	tempFile.Close()

	err = ExportRAG(exportCollectionName, tempFile.Name(), WithDB(db))
	assert.NoError(t, err)

	// 测试自动导入
	ragSystem, err = Get(exportCollectionName+"_new",
		WithImportFile(tempFile.Name()),
		WithDB(db),
	)
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)

	num, err := ragSystem.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 1, num)

	// 测试序列一致
	file, err := os.Open(tempFile.Name())
	if err != nil {
		t.Fatalf("open temp file failed: %v", err)
	}
	defer file.Close()

	ragSystem, _ = Get(exportCollectionName+"_new1",
		WithDB(db),
	)

	headerIndo, err := LoadRAGFileHeader(file)
	assert.NoError(t, err)
	assert.NotNil(t, headerIndo)
	db.Model(&schema.VectorStoreCollection{}).Where("name = ?", exportCollectionName+"_new1").Update("serial_version_uid", headerIndo.Collection.SerialVersionUID)
	ragSystem, err = Get(exportCollectionName+"_new1",
		WithImportFile(tempFile.Name()),
		WithDB(db),
	)
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)

	num, err = ragSystem.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 0, num)
}
