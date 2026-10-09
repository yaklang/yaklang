package aimemory

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestAmendMemoryLifecycleAndIsolation(t *testing.T) {
	previousMock := vectorstore.IsMockMode
	vectorstore.IsMockMode = true
	defer func() { vectorstore.IsMockMode = previousMock }()
	db := memoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{}).Error)
	opts := []Option{WithDatabase(db), WithMemoryNamespace("team-a")}
	_, err := AmendMemory("add", "", "ambiguous tag", []string{"a,b"}, opts...)
	require.ErrorContains(t, err, "no comma")
	row, err := AmendMemory("add", "", "preferred report markdown", []string{"preference"}, opts...)
	require.NoError(t, err)
	require.NotEmpty(t, row.MemoryID)
	require.Equal(t, "team-a", row.SessionID)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: "foreign", SessionID: "team-b", Content: "foreign report"}).Error)
	_, err = AmendMemory("delete", "foreign", "", nil, opts...)
	require.Error(t, err)
	_, err = AmendMemory("change", row.MemoryID, "", nil, opts...)
	require.Error(t, err)
	hits, err := Query("markdown", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Contains(t, hits[0].Dump(), row.MemoryID)
	require.Contains(t, hits[0].Dump(), "preferred report markdown")
	replacement, err := Amend("change", row.MemoryID, "preferred report JSON", []string{"new"}, opts...)
	require.NoError(t, err)
	require.NotEqual(t, row.MemoryID, replacement.MemoryID)
	hits, err = Query("markdown", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Empty(t, hits)
	hits, err = Query("JSON", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_memory_replacement BEFORE INSERT ON ai_memory_entities_v1 WHEN NEW.content = 'reject replacement' BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`).Error)
	_, err = AmendMemory("change", replacement.MemoryID, "reject replacement", nil, opts...)
	require.Error(t, err)
	var restored schema.AIMemoryEntity
	require.NoError(t, db.Where("memory_id = ?", replacement.MemoryID).First(&restored).Error)
	require.Equal(t, replacement.Content, restored.Content)
	_, err = AmendMemory("delete", replacement.MemoryID, "", nil, opts...)
	require.NoError(t, err)
	hits, err = Query("report", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Empty(t, hits)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = AmendMemory("add", "", "cancelled", nil, append(opts, WithContext(ctx))...)
	require.ErrorIs(t, err, context.Canceled)
}
