package vectorstore

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
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

func TestMUSTPASS_SQLiteVectorStoreHNSW_AddPerformance(t *testing.T) {
	mockEmbed := NewMockEmbedder(func(text string) ([]float32, error) {
		return mockVector(), nil
	})
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	store, err := NewSQLiteVectorStoreHNSW("test_collection", "test", "Qwen3-Embedding-0.6B-Q4_K_M", 1024, mockEmbed, db)
	assert.NoError(t, err)
	defer store.Remove()

	startTime := time.Now()
	for i := 0; i < 100; i++ {
		store.Add(&Document{
			ID:        fmt.Sprintf("doc%d", i),
			Content:   fmt.Sprintf("Yaklang是一种安全研究编程语言%d", i),
			Metadata:  map[string]any{"source": "Yaklang介绍"},
			Embedding: mockVector(),
		})
	}
	elapsed := time.Since(startTime)
	// 10秒内完成
	assert.Less(t, elapsed, 10*time.Second)
	log.Infof("AddPerformance time: %v", elapsed)
}

func mockVector() []float32 {
	vectorDim := 1024
	vector := make([]float32, vectorDim)
	for j := 0; j < vectorDim; j++ {
		vector[j] = rand.Float32()
	}
	return vector
}
