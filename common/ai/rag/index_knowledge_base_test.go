package rag

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TestBuildVectorIndexForKnowledgeBase 测试知识库向量索引构建功能
func TestMUSTPASS_BuildVectorIndexForKnowledgeBase(t *testing.T) {
	// 1. 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 2. 自动迁移数据库表结构
	db.AutoMigrate(
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
	)

	// 3. 创建测试知识库
	knowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "test_knowledge_base",
		KnowledgeBaseDescription: "测试知识库，用于单元测试",
		KnowledgeBaseType:        "test",
	}
	err = yakit.CreateKnowledgeBase(db, knowledgeBase)
	assert.NoError(t, err)

	// 获取创建的知识库ID
	var savedKnowledgeBase schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", "test_knowledge_base").First(&savedKnowledgeBase).Error
	assert.NoError(t, err)
	knowledgeBaseID := int64(savedKnowledgeBase.ID)

	// 4. 创建测试知识库条目
	testEntries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     "Yaklang基础概念",
			KnowledgeType:      "CoreConcept",
			ImportanceScore:    9,
			Keywords:           schema.StringArray{"Yaklang", "编程语言", "安全研究"},
			KnowledgeDetails:   "Yaklang是一种专为安全研究设计的编程语言，提供了丰富的安全工具和库。它具有易用性强、功能强大的特点，适合渗透测试、漏洞挖掘等安全研究工作。",
			Summary:            "Yaklang是专为安全研究设计的编程语言",
			SourcePage:         1,
			PotentialQuestions: schema.StringArray{"什么是Yaklang", "Yaklang有什么特点", "Yaklang适用于什么场景"},
		},
		{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     "RAG技术原理",
			KnowledgeType:      "Technology",
			ImportanceScore:    8,
			Keywords:           schema.StringArray{"RAG", "检索增强生成", "AI技术"},
			KnowledgeDetails:   "RAG(Retrieval-Augmented Generation)是一种结合检索和生成的AI技术。它首先从知识库中检索相关信息，然后基于检索到的信息生成回答，能够提供更准确、更具体的回答。",
			Summary:            "RAG是结合检索和生成的AI技术",
			SourcePage:         2,
			PotentialQuestions: schema.StringArray{"什么是RAG", "RAG如何工作", "RAG的优势是什么"},
		},
		{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     "向量数据库应用",
			KnowledgeType:      "Application",
			ImportanceScore:    7,
			Keywords:           schema.StringArray{"向量数据库", "嵌入向量", "相似性搜索"},
			KnowledgeDetails:   "向量数据库是专门用于存储和检索高维向量数据的数据库系统。在AI应用中，它能够高效地进行相似性搜索，是RAG系统的重要组成部分。常见的向量数据库包括Pinecone、Weaviate、Chroma等。",
			Summary:            "向量数据库专门用于存储和检索高维向量数据",
			SourcePage:         3,
			PotentialQuestions: schema.StringArray{"什么是向量数据库", "向量数据库有什么用", "常见的向量数据库有哪些"},
		},
	}

	// 添加知识库条目到数据库
	for _, entry := range testEntries {
		err = yakit.CreateKnowledgeBaseEntry(db, entry)
		assert.NoError(t, err)
	}

	// 5. 构建向量索引（核心测试功能）
	// 使用模拟嵌入器配置
	_, err = BuildVectorIndexForKnowledgeBase(db, knowledgeBaseID, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.NoError(t, err)

	// 6. 验证索引构建结果
	// 检查RAG集合是否已创建
	ragCollectionName := savedKnowledgeBase.KnowledgeBaseName
	assert.True(t, CollectionIsExists(db, ragCollectionName))

	// 7. 测试搜索功能
	// 创建真实的RAG系统来进行测试
	mockEmbedder := NewMockEmbedder(testEmbedder)
	store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, mockEmbedder)
	assert.NoError(t, err)
	defer store.Remove()

	ragSystem := NewRAGSystem(mockEmbedder, store)

	// 测试文档计数
	docCount, err := ragSystem.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 3, docCount) // 应该有3个文档

	// 测试搜索Yaklang相关内容
	searchResults, err := ragSystem.QueryWithPage("什么是Yaklang", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, searchResults)

	// 验证搜索结果包含正确的内容
	found := false
	for _, result := range searchResults {
		if result.Document.Metadata["knowledge_title"] == "Yaklang基础概念" {
			found = true
			assert.Contains(t, result.Document.Content, "Yaklang是一种专为安全研究设计的编程语言")
			break
		}
	}
	assert.True(t, found, "应该能够找到Yaklang相关的知识条目")

	// 测试搜索RAG相关内容
	ragSearchResults, err := ragSystem.QueryWithPage("RAG技术", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, ragSearchResults)

	// 验证RAG搜索结果
	ragFound := false
	for _, result := range ragSearchResults {
		if result.Document.Metadata["knowledge_title"] == "RAG技术原理" {
			ragFound = true
			assert.Contains(t, result.Document.Content, "RAG(Retrieval-Augmented Generation)")
			break
		}
	}
	assert.True(t, ragFound, "应该能够找到RAG相关的知识条目")

	// 8. 测试文档删除
	// 删除一个知识库条目
	var firstEntry schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", knowledgeBaseID).First(&firstEntry).Error
	assert.NoError(t, err)

	err = yakit.DeleteKnowledgeBaseEntryByHiddenIndex(db, firstEntry.HiddenIndex)
	assert.NoError(t, err)

	// 重新构建索引
	_, err = BuildVectorIndexForKnowledgeBase(db, knowledgeBaseID, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.NoError(t, err)

	// 验证文档数量减少
	docCountAfterDelete, err := ragSystem.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 2, docCountAfterDelete) // 应该剩下2个文档

	// 9. 清理测试数据
	err = yakit.DeleteKnowledgeBase(db, knowledgeBaseID)
	assert.NoError(t, err)

	err = DeleteCollection(db, ragCollectionName)
	assert.NoError(t, err)
}

// TestBuildVectorIndexEmptyKnowledgeBase 测试空知识库的索引构建
func TestMUSTPASS_BuildVectorIndexEmptyKnowledgeBase(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
	)

	// 创建空的知识库
	knowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "empty_test_knowledge_base",
		KnowledgeBaseDescription: "空的测试知识库",
		KnowledgeBaseType:        "test",
	}
	err = yakit.CreateKnowledgeBase(db, knowledgeBase)
	assert.NoError(t, err)

	// 获取创建的知识库ID
	var savedKnowledgeBase schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", "empty_test_knowledge_base").First(&savedKnowledgeBase).Error
	assert.NoError(t, err)
	knowledgeBaseID := int64(savedKnowledgeBase.ID)

	// 构建空知识库的向量索引（应该成功但不创建任何文档）
	_, err = BuildVectorIndexForKnowledgeBase(db, knowledgeBaseID, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.NoError(t, err)

	// 验证空索引
	ragCollectionName := savedKnowledgeBase.KnowledgeBaseName
	if CollectionIsExists(db, ragCollectionName) {
		mockEmbedder := NewMockEmbedder(testEmbedder)
		store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, mockEmbedder)
		assert.NoError(t, err)
		defer store.Remove()

		ragSystem := NewRAGSystem(mockEmbedder, store)

		docCount, err := ragSystem.CountDocuments()
		assert.NoError(t, err)
		assert.Equal(t, 0, docCount) // 应该没有文档
	}

	// 清理
	err = yakit.DeleteKnowledgeBase(db, knowledgeBaseID)
	assert.NoError(t, err)
}

// TestBuildVectorIndexNonExistentKnowledgeBase 测试不存在的知识库
func TestMUSTPASS_BuildVectorIndexNonExistentKnowledgeBase(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
	)

	// 尝试为不存在的知识库构建索引
	nonExistentID := int64(99999)
	_, err = BuildVectorIndexForKnowledgeBase(db, nonExistentID, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.Error(t, err) // 应该返回错误
	assert.Contains(t, err.Error(), "record not found")
}

// TestBuildVectorIndexForKnowledgeBaseEntry 测试单个知识库条目的向量索引构建功能
func TestMUSTPASS_BuildVectorIndexForKnowledgeBaseEntry(t *testing.T) {
	// 1. 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 2. 自动迁移数据库表结构
	db.AutoMigrate(
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
	)

	// 3. 创建测试知识库
	knowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "test_single_entry_kb",
		KnowledgeBaseDescription: "测试单个条目的知识库",
		KnowledgeBaseType:        "test",
	}
	err = yakit.CreateKnowledgeBase(db, knowledgeBase)
	assert.NoError(t, err)

	// 获取创建的知识库ID
	var savedKnowledgeBase schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", "test_single_entry_kb").First(&savedKnowledgeBase).Error
	assert.NoError(t, err)
	knowledgeBaseID := int64(savedKnowledgeBase.ID)

	// 4. 创建测试知识库条目
	testEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    knowledgeBaseID,
		KnowledgeTitle:     "Go语言基础",
		KnowledgeType:      "ProgrammingLanguage",
		ImportanceScore:    8,
		Keywords:           schema.StringArray{"Go", "Golang", "编程语言", "并发"},
		KnowledgeDetails:   "Go是Google开发的一种静态强类型、编译型语言。Go语言语法与C相近，但功能上有：内存安全，GC（垃圾回收），结构形态及CSP-style并发计算。",
		Summary:            "Go是Google开发的编程语言",
		SourcePage:         1,
		PotentialQuestions: schema.StringArray{"什么是Go语言", "Go语言有什么特点", "Go语言适用于什么场景"},
	}

	// 添加知识库条目到数据库
	err = yakit.CreateKnowledgeBaseEntry(db, testEntry)
	assert.NoError(t, err)

	// 获取保存后的条目ID
	var savedEntry schema.KnowledgeBaseEntry
	err = db.Where("knowledge_title = ?", "Go语言基础").First(&savedEntry).Error
	assert.NoError(t, err)
	entryID := savedEntry.HiddenIndex

	// 5. 构建单个条目的向量索引（核心测试功能）
	_, err = BuildVectorIndexForKnowledgeBaseEntry(db, savedEntry.KnowledgeBaseID, entryID, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.NoError(t, err)

	// 6. 验证索引构建结果
	// 检查RAG集合是否已创建
	ragCollectionName := savedKnowledgeBase.KnowledgeBaseName
	assert.True(t, CollectionIsExists(db, ragCollectionName))

	// 7. 测试搜索功能
	// 创建RAG系统来进行测试
	mockEmbedder := NewMockEmbedder(testEmbedder)
	store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, mockEmbedder)
	assert.NoError(t, err)
	defer store.Remove()

	ragSystem := NewRAGSystem(mockEmbedder, store)

	// 测试文档计数
	docCount, err := ragSystem.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 1, docCount) // 应该有1个文档

	// 测试搜索功能
	searchResults, err := ragSystem.QueryWithPage("什么是Go语言", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, searchResults)

	// 验证搜索结果包含正确的内容
	found := false
	for _, result := range searchResults {
		if result.Document.Metadata["knowledge_title"] == "Go语言基础" {
			found = true
			assert.Contains(t, result.Document.Content, "Go是Google开发的一种静态强类型、编译型语言")
			assert.Equal(t, utils.InterfaceToString(entryID), result.Document.ID)
			// 验证元数据
			assert.Equal(t, float64(knowledgeBaseID), result.Document.Metadata["knowledge_base_id"])
			assert.Equal(t, "ProgrammingLanguage", result.Document.Metadata["knowledge_type"])
			assert.Equal(t, float64(8), result.Document.Metadata["importance_score"])
			assert.Equal(t, float64(1), result.Document.Metadata["source_page"])
			break
		}
	}
	assert.True(t, found, "应该能够找到Go语言相关的知识条目")

	// 8. 测试更新条目后重新索引
	// 更新知识库条目
	savedEntry.KnowledgeDetails = "Go语言（又称Golang）是Google开发的一种静态强类型、编译型的程序设计语言。Go语言有着简洁的语法和高效的性能，特别适合云计算和微服务开发。"
	err = yakit.UpdateKnowledgeBaseEntryByHiddenIndex(db, savedEntry.HiddenIndex, &savedEntry)
	assert.NoError(t, err)

	// 重新为该条目构建索引
	_, err = BuildVectorIndexForKnowledgeBaseEntry(db, savedEntry.KnowledgeBaseID, savedEntry.HiddenIndex, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.NoError(t, err)

	// 验证更新后的内容
	updatedSearchResults, err := ragSystem.QueryWithPage("Go语言微服务", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, updatedSearchResults)

	// 验证更新后的搜索结果
	updatedFound := false
	for _, result := range updatedSearchResults {
		if result.Document.Metadata["knowledge_title"] == "Go语言基础" {
			updatedFound = true
			assert.Contains(t, result.Document.Content, "特别适合云计算和微服务开发")
			break
		}
	}
	assert.True(t, updatedFound, "应该能够找到更新后的Go语言知识条目")

	// 9. 清理测试数据
	err = yakit.DeleteKnowledgeBase(db, knowledgeBaseID)
	assert.NoError(t, err)

	err = DeleteCollection(db, ragCollectionName)
	assert.NoError(t, err)
}

// TestBuildVectorIndexForNonExistentEntry 测试不存在的知识库条目
func TestMUSTPASS_BuildVectorIndexForNonExistentEntry(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
	)

	// 尝试为不存在的知识库条目构建索引
	nonExistentEntryID := uuid.NewString()
	_, err = BuildVectorIndexForKnowledgeBaseEntry(db, 0, nonExistentEntryID, WithEmbeddingModel("mock-model"), WithModelDimension(3), WithEmbeddingClient(NewMockEmbedder(testEmbedder)))
	assert.Error(t, err) // 应该返回错误
}
