package rag

import (
	"os"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// createTempTestDatabase 创建临时测试数据库
func createTempTestDatabase() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	// 迁移所有相关的表
	db.AutoMigrate(
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.EntityRepository{},
		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
	)
	return db, nil
}

// createTestRAGCollection 创建测试用的 RAG 集合
func createTestRAGCollection(db *gorm.DB, name string) (*schema.VectorStoreCollection, error) {
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	collection, err := vectorstore.GetCollection(db, name,
		vectorstore.WithEmbeddingClient(mockEmbedding),
		vectorstore.WithDescription("测试集合描述"),
	)
	if err != nil {
		return nil, err
	}

	// 确保集合有 RAG ID
	collectionInfo := collection.GetCollectionInfo()
	if collectionInfo.RAGID == "" {
		ragID := utils.RandStringBytes(16)
		collectionInfo.RAGID = ragID
		err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionInfo.ID).Update("rag_id", ragID).Error
		if err != nil {
			return nil, err
		}
	}

	return collectionInfo, nil
}

// createTestKnowledgeBase 创建测试用的知识库
func createTestKnowledgeBase(db *gorm.DB, name, ragID string) (*schema.KnowledgeBaseInfo, error) {
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	kb, err := knowledgebase.NewKnowledgeBase(db, name, "测试知识库描述", "test",
		vectorstore.WithEmbeddingClient(mockEmbedding),
		vectorstore.WithDB(db),
	)
	if err != nil {
		return nil, err
	}

	// 设置 RAG ID
	kbInfo := kb.GetKnowledgeBaseInfo()
	err = db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", kbInfo.ID).Update("rag_id", ragID).Error
	if err != nil {
		return nil, err
	}
	kbInfo.RAGID = ragID

	// 同时更新向量存储集合的 RAG ID
	collectionName := name // knowledgebase.NewKnowledgeBase 使用知识库名称作为集合名称
	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Update("rag_id", ragID).Error
	if err != nil {
		return nil, err
	}

	return kbInfo, nil
}

// createTestEntityRepository 创建测试用的实体仓库
func createTestEntityRepository(db *gorm.DB, name, ragID string) (*schema.EntityRepository, error) {
	entityRepo := &schema.EntityRepository{
		EntityBaseName: name,
		Description:    "测试实体仓库描述",
		RAGID:          ragID,
	}
	if err := db.Create(entityRepo).Error; err != nil {
		return nil, err
	}
	return entityRepo, nil
}

// addTestKnowledgeBaseEntries 为知识库添加测试条目
func addTestKnowledgeBaseEntries(db *gorm.DB, kbID int64) error {
	entries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:    kbID,
			RelatedEntityUUIDS: "uuid1,uuid2",
			KnowledgeTitle:     "Go语言并发编程",
			KnowledgeType:      "Programming",
			ImportanceScore:    9,
			Keywords:           schema.StringArray{"Go", "并发", "goroutine", "channel"},
			KnowledgeDetails:   "Go语言的并发模型基于goroutine和channel，提供了简洁而强大的并发编程能力。",
		},
		{
			KnowledgeBaseID:    kbID,
			RelatedEntityUUIDS: "uuid3",
			KnowledgeTitle:     "Python数据分析",
			KnowledgeType:      "Data Science",
			ImportanceScore:    8,
			Keywords:           schema.StringArray{"Python", "数据分析", "pandas", "numpy"},
			KnowledgeDetails:   "Python在数据分析领域有着广泛的应用，pandas和numpy是核心库。",
		},
	}

	for _, entry := range entries {
		if err := db.Create(entry).Error; err != nil {
			return err
		}
	}
	return nil
}

// addTestEntities 为实体仓库添加测试实体
func addTestEntities(db *gorm.DB, repoUUID string) error {
	entities := []*schema.ERModelEntity{
		{
			RepositoryUUID:    repoUUID,
			EntityName:        "测试实体1",
			Description:       "这是第一个测试实体",
			EntityType:        "Person",
			EntityTypeVerbose: "人物",
			Attributes: map[string]any{
				"age":  30,
				"city": "北京",
			},
		},
		{
			RepositoryUUID:    repoUUID,
			EntityName:        "测试实体2",
			Description:       "这是第二个测试实体",
			EntityType:        "Company",
			EntityTypeVerbose: "公司",
			Attributes: map[string]any{
				"industry": "科技",
				"founded":  2020,
			},
		},
	}

	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			return err
		}
	}
	return nil
}

// TestExportRAG_CollectionNotFound 测试导出时集合不存在的情况
func TestExportRAG_CollectionNotFound(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 尝试导出不存在的集合
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ExportRAG("nonexistent_collection", tempFile.Name(), WithDB(db))

	// 应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get collection failed")
}

// TestExportRAG_NoKnowledgeBase 测试导出时没有知识库的情况
func TestExportRAG_NoKnowledgeBase(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建集合但不创建知识库
	collectionName := "test_collection_no_kb_" + utils.RandStringBytes(8)
	_, err = createTestRAGCollection(db, collectionName)
	assert.NoError(t, err)

	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ExportRAG(collectionName, tempFile.Name(), WithDB(db))
	// 应该返回错误，因为没有知识库
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "knowledge base not found")

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
}

// TestExportRAG_Success 测试成功导出 RAG
func TestExportRAG_Success(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建测试数据
	collectionName := "test_collection_export_" + utils.RandStringBytes(8)
	collection, err := createTestRAGCollection(db, collectionName)
	assert.NoError(t, err)

	// 创建知识库
	kbInfo, err := createTestKnowledgeBase(db, collectionName+"_kb", collection.RAGID)
	assert.NoError(t, err)

	// 验证知识库的 RAG ID 被正确设置
	assert.Equal(t, collection.RAGID, kbInfo.RAGID)

	// 添加知识库条目
	err = addTestKnowledgeBaseEntries(db, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 创建实体仓库
	entityRepo, err := createTestEntityRepository(db, collectionName+"_entity", collection.RAGID)
	assert.NoError(t, err)

	// 添加实体
	err = addTestEntities(db, entityRepo.Uuid)
	assert.NoError(t, err)

	// 验证知识库是否正确创建
	var kbCount int64
	err = db.Model(&schema.KnowledgeBaseInfo{}).Where("rag_id = ?", collection.RAGID).Count(&kbCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(1), kbCount)

	// 打印调试信息
	t.Logf("Collection RAGID: %s", collection.RAGID)
	t.Logf("KB ID: %d, KB RAGID: %s", kbInfo.ID, kbInfo.RAGID)

	// 执行导出
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ExportRAG(collectionName, tempFile.Name(), WithDB(db))

	// 验证导出成功
	if err != nil {
		t.Fatalf("ExportRAG failed: %v", err)
	}
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("read temp file failed: %v", err)
	}
	assert.True(t, len(content) > 0, "导出的数据应该是非空的")

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
	yakit.DeleteKnowledgeBase(db, int64(kbInfo.ID))
}

// TestExportRAG_OnlyKnowledgeBase 测试只导出知识库（没有实体仓库）
func TestExportRAG_OnlyKnowledgeBase(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建测试数据
	collectionName := "test_collection_kb_only_" + utils.RandStringBytes(8)
	collection, err := createTestRAGCollection(db, collectionName)
	assert.NoError(t, err)

	// 只创建知识库，不创建实体仓库
	kbInfo, err := createTestKnowledgeBase(db, collectionName+"_kb", collection.RAGID)
	assert.NoError(t, err)

	// 添加知识库条目
	err = addTestKnowledgeBaseEntries(db, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 执行导出
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ExportRAG(collectionName, tempFile.Name(), WithDB(db))
	if err != nil {
		t.Fatalf("ExportRAG failed: %v", err)
	}
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("read temp file failed: %v", err)
	}
	assert.True(t, len(content) > 0, "导出的数据应该是非空的")

	// 清理
	vectorstore.DeleteCollection(db, collectionName)
	yakit.DeleteKnowledgeBase(db, int64(kbInfo.ID))
}

// TestImportRAG_Success 测试成功导入 RAG
func TestImportRAG_Success(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 首先导出数据
	exportCollectionName := "test_export_import_" + utils.RandStringBytes(8)
	exportCollection, err := createTestRAGCollection(db, exportCollectionName)
	assert.NoError(t, err)

	// 创建知识库
	kbInfo, err := createTestKnowledgeBase(db, exportCollectionName+"_kb", exportCollection.RAGID)
	assert.NoError(t, err)

	// 添加知识库条目
	err = addTestKnowledgeBaseEntries(db, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 创建实体仓库
	entityRepo, err := createTestEntityRepository(db, exportCollectionName+"_entity", exportCollection.RAGID)
	assert.NoError(t, err)

	// 添加实体
	err = addTestEntities(db, entityRepo.Uuid)
	assert.NoError(t, err)

	// 执行导出
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ExportRAG(exportCollectionName, tempFile.Name(), WithDB(db))
	assert.NoError(t, err)

	// 读取导出的数据到缓冲区
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("read temp file failed: %v", err)
	}
	assert.True(t, len(content) > 0, "导出的数据应该是非空的")
	assert.NoError(t, err)

	// 创建新的数据库用于导入
	importDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 执行导入
	importCollectionName := "test_import_collection_" + utils.RandStringBytes(8)
	err = ImportRAG(tempFile.Name(),
		WithDB(importDB),
		WithRAGCollectionName(importCollectionName),
		WithExportOverwriteExisting(true),
	)

	// 验证导入成功
	assert.NoError(t, err)

	// 验证导入后的数据
	importedCollection, err := yakit.GetRAGCollectionInfoByName(importDB, importCollectionName)
	assert.NoError(t, err)
	assert.NotNil(t, importedCollection)
	assert.Equal(t, importCollectionName, importedCollection.Name)

	// 验证知识库是否被导入
	var importedKB schema.KnowledgeBaseInfo
	err = importDB.Model(&schema.KnowledgeBaseInfo{}).Where("rag_id = ?", importedCollection.RAGID).First(&importedKB).Error
	assert.NoError(t, err)
	assert.Equal(t, importCollectionName, importedKB.KnowledgeBaseName)

	// 验证知识库条目是否被导入
	var kbEntries []schema.KnowledgeBaseEntry
	err = importDB.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", importedKB.ID).Find(&kbEntries).Error
	assert.NoError(t, err)
	assert.Len(t, kbEntries, 2) // 我们添加了2个条目

	// 验证实体仓库是否被导入
	var importedEntityRepo schema.EntityRepository
	err = importDB.Model(&schema.EntityRepository{}).Where("rag_id = ?", importedCollection.RAGID).First(&importedEntityRepo).Error
	assert.NoError(t, err)
	assert.Equal(t, importCollectionName, importedEntityRepo.EntityBaseName)

	// 验证实体是否被导入
	var entities []schema.ERModelEntity
	err = importDB.Model(&schema.ERModelEntity{}).Where("repository_uuid = ?", importedEntityRepo.Uuid).Find(&entities).Error
	assert.NoError(t, err)
	assert.Len(t, entities, 2) // 我们添加了2个实体

	// 清理
	vectorstore.DeleteCollection(db, exportCollectionName)
	vectorstore.DeleteCollection(importDB, importCollectionName)
	yakit.DeleteKnowledgeBase(db, int64(kbInfo.ID))
}

// TestImportRAG_EmptyReader 测试导入空数据的情况
func TestImportRAG_EmptyReader(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 尝试导入空数据
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ImportRAG(tempFile.Name(), WithDB(db))
	if err != nil {
		t.Fatalf("ImportRAG failed: %v", err)
	}

	// 应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "import knowledge base failed")
}

// TestImportRAG_InvalidData 测试导入无效数据的情况
func TestImportRAG_InvalidData(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 尝试导入无效的JSON数据
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ImportRAG(tempFile.Name(), WithDB(db))
	if err != nil {
		t.Fatalf("ImportRAG failed: %v", err)
	}

	// 应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "import knowledge base failed")
}
