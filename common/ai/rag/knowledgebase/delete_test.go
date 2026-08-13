package knowledgebase

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_DeleteKnowledgeEntryRemovesQuestionIndexes(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, AutoMigrate(db))

	store, err := vectorstore.NewSQLiteVectorStoreHNSW("question-delete", "", "mock", 3, vectorstore.NewMockEmbedder(func(string) ([]float32, error) {
		return []float32{1, 0, 0}, nil
	}), db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Remove() })
	kb, err := NewKnowledgeBaseWithVectorStore(db, "question-delete", "", "test", nil, store)
	require.NoError(t, err)

	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    kb.GetID(),
		KnowledgeTitle:     "entry",
		KnowledgeType:      "test",
		PotentialQuestions: schema.StringArray{"question one", "question two"},
	}
	require.NoError(t, kb.AddKnowledgeEntryQuestion(entry, false))
	require.NoError(t, kb.DeleteKnowledgeEntry(entry.HiddenIndex))

	var entryCount, documentCount int64
	require.NoError(t, db.Unscoped().Model(&schema.KnowledgeBaseEntry{}).Where("hidden_index = ?", entry.HiddenIndex).Count(&entryCount).Error)
	require.NoError(t, db.Unscoped().Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", store.GetCollectionInfo().ID).Count(&documentCount).Error)
	require.Zero(t, entryCount)
	require.Zero(t, documentCount)
	require.False(t, store.Has(entry.HiddenIndex))
}
