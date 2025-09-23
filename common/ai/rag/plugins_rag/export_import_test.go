package plugins_rag

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TestExportVectorData 测试向量数据的导出和导入功能的基础流程
// 测试场景：在空白数据库环境下的完整导出导入周期
func TestMUSTPASS_ExportVectorData(t *testing.T) {
	// 1. 准备测试环境：创建内存数据库并进行数据库迁移
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})

	// 2. 创建向量存储并添加测试数据
	store, err := rag.NewSQLiteVectorStoreHNSW(PLUGIN_RAG_COLLECTION_NAME, "test", "text-embedding-3-small", 1536, nil, db)
	if err != nil {
		t.Fatal(err)
	}

	// 添加第一个测试文档
	err = store.Add(rag.Document{
		ID:      "Yakit 权威使用指南 v1",
		Content: "Yakit 权威使用指南",
		Embedding: []float32{
			0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
		},
	})
	assert.NoError(t, err)

	// 添加第二个测试文档
	err = store.Add(rag.Document{
		ID:      "Yakit 权威使用指南 v2",
		Content: "Yakit 权威使用指南",
		Embedding: []float32{
			0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
		},
	})
	assert.NoError(t, err)

	// 3. 执行数据导出操作
	tmpFilePath := "/tmp/plugins_rag.zip"
	err = rag.ExportVectorData(db, PLUGIN_RAG_COLLECTION_NAME, tmpFilePath)
	assert.NoError(t, err)
	assert.FileExists(t, tmpFilePath) // 验证导出文件是否成功创建
	defer os.Remove(tmpFilePath)      // 清理临时文件

	// 4. 清空数据库以模拟全新环境
	// 使用Unscoped()确保彻底删除所有记录（包括软删除的记录）
	db.Unscoped().Delete(&schema.VectorStoreDocument{})
	db.Unscoped().Delete(&schema.VectorStoreCollection{})

	// 5. 验证数据库已完全清空
	var count int64

	// 验证文档表为空
	err = db.Model(&schema.VectorStoreDocument{}).Count(&count).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// 验证集合表为空
	err = db.Model(&schema.VectorStoreCollection{}).Count(&count).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// 额外验证：确保能够正常查询空的集合表
	_, err = yakit.GetAllRAGCollectionInfos(db)
	assert.NoError(t, err)

	// 6. 执行数据导入操作
	err = rag.ImportVectorData(db, tmpFilePath)
	if err != nil {
		t.Fatal(err)
	}

	// 7. 验证导入结果：确保数据完整恢复
	// 验证文档数量恢复为导出前的状态
	err = db.Model(&schema.VectorStoreDocument{}).Count(&count).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count) // 应该恢复2个文档

	// 验证集合数量恢复为导出前的状态
	err = db.Model(&schema.VectorStoreCollection{}).Count(&count).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count) // 应该恢复1个集合
}

// TestImportVectorData_ExistingCollection 测试当导入的VectorStoreCollection已经存在时，应该覆盖旧的Collection
func TestMUSTPASS_ImportVectorData_ExistingCollection(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})

	// 创建初始的Collection和Document
	store, err := rag.NewSQLiteVectorStoreHNSW(PLUGIN_RAG_COLLECTION_NAME, "test", "text-embedding-3-small", 1536, nil, db)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Add(rag.Document{
		ID:      "original_doc",
		Content: "Original document content",
		Embedding: []float32{
			0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
		},
	})
	assert.NoError(t, err)
	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", PLUGIN_RAG_COLLECTION_NAME).Update("description", "Original description").Error
	assert.NoError(t, err)
	// 导出数据
	tmpFilePath := "/tmp/plugins_rag_existing_collection.zip"
	err = rag.ExportVectorData(db, PLUGIN_RAG_COLLECTION_NAME, tmpFilePath)
	assert.NoError(t, err)
	assert.FileExists(t, tmpFilePath)
	defer os.Remove(tmpFilePath)

	// 获取原始Collection的ID
	var originalCollection schema.VectorStoreCollection
	err = db.Where("name = ?", PLUGIN_RAG_COLLECTION_NAME).First(&originalCollection).Error
	assert.NoError(t, err)
	originalCollectionID := originalCollection.ID

	// 修改Collection的描述来模拟数据更新
	err = db.Model(&originalCollection).Update("description", "Updated description").Error
	assert.NoError(t, err)

	// 导入数据（这应该覆盖现有的Collection）
	err = rag.ImportVectorData(db, tmpFilePath)
	assert.NoError(t, err)

	var collectionCount int64
	err = db.Model(&schema.VectorStoreCollection{}).Count(&collectionCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(1), collectionCount) // 仍然应该只有1个集合

	// 验证Collection是否被覆盖（ID应该保持相同，但内容被重置）
	var updatedCollection schema.VectorStoreCollection
	err = db.Where("name = ?", PLUGIN_RAG_COLLECTION_NAME).First(&updatedCollection).Error
	assert.NoError(t, err)

	// Collection的ID应该保持不变，但描述应该被重置为空或默认值
	assert.Equal(t, originalCollectionID, updatedCollection.ID)
	assert.Equal(t, "Original description", updatedCollection.Description) // 描述应该被重置
}

// TestImportVectorData_ExistingDocument 测试当导入的VectorStoreDocument已经存在时，应该覆盖同DocumentID的Document
func TestMUSTPASS_ImportVectorData_ExistingDocument(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})

	// 创建初始的Collection和Document
	store, err := rag.NewSQLiteVectorStoreHNSW(PLUGIN_RAG_COLLECTION_NAME, "test", "text-embedding-3-small", 1536, nil, db)
	if err != nil {
		t.Fatal(err)
	}

	originalEmbedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	err = store.Add(rag.Document{
		ID:        "shared_doc_id",
		Content:   "Original content",
		Embedding: originalEmbedding,
	})
	assert.NoError(t, err)

	// 导出数据
	tmpFilePath := "/tmp/plugins_rag_existing_document.zip"
	err = rag.ExportVectorData(db, PLUGIN_RAG_COLLECTION_NAME, tmpFilePath)
	assert.NoError(t, err)
	assert.FileExists(t, tmpFilePath)
	defer os.Remove(tmpFilePath)

	// 修改现有文档的内容
	var existingDoc schema.VectorStoreDocument
	err = db.Where("document_id = ?", "shared_doc_id").First(&existingDoc).Error
	assert.NoError(t, err)

	// 更新文档的嵌入向量和元数据
	modifiedEmbedding := schema.FloatArray{0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1}
	modifiedMetadata := schema.MetadataMap{"content": "Modified content", "version": "2.0"}

	err = db.Model(&existingDoc).Updates(map[string]interface{}{
		"embedding": modifiedEmbedding,
		"metadata":  modifiedMetadata,
	}).Error
	assert.NoError(t, err)

	// 添加另一个不同ID的文档
	err = store.Add(rag.Document{
		ID:        "different_doc_id",
		Content:   "Different document content",
		Embedding: []float32{1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2.0},
	})
	assert.NoError(t, err)

	// 验证导入前的状态
	var docCount int64
	err = db.Model(&schema.VectorStoreDocument{}).Count(&docCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(2), docCount) // 应该有2个文档

	// 验证修改后的文档内容
	var modifiedDoc schema.VectorStoreDocument
	err = db.Where("document_id = ?", "shared_doc_id").First(&modifiedDoc).Error
	assert.NoError(t, err)
	assert.Equal(t, modifiedEmbedding, modifiedDoc.Embedding)
	assert.Equal(t, modifiedMetadata, modifiedDoc.Metadata)

	// 导入数据（这应该覆盖相同DocumentID的文档）
	err = rag.ImportVectorData(db, tmpFilePath)
	assert.NoError(t, err)

	// 验证导入后的状态
	err = db.Model(&schema.VectorStoreDocument{}).Count(&docCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(2), docCount) // 导入后应该只有1个文档（因为覆盖了共享ID的文档，删除了不同ID的文档）

	// 验证文档是否被覆盖回原始内容
	var restoredDoc schema.VectorStoreDocument
	err = db.Where("document_id = ?", "shared_doc_id").First(&restoredDoc).Error
	assert.NoError(t, err)

	// 文档内容应该被恢复为导出时的原始内容
	assert.Equal(t, schema.FloatArray(originalEmbedding), restoredDoc.Embedding)
}

// TestImportVectorDataFullUpdate 测试全量导入
func TestMUSTPASS_ImportVectorDataFullUpdate(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})

	store, err := rag.NewSQLiteVectorStoreHNSW(PLUGIN_RAG_COLLECTION_NAME, "test", "text-embedding-3-small", 1536, nil, db)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Add(rag.Document{
		ID:      "Yakit 权威使用指南 v1",
		Content: "Yakit 权威使用指南",
		Embedding: []float32{
			0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
		},
	})
	assert.NoError(t, err)

	tmpFilePath := "/tmp/plugins_rag_full_update.zip"
	err = rag.ExportVectorData(db, PLUGIN_RAG_COLLECTION_NAME, tmpFilePath)
	assert.NoError(t, err)
	assert.FileExists(t, tmpFilePath)
	defer os.Remove(tmpFilePath)

	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", PLUGIN_RAG_COLLECTION_NAME).Update("description", "Original description").Error
	assert.NoError(t, err)

	err = store.Add(rag.Document{
		ID:      "Yakit 权威使用指南 v2",
		Content: "Yakit 权威使用指南",
		Embedding: []float32{
			0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
		},
	})
	assert.NoError(t, err)

	err = rag.ImportVectorDataFullUpdate(db, tmpFilePath)
	assert.NoError(t, err)

	var newCollection schema.VectorStoreCollection
	err = db.Where("name = ?", PLUGIN_RAG_COLLECTION_NAME).First(&newCollection).Error
	assert.NoError(t, err)
	assert.Equal(t, "test", newCollection.Description)

	var newDocs []*schema.VectorStoreDocument
	err = db.Model(&schema.VectorStoreDocument{}).Find(&newDocs).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(newDocs))
	assert.Equal(t, "Yakit 权威使用指南 v1", newDocs[0].DocumentID)
}
