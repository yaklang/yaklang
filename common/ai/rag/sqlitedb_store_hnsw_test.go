package rag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 测试 SQLiteVectorStore
func TestMUSTPASS_SQLiteVectorStoreHNSW(t *testing.T) {
	mockEmbed := &MockEmbedder{}

	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	// 创建 SQLite 向量存储
	store, err := NewSQLiteVectorStoreHNSW("test_collection", "test", "Qwen3-Embedding-0.6B-Q4_K_M", 1024, mockEmbed, db)
	assert.NoError(t, err)
	defer store.Remove()

}
