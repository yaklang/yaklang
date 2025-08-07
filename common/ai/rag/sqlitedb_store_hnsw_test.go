package rag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
)

// 测试 SQLiteVectorStore
func TestSQLiteVectorStoreHNSW(t *testing.T) {
	mockEmbed := &MockEmbedder{}

	db := consts.GetGormProfileDatabase()
	// 创建 SQLite 向量存储
	store, err := NewSQLiteVectorStore(db, "test_collection", "Qwen3-Embedding-0.6B-Q4_K_M", 1024, mockEmbed)
	assert.NoError(t, err)
	defer store.Remove()

}
