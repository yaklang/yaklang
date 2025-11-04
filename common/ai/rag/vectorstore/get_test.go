package vectorstore

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func createTempTestDatabase() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	return db, nil

}

func TestMUSTPASS_LoadCollectionWithInvalidGraphBinary(t *testing.T) {
	// 创建临时测试数据库
	testDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}

	collectionName := utils.RandStringBytes(10)

	// 先创建一个已存在的集合
	_, err = GetCollection(testDB, collectionName, WithEmbeddingClient(NewDefaultMockEmbedding()))
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	// 修改集合的 HNSW Graph Binary 为无效数据
	err = testDB.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Update("graph_binary", []byte{0x00, 0x01, 0x02, 0x03}).Error
	if err != nil {
		t.Fatalf("failed to update collection graph binary: %v", err)
	}

	// 验证集合已经存在
	assert.True(t, HasCollection(testDB, collectionName), "collection should exist")

	_, err = GetCollection(testDB, collectionName, WithEmbeddingClient(NewDefaultMockEmbedding()))
	assert.Error(t, err, "should return error when loading collection with invalid graph binary")
}

// TestGet_RecordNotFoundError 测试确保 gorm.IsRecordNotFoundError 能正确识别
func TestMUSTPASS_RecordNotFoundError(t *testing.T) {
	// 创建临时测试数据库
	testDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}

	collectionName := utils.RandStringBytes(10)

	// 验证集合不存在
	assert.False(t, HasCollection(testDB, collectionName), "collection should not exist")

	// 尝试直接加载不存在的集合
	collectionMg, err := GetCollection(testDB, collectionName, WithEmbeddingClient(NewDefaultMockEmbedding()))
	assert.NoError(t, err, "should create new collection when record not found")
	assert.True(t, HasCollection(testDB, collectionName), "collection should exist")
	assert.NotNil(t, collectionMg, "collection should not be nil")
}
