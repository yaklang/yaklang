package ragtests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	_ "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_RAGSystem_GenerateQuestionIndex(t *testing.T) {
	db, err := rag.NewTemporaryRAGDB()
	assert.NoError(t, err)
	exportCollectionName := "test_generate_question_index_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	// 模拟知识库内容
	knowledgeDetails := mockEmbedding.GenerateRandomText(20)

	// 模拟生成的问题
	question1 := mockEmbedding.GenerateRandomText(5)
	question2 := mockEmbedding.GenerateRandomText(5)

	mockAICallback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		rspStr := `{
			"@action": "object",
			"question_list": [
				{"question": "%s","answer_location": {"start_line": 1, "end_line": 2}},
				{"question": "%s","answer_location": {"start_line": 2, "end_line": 3}}
			]
		}`
		return fmt.Sprintf(rspStr, question1, question2), nil
	})

	ragSystem, err := rag.Get(exportCollectionName,
		rag.WithDB(db),
		rag.WithDisableEmbedCollectionInfo(true),
		rag.WithLazyLoadEmbeddingClient(true),
		rag.WithEmbeddingClient(mockEmbedding),
		rag.WithAIService(mockAICallback),
	)
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
	defer rag.DeleteRAG(db, exportCollectionName)

	// 1. 添加一个知识库条目，但不开启 EnableDocumentQuestionIndex
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  ragSystem.GetKnowledgeBaseID(),
		KnowledgeTitle:   "测试知识",
		KnowledgeType:    "Test",
		KnowledgeDetails: knowledgeDetails,
		ImportanceScore:  5,
	}
	err = ragSystem.AddKnowledgeEntry(entry, rag.WithEnableDocumentQuestionIndex(false))
	assert.NoError(t, err)

	// 验证此刻只有1个文档（主文档），且没有问题索引
	var documents []schema.VectorStoreDocument
	db.Model(&schema.VectorStoreDocument{}).Find(&documents)
	assert.Equal(t, 1, len(documents))

	// 确保主文档存在
	var doc schema.VectorStoreDocument
	db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ? AND metadata LIKE ?", ragSystem.VectorStore.GetCollectionInfo().ID, "%"+entry.HiddenIndex+"%").First(&doc)
	assert.NotZero(t, doc.ID)

	// 2. 调用 GenerateQuestionIndex 方法
	err = ragSystem.GenerateQuestionIndex(rag.WithAIService(mockAICallback))
	assert.NoError(t, err)

	// 3. 验证现在应该有 3 个文档（1个主文档 + 2个问题索引）
	db.Model(&schema.VectorStoreDocument{}).Find(&documents)
	assert.Equal(t, 3, len(documents))

	// 验证问题索引是否存在
	question1Res, err := ragSystem.Query(question1, 1, 0.5)
	assert.NoError(t, err)
	assert.NotEmpty(t, question1Res)
	assert.True(t, question1Res[0].Document.IsQuestionIndex())
	assert.Equal(t, entry.HiddenIndex, question1Res[0].Document.Metadata[schema.META_Data_UUID])

	// 4. 再次调用 GenerateQuestionIndex，验证不会重复生成
	err = ragSystem.GenerateQuestionIndex(rag.WithAIService(mockAICallback))
	assert.NoError(t, err)

	db.Model(&schema.VectorStoreDocument{}).Find(&documents)
	assert.Equal(t, 3, len(documents))
}

func TestMUSTPASS_RAGSystem_GenerateQuestionIndex_With_Multiple_Inputs(t *testing.T) {
	db, err := rag.NewTemporaryRAGDB()
	assert.NoError(t, err)
	exportCollectionName := "test_generate_question_index_multi_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	input1 := mockEmbedding.GenerateRandomText(20)
	input2 := mockEmbedding.GenerateRandomText(20)
	input3 := mockEmbedding.GenerateRandomText(20)
	input4 := mockEmbedding.GenerateRandomText(20)
	input5 := mockEmbedding.GenerateRandomText(20)

	question1 := mockEmbedding.GenerateRandomText(5)
	question2 := mockEmbedding.GenerateRandomText(5)
	question3 := mockEmbedding.GenerateRandomText(5)
	question4 := mockEmbedding.GenerateRandomText(5)
	question5 := mockEmbedding.GenerateRandomText(5)

	mockAICallback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		// 简单的模拟返回，根据 prompt 内容或者直接返回固定结构
		// 这里我们直接返回两个问题，分别对应第1行和第2行
		rspStr := `{
			"@action": "object",
			"question_list": [
				{"question": "%s","answer_location": {"start_line": 1, "end_line": 3}},
				{"question": "%s","answer_location": {"start_line": 2, "end_line": 3}},
				{"question": "%s","answer_location": {"start_line": 3, "end_line": 5}},
				{"question": "%s","answer_location": {"start_line": 4, "end_line": 5}},
				{"question": "%s","answer_location": {"start_line": 5, "end_line": 5}}
			]
		}`
		return fmt.Sprintf(rspStr, question1, question2, question3, question4, question5), nil
	})

	ragSystem, err := rag.Get(exportCollectionName,
		rag.WithDB(db),
		rag.WithDisableEmbedCollectionInfo(true),
		rag.WithLazyLoadEmbeddingClient(true),
		rag.WithEmbeddingClient(mockEmbedding),
		rag.WithAIService(mockAICallback),
	)
	assert.NoError(t, err)
	defer rag.DeleteRAG(db, exportCollectionName)

	err = ragSystem.AddKnowledge(input1, rag.WithEnableDocumentQuestionIndex(false))
	assert.NoError(t, err)

	err = ragSystem.AddKnowledge(input2, rag.WithEnableDocumentQuestionIndex(false))
	assert.NoError(t, err)

	err = ragSystem.AddKnowledge(input3, rag.WithEnableDocumentQuestionIndex(false))
	assert.NoError(t, err)

	err = ragSystem.AddKnowledge(input4, rag.WithEnableDocumentQuestionIndex(false))
	assert.NoError(t, err)

	err = ragSystem.AddKnowledge(input5, rag.WithEnableDocumentQuestionIndex(false))
	assert.NoError(t, err)

	// 调用 GenerateQuestionIndex，它应该会批量处理这两个条目
	err = ragSystem.GenerateQuestionIndex(rag.WithAIService(mockAICallback))
	assert.NoError(t, err)

	// 验证生成的问题索引是否正确关联到了对应的知识条目
	// 1. 获取所有知识条目，建立 内容 -> UUID 的映射
	var entries []*schema.KnowledgeBaseEntry
	err = db.Model(&schema.KnowledgeBaseEntry{}).Find(&entries).Error
	assert.NoError(t, err)

	contentToUUID := make(map[string]string)
	for _, e := range entries {
		contentToUUID[e.KnowledgeDetails] = e.HiddenIndex
	}

	// 2. 获取所有向量文档，筛选出问题索引
	docs, err := ragSystem.VectorStore.List()
	assert.NoError(t, err)

	// 问题 -> 关联的知识条目UUID列表
	actualMapping := make(map[string][]string)

	for _, doc := range docs {
		if doc.Metadata[schema.META_QUESTION_INDEX] == true {
			q := doc.Content
			uuidStr := doc.Metadata[schema.META_Data_UUID].(string)
			actualMapping[q] = append(actualMapping[q], uuidStr)
		}
	}

	// 3. 定义预期的映射关系
	// 根据 mockAICallback 的定义：
	// question1 (1-3) -> input1, input2, input3
	// question2 (2-3) -> input2, input3
	// question3 (3-5) -> input3, input4, input5
	// question4 (4-5) -> input4, input5
	// question5 (5-5) -> input5
	expectedMapping := map[string][]string{
		question1: {contentToUUID[input1], contentToUUID[input2], contentToUUID[input3]},
		question2: {contentToUUID[input2], contentToUUID[input3]},
		question3: {contentToUUID[input3], contentToUUID[input4], contentToUUID[input5]},
		question4: {contentToUUID[input4], contentToUUID[input5]},
		question5: {contentToUUID[input5]},
	}

	// 4. 验证
	for q, expectedUUIDs := range expectedMapping {
		actualUUIDs, ok := actualMapping[q]
		assert.True(t, ok, "Question %s not found in actual mapping", q)
		assert.ElementsMatch(t, expectedUUIDs, actualUUIDs, "Question %s mapping mismatch", q)
	}
}
