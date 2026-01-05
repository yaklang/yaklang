package ragtests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	_ "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_RAGSystem_AddKnowledgeEntryQuestion(t *testing.T) {
	db, err := rag.NewTemporaryRAGDB()
	assert.NoError(t, err)
	exportCollectionName := "test_add_knowledge_entry_question_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	question1 := mockEmbedding.GenerateRandomText(5)
	question2 := mockEmbedding.GenerateRandomText(5)
	question3 := mockEmbedding.GenerateRandomText(5)

	knowledgeDetails := mockEmbedding.GenerateRandomText(10)

	mockAICallback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		rspStr := `{
			"@action": "object",
			"question_list": [
	{"question": "%s","answer_location": {"start_line": 1, "end_line": 2}},
				{"question": "%s","answer_location": {"start_line": 2, "end_line": 3}},
				{"question": "%s","answer_location": {"start_line": 3, "end_line": 4}}
			]
		}`
		return fmt.Sprintf(rspStr, question1, question2, question3), nil
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

	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  ragSystem.GetKnowledgeBaseID(),
		KnowledgeTitle:   "Go语言入门",
		KnowledgeType:    "Programming",
		KnowledgeDetails: knowledgeDetails,
		Keywords:         schema.StringArray{"Go", "编程语言", "Google"},
		ImportanceScore:  8,
	}
	err = ragSystem.AddKnowledgeEntry(entry, rag.WithEnableDocumentQuestionIndex(true))
	assert.NoError(t, err)

	var documents []schema.VectorStoreDocument
	db.Model(&schema.VectorStoreDocument{}).Find(&documents)
	assert.NotEmpty(t, documents)

	assert.Equal(t, 4, len(documents))

	var knowledgeBaseEntries []schema.KnowledgeBaseEntry
	db.Model(&schema.KnowledgeBaseEntry{}).Find(&knowledgeBaseEntries)
	assert.NotEmpty(t, knowledgeBaseEntries)

	assert.Equal(t, 1, len(knowledgeBaseEntries))
	assert.Equal(t, entry.KnowledgeTitle, knowledgeBaseEntries[0].KnowledgeTitle)
	assert.Equal(t, entry.KnowledgeType, knowledgeBaseEntries[0].KnowledgeType)
	assert.Equal(t, entry.KnowledgeDetails, knowledgeBaseEntries[0].KnowledgeDetails)
	assert.Equal(t, entry.Keywords, knowledgeBaseEntries[0].Keywords)
	assert.Equal(t, entry.ImportanceScore, knowledgeBaseEntries[0].ImportanceScore)

	question1Res, err := ragSystem.Query(question1, 1, 0.5)
	if err != nil {
		t.Fatalf("failed to query question1: %v", err)
	}
	assert.NotNil(t, question1Res)
	assert.Equal(t, 1, len(question1Res))
	question1ResDocument := question1Res[0].Document
	assert.True(t, question1ResDocument.IsQuestionIndex())
	assert.Equal(t, entry.HiddenIndex, question1ResDocument.Metadata[schema.META_Data_UUID])
	assert.Equal(t, question1ResDocument.Content, question1)

	question2Res, err := ragSystem.Query(question2, 1, 0.5)
	if err != nil {
		t.Fatalf("failed to query question2: %v", err)
	}
	assert.NotNil(t, question2Res)
	assert.Equal(t, 1, len(question2Res))
	question2ResDocument := question2Res[0].Document
	assert.True(t, question2ResDocument.IsQuestionIndex())
	assert.Equal(t, entry.HiddenIndex, question2ResDocument.Metadata[schema.META_Data_UUID])
	assert.Equal(t, question2ResDocument.Content, question2)

	question3Res, err := ragSystem.Query(question3, 1, 0.5)
	if err != nil {
		t.Fatalf("failed to query question3: %v", err)
	}
	assert.NotNil(t, question3Res)
	assert.Equal(t, 1, len(question3Res))
	question3ResDocument := question3Res[0].Document
	assert.True(t, question3ResDocument.IsQuestionIndex())
	assert.Equal(t, entry.HiddenIndex, question3ResDocument.Metadata[schema.META_Data_UUID])
	assert.Equal(t, question3ResDocument.Content, question3)

	entryRes, err := ragSystem.Query(knowledgeDetails, 1, 0.5)
	if err != nil {
		t.Fatalf("failed to query knowledge details: %v", err)
	}
	assert.NotNil(t, entryRes)
	assert.Equal(t, 1, len(entryRes))
	entryResDocument := entryRes[0].Document
	assert.False(t, entryResDocument.IsQuestionIndex())
	assert.Equal(t, entry.HiddenIndex, entryResDocument.Metadata[schema.META_Data_UUID])
	assert.Contains(t, entryResDocument.Content, knowledgeDetails)

	resCh, err := knowledgebase.Query(db, question1, knowledgebase.WithCollectionName(exportCollectionName), knowledgebase.WithEmbeddingClient(mockEmbedding))
	if err != nil {
		t.Fatalf("failed to query question1: %v", err)
	}
	assert.NotNil(t, resCh)
	var results []*knowledgebase.SearchKnowledgebaseResult
	for res := range resCh {
		if res.Type == "result" {
			results = append(results, res)
		}
	}
	assert.Len(t, results, 1)
	data, ok := results[0].Data.(*schema.KnowledgeBaseEntry)
	if !ok {
		t.Fatalf("failed to convert data to KnowledgeBaseEntry: %v", results[0].Data)
	}
	assert.Equal(t, entry.KnowledgeTitle, data.KnowledgeTitle)
	assert.Equal(t, entry.KnowledgeType, data.KnowledgeType)
	assert.Equal(t, entry.KnowledgeDetails, data.KnowledgeDetails)
	assert.Equal(t, entry.Keywords, data.Keywords)
	assert.Equal(t, entry.ImportanceScore, data.ImportanceScore)
	assert.Equal(t, entry.HiddenIndex, data.HiddenIndex)
}
