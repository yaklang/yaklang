package knowledgebase

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// TestDeleteKnowledgeBase_Success 测试成功删除知识库
func TestDeleteKnowledgeBase_Success(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kbName := "test-kb-delete-" + uuid.New().String()
	kb, err := NewKnowledgeBase(
		db,
		kbName,
		"测试删除知识库",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 添加一些知识条目
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID: kb.GetID(),
		KnowledgeTitle:  "测试知识1",
	}
	err = db.Create(entry).Error
	require.NoError(t, err)

	// 验证知识库存在
	var kbInfo schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", kbName).First(&kbInfo).Error
	require.NoError(t, err)

	// 验证知识条目存在
	var entries []schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", kbInfo.ID).Find(&entries).Error
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// 验证 VectorStoreCollection 存在
	var collection schema.VectorStoreCollection
	err = db.Where("name = ?", kbName).First(&collection).Error
	require.NoError(t, err)

	// 删除知识库
	err = DeleteKnowledgeBase(db, kbName)
	assert.NoError(t, err)

	// 验证 KnowledgeBaseInfo 被删除
	err = db.Where("knowledge_base_name = ?", kbName).First(&kbInfo).Error
	assert.Error(t, err, "KnowledgeBaseInfo should be deleted")

	// 验证 KnowledgeBaseEntry 被删除
	var entriesAfter []schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", kbInfo.ID).Find(&entriesAfter).Error
	assert.NoError(t, err)
	assert.Len(t, entriesAfter, 0, "All entries should be deleted")

	// 验证 VectorStoreCollection 被删除
	err = db.Where("name = ?", kbName).First(&collection).Error
	assert.Error(t, err, "VectorStoreCollection should be deleted")

	// 验证 VectorStoreDocument 被删除
	var documents []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collection.ID).Find(&documents).Error
	assert.NoError(t, err)
	assert.Len(t, documents, 0, "All documents should be deleted")
}

// TestDeleteKnowledgeBase_NotFound 测试删除不存在的知识库
func TestDeleteKnowledgeBase_NotFound(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 尝试删除不存在的知识库
	kbName := "non-existent-kb-" + uuid.New().String()
	err = DeleteKnowledgeBase(db, kbName)
	assert.Error(t, err, "Should return error when knowledge base not found")
	assert.Contains(t, err.Error(), "get KnowledgeBaseInfo failed")
}

// TestDeleteKnowledgeBase_WithMultipleEntries 测试删除包含多个条目的知识库
func TestDeleteKnowledgeBase_WithMultipleEntries(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kbName := "test-kb-multi-" + uuid.New().String()
	kb, err := NewKnowledgeBase(
		db,
		kbName,
		"测试多条目删除",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 添加多个知识条目
	kbID := kb.GetID()
	for i := 0; i < 5; i++ {
		entry := &schema.KnowledgeBaseEntry{
			KnowledgeBaseID: kbID,
			KnowledgeTitle:  uuid.New().String(),
		}
		err = db.Create(entry).Error
		require.NoError(t, err)
	}

	// 验证知识条目存在
	var entriesBefore []schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", kbID).Find(&entriesBefore).Error
	require.NoError(t, err)
	require.Len(t, entriesBefore, 5)

	// 删除知识库
	err = DeleteKnowledgeBase(db, kbName)
	assert.NoError(t, err)

	// 验证所有条目都被删除
	var entriesAfter []schema.KnowledgeBaseEntry
	err = db.Where("knowledge_base_id = ?", kbID).Find(&entriesAfter).Error
	assert.NoError(t, err)
	assert.Len(t, entriesAfter, 0, "All entries should be deleted")
}

// TestDeleteKnowledgeBase_WithDocuments 测试删除包含文档的知识库
func TestDeleteKnowledgeBase_WithDocuments(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kbName := "test-kb-docs-" + uuid.New().String()
	kb, err := NewKnowledgeBase(
		db,
		kbName,
		"测试文档删除",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 获取 collection
	var collection schema.VectorStoreCollection
	err = db.Where("name = ?", kbName).First(&collection).Error
	require.NoError(t, err)

	// 添加多个文档
	for i := 0; i < 3; i++ {
		doc := &schema.VectorStoreDocument{
			CollectionID: collection.ID,
			DocumentID:   uuid.New().String(),
			Content:      "测试文档内容 " + uuid.New().String(),
		}
		err = db.Create(doc).Error
		require.NoError(t, err)
	}

	// 验证文档存在（注意：RAG系统会自动创建一个 __collection_info__ 文档，所以总数是4）
	var docsBefore []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collection.ID).Find(&docsBefore).Error
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(docsBefore), 3, "Should have at least 3 documents")

	// 删除知识库
	err = DeleteKnowledgeBase(db, kbName)
	assert.NoError(t, err)

	// 验证所有文档都被删除
	var docsAfter []schema.VectorStoreDocument
	err = db.Where("collection_id = ?", collection.ID).Find(&docsAfter).Error
	assert.NoError(t, err)
	assert.Len(t, docsAfter, 0, "All documents should be deleted")
}

// TestDeleteKnowledgeBase_Transaction 测试删除过程的事务性
func TestDeleteKnowledgeBase_Transaction(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kbName := "test-kb-txn-" + uuid.New().String()
	kb, err := NewKnowledgeBase(
		db,
		kbName,
		"测试事务性",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 验证知识库被创建
	var kbInfoBefore schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", kbName).First(&kbInfoBefore).Error
	require.NoError(t, err)

	// 正常删除应该成功
	err = DeleteKnowledgeBase(db, kbName)
	assert.NoError(t, err)

	// 再次尝试删除同一个知识库应该失败（因为已经不存在了）
	err = DeleteKnowledgeBase(db, kbName)
	assert.Error(t, err)
}

// TestExportImportKnowledgeBase_WithExtraData 测试导出导入知识库时的额外数据功能
func TestExportImportKnowledgeBase_WithExtraData(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kbName := "test-kb-extra-data-" + uuid.New().String()
	kb, err := NewKnowledgeBase(
		db,
		kbName,
		"测试额外数据",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 添加知识条目
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  kb.GetID(),
		KnowledgeTitle:   "测试知识",
		KnowledgeDetails: "测试内容",
	}
	err = db.Create(entry).Error
	require.NoError(t, err)

	// 准备额外数据
	extraDataContent := "这是额外数据的内容，可以是任何格式的数据"
	extraDataReader := bytes.NewBufferString(extraDataContent)

	// 导出知识库（带额外数据）
	ctx := context.Background()
	exportedReader, err := ExportKnowledgeBase(ctx, db, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: kb.GetID(),
		ExtraDataReader: extraDataReader,
	})
	require.NoError(t, err)
	require.NotNil(t, exportedReader)

	// 读取导出的数据
	exportedData, err := io.ReadAll(exportedReader)
	require.NoError(t, err)
	require.NotEmpty(t, exportedData)

	// 创建新数据库用于导入
	path2 := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db2, err := rag.NewRagDatabase(path2)
	require.NoError(t, err)
	defer db2.Close()

	// 用于存储导入的额外数据
	var importedExtraData []byte

	// 导入知识库（带额外数据处理）
	newKbName := "imported-kb-" + uuid.New().String()
	importReader := bytes.NewReader(exportedData)
	err = ImportKnowledgeBase(ctx, db2, importReader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: newKbName,
		OverwriteExisting:    false,
		ExtraDataHandler: func(extraData io.Reader) error {
			// 读取额外数据
			data, err := io.ReadAll(extraData)
			if err != nil {
				return err
			}
			importedExtraData = data
			return nil
		},
	})
	require.NoError(t, err)

	// 验证额外数据被正确导入
	assert.Equal(t, extraDataContent, string(importedExtraData), "额外数据应该与导出的内容一致")

	// 验证知识库被正确导入
	var importedKb schema.KnowledgeBaseInfo
	err = db2.Where("knowledge_base_name = ?", newKbName).First(&importedKb).Error
	require.NoError(t, err)
	assert.Equal(t, "测试额外数据", importedKb.KnowledgeBaseDescription)

	// 验证知识条目被正确导入
	var importedEntries []schema.KnowledgeBaseEntry
	err = db2.Where("knowledge_base_id = ?", importedKb.ID).Find(&importedEntries).Error
	require.NoError(t, err)
	require.Len(t, importedEntries, 1)
	assert.Equal(t, "测试知识", importedEntries[0].KnowledgeTitle)
}

// TestExportImportKnowledgeBase_WithoutExtraData 测试导出导入知识库时没有额外数据的情况
func TestExportImportKnowledgeBase_WithoutExtraData(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	require.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kbName := "test-kb-no-extra-" + uuid.New().String()
	kb, err := NewKnowledgeBase(
		db,
		kbName,
		"测试无额外数据",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 导出知识库（不带额外数据）
	ctx := context.Background()
	exportedReader, err := ExportKnowledgeBase(ctx, db, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: kb.GetID(),
		// ExtraDataReader 为 nil
	})
	require.NoError(t, err)
	require.NotNil(t, exportedReader)

	// 读取导出的数据
	exportedData, err := io.ReadAll(exportedReader)
	require.NoError(t, err)
	require.NotEmpty(t, exportedData)

	// 创建新数据库用于导入
	path2 := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db2, err := rag.NewRagDatabase(path2)
	require.NoError(t, err)
	defer db2.Close()

	// 标记额外数据处理器是否被调用
	handlerCalled := false

	// 导入知识库
	newKbName := "imported-kb-no-extra-" + uuid.New().String()
	importReader := bytes.NewReader(exportedData)
	err = ImportKnowledgeBase(ctx, db2, importReader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: newKbName,
		OverwriteExisting:    false,
		ExtraDataHandler: func(extraData io.Reader) error {
			handlerCalled = true
			// 如果额外数据为空，这个处理器不应该被调用
			data, err := io.ReadAll(extraData)
			if err != nil {
				return err
			}
			assert.Empty(t, data, "额外数据应该为空")
			return nil
		},
	})
	require.NoError(t, err)

	// 验证额外数据处理器没有被调用（因为额外数据为空）
	assert.False(t, handlerCalled, "额外数据为空时，处理器不应该被调用")

	// 验证知识库被正确导入
	var importedKb schema.KnowledgeBaseInfo
	err = db2.Where("knowledge_base_name = ?", newKbName).First(&importedKb).Error
	require.NoError(t, err)
	assert.Equal(t, "测试无额外数据", importedKb.KnowledgeBaseDescription)
}
