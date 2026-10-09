package aimemory

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func memoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "memory.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}).Error)
	return db
}

func TestSearchExistingRAGAndIsolation(t *testing.T) {
	db := memoryTestDB(t)
	oldMock := vectorstore.IsMockMode
	vectorstore.IsMockMode = true
	defer func() { vectorstore.IsMockMode = oldMock }()
	store, err := vectorstore.CreateCollection(db, "ai-memory-team-a", "", vectorstore.WithDisableEmbedCollectionInfo(true), vectorstore.WithModelDimension(1024))
	require.NoError(t, err)
	past := time.Now().Add(-time.Hour)
	for _, row := range []*schema.AIMemoryEntity{
		{MemoryID: "a", SessionID: "team-a", Content: "report preferred format"},
		{MemoryID: "other", SessionID: "team-b", Content: "report foreign memory"},
		{MemoryID: "expired", SessionID: "team-a", Content: "report expired memory", ExpiresAt: &past},
		{MemoryID: "unindexed", SessionID: "team-a", Content: "report unindexed body"},
	} {
		require.NoError(t, db.Create(row).Error)
	}
	deleted := &schema.AIMemoryEntity{MemoryID: "deleted", SessionID: "team-a", Content: "report deleted memory"}
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)
	for _, id := range []string{"a", "a", "other", "expired", "deleted", "orphan"} {
		require.NoError(t, store.Add(&vectorstore.Document{ID: id + time.Now().Format("150405.000000000"), Content: "report format", Metadata: schema.MetadataMap{"memory_id": id}}))
	}
	require.NoError(t, yakit.EnsureVectorStoreDocumentFTS5(db))
	require.True(t, db.HasTable(yakit.VectorDocumentVTableName()))
	for _, mode := range []string{"bm25", "vector", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			rows, err := Search("report", WithDatabase(db), WithMemoryNamespace("team-a"), WithMemorySearchMode(mode))
			require.NoError(t, err)
			seen := map[string]bool{}
			for _, row := range rows {
				require.False(t, seen[row.MemoryID])
				seen[row.MemoryID] = true
				require.Contains(t, []string{"a", "unindexed"}, row.MemoryID)
			}
			require.True(t, seen["a"])
			if mode != "vector" {
				require.True(t, seen["unindexed"])
			}
		})
	}
	rows, err := Search("report", WithDatabase(db), WithMemoryNamespace("team-b"), WithMemorySearchMode("bm25"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "other", rows[0].MemoryID)
	rows, err = Search("report", WithDatabase(db), WithMemoryNamespace("unknown"))
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = yakit.GetRAGCollectionInfoByName(db, "ai-memory-unknown")
	require.Error(t, err, "search must not create a memory set")
	require.False(t, db.HasTable(&schema.KnowledgeBaseInfo{}), "search must not initialize a knowledge base")
}

func TestSearchBudgetFallbackAndCancellation(t *testing.T) {
	db := memoryTestDB(t)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: "long", SessionID: "default", Content: strings.Repeat("中文 report ", 1000)}).Error)
	rows, err := Search("report", WithDatabase(db), WithMemoryTokenLimit(128))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.LessOrEqual(t, ytoken.CalcTokenCount(rows[0].Content), 128)
	_, err = Search("report", WithDatabase(db), WithMemorySearchMode("vector"))
	require.ErrorContains(t, err, "vector index is unavailable")
	for _, opt := range []Option{WithMemoryLimit(21), WithMemoryTokenLimit(63), WithMemorySearchMode("wrong")} {
		_, err := Search("report", WithDatabase(db), opt)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Search("report", WithDatabase(db), WithContext(ctx))
	require.ErrorIs(t, err, context.Canceled)
}

func TestSearchProjectDatabaseIsolation(t *testing.T) {
	for _, marker := range []string{"first", "second"} {
		db := memoryTestDB(t)
		require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: "same-id", SessionID: "same-namespace", Content: "report " + marker}).Error)
		rows, err := Search("report", WithDatabase(db), WithMemoryNamespace("same-namespace"), WithMemorySearchMode("bm25"))
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "report "+marker, rows[0].Content)
	}
}
