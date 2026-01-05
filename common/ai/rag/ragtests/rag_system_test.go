package ragtests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestRAGSystem_AddKnowledge(t *testing.T) {
	db, err := rag.NewTemporaryRAGDB()
	assert.NoError(t, err)

	collectionName := "test_add_knowledge_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	ragSystem, err := rag.NewRAGSystem(
		rag.WithDB(db),
		rag.WithName(collectionName),
		rag.WithEmbeddingClient(mockEmbedding),
		rag.WithEnableKnowledgeBase(true),
	)
	assert.NoError(t, err)
	defer rag.DeleteRAG(db, collectionName)

	// 1. 测试添加 String 类型的知识
	knowledgeStr := "这是一条测试知识：Go语言是静态类型的编译语言。"
	err = ragSystem.AddKnowledge(knowledgeStr)
	assert.NoError(t, err)

	// 验证
	kbID := ragSystem.GetKnowledgeBaseID()
	var entries []schema.KnowledgeBaseEntry
	err = db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kbID).Find(&entries).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(entries))
	assert.Equal(t, knowledgeStr, entries[0].KnowledgeDetails)
	assert.Equal(t, "Standard", entries[0].KnowledgeType)

	// 2. 测试添加 Map 类型的知识
	knowledgeMap := map[string]any{
		"title":            "Map知识标题",
		"details":          "Map知识详情",
		"knowledge_type":   "MapType",
		"summary":          "Map知识摘要",
		"keywords":         []string{"Map", "Test"},
		"importance_score": 8,
	}
	err = ragSystem.AddKnowledge(knowledgeMap)
	assert.NoError(t, err)

	err = db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kbID).Find(&entries).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(entries))

	var mapEntry schema.KnowledgeBaseEntry
	for _, e := range entries {
		if e.KnowledgeTitle == "Map知识标题" {
			mapEntry = e
			break
		}
	}
	assert.Equal(t, "Map知识详情", mapEntry.KnowledgeDetails)
	assert.Equal(t, "MapType", mapEntry.KnowledgeType)
	assert.Equal(t, "Map知识摘要", mapEntry.Summary)
	assert.Equal(t, 8, mapEntry.ImportanceScore)

	// 3. 测试添加 *schema.KnowledgeBaseEntry 类型的知识
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  kbID,
		KnowledgeTitle:   "结构体知识标题",
		KnowledgeDetails: "结构体知识详情",
		KnowledgeType:    "StructType",
	}
	err = ragSystem.AddKnowledge(entry)
	assert.NoError(t, err)

	err = db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kbID).Find(&entries).Error
	assert.NoError(t, err)
	assert.Equal(t, 3, len(entries))

	var structEntry schema.KnowledgeBaseEntry
	for _, e := range entries {
		if e.KnowledgeTitle == "结构体知识标题" {
			structEntry = e
			break
		}
	}
	assert.Equal(t, "结构体知识详情", structEntry.KnowledgeDetails)
	assert.Equal(t, "StructType", structEntry.KnowledgeType)
}

func TestRAGSystem_QueryKnowledge(t *testing.T) {
	db, err := rag.NewTemporaryRAGDB()
	assert.NoError(t, err)

	collectionName := "test_query_knowledge_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	knowledge1 := mockEmbedding.GenerateRandomText(5)
	knowledge2 := mockEmbedding.GenerateRandomText(5)
	knowledge3 := mockEmbedding.GenerateRandomText(5)

	ragSystem, err := rag.NewRAGSystem(
		rag.WithDB(db),
		rag.WithName(collectionName),
		rag.WithEmbeddingClient(mockEmbedding),
		rag.WithEnableKnowledgeBase(true),
	)
	assert.NoError(t, err)
	defer rag.DeleteRAG(db, collectionName)

	// 添加一些知识
	knowledgeList := []string{
		knowledge1,
		knowledge2,
		knowledge3,
	}

	for _, k := range knowledgeList {
		err := ragSystem.AddKnowledge(k)
		assert.NoError(t, err)
	}

	// 验证文档已添加到向量库
	count, err := ragSystem.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 3, count)

	// 测试查询
	// 使用极低的阈值 (0.0 或 -1.0) 确保即便 mock embedding 生成随机向量也能召回结果
	// query 的 limits 参数: limits[0] 是 scoreThreshold
	// 使用其中一个知识作为查询词
	results, err := ragSystem.QueryKnowledge(knowledge1, 10)
	assert.NoError(t, err)
	assert.NotNil(t, results)

	// 验证召回数量
	// QueryKnowledge 返回的结果中包含多种消息类型（如 Message, Result, AISummary 等）
	// 我们需要统计 Type 为 Result 的数量
	resultCount := 0
	detailsFound := make(map[string]bool)
	for _, res := range results {
		// 检查类型
		assert.IsType(t, &knowledgebase.SearchKnowledgebaseResult{}, res)
		if res.Type == knowledgebase.SearchResultTypeResult {
			resultCount++
			entry, ok := res.Data.(*schema.KnowledgeBaseEntry)
			if ok {
				detailsFound[entry.KnowledgeDetails] = true
			}
		}
	}
	// 应该找回所有3个结果
	assert.Equal(t, 3, resultCount)

	for _, k := range knowledgeList {
		assert.True(t, detailsFound[k], "Knowledge '%s' should be found", k)
	}

	// 验证所有返回的 Data 都是我们添加的知识之一
	for detail := range detailsFound {
		found := false
		for _, k := range knowledgeList {
			if detail == k {
				found = true
				break
			}
		}
		assert.True(t, found, "Found unknown knowledge: %s", detail)
	}
}
