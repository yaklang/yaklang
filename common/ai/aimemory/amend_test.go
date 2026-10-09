package aimemory

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestAmendMemoryLifecycleAndIsolation(t *testing.T) {
	db := memoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{}).Error)
	opts := []Option{WithDatabase(db), WithMemoryNamespace("team-a")}
	row, err := AmendMemory("add", "", "preferred report markdown", []string{"preference"}, opts...)
	require.NoError(t, err)
	require.NotEmpty(t, row.MemoryID)
	require.Equal(t, "team-a", row.SessionID)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: "foreign", SessionID: "team-b", Content: "foreign report"}).Error)
	_, err = AmendMemory("delete", "foreign", "", nil, opts...)
	require.Error(t, err)
	_, err = AmendMemory("change", row.MemoryID, "", nil, opts...)
	require.Error(t, err)
	hits, err := SearchMemory("markdown", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	replacement, err := AmendMemory("change", row.MemoryID, "preferred report JSON", []string{"new"}, opts...)
	require.NoError(t, err)
	require.NotEqual(t, row.MemoryID, replacement.MemoryID)
	hits, err = SearchMemory("markdown", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Empty(t, hits)
	hits, err = SearchMemory("JSON", append(opts, WithMemorySearchMode("bm25"))...)
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
	hits, err = SearchMemory("report", append(opts, WithMemorySearchMode("bm25"))...)
	require.NoError(t, err)
	require.Empty(t, hits)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = AmendMemory("add", "", "cancelled", nil, append(opts, WithContext(ctx))...)
	require.ErrorIs(t, err, context.Canceled)
}
