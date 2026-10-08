package test

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestManageKnowledgeEvictsLiveGraph(t *testing.T) {
	db, runtime := resourceToolFixture(t)
	embedder := vectorstore.NewMockEmbedder(func(string) ([]float32, error) { return []float32{1, 0, 0}, nil })
	store, err := vectorstore.NewSQLiteVectorStoreHNSW(t.Name(), "", "mock", 3, embedder, db, vectorstore.WithDisableEmbedCollectionInfo(true))
	require.NoError(t, err)
	kb := &schema.KnowledgeBaseInfo{KnowledgeBaseName: t.Name(), KnowledgeBaseType: "knowledge"}
	require.NoError(t, db.Create(kb).Error)
	entry := &schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(kb.ID), KnowledgeTitle: "old", KnowledgeType: "note", KnowledgeDetails: "old"}
	require.NoError(t, db.Create(entry).Error)
	require.NoError(t, store.AddWithOptions(entry.HiddenIndex, "old"))
	require.NoError(t, store.AddWithOptions("survivor", "survivor"))
	require.True(t, store.Has(entry.HiddenIndex))
	invokeResourceTool(t, "manage_knowledge", map[string]any{"operator": "delete", "knowledge_base_id": kb.ID, "entry_id": entry.ID}, runtime)
	require.False(t, store.Has(entry.HiddenIndex))
	loaded, err := vectorstore.LoadCollection(db, t.Name(), vectorstore.WithEmbeddingClient(embedder))
	require.NoError(t, err)
	t.Cleanup(func() { _ = loaded.Remove() })
	require.False(t, loaded.Has(entry.HiddenIndex))
	require.True(t, loaded.Has("survivor"))
}
