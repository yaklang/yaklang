package rag

import (
	"crypto/md5"
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

	var vectorDocument []*schema.VectorStoreDocument = []*schema.VectorStoreDocument{
		{
			CollectionUUID: collectionInfo.UUID,
			DocumentID:     "test_doc_1",
			Content:        "test_content_1",
			Metadata: schema.MetadataMap{
				schema.META_Data_UUID: "hidden_index_1",
			},
			Embedding: []float32{0.1, 0.2, 0.3},
		},
		{
			CollectionUUID: collectionInfo.UUID,
			DocumentID:     "test_doc_2",
			Content:        "test_content_2",
			Metadata: schema.MetadataMap{
				schema.META_Data_UUID: "hidden_index_2",
			},
			Embedding: []float32{0.4, 0.5, 0.6},
		},
		{
			CollectionUUID: collectionInfo.UUID,
			DocumentID:     "test_doc_3",
			Content:        "test_content_3",
			Metadata: schema.MetadataMap{
				schema.META_Data_UUID: "hidden_index_3",
			},
			Embedding: []float32{0.7, 0.8, 0.9},
		},
		{
			CollectionUUID: collectionInfo.UUID,
			DocumentID:     "test_doc_4",
			Content:        "test_content_4",
			Metadata: schema.MetadataMap{
				schema.META_Data_UUID: "hidden_index_4",
			},
			Embedding: []float32{0.1, 0.2, 0.3},
		},
	}
	for _, document := range vectorDocument {
		if err := db.Create(document).Error; err != nil {
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
			HiddenIndex:        "hidden_index_1",
			Keywords:           schema.StringArray{"Go", "并发", "goroutine", "channel"},
			KnowledgeDetails:   "Go语言的并发模型基于goroutine和channel，提供了简洁而强大的并发编程能力。",
		},
		{
			KnowledgeBaseID:    kbID,
			RelatedEntityUUIDS: "uuid3",
			KnowledgeTitle:     "Python数据分析",
			KnowledgeType:      "Data Science",
			ImportanceScore:    8,
			HiddenIndex:        "hidden_index_2",
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
			Uuid:              "hidden_index_3",
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
			Uuid:              "hidden_index_4",
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
func TestMUSTPASS_ExportRAG_CollectionNotFound(t *testing.T) {
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
	assert.Contains(t, err.Error(), "not existed")
}

// TestExportRAG_Success 测试成功导出 RAG
func TestMUSTPASS_ExportRAG_Success(t *testing.T) {
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
func TestMUSTPASS_ExportRAG_OnlyKnowledgeBase(t *testing.T) {
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

// TestImportRAG_EmptyReader 测试导入空数据的情况
func TestMUSTPASS_ImportRAG_EmptyReader(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 尝试导入空数据
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ImportRAG(tempFile.Name(), WithDB(db))

	// 应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read magic header")
}

// TestImportRAG_InvalidData 测试导入无效数据的情况
func TestMUSTPASS_ImportRAG_InvalidData(t *testing.T) {
	db, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 尝试导入无效的JSON数据
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	defer tempFile.Close()
	err = ImportRAG(tempFile.Name(), WithDB(db))
	// 应该返回错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read magic header")
}

func TestMUSTPASS_ImportRAGFile(t *testing.T) {
	// 生成导出数据
	db, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)
	exportCollectionName := "test_export_import_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	ragSystem, err := Get(exportCollectionName, WithDB(db), WithDisableEmbedCollectionInfo(true), WithLazyLoadEmbeddingClient(true), WithEmbeddingClient(mockEmbedding))
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)

	ragSystem.VectorStore.AddWithOptions("test_doc", "test_content")
	// 执行导出
	tempFile, err := os.CreateTemp("", "test_export_rag_*.zip")
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}
	tempFile.Close()

	err = ExportRAG(exportCollectionName, tempFile.Name(), WithDB(db))
	assert.NoError(t, err)

	// 测试导入后的uid
	db, err = createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)
	err = ImportRAG(tempFile.Name(), WithDB(db))
	assert.NoError(t, err)

	var collection schema.VectorStoreCollection
	db.Model(&schema.VectorStoreCollection{}).Where("name = ?", exportCollectionName).First(&collection)
	assert.NotNil(t, collection)
	assert.Equal(t, exportCollectionName, collection.Name)

	var document schema.VectorStoreDocument
	db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).First(&document)
	assert.NotNil(t, document)
	calcUID := md5.Sum([]byte(document.CollectionUUID + document.DocumentID))
	assert.Equal(t, calcUID[:], document.UID)
	assert.Equal(t, collection.UUID, document.CollectionUUID)
}

func TestMUSTPASS_ExportWithNewUUID(t *testing.T) {
	// 创建源数据库并准备测试数据
	sourceDB, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	collectionName := "test_export_new_uuid_" + utils.RandStringBytes(8)
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()

	// 创建RAG系统
	ragSystem, err := Get(collectionName,
		WithDB(sourceDB),
		WithDisableEmbedCollectionInfo(true),
		WithLazyLoadEmbeddingClient(true),
		WithEmbeddingClient(mockEmbedding),
		WithEnableKnowledgeBase(true),
		WithEnableEntityRepository(true),
	)
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)

	// 获取实体仓库信息
	entityRepoInfo, err := ragSystem.EntityRepository.GetInfo()
	assert.NoError(t, err)

	// 添加实体到实体仓库
	entity1 := &schema.ERModelEntity{
		RepositoryUUID:    entityRepoInfo.Uuid,
		EntityName:        "测试实体1",
		Description:       "这是第一个测试实体",
		EntityType:        "Person",
		EntityTypeVerbose: "人物",
		Attributes: map[string]any{
			"age":  30,
			"city": "北京",
		},
	}
	err = sourceDB.Create(entity1).Error
	assert.NoError(t, err)

	entity2 := &schema.ERModelEntity{
		RepositoryUUID:    entityRepoInfo.Uuid,
		EntityName:        "测试实体2",
		Description:       "这是第二个测试实体",
		EntityType:        "Company",
		EntityTypeVerbose: "公司",
		Attributes: map[string]any{
			"industry": "科技",
			"founded":  2020,
		},
	}
	err = sourceDB.Create(entity2).Error
	assert.NoError(t, err)

	// 保存原始实体的UUID
	originalEntityUUIDs := []string{entity1.Uuid, entity2.Uuid}
	t.Logf("原始实体UUID: %v", originalEntityUUIDs)

	// 添加知识库条目
	kbInfo := ragSystem.KnowledgeBase.GetKnowledgeBaseInfo()
	entry1 := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(kbInfo.ID),
		RelatedEntityUUIDS: entity1.Uuid,
		KnowledgeTitle:     "Go语言并发编程",
		KnowledgeType:      "Programming",
		ImportanceScore:    9,
		Keywords:           schema.StringArray{"Go", "并发", "goroutine", "channel"},
		KnowledgeDetails:   "Go语言的并发模型基于goroutine和channel，提供了简洁而强大的并发编程能力。",
	}
	err = sourceDB.Create(entry1).Error
	assert.NoError(t, err)

	entry2 := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(kbInfo.ID),
		RelatedEntityUUIDS: entity2.Uuid,
		KnowledgeTitle:     "Python数据分析",
		KnowledgeType:      "Data Science",
		ImportanceScore:    8,
		Keywords:           schema.StringArray{"Python", "数据分析", "pandas", "numpy"},
		KnowledgeDetails:   "Python在数据分析领域有着广泛的应用，pandas和numpy是核心库。",
	}
	err = sourceDB.Create(entry2).Error
	assert.NoError(t, err)

	// 保存原始知识条目的HiddenIndex
	originalKnowledgeHiddenIndexes := []string{entry1.HiddenIndex, entry2.HiddenIndex}
	t.Logf("原始知识条目HiddenIndex: %v", originalKnowledgeHiddenIndexes)

	// 添加向量文档（关联实体）
	ragSystem.VectorStore.AddWithOptions("entity_doc_1", entity1.ToRAGContent(),
		vectorstore.WithDocumentRawMetadata(map[string]interface{}{
			schema.META_Data_UUID: entity1.Uuid,
		}),
		vectorstore.WithDocumentEntityID(entity1.Uuid),
	)
	ragSystem.VectorStore.AddWithOptions("entity_doc_2", entity2.ToRAGContent(),
		vectorstore.WithDocumentRawMetadata(map[string]interface{}{
			schema.META_Data_UUID: entity2.Uuid,
		}),
		vectorstore.WithDocumentEntityID(entity2.Uuid),
	)

	// 添加向量文档（关联知识条目）
	ragSystem.VectorStore.AddWithOptions("knowledge_doc_1", entry1.KnowledgeDetails,
		vectorstore.WithDocumentRawMetadata(map[string]interface{}{
			schema.META_Data_UUID: entry1.HiddenIndex,
		}),
	)
	ragSystem.VectorStore.AddWithOptions("knowledge_doc_2", entry2.KnowledgeDetails,
		vectorstore.WithDocumentRawMetadata(map[string]interface{}{
			schema.META_Data_UUID: entry2.HiddenIndex,
		}),
	)

	// 执行导出
	tempFile, err := os.CreateTemp("", "test_export_new_uuid_*.zip")
	assert.NoError(t, err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	err = ExportRAG(collectionName, tempFile.Name(), WithDB(sourceDB))
	assert.NoError(t, err)

	// 创建新的数据库用于导入
	targetDB, err := createTempTestDatabaseForRAGSystem()
	assert.NoError(t, err)

	// 执行导入
	err = ImportRAG(tempFile.Name(), WithDB(targetDB))
	assert.NoError(t, err)

	// 验证导入后的数据

	// 1. 验证实体的UUID已经改变
	var importedEntities []schema.ERModelEntity
	err = targetDB.Find(&importedEntities).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(importedEntities), "应该导入2个实体")

	for _, entity := range importedEntities {
		t.Logf("导入后的实体UUID: %s (原名称: %s)", entity.Uuid, entity.EntityName)
		// 验证UUID已经改变
		assert.NotContains(t, originalEntityUUIDs, entity.Uuid, "实体UUID应该已经改变")
		assert.NotEmpty(t, entity.Uuid, "实体UUID不应为空")
	}

	// 2. 验证知识条目的HiddenIndex已经改变
	var importedKnowledgeEntries []schema.KnowledgeBaseEntry
	err = targetDB.Find(&importedKnowledgeEntries).Error
	assert.NoError(t, err)
	assert.Equal(t, 2, len(importedKnowledgeEntries), "应该导入2个知识条目")

	for _, entry := range importedKnowledgeEntries {
		t.Logf("导入后的知识条目HiddenIndex: %s (原标题: %s)", entry.HiddenIndex, entry.KnowledgeTitle)
		// 验证HiddenIndex已经改变
		assert.NotContains(t, originalKnowledgeHiddenIndexes, entry.HiddenIndex, "知识条目HiddenIndex应该已经改变")
		assert.NotEmpty(t, entry.HiddenIndex, "知识条目HiddenIndex不应为空")
	}

	// 3. 验证向量文档中的metadata UUID也已经更新
	var importedDocuments []schema.VectorStoreDocument
	err = targetDB.Find(&importedDocuments).Error
	assert.NoError(t, err)
	assert.Greater(t, len(importedDocuments), 0, "应该有导入的向量文档")

	for _, doc := range importedDocuments {
		if metaUUID, ok := doc.Metadata.GetDataUUID(); ok {
			t.Logf("导入后的向量文档metadata UUID: %s (DocumentID: %s)", metaUUID, doc.DocumentID)
			// 验证metadata中的UUID不是原始的UUID
			assert.NotContains(t, originalEntityUUIDs, metaUUID, "向量文档metadata中的实体UUID应该已经改变")
			assert.NotContains(t, originalKnowledgeHiddenIndexes, metaUUID, "向量文档metadata中的知识条目HiddenIndex应该已经改变")
		}
	}

	// 4. 验证实体仓库的UUID也已经改变
	var importedRepos []schema.EntityRepository
	err = targetDB.Find(&importedRepos).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(importedRepos), "应该有1个实体仓库")

	originalReposUUID := entityRepoInfo.Uuid
	importedReposUUID := importedRepos[0].Uuid
	t.Logf("原始实体仓库UUID: %s, 导入后: %s", originalReposUUID, importedReposUUID)
	assert.NotEqual(t, originalReposUUID, importedReposUUID, "实体仓库UUID应该已经改变")

	// 5. 验证知识库信息的UUID关联正确
	var importedKBInfo []schema.KnowledgeBaseInfo
	err = targetDB.Find(&importedKBInfo).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(importedKBInfo), "应该有1个知识库")

	// 验证所有知识条目都关联到新的知识库ID
	for _, entry := range importedKnowledgeEntries {
		assert.Equal(t, int64(importedKBInfo[0].ID), entry.KnowledgeBaseID, "知识条目应该关联到新的知识库ID")
	}

	// 6. 验证所有实体都关联到新的实体仓库UUID
	for _, entity := range importedEntities {
		assert.Equal(t, importedReposUUID, entity.RepositoryUUID, "实体应该关联到新的实体仓库UUID")
	}

	t.Log("所有UUID和HiddenIndex验证通过，导入后的数据已经使用新的标识符")
}
