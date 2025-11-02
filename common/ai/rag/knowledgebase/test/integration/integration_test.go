package integration

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// TestIntegrationWithRealEmbedding 完整的集成测试，使用真实的 embedding 接口
func TestIntegrationWithRealEmbedding(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String()+".db")
	db, err := vectorstore.NewVectorStoreDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 知识库名称
	kbName := "integration-test-kb"
	kbDescription := "集成测试知识库"
	kbType := "integration"

	// 步骤1: 创建知识库
	t.Log("步骤1: 创建知识库")
	kb, err := knowledgebase.NewKnowledgeBase(db, kbName, kbDescription, kbType)
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	// 验证 KnowledgeBaseInfo 表
	t.Log("验证 KnowledgeBaseInfo 表")
	var kbInfos []schema.KnowledgeBaseInfo
	err = db.Find(&kbInfos).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(kbInfos))
	assert.Equal(t, kbName, kbInfos[0].KnowledgeBaseName)
	assert.Equal(t, kbDescription, kbInfos[0].KnowledgeBaseDescription)
	assert.Equal(t, kbType, kbInfos[0].KnowledgeBaseType)

	// 验证 VectorStoreCollection 表
	t.Log("验证 VectorStoreCollection 表")
	var collections []schema.VectorStoreCollection
	err = db.Find(&collections).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(collections))
	assert.Equal(t, kbName, collections[0].Name)
	assert.Equal(t, kbDescription, collections[0].Description)

	// 步骤2: 添加知识条目
	t.Log("步骤2: 添加知识条目")
	entries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:  int64(kbInfos[0].ID),
			KnowledgeTitle:   "Yaklang 编程语言介绍",
			KnowledgeType:    "CoreConcept",
			ImportanceScore:  9,
			Keywords:         []string{"yaklang", "编程语言", "安全"},
			KnowledgeDetails: "Yaklang 是一种专门为网络安全领域设计的编程语言，提供了丰富的安全测试和漏洞挖掘功能。它集成了多种安全工具和框架，使安全研究人员能够更高效地进行安全测试工作。",
			Summary:          "Yaklang 是专为网络安全设计的编程语言",
			SourcePage:       1,
			PotentialQuestions: []string{
				"什么是Yaklang?",
				"Yaklang有什么特点?",
				"如何使用Yaklang进行安全测试?",
			},
		},
		{
			KnowledgeBaseID:  int64(kbInfos[0].ID),
			KnowledgeTitle:   "RAG技术原理",
			KnowledgeType:    "Technology",
			ImportanceScore:  8,
			Keywords:         []string{"RAG", "检索", "生成", "AI"},
			KnowledgeDetails: "RAG (Retrieval-Augmented Generation) 是一种结合了信息检索和文本生成的人工智能技术。它通过先检索相关文档，然后基于检索到的信息生成回答，从而提高了生成内容的准确性和相关性。",
			Summary:          "RAG 结合检索和生成技术提高AI回答质量",
			SourcePage:       2,
			PotentialQuestions: []string{
				"什么是RAG技术?",
				"RAG如何工作?",
				"RAG的优势是什么?",
			},
		},
		{
			KnowledgeBaseID:  int64(kbInfos[0].ID),
			KnowledgeTitle:   "向量数据库应用",
			KnowledgeType:    "Application",
			ImportanceScore:  7,
			Keywords:         []string{"向量数据库", "嵌入", "相似性搜索"},
			KnowledgeDetails: "向量数据库是专门用于存储和检索高维向量数据的数据库系统。它广泛应用于推荐系统、图像搜索、自然语言处理等领域，通过计算向量间的相似度来找到最相关的结果。",
			Summary:          "向量数据库专门处理高维向量数据的存储和检索",
			SourcePage:       3,
			PotentialQuestions: []string{
				"什么是向量数据库?",
				"向量数据库有什么用途?",
				"如何使用向量数据库?",
			},
		},
	}

	// 添加每个知识条目
	for i, entry := range entries {
		t.Logf("添加第 %d 个知识条目: %s", i+1, entry.KnowledgeTitle)
		err = kb.AddKnowledgeEntry(entry)
		assert.NoError(t, err)

		// 等待一下，让向量化完成
		time.Sleep(time.Millisecond * 100)
	}

	// 验证 KnowledgeBaseEntry 表
	t.Log("验证 KnowledgeBaseEntry 表")
	var dbEntries []schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", kbInfos[0].ID).Find(&dbEntries).Error
	assert.NoError(t, err)
	assert.Equal(t, 3, len(dbEntries))

	// 验证每个条目的详细信息
	for i, dbEntry := range dbEntries {
		assert.NotZero(t, dbEntry.ID)
		assert.Equal(t, int64(kbInfos[0].ID), dbEntry.KnowledgeBaseID)
		assert.NotEmpty(t, dbEntry.KnowledgeTitle)
		assert.NotEmpty(t, dbEntry.KnowledgeDetails)
		t.Logf("条目 %d: ID=%d, 标题=%s", i+1, dbEntry.ID, dbEntry.KnowledgeTitle)
	}

	// 验证 VectorStoreDocument 表
	t.Log("验证 VectorStoreDocument 表")
	var documents []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collections[0].ID).Find(&documents).Error
	assert.NoError(t, err)
	assert.Equal(t, 3, len(documents))

	// 验证每个文档的向量数据
	for i, doc := range documents {
		assert.NotEmpty(t, doc.DocumentID)
		assert.NotEmpty(t, doc.Content)
		assert.NotNil(t, doc.Embedding)
		assert.True(t, len(doc.Embedding) > 0)
		t.Logf("文档 %d: ID=%s, 向量维度=%d, 内容长度=%d",
			i+1, doc.DocumentID, len(doc.Embedding), len(doc.Content))
	}

	// 步骤3: 测试搜索功能
	t.Log("步骤3: 测试搜索功能")

	// 测试基本搜索
	searchResults, err := kb.SearchKnowledgeEntries("Yaklang编程语言", 5)
	assert.NoError(t, err)
	assert.True(t, len(searchResults) > 0)
	t.Logf("搜索 'Yaklang编程语言' 返回 %d 个结果", len(searchResults))

	// 验证搜索结果
	found := false
	for _, result := range searchResults {
		t.Logf("搜索结果: %s", result.KnowledgeTitle)
		if result.KnowledgeTitle == "Yaklang 编程语言介绍" {
			found = true
		}
	}
	assert.True(t, found, "应该能找到 'Yaklang 编程语言介绍' 条目")

	// 步骤4: 测试同步功能
	t.Log("步骤4: 测试同步功能")

	// 检查同步状态
	syncStatus, err := kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.Equal(t, 3, syncStatus.DatabaseEntries)
	assert.Equal(t, 3, syncStatus.RAGDocuments)
	assert.True(t, syncStatus.InSync)
	t.Logf("同步状态: 数据库条目=%d, RAG文档=%d, 同步=%v",
		syncStatus.DatabaseEntries, syncStatus.RAGDocuments, syncStatus.InSync)

	// 步骤5: 测试更新操作
	t.Log("步骤5: 测试更新操作")

	// 更新第一个条目
	firstEntry := &dbEntries[0]
	originalTitle := firstEntry.KnowledgeTitle
	firstEntry.KnowledgeTitle = "Yaklang 编程语言介绍 (已更新)"
	firstEntry.KnowledgeDetails += "\n\n这是更新后的内容。"

	err = kb.UpdateKnowledgeEntry(firstEntry.HiddenIndex, firstEntry)
	assert.NoError(t, err)

	// 验证更新后的数据
	updatedEntry, err := kb.GetKnowledgeEntry(firstEntry.HiddenIndex)
	assert.NoError(t, err)
	assert.Equal(t, "Yaklang 编程语言介绍 (已更新)", updatedEntry.KnowledgeTitle)
	assert.Contains(t, updatedEntry.KnowledgeDetails, "这是更新后的内容")
	t.Logf("更新成功: %s -> %s", originalTitle, updatedEntry.KnowledgeTitle)

	// 步骤6: 测试删除操作
	t.Log("步骤6: 测试删除操作")

	// 删除最后一个条目
	lastEntry := &dbEntries[len(dbEntries)-1]
	deletedTitle := lastEntry.KnowledgeTitle

	err = kb.DeleteKnowledgeEntry(lastEntry.HiddenIndex)
	assert.NoError(t, err)
	t.Logf("删除条目: %s", deletedTitle)

	// 验证删除后的状态
	var remainingEntries []schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", kbInfos[0].ID).Find(&remainingEntries).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(remainingEntries))

	var remainingDocuments []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collections[0].ID).Find(&remainingDocuments).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(remainingDocuments))

	// 检查最终同步状态
	finalSyncStatus, err := kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.Equal(t, 2, finalSyncStatus.DatabaseEntries)
	assert.Equal(t, 2, finalSyncStatus.RAGDocuments)
	assert.True(t, finalSyncStatus.InSync)
	t.Logf("最终同步状态: 数据库条目=%d, RAG文档=%d, 同步=%v",
		finalSyncStatus.DatabaseEntries, finalSyncStatus.RAGDocuments, finalSyncStatus.InSync)

	// 步骤7: 测试跨知识库搜索
	t.Log("步骤7: 测试跨知识库搜索")

	// 创建第二个知识库进行跨库搜索测试
	kb2, err := knowledgebase.NewKnowledgeBase(db, "test-kb-2", "第二个测试知识库", "test")
	assert.NoError(t, err)

	// 在第二个知识库中添加一个条目
	var secondKbInfo schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", "test-kb-2").First(&secondKbInfo).Error
	assert.NoError(t, err)

	entry2 := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  int64(secondKbInfo.ID),
		KnowledgeTitle:   "机器学习基础",
		KnowledgeType:    "Technology",
		ImportanceScore:  8,
		Keywords:         []string{"机器学习", "AI", "算法"},
		KnowledgeDetails: "机器学习是人工智能的一个重要分支，通过算法让计算机从数据中学习规律。",
		Summary:          "机器学习让计算机从数据中学习",
		SourcePage:       1,
	}

	err = kb2.AddKnowledgeEntry(entry2)
	assert.NoError(t, err)

	// 等待向量化完成
	time.Sleep(time.Millisecond * 200)

	// 步骤8: 验证最终的数据库状态
	t.Log("步骤8: 验证最终的数据库状态")

	// 检查知识库信息表
	var finalKbInfos []schema.KnowledgeBaseInfo
	err = db.Find(&finalKbInfos).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(finalKbInfos))
	t.Logf("最终知识库信息表有 %d 条记录", len(finalKbInfos))

	// 检查知识库条目表
	var finalEntries []schema.KnowledgeBaseEntry
	err = db.Find(&finalEntries).Error
	assert.NoError(t, err)
	assert.Equal(t, 3, len(finalEntries)) // 第一个知识库2个 + 第二个知识库1个
	t.Logf("最终知识库条目表有 %d 条记录", len(finalEntries))

	// 检查向量集合表
	var finalCollections []schema.VectorStoreCollection
	err = db.Find(&finalCollections).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(finalCollections))
	t.Logf("最终向量集合表有 %d 条记录", len(finalCollections))

	// 检查向量文档表
	var finalDocuments []schema.VectorStoreDocument
	err = db.Find(&finalDocuments).Error
	assert.NoError(t, err)
	assert.Equal(t, 3, len(finalDocuments))
	t.Logf("最终向量文档表有 %d 条记录", len(finalDocuments))

	// 输出数据库表的详细统计信息
	t.Log("=== 数据库表统计信息 ===")
	printTableStats(t, db)

	t.Log("🎉 集成测试完成！所有功能正常工作。")
}

// printTableStats 打印数据库表的统计信息
func printTableStats(t *testing.T, db *gorm.DB) {
	// KnowledgeBaseInfo 表统计
	var kbInfoCount int64
	db.Model(&schema.KnowledgeBaseInfo{}).Count(&kbInfoCount)
	t.Logf("📊 KnowledgeBaseInfo 表: %d 条记录", kbInfoCount)

	var kbInfos []schema.KnowledgeBaseInfo
	db.Find(&kbInfos)
	for i, info := range kbInfos {
		t.Logf("  %d. ID=%d, 名称=%s, 类型=%s",
			i+1, info.ID, info.KnowledgeBaseName, info.KnowledgeBaseType)
	}

	// KnowledgeBaseEntry 表统计
	var entryCount int64
	db.Model(&schema.KnowledgeBaseEntry{}).Count(&entryCount)
	t.Logf("📊 KnowledgeBaseEntry 表: %d 条记录", entryCount)

	var entries []schema.KnowledgeBaseEntry
	db.Find(&entries)
	for i, entry := range entries {
		t.Logf("  %d. ID=%d, 知识库ID=%d, 标题=%s",
			i+1, entry.ID, entry.KnowledgeBaseID, entry.KnowledgeTitle)
	}

	// VectorStoreCollection 表统计
	var collectionCount int64
	db.Model(&schema.VectorStoreCollection{}).Count(&collectionCount)
	t.Logf("📊 VectorStoreCollection 表: %d 条记录", collectionCount)

	var collections []schema.VectorStoreCollection
	db.Find(&collections)
	for i, collection := range collections {
		t.Logf("  %d. ID=%d, 名称=%s, 维度=%d",
			i+1, collection.ID, collection.Name, collection.Dimension)
	}

	// VectorStoreDocument 表统计
	var documentCount int64
	db.Model(&schema.VectorStoreDocument{}).Count(&documentCount)
	t.Logf("📊 VectorStoreDocument 表: %d 条记录", documentCount)

	var documents []schema.VectorStoreDocument
	db.Find(&documents)
	for i, doc := range documents {
		t.Logf("  %d. ID=%d, 文档ID=%s, 集合ID=%d, 向量维度=%d",
			i+1, doc.ID, doc.DocumentID, doc.CollectionID, len(doc.Embedding))
	}
}
