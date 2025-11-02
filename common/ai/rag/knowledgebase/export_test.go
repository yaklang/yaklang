package knowledgebase

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func createTempTestDatabase() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	return db, nil
}

func createTestKnowledgeBase(db *gorm.DB, name string) (*schema.KnowledgeBaseInfo, error) {
	kbInfo := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        name,
		KnowledgeBaseDescription: "测试知识库描述",
		KnowledgeBaseType:        "test",
	}
	if err := yakit.CreateKnowledgeBase(db, kbInfo); err != nil {
		return nil, err
	}
	return kbInfo, nil
}

func createTestKnowledgeBaseEntries(db *gorm.DB, kbID int64) ([]*schema.KnowledgeBaseEntry, error) {
	entries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:          kbID,
			RelatedEntityUUIDS:       "uuid1,uuid2",
			KnowledgeTitle:           "Go语言并发编程",
			KnowledgeType:            "Programming",
			ImportanceScore:          9,
			Keywords:                 schema.StringArray{"Go", "并发", "goroutine", "channel"},
			KnowledgeDetails:         "Go语言的并发模型基于goroutine和channel，提供了简洁而强大的并发编程能力。",
			Summary:                  "Go语言并发编程基础",
			SourcePage:               42,
			PotentialQuestions:       schema.StringArray{"什么是goroutine", "如何使用channel", "Go并发有什么优势"},
			PotentialQuestionsVector: schema.FloatArray{0.1, 0.2, 0.3, 0.4, 0.5},
			HiddenIndex:              "hidden_index_1",
		},
		{
			KnowledgeBaseID:          kbID,
			RelatedEntityUUIDS:       "uuid3,uuid4",
			KnowledgeTitle:           "数据库设计原则",
			KnowledgeType:            "Database",
			ImportanceScore:          8,
			Keywords:                 schema.StringArray{"数据库", "设计", "范式", "索引"},
			KnowledgeDetails:         "数据库设计需要遵循范式理论，合理设计表结构和索引，确保数据一致性和查询效率。",
			Summary:                  "数据库设计基本原则",
			SourcePage:               15,
			PotentialQuestions:       schema.StringArray{"什么是数据库范式", "如何设计索引", "数据库优化技巧"},
			PotentialQuestionsVector: schema.FloatArray{0.6, 0.7, 0.8, 0.9, 1.0},
			HiddenIndex:              "hidden_index_2",
		},
		{
			KnowledgeBaseID:          kbID,
			RelatedEntityUUIDS:       "uuid5",
			KnowledgeTitle:           "机器学习算法",
			KnowledgeType:            "AI",
			ImportanceScore:          10,
			Keywords:                 schema.StringArray{"机器学习", "算法", "神经网络", "深度学习"},
			KnowledgeDetails:         "机器学习算法包括监督学习、无监督学习和强化学习，深度学习是机器学习的重要分支。",
			Summary:                  "机器学习算法概述",
			SourcePage:               88,
			PotentialQuestions:       schema.StringArray{"什么是监督学习", "神经网络如何工作", "深度学习的应用"},
			PotentialQuestionsVector: schema.FloatArray{1.1, 1.2, 1.3, 1.4, 1.5},
			HiddenIndex:              "hidden_index_3",
		},
	}

	for _, entry := range entries {
		if err := yakit.CreateKnowledgeBaseEntry(db, entry); err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func createTestRAGData(db *gorm.DB, collectionName string) error {
	// 创建模拟嵌入器
	embedding := vectorstore.NewDefaultMockEmbedding()
	// 创建RAG向量存储
	store, err := vectorstore.NewSQLiteVectorStoreHNSW(collectionName, "测试向量集合", "text-embedding-3-small", 1024, embedding, db)
	if err != nil {
		return err
	}

	// 添加测试文档
	testDocs := []struct {
		id      string
		content string
	}{
		{"doc1", "Go语言并发编程相关内容"},
		{"doc2", "数据库设计原则相关内容"},
		{"doc3", "机器学习算法相关内容"},
	}

	for _, doc := range testDocs {
		store.AddWithOptions(doc.id, doc.content, vectorstore.WithDocumentRawMetadata(map[string]any{"source": "test"}))
	}

	return nil
}

func TestKnowledgeBaseExportImport(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建测试知识库
	kbName := utils.RandStringBytes(10)
	kbInfo, err := createTestKnowledgeBase(testDB, kbName)
	assert.NoError(t, err)

	// 创建测试知识库条目
	originalEntries, err := createTestKnowledgeBaseEntries(testDB, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 创建测试RAG数据
	err = createTestRAGData(testDB, kbName)
	assert.NoError(t, err)

	// 进度跟踪
	var progressMessages []string
	var lastProgress float64

	// 导出知识库
	ctx := context.Background()
	reader, err := ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: int64(kbInfo.ID),
		OnProgressHandler: func(percent float64, message string, messageType string) {
			progressMessages = append(progressMessages, fmt.Sprintf("%.1f%%: %s (%s)", percent, message, messageType))
			lastProgress = percent
			t.Logf("Export Progress: %.1f%% - %s (%s)", percent, message, messageType)
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Equal(t, 100.0, lastProgress, "导出应该达到100%进度")
	assert.Greater(t, len(progressMessages), 0, "应该有进度消息")

	// 创建新的测试数据库用于导入
	newTestDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 重置进度跟踪
	progressMessages = []string{}
	lastProgress = 0

	// 导入知识库（不覆盖，因为是新数据库）
	err = ImportKnowledgeBase(ctx, newTestDB, reader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    false,
		OnProgressHandler: func(percent float64, message string, messageType string) {
			progressMessages = append(progressMessages, fmt.Sprintf("%.1f%%: %s (%s)", percent, message, messageType))
			lastProgress = percent
			t.Logf("Import Progress: %.1f%% - %s (%s)", percent, message, messageType)
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 100.0, lastProgress, "导入应该达到100%进度")
	assert.Greater(t, len(progressMessages), 0, "应该有进度消息")

	// 验证知识库信息
	importedKBInfo, err := yakit.GetKnowledgeBaseByName(newTestDB, kbName)
	assert.NoError(t, err)
	assert.NotNil(t, importedKBInfo)
	assert.Equal(t, kbInfo.KnowledgeBaseName, importedKBInfo.KnowledgeBaseName)
	assert.Equal(t, kbInfo.KnowledgeBaseDescription, importedKBInfo.KnowledgeBaseDescription)
	assert.Equal(t, kbInfo.KnowledgeBaseType, importedKBInfo.KnowledgeBaseType)

	// 验证知识库条目
	var importedEntries []*schema.KnowledgeBaseEntry
	err = newTestDB.Where("knowledge_base_id = ?", importedKBInfo.ID).Find(&importedEntries).Error
	assert.NoError(t, err)
	assert.Len(t, importedEntries, len(originalEntries))

	// 按标题排序以便比较
	sort.Slice(originalEntries, func(i, j int) bool {
		return originalEntries[i].KnowledgeTitle < originalEntries[j].KnowledgeTitle
	})
	sort.Slice(importedEntries, func(i, j int) bool {
		return importedEntries[i].KnowledgeTitle < importedEntries[j].KnowledgeTitle
	})

	// 逐个比较条目
	for i := range originalEntries {
		orig := originalEntries[i]
		imported := importedEntries[i]

		assert.Equal(t, orig.RelatedEntityUUIDS, imported.RelatedEntityUUIDS)
		assert.Equal(t, orig.KnowledgeTitle, imported.KnowledgeTitle)
		assert.Equal(t, orig.KnowledgeType, imported.KnowledgeType)
		assert.Equal(t, orig.ImportanceScore, imported.ImportanceScore)
		assert.Equal(t, orig.Keywords, imported.Keywords)
		assert.Equal(t, orig.KnowledgeDetails, imported.KnowledgeDetails)
		assert.Equal(t, orig.Summary, imported.Summary)
		assert.Equal(t, orig.SourcePage, imported.SourcePage)
		assert.Equal(t, orig.PotentialQuestions, imported.PotentialQuestions)
		assert.Equal(t, orig.PotentialQuestionsVector, imported.PotentialQuestionsVector)
		assert.Equal(t, orig.HiddenIndex, imported.HiddenIndex)
		assert.Equal(t, int64(importedKBInfo.ID), imported.KnowledgeBaseID)
	}

	// 验证RAG数据是否也被导入
	var collections []schema.VectorStoreCollection
	err = newTestDB.Where("name = ?", kbName).Find(&collections).Error
	assert.NoError(t, err)
	assert.Len(t, collections, 1)

	var documents []schema.VectorStoreDocument
	err = newTestDB.Where("collection_id = ?", collections[0].ID).Find(&documents).Error
	assert.NoError(t, err)
	assert.Greater(t, len(documents), 0, "应该有导入的向量文档")

	t.Logf("成功导出和导入知识库 '%s'，包含 %d 个条目和 %d 个向量文档", kbName, len(importedEntries), len(documents))
}

func TestKnowledgeBaseExportImportOverwrite(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建第一个知识库
	kbName := utils.RandStringBytes(10)
	kbInfo1, err := createTestKnowledgeBase(testDB, kbName)
	assert.NoError(t, err)

	originalEntries1, err := createTestKnowledgeBaseEntries(testDB, int64(kbInfo1.ID))
	assert.NoError(t, err)

	// 创建对应的RAG数据
	err = createTestRAGData(testDB, kbName)
	assert.NoError(t, err)

	// 导出第一个知识库
	ctx := context.Background()
	exportData1, err := ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: int64(kbInfo1.ID),
	})
	assert.NoError(t, err)

	// 将导出数据读取到内存中，以便多次使用
	exportBytes1, err := io.ReadAll(exportData1)
	assert.NoError(t, err)
	reader1 := bytes.NewReader(exportBytes1)

	// 创建第二个测试数据库
	testDB2, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 先导入第一个知识库
	err = ImportKnowledgeBase(ctx, testDB2, reader1, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    false,
	})
	assert.NoError(t, err)

	// 修改原数据库中的知识库信息
	kbInfo1.KnowledgeBaseDescription = "修改后的描述"
	err = yakit.UpdateKnowledgeBaseInfo(testDB, int64(kbInfo1.ID), kbInfo1)
	assert.NoError(t, err)

	// 添加一个新条目
	newEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:          int64(kbInfo1.ID),
		RelatedEntityUUIDS:       "uuid_new",
		KnowledgeTitle:           "新增知识条目",
		KnowledgeType:            "New",
		ImportanceScore:          7,
		Keywords:                 schema.StringArray{"新增", "测试"},
		KnowledgeDetails:         "这是一个新增的测试条目",
		Summary:                  "新增条目摘要",
		SourcePage:               100,
		PotentialQuestions:       schema.StringArray{"这是新增的吗"},
		PotentialQuestionsVector: schema.FloatArray{2.0, 2.1, 2.2},
		HiddenIndex:              "hidden_index_new",
	}
	err = yakit.CreateKnowledgeBaseEntry(testDB, newEntry)
	assert.NoError(t, err)

	// 重新导出修改后的知识库
	exportData2, err := ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: int64(kbInfo1.ID),
	})
	assert.NoError(t, err)

	// 将导出数据读取到内存中，以便多次使用
	exportBytes2, err := io.ReadAll(exportData2)
	assert.NoError(t, err)

	// 尝试不覆盖导入（应该失败）
	reader2 := bytes.NewReader(exportBytes2)
	err = ImportKnowledgeBase(ctx, testDB2, reader2, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    false,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// 覆盖导入（应该成功）
	reader3 := bytes.NewReader(exportBytes2)
	err = ImportKnowledgeBase(ctx, testDB2, reader3, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    true,
	})
	assert.NoError(t, err)

	// 验证覆盖后的数据
	updatedKBInfo, err := yakit.GetKnowledgeBaseByName(testDB2, kbName)
	assert.NoError(t, err)
	assert.Equal(t, "修改后的描述", updatedKBInfo.KnowledgeBaseDescription)

	// 验证条目数量增加了
	var finalEntries []*schema.KnowledgeBaseEntry
	err = testDB2.Where("knowledge_base_id = ?", updatedKBInfo.ID).Find(&finalEntries).Error
	assert.NoError(t, err)
	assert.Len(t, finalEntries, len(originalEntries1)+1, "应该有原来的条目数+1个新条目")

	// 验证新条目存在
	var newEntryExists bool
	for _, entry := range finalEntries {
		if entry.KnowledgeTitle == "新增知识条目" {
			newEntryExists = true
			assert.Equal(t, "新增", entry.Keywords[0])
			break
		}
	}
	assert.True(t, newEntryExists, "新增的条目应该存在")

	t.Logf("成功测试知识库覆盖导入功能")
}

func TestKnowledgeBaseExportNonExistent(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 尝试导出不存在的知识库
	ctx := context.Background()
	_, err = ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: 99999, // 不存在的ID
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestKnowledgeBaseImportInvalidData(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	ctx := context.Background()
	// 测试正确的魔数头但数据不完整
	incompleteReader := bytes.NewReader([]byte("YAKKNOWLEDGEBASE"))
	err = ImportKnowledgeBase(ctx, testDB, incompleteReader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    false,
	})
	assert.Error(t, err)
	// 这个错误应该是读取知识库名称时的 EOF 错误
	assert.Contains(t, err.Error(), "read knowledge base name")
}

func TestKnowledgeBaseImportWithCustomName(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建测试知识库
	originalKbName := utils.RandStringBytes(10)
	kbInfo, err := createTestKnowledgeBase(testDB, originalKbName)
	assert.NoError(t, err)

	// 创建测试知识库条目
	_, err = createTestKnowledgeBaseEntries(testDB, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 创建测试RAG数据
	err = createTestRAGData(testDB, originalKbName)
	assert.NoError(t, err)

	// 导出知识库
	ctx := context.Background()
	exportData, err := ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: int64(kbInfo.ID),
	})
	assert.NoError(t, err)

	// 将导出数据读取到内存中
	exportBytes, err := io.ReadAll(exportData)
	assert.NoError(t, err)

	// 创建新的测试数据库用于导入
	newTestDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 使用自定义名称导入知识库
	customKbName := "custom_" + utils.RandStringBytes(8)
	reader := bytes.NewReader(exportBytes)
	err = ImportKnowledgeBase(ctx, newTestDB, reader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: customKbName,
		OverwriteExisting:    false,
	})
	assert.NoError(t, err)

	// 验证知识库信息使用了自定义名称
	importedKBInfo, err := yakit.GetKnowledgeBaseByName(newTestDB, customKbName)
	assert.NoError(t, err)
	assert.NotNil(t, importedKBInfo)
	assert.Equal(t, customKbName, importedKBInfo.KnowledgeBaseName)

	// 验证原始名称的知识库不存在
	_, err = yakit.GetKnowledgeBaseByName(newTestDB, originalKbName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// 验证知识库条目正确导入
	var importedEntries []*schema.KnowledgeBaseEntry
	err = newTestDB.Where("knowledge_base_id = ?", importedKBInfo.ID).Find(&importedEntries).Error
	assert.NoError(t, err)
	assert.Len(t, importedEntries, 3) // 应该有3个测试条目

	// 验证RAG数据也使用了自定义名称
	var collections []schema.VectorStoreCollection
	err = newTestDB.Where("name = ?", customKbName).Find(&collections).Error
	assert.NoError(t, err)
	assert.Len(t, collections, 1, "应该有一个使用自定义名称的向量集合")

	// 验证原始名称的集合不存在
	var originalCollections []schema.VectorStoreCollection
	err = newTestDB.Where("name = ?", originalKbName).Find(&originalCollections).Error
	assert.NoError(t, err)
	assert.Len(t, originalCollections, 0, "不应该有使用原始名称的向量集合")

	t.Logf("成功使用自定义名称 '%s' 导入知识库（原名称: '%s'）", customKbName, originalKbName)
}

func TestKnowledgeBaseImportWithEmptyName(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建测试知识库
	originalKbName := utils.RandStringBytes(10)
	kbInfo, err := createTestKnowledgeBase(testDB, originalKbName)
	assert.NoError(t, err)

	// 创建测试知识库条目
	_, err = createTestKnowledgeBaseEntries(testDB, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 创建测试RAG数据
	err = createTestRAGData(testDB, originalKbName)
	assert.NoError(t, err)

	// 导出知识库
	ctx := context.Background()
	exportData, err := ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: int64(kbInfo.ID),
	})
	assert.NoError(t, err)

	// 将导出数据读取到内存中
	exportBytes, err := io.ReadAll(exportData)
	assert.NoError(t, err)

	// 创建新的测试数据库用于导入
	newTestDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 使用空名称导入知识库（应该使用原始名称）
	reader := bytes.NewReader(exportBytes)
	err = ImportKnowledgeBase(ctx, newTestDB, reader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    false,
	})
	assert.NoError(t, err)

	// 验证知识库信息使用了原始名称
	importedKBInfo, err := yakit.GetKnowledgeBaseByName(newTestDB, originalKbName)
	assert.NoError(t, err)
	assert.NotNil(t, importedKBInfo)
	assert.Equal(t, originalKbName, importedKBInfo.KnowledgeBaseName)

	// 验证RAG数据也使用了原始名称
	var collections []schema.VectorStoreCollection
	err = newTestDB.Where("name = ?", originalKbName).Find(&collections).Error
	assert.NoError(t, err)
	assert.Len(t, collections, 1, "应该有一个使用原始名称的向量集合")

	t.Logf("成功使用原始名称 '%s' 导入知识库", originalKbName)
}

func TestKnowledgeBaseProgressCallback(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 创建测试知识库
	kbName := utils.RandStringBytes(10)
	kbInfo, err := createTestKnowledgeBase(testDB, kbName)
	assert.NoError(t, err)

	// 创建测试知识库条目
	_, err = createTestKnowledgeBaseEntries(testDB, int64(kbInfo.ID))
	assert.NoError(t, err)

	// 创建测试RAG数据
	err = createTestRAGData(testDB, kbName)
	assert.NoError(t, err)

	// 测试导出进度回调
	var exportProgressCalls []float64
	var exportMessages []string

	ctx := context.Background()
	exportData, err := ExportKnowledgeBase(ctx, testDB, &ExportKnowledgeBaseOptions{
		KnowledgeBaseId: int64(kbInfo.ID),
		OnProgressHandler: func(percent float64, message string, messageType string) {
			exportProgressCalls = append(exportProgressCalls, percent)
			exportMessages = append(exportMessages, message)
			t.Logf("Export: %.1f%% - %s (%s)", percent, message, messageType)
		},
	})
	assert.NoError(t, err)

	// 验证导出进度
	assert.Greater(t, len(exportProgressCalls), 0, "应该有导出进度回调")
	assert.Equal(t, 0.0, exportProgressCalls[0], "第一个进度应该是0%")
	assert.Equal(t, 100.0, exportProgressCalls[len(exportProgressCalls)-1], "最后一个进度应该是100%")

	// 验证进度是递增的
	for i := 1; i < len(exportProgressCalls); i++ {
		assert.GreaterOrEqual(t, exportProgressCalls[i], exportProgressCalls[i-1],
			fmt.Sprintf("进度应该递增: %f >= %f", exportProgressCalls[i], exportProgressCalls[i-1]))
	}

	// 将导出数据读取到内存中
	exportBytes, err := io.ReadAll(exportData)
	assert.NoError(t, err)

	// 创建新的测试数据库用于导入
	newTestDB, err := createTempTestDatabase()
	assert.NoError(t, err)

	// 测试导入进度回调
	var importProgressCalls []float64
	var importMessages []string

	reader := bytes.NewReader(exportBytes)
	err = ImportKnowledgeBase(ctx, newTestDB, reader, &ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: "",
		OverwriteExisting:    false,
		OnProgressHandler: func(percent float64, message string, messageType string) {
			importProgressCalls = append(importProgressCalls, percent)
			importMessages = append(importMessages, message)
			t.Logf("Import: %.1f%% - %s (%s)", percent, message, messageType)
		},
	})
	assert.NoError(t, err)

	// 验证导入进度
	assert.Greater(t, len(importProgressCalls), 0, "应该有导入进度回调")
	assert.Equal(t, 0.0, importProgressCalls[0], "第一个进度应该是0%")
	assert.Equal(t, 100.0, importProgressCalls[len(importProgressCalls)-1], "最后一个进度应该是100%")

	// 验证进度是递增的
	for i := 1; i < len(importProgressCalls); i++ {
		assert.GreaterOrEqual(t, importProgressCalls[i], importProgressCalls[i-1],
			fmt.Sprintf("进度应该递增: %f >= %f", importProgressCalls[i], importProgressCalls[i-1]))
	}

	// 验证进度消息包含关键阶段
	exportMessageStr := strings.Join(exportMessages, " ")
	assert.Contains(t, exportMessageStr, "开始导出", "应该包含开始导出消息")
	assert.Contains(t, exportMessageStr, "导出完成", "应该包含导出完成消息")

	importMessageStr := strings.Join(importMessages, " ")
	assert.Contains(t, importMessageStr, "开始导入", "应该包含开始导入消息")
	assert.Contains(t, importMessageStr, "导入完成", "应该包含导入完成消息")

	t.Logf("导出进度回调次数: %d, 导入进度回调次数: %d", len(exportProgressCalls), len(importProgressCalls))
}
