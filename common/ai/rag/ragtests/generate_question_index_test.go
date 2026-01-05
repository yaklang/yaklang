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
