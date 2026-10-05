package aimem

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMemoryEntityFromCompressionScoresAndCompatibility(t *testing.T) {
	candidate := map[string]any{
		"content": "用户要求报告列出实际验证范围。", "tags": []string{"报告"},
		"potential_questions": []string{"报告需要哪些验证信息？"},
		"scores": map[string]any{"connectivity": 0.1, "origin": 0.2, "relevance": 0.3,
			"emotion": 0.4, "perference": 0.5, "actionability": 0.6, "temporality": 0.9},
		"title": "ignored old field", "extra": "compatible extension",
	}
	entity, err := MemoryEntityFromCompression(candidate)
	require.NoError(t, err)
	require.NotEmpty(t, entity.Id)
	require.False(t, entity.CreatedAt.IsZero())
	require.Equal(t, candidate["content"], entity.Content)
	require.Equal(t, []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.9}, entity.CorePactVector)
	require.Equal(t, 0.5, entity.P_Score)
	require.Equal(t, 0.9, entity.T_Score)
	for _, score := range []any{nil, -0.1, 1.1, "0.8"} {
		candidate["scores"].(map[string]any)["temporality"] = score
		_, err := MemoryEntityFromCompression(candidate)
		require.Error(t, err, "invalid scores must not be invented")
	}
	delete(candidate["scores"].(map[string]any), "temporality")
	_, err = MemoryEntityFromCompression(candidate)
	require.Error(t, err)
}

func TestMemoryStoreWithoutInvokerKeepsTTLAndIndexes(t *testing.T) {
	db, err := getTestDatabase(t)
	require.NoError(t, err)
	store, err := NewMemoryStore(uuid.NewString(), WithDatabase(db),
		WithRAGOptions(rag.WithEmbeddingClient(vectorstore.NewMockEmbedder(func(string) ([]float32, error) {
			return []float32{1, 0}, nil
		}))))
	require.NoError(t, err)
	defer store.Close()
	entity := &aicommon.MemoryEntity{Id: uuid.NewString(), Content: "下周复核短期项目约束。", T_Score: 0.2,
		PotentialQuestions: []string{"下周复核哪些约束？"}, CorePactVector: []float32{0.2, 1, 0.8, 0, 0, 0.7, 0.2}}
	require.NoError(t, store.SaveMemoryEntities(entity))
	var row schema.AIMemoryEntity
	require.NoError(t, db.Where("memory_id = ? AND session_id = ?", entity.Id, store.sessionID).First(&row).Error)
	require.NotNil(t, row.ExpiresAt)
	require.WithinDuration(t, time.Now().Add(7*24*time.Hour), *row.ExpiresAt, 5*time.Second)
	results, err := store.hnswBackend.Search(entity.CorePactVector, 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, entity.Id, results[0].Entity.Id)
	var documents []schema.VectorStoreDocument
	require.NoError(t, db.Where("document_id = ?", row.QuestionHashID(row.PotentialQuestions[0])).Find(&documents).Error)
	require.Len(t, documents, 1)
}
