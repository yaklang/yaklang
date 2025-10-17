package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// setupTestDB 设置测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatalf("创建临时数据库失败: %v", err)
	}

	// 自动迁移数据库表结构
	err = db.AutoMigrate(&schema.VectorStoreCollection{}).Error
	require.NoError(t, err, "数据库表结构迁移失败")

	return db
}

// createTestCollection 创建测试用的 VectorStoreCollection
func createTestCollection(t *testing.T, db *gorm.DB, name string) *schema.VectorStoreCollection {
	collection := &schema.VectorStoreCollection{
		Name:             name,
		Description:      "测试用的向量存储集合 - " + name,
		ModelName:        "text-embedding-ada-002",
		Dimension:        1536,
		M:                16,
		Ml:               0.25,
		EfSearch:         20,
		EfConstruct:      200,
		DistanceFuncType: "cosine",
	}

	err := db.Create(collection).Error
	require.NoError(t, err, "创建测试数据失败")

	return collection
}

// TestQueryRAGCollectionByName 测试根据名称查询 RAG 集合
func TestQueryRAGCollectionByName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 创建测试数据
	testName := "test_collection_by_name"
	originalCollection := createTestCollection(t, db, testName)

	// 测试查询存在的集合
	t.Run("查询存在的集合", func(t *testing.T) {
		collection, err := QueryRAGCollectionByName(db, testName)
		assert.NoError(t, err)
		assert.NotNil(t, collection)
		assert.Equal(t, testName, collection.Name)
		assert.Equal(t, originalCollection.Description, collection.Description)
		assert.Equal(t, originalCollection.ModelName, collection.ModelName)
		assert.Equal(t, originalCollection.Dimension, collection.Dimension)
	})

	// 测试查询不存在的集合
	t.Run("查询不存在的集合", func(t *testing.T) {
		collection, err := QueryRAGCollectionByName(db, "不存在的集合名")
		assert.Error(t, err)
		assert.Nil(t, collection)
	})
}

// TestQueryRAGCollectionByID 测试根据ID查询 RAG 集合
func TestQueryRAGCollectionByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 创建测试数据
	originalCollection := createTestCollection(t, db, "test_collection_by_id")

	// 测试查询存在的集合
	t.Run("查询存在的集合", func(t *testing.T) {
		collection, err := QueryRAGCollectionByID(db, int64(originalCollection.ID))
		assert.NoError(t, err)
		assert.NotNil(t, collection)
		assert.Equal(t, originalCollection.ID, collection.ID)
		assert.Equal(t, originalCollection.Name, collection.Name)
		assert.Equal(t, originalCollection.Description, collection.Description)
	})

	// 测试查询不存在的集合
	t.Run("查询不存在的集合", func(t *testing.T) {
		collection, err := QueryRAGCollectionByID(db, 99999)
		assert.Error(t, err)
		assert.Nil(t, collection)
	})
}

// TestGetAllRAGCollectionNames 测试获取所有 RAG 集合名称
func TestGetAllRAGCollectionNames(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 创建测试数据
	createTestCollection(t, db, "collection_1")
	createTestCollection(t, db, "collection_2")
	createTestCollection(t, db, "collection_3")

	// 测试获取所有集合名称
	names, err := GetAllRAGCollectionNames(db)
	assert.NoError(t, err)
	assert.Len(t, names, 3)
	assert.Contains(t, names, "collection_1")
	assert.Contains(t, names, "collection_2")
	assert.Contains(t, names, "collection_3")
}

// TestGetAllRAGCollectionInfos 测试获取所有 RAG 集合信息
func TestGetAllRAGCollectionInfos(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 创建测试数据
	collection1 := createTestCollection(t, db, "info_collection_1")
	collection2 := createTestCollection(t, db, "info_collection_2")

	// 测试获取所有集合信息
	collections, err := GetAllRAGCollectionInfos(db)
	assert.NoError(t, err)
	assert.Len(t, collections, 2)

	// 验证返回的数据
	names := make(map[string]bool)
	for _, collection := range collections {
		names[collection.Name] = true
		// 验证核心字段已填充
		assert.NotEmpty(t, collection.Name)
		assert.NotEmpty(t, collection.ModelName)
		assert.Greater(t, collection.Dimension, 0)
	}

	assert.True(t, names[collection1.Name])
	assert.True(t, names[collection2.Name])
}

// TestSelectRAGCollectionCoreFields 测试核心字段选择功能
func TestSelectRAGCollectionCoreFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 创建测试数据
	createTestCollection(t, db, "core_fields_test")

	// 直接测试 selectRAGCollectionCoreFields 函数
	var collections []schema.VectorStoreCollection
	err := selectRAGCollectionCoreFields(db).Find(&collections).Error
	assert.NoError(t, err)
	assert.Len(t, collections, 1)

	// 验证返回的记录包含预期的核心字段
	collection := collections[0]
	assert.NotZero(t, collection.ID)
	assert.NotEmpty(t, collection.Name)
	assert.NotEmpty(t, collection.ModelName)
	assert.Greater(t, collection.Dimension, 0)
}

// TestEmptyDatabase 测试空数据库的情况
func TestEmptyDatabase(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 测试在空数据库中查询
	t.Run("空数据库查询集合名称", func(t *testing.T) {
		names, err := GetAllRAGCollectionNames(db)
		assert.NoError(t, err)
		assert.Len(t, names, 0)
	})

	t.Run("空数据库查询集合信息", func(t *testing.T) {
		collections, err := GetAllRAGCollectionInfos(db)
		assert.NoError(t, err)
		assert.Len(t, collections, 0)
	})

	t.Run("空数据库根据名称查询", func(t *testing.T) {
		collection, err := QueryRAGCollectionByName(db, "不存在的集合")
		assert.Error(t, err)
		assert.Nil(t, collection)
	})
}
