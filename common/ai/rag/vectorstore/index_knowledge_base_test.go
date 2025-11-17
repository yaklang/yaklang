package vectorstore

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// testEmbedder 测试用的嵌入器函数
func testEmbedder(text string) ([]float32, error) {
	// 简单地生成一个固定的向量作为嵌入
	// 根据文本内容生成不同的向量
	if len(text) > 10 {
		return []float32{1.0, 0.0, 0.0}, nil
	} else if len(text) > 5 {
		return []float32{0.0, 1.0, 0.0}, nil
	}
	return []float32{0.0, 0.0, 1.0}, nil
}

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
	assert.True(t, HasCollection(db, ragCollectionName))

	// 7. 测试搜索功能
	// 创建真实的RAG系统来进行测试
	mockEmbedder := NewMockEmbedder(testEmbedder)
	store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, WithEmbeddingClient(mockEmbedder))
	assert.NoError(t, err)
	defer store.Remove()

	// 测试文档计数
	docCount, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 3, docCount) // 应该有3个文档

	// 测试搜索Yaklang相关内容
	searchResults, err := store.QueryWithPage("什么是Yaklang", 1, 5)
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
	ragSearchResults, err := store.QueryWithPage("RAG技术", 1, 5)
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
	docCountAfterDelete, err := store.Count()
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
	if HasCollection(db, ragCollectionName) {
		mockEmbedder := NewMockEmbedder(testEmbedder)
		store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, WithEmbeddingClient(mockEmbedder))
		assert.NoError(t, err)
		defer store.Remove()

		docCount, err := store.Count()
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
	assert.True(t, HasCollection(db, ragCollectionName))

	// 7. 测试搜索功能
	// 创建RAG系统来进行测试
	mockEmbedder := NewMockEmbedder(testEmbedder)
	store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, WithEmbeddingClient(mockEmbedder))
	assert.NoError(t, err)
	defer store.Remove()

	// 测试文档计数
	docCount, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, docCount) // 应该有1个文档

	// 测试搜索功能
	searchResults, err := store.QueryWithPage("什么是Go语言", 1, 5)
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
	updatedSearchResults, err := store.QueryWithPage("Go语言微服务", 1, 5)
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

// TestMUSTPASS_DeleteEmbeddingData 测试删除嵌入数据
// 测试场景：在空白数据库环境下的完整删除嵌入数据周期
// 测试步骤：
// 1. 创建测试知识库
// 2. 创建测试知识库条目
// 3. 构建向量索引
// 4. 转换为PQ模式
// 5. 删除embedding数据
// 6. 验证PQ模式下的查询功能
// 7. 验证文档计数在删除embedding后保持一致
// 8. 再次查询所有VectorStoreDocument文档，验证embedding字段已被删除
// 9. 测试PQ模式下的查询功能
func TestMUSTPASS_DeleteEmbeddingData(t *testing.T) {
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

	knName := uuid.New().String()

	// 3. 创建测试知识库
	knowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        knName,
		KnowledgeBaseDescription: "测试删除嵌入数据的知识库",
		KnowledgeBaseType:        "test",
	}
	err = yakit.CreateKnowledgeBase(db, knowledgeBase)
	assert.NoError(t, err)

	// 获取创建的知识库ID
	var savedKnowledgeBase schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", knName).First(&savedKnowledgeBase).Error
	assert.NoError(t, err)
	knowledgeBaseID := int64(savedKnowledgeBase.ID)

	// 4. 创建测试知识库条目
	testEntries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     "机器学习基础",
			KnowledgeType:      "Technology",
			ImportanceScore:    9,
			Keywords:           schema.StringArray{"机器学习", "AI", "算法"},
			KnowledgeDetails:   "机器学习是人工智能的一个分支，它是一种通过算法使计算机能够从数据中学习并做出决策或预测的技术。机器学习包括监督学习、无监督学习和强化学习等多种方法。",
			Summary:            "机器学习是AI的重要分支",
			SourcePage:         1,
			PotentialQuestions: schema.StringArray{"什么是机器学习", "机器学习的分类", "机器学习的应用"},
		},
		{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     "深度学习原理",
			KnowledgeType:      "Technology",
			ImportanceScore:    8,
			Keywords:           schema.StringArray{"深度学习", "神经网络", "AI"},
			KnowledgeDetails:   "深度学习是机器学习的一个子集，它使用多层神经网络来学习数据的复杂表示。深度学习在图像识别、自然语言处理、语音识别等领域取得了重大突破。",
			Summary:            "深度学习使用多层神经网络学习复杂表示",
			SourcePage:         2,
			PotentialQuestions: schema.StringArray{"什么是深度学习", "深度学习和机器学习的区别", "深度学习的应用领域"},
		},
		{
			KnowledgeBaseID:    knowledgeBaseID,
			KnowledgeTitle:     "自然语言处理技术",
			KnowledgeType:      "Technology",
			ImportanceScore:    7,
			Keywords:           schema.StringArray{"NLP", "自然语言处理", "文本处理"},
			KnowledgeDetails:   "自然语言处理（NLP）是计算机科学和人工智能的一个分支，旨在让计算机能够理解、解释和生成人类语言。NLP技术包括分词、词性标注、命名实体识别、情感分析、机器翻译等。",
			Summary:            "NLP让计算机理解和处理人类语言",
			SourcePage:         3,
			PotentialQuestions: schema.StringArray{"什么是NLP", "NLP的主要技术", "NLP的应用场景"},
		},
	}

	// 添加知识库条目到数据库
	for _, entry := range testEntries {
		err = yakit.CreateKnowledgeBaseEntry(db, entry)
		assert.NoError(t, err)
	}

	test1024Embedder := func(text string) ([]float32, error) {
		embedding := make([]float32, 1024)
		if strings.Contains(text, "机器学习") {
			embedding[100] = 1.0
			return embedding, nil
		}
		if strings.Contains(text, "自然语言处理") {
			embedding[200] = 1.0
			return embedding, nil
		}
		for i := range 1024 {
			embedding[i] = float32(i)
		}
		return embedding, nil
	}

	// 5. 构建向量索引（核心测试功能）
	// 使用模拟嵌入器配置
	_, err = BuildVectorIndexForKnowledgeBase(db, knowledgeBaseID, WithEmbeddingModel("mock-model"), WithModelDimension(1024), WithEmbeddingClient(NewMockEmbedder(test1024Embedder)))
	assert.NoError(t, err)

	// 6. 验证索引构建结果
	// 检查RAG集合是否已创建
	ragCollectionName := savedKnowledgeBase.KnowledgeBaseName
	assert.True(t, HasCollection(db, ragCollectionName))

	// 7. 创建RAG系统来进行测试
	mockEmbedder := NewMockEmbedder(test1024Embedder)
	store, err := LoadSQLiteVectorStoreHNSW(db, ragCollectionName, WithEmbeddingClient(mockEmbedder))
	assert.NoError(t, err)
	defer store.Remove()

	// 验证初始文档数量
	docCount, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 3, docCount) // 应该有3个文档

	// 8. 转换为PQ模式
	err = store.ConvertToPQMode()
	assert.NoError(t, err)

	// 9. 查询所有VectorStoreDocument文档，检查embedding字段是否存在
	var vectorDocs []schema.VectorStoreDocument
	// 通过collection name获取collection ID
	var collection schema.VectorStoreCollection
	err = db.Where("name = ?", ragCollectionName).First(&collection).Error
	assert.NoError(t, err)
	err = db.Where("collection_id = ?", collection.ID).Find(&vectorDocs).Error
	assert.NoError(t, err)
	assert.Equal(t, 4, len(vectorDocs)) // 应该有3个向量文档

	// 验证转换为PQ模式前embedding字段不为空
	for _, doc := range vectorDocs {
		assert.NotEmpty(t, doc.Embedding, "转换为PQ模式前embedding字段应该不为空")
	}

	// 10. 删除embedding数据
	err = store.DeleteEmbeddingData()
	assert.NoError(t, err)

	// 11. 再次查询所有VectorStoreDocument文档，验证embedding字段已被删除
	var vectorDocsAfterDelete []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collection.ID).Find(&vectorDocsAfterDelete).Error
	assert.NoError(t, err)
	assert.Equal(t, 4, len(vectorDocsAfterDelete)) // 文档数量应该保持不变

	// 验证embedding字段已被删除（应该为空）
	for _, doc := range vectorDocsAfterDelete {
		assert.Empty(t, doc.Embedding, "删除embedding数据后embedding字段应该为空")
		assert.NotEmpty(t, doc.PQCode, "PQ编码应该仍然存在")
	}

	// 12. 测试PQ模式下的查询功能
	// 验证在删除embedding数据后，PQ模式查询仍然能正常工作
	searchResults, err := store.QueryWithPage("什么是机器学习", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, searchResults, "PQ模式下应该能够正常进行查询")

	// 验证搜索结果包含正确的内容
	found := searchResults[0].Document.Metadata["knowledge_title"] == "机器学习基础"
	assert.True(t, found, "PQ模式下应该能够找到机器学习相关的知识条目")

	// 测试另一个查询
	nlpSearchResults, err := store.QueryWithPage("自然语言处理", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, nlpSearchResults, "PQ模式下应该能够查询NLP相关内容")

	// 验证NLP搜索结果
	nlpFound := nlpSearchResults[0].Document.Metadata["knowledge_title"] == "自然语言处理技术"
	assert.True(t, nlpFound, "PQ模式下应该能够找到NLP相关的知识条目")

	// 13. 验证文档计数在删除embedding后保持一致
	finalDocCount, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 3, finalDocCount, "删除embedding数据后文档数量应该保持不变")

	// 14. 验证归档检查
	err = store.SetArchived(true)
	assert.NoError(t, err)
	assert.True(t, store.GetArchived(), "归档检查应该返回true")

	err = store.AddWithOptions("test_document_id", "test_content")
	if err == nil {
		t.Fatalf("should be error: %v", err)
	}
	assert.Contains(t, err.Error(), "archived")

	err = store.Delete("test_document_id")
	if err == nil {
		t.Fatalf("should be error: %v", err)
	}
	assert.Contains(t, err.Error(), "archived")

	// 15. 验证修复embedding数据
	err = store.ConvertToStandardMode()
	assert.NoError(t, err)

	var vectorDocsAfterConvertToStandardMode []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collection.ID).Find(&vectorDocsAfterConvertToStandardMode).Error
	assert.NoError(t, err)
	assert.Equal(t, 4, len(vectorDocsAfterConvertToStandardMode)) // 文档数量应该保持不变
	for _, doc := range vectorDocsAfterConvertToStandardMode {
		assert.NotEmpty(t, doc.Embedding, "修复embedding数据后embedding字段应该不为空")
		assert.Len(t, doc.Embedding, 1024, "修复embedding数据后embedding字段应该为1024维")
	}
	// 16. 验证查询
	store, err = LoadSQLiteVectorStoreHNSW(db, ragCollectionName, WithEmbeddingClient(mockEmbedder))
	assert.NoError(t, err)
	defer store.Remove()

	searchResults, err = store.QueryWithPage("什么是机器学习", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, searchResults, "标准模式下应该能够正常进行查询")

	assert.Equal(t, "机器学习基础", searchResults[0].Document.Metadata["knowledge_title"], "标准模式下应该能够找到机器学习相关的知识条目")

	searchResults, err = store.QueryWithPage("什么是自然语言处理", 1, 5)
	assert.NoError(t, err)
	assert.NotEmpty(t, searchResults, "标准模式下应该能够正常进行查询")

	assert.Equal(t, "自然语言处理技术", searchResults[0].Document.Metadata["knowledge_title"], "标准模式下应该能够找到自然语言处理相关的知识条目")
}
