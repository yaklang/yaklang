package loop_yaklangcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// TestCleanupOldRAGData 测试清理旧的 RAG 数据
func TestCleanupOldRAGData(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName

	log.Infof("attempting to delete old RAG collection: %s", collectionName)

	// 检查集合是否存在
	exists := vectorstore.HasCollection(db, collectionName)
	if !exists {
		log.Infof("RAG collection '%s' does not exist, nothing to delete", collectionName)
		return
	}

	log.Infof("RAG collection '%s' exists, deleting...", collectionName)

	// 删除旧的集合数据
	err := vectorstore.DeleteCollection(db, collectionName)
	if err != nil {
		log.Errorf("failed to delete old collection: %v", err)
		t.Fatalf("failed to delete old collection: %v", err)
	}

	log.Infof("successfully deleted old RAG collection: %s", collectionName)

	// 验证集合已被删除
	exists = vectorstore.HasCollection(db, collectionName)
	assert.False(t, exists, "collection should not exist after deletion")

	log.Infof("old RAG data cleanup completed successfully")
}

// TestRAGSystemInitialization 测试 RAG 系统初始化
func TestRAGSystemInitialization(t *testing.T) {
	// 跳过此测试，因为需要实际的 RAG 文件
	t.Skip("skipping RAG system initialization test - requires actual RAG file")

	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName

	// 这里可以测试实际的初始化逻辑
	ragSystem, err := createDocumentSearcherByRag(db, collectionName, "")
	assert.NoError(t, err)
	assert.NotNil(t, ragSystem)
}

// TestForceReimportRAGData 测试强制重新导入 RAG 数据
func TestForceReimportRAGData(t *testing.T) {
	// 跳过此测试，因为需要实际的 RAG 文件
	t.Skip("skipping force reimport test - requires actual RAG file")

	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"

	// 强制重新导入
	err := rag.ImportRAG(aikbPath,
		rag.WithRAGCollectionName(collectionName),
		rag.WithDB(db),
		rag.WithExportOverwriteExisting(true),
	)
	assert.NoError(t, err)

	// 验证导入成功
	exists := vectorstore.HasCollection(db, collectionName)
	assert.True(t, exists, "collection should exist after import")

	log.Infof("force reimport completed successfully")
}

// TestRAGSystemLoadWithAutoRecovery 测试 RAG 系统加载（包含自动恢复）
func TestRAGSystemLoadWithAutoRecovery(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"

	log.Infof("=== Testing RAG System Load with Auto Recovery ===")

	// 测试 createDocumentSearcherByRag 函数（包含自动恢复逻辑）
	ragSystem, err := createDocumentSearcherByRag(db, collectionName, aikbPath)

	// 验证加载成功
	assert.NoError(t, err, "RAG system should load successfully (with auto recovery if needed)")
	assert.NotNil(t, ragSystem, "RAG system should not be nil")

	if ragSystem != nil {
		// 验证可以查询
		log.Infof("testing RAG system query functionality")
		results, queryErr := ragSystem.QueryTopN("Yaklang中如何发送HTTP请求？", 5, 0.3)
		assert.NoError(t, queryErr, "query should succeed")
		log.Infof("query returned %d results", len(results))

		// 验证集合存在
		exists := vectorstore.HasCollection(db, collectionName)
		assert.True(t, exists, "collection should exist after successful load")

		log.Infof("=== RAG System Load Test PASSED ===")
	}
}
