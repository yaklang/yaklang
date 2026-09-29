package knowledgebase

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// V1 had no entry identity field. It must remain readable; V2 fixes the
// identity loss for newly exported files without shifting V1 field boundaries.
func TestKnowledgeBaseImportLegacyV1(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()
	var data bytes.Buffer
	data.WriteString(knowledgeBaseMagicV1)
	for _, value := range []string{"legacy-v1", "legacy description", "test"} {
		require.NoError(t, pbWriteBytes(&data, []byte(value)))
	}
	require.NoError(t, pbWriteVarint(&data, 1))
	require.NoError(t, writeEntryToBinary(&data, &schema.KnowledgeBaseEntry{
		KnowledgeTitle: "legacy entry", KnowledgeDetails: "legacy contents", ImportanceScore: 5,
	}))
	require.NoError(t, pbWriteBytes(&data, nil)) // no vector payload
	require.NoError(t, pbWriteBytes(&data, []byte("extra")))
	require.NoError(t, ImportKnowledgeBase(context.Background(), db, &data, &ImportKnowledgeBaseOptions{}))
	info, err := yakit.GetKnowledgeBaseByName(db, "legacy-v1")
	require.NoError(t, err)
	var entries []schema.KnowledgeBaseEntry
	require.NoError(t, db.Where("knowledge_base_id = ?", info.ID).Find(&entries).Error)
	require.Len(t, entries, 1)
	require.Equal(t, "legacy contents", entries[0].KnowledgeDetails)
	require.Equal(t, 5, entries[0].ImportanceScore)
	require.NotEmpty(t, entries[0].HiddenIndex, "legacy imports still receive a generated identity")
}

// A restored knowledge entry and its vector/question documents must still refer
// to the same identity. A second copy in the same DB must remap both together.
func TestKnowledgeBaseExportIdentityReferences(t *testing.T) {
	source, err := createTempTestDatabase()
	require.NoError(t, err)
	defer source.Close()
	kb, err := NewKnowledgeBase(source, "identity-source", "", "test",
		vectorstore.WithEmbeddingModel("mock-model"), vectorstore.WithModelDimension(3),
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)))
	require.NoError(t, err)
	entry := &schema.KnowledgeBaseEntry{KnowledgeBaseID: kb.GetID(), HiddenIndex: "stable-entry-id", KnowledgeTitle: "identity test", KnowledgeDetails: "preserve vector references", PotentialQuestions: schema.StringArray{"where is this entry?"}}
	require.NoError(t, source.Create(entry).Error)
	require.NoError(t, kb.addEntryToVectorIndex(entry))
	require.NoError(t, kb.addQuestionToVectorIndex(entry, false))
	reader, err := ExportKnowledgeBase(context.Background(), source, &ExportKnowledgeBaseOptions{KnowledgeBaseId: kb.GetID()})
	require.NoError(t, err)
	payload, err := io.ReadAll(reader)
	require.NoError(t, err)
	for _, copyInSource := range []bool{false, true} {
		name := "restored"
		dest := source
		if !copyInSource {
			dest, err = createTempTestDatabase()
			require.NoError(t, err)
			defer dest.Close()
		} else {
			name = "second-copy"
		}
		require.NoError(t, ImportKnowledgeBase(context.Background(), dest, bytes.NewReader(payload), &ImportKnowledgeBaseOptions{NewKnowledgeBaseName: name}))
		restored, err := LoadKnowledgeBase(dest, name, vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)))
		require.NoError(t, err)
		var entries []schema.KnowledgeBaseEntry
		require.NoError(t, dest.Where("knowledge_base_id = ?", restored.GetID()).Find(&entries).Error)
		require.Len(t, entries, 1)
		if copyInSource {
			require.NotEqual(t, entry.HiddenIndex, entries[0].HiddenIndex)
		} else {
			require.Equal(t, entry.HiddenIndex, entries[0].HiddenIndex)
		}
		docs, err := restored.vectorStore.List()
		require.NoError(t, err)
		require.Len(t, docs, 2)
		for _, doc := range docs {
			require.True(t, doc.ID == entries[0].HiddenIndex || strings.HasPrefix(doc.ID, entries[0].HiddenIndex+"_question_"), doc.ID)
			require.Equal(t, entries[0].HiddenIndex, doc.Metadata[schema.META_Data_UUID])
			require.True(t, restored.vectorStore.Has(doc.ID), "the HNSW graph must use the restored ID")
		}
	}
}

func TestKnowledgeBaseImportTruncatedIdentity(t *testing.T) {
	db, err := createTempTestDatabase()
	require.NoError(t, err)
	defer db.Close()
	var data bytes.Buffer
	data.WriteString(knowledgeBaseMagicV2)
	for _, value := range []string{"truncated", "", "test"} {
		require.NoError(t, pbWriteBytes(&data, []byte(value)))
	}
	require.NoError(t, pbWriteVarint(&data, 1))
	require.NoError(t, writeEntryToBinary(&data, &schema.KnowledgeBaseEntry{KnowledgeTitle: "entry"}))
	err = ImportKnowledgeBase(context.Background(), db, &data, &ImportKnowledgeBaseOptions{})
	require.ErrorContains(t, err, "read knowledge entry identity")
	var count int
	require.NoError(t, db.Model(&schema.KnowledgeBaseEntry{}).Count(&count).Error)
	require.Zero(t, count, "a truncated identity must not silently become a new UUID")
}
