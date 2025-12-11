package vectorstore

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMUSTPASS_ExportHNSW(t *testing.T) {
	// 生成测试数据
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	collectionName := utils.RandStringBytes(10)
	embedding := NewDefaultMockEmbedding()
	store, err := NewSQLiteVectorStoreHNSW(collectionName, "test", "Qwen3-Embedding-0.6B-Q4_K_M", 1024, embedding, db)
	assert.NoError(t, err)
	defer store.Remove()
	keyflag := utils.RandStringBytes(10)
	store.AddWithOptions(keyflag, "test")

	// 查询数据库中的 collection 信息
	var collection schema.VectorStoreCollection
	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).First(&collection).Error
	assert.NoError(t, err)
	assert.Equal(t, collectionName, collection.Name)

	// 验证导出的 HNSW 二进制数据中节点Code应该是 KeyFlag
	hnswBinary, err := hnsw.LoadBinary[string](bytes.NewReader(collection.GraphBinary))
	assert.NoError(t, err)
	assert.NotNil(t, hnswBinary)
	code := hnswBinary.OffsetToKey[1].Code.(string)
	assert.Equal(t, code, keyflag)

	// 修改 HNSW 二进制数据中节点Code为随机值
	hnswBinary.OffsetToKey[1].Code = utils.RandStringBytes(10)
	binaryReader, err := hnswBinary.ToBinary(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, binaryReader)

	// 更新数据库中的 GraphBinary
	binary, err := io.ReadAll(binaryReader)
	assert.NoError(t, err)
	assert.NotNil(t, binary)
	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Update("graph_binary", binary).Error
	assert.NoError(t, err)

	// 清理缓存
	GraphWrapperManager.ClearCache()

	// 验证使用错误的 code 导入
	_, err = LoadSQLiteVectorStoreHNSW(db, collectionName, WithEmbeddingClient(embedding))
	assert.Contains(t, err.Error(), "record not found")

	// 验证使用Key作为code导入
	store, err = LoadSQLiteVectorStoreHNSW(db, collectionName, WithKeyAsUID(true), WithEmbeddingClient(embedding))
	assert.NoError(t, err)
	assert.NotNil(t, store)

	// 测试尝试重建HNSW索引
	store, err = LoadSQLiteVectorStoreHNSW(db, collectionName, WithTryRebuildHNSWIndex(true), WithEmbeddingClient(embedding))
	assert.NoError(t, err)
	assert.NotNil(t, store)
}
