package loop_yaklangcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// TestDirectImportFromRealFile 直接测试从真实文件导入
func TestDirectImportFromRealFile(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := "test_real_import_" + defaultYaklangAIKBRagCollectionName
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"

	log.Infof("=== Testing Direct Import from Real File ===")

	// 先删除旧数据（如果存在）
	if vectorstore.HasCollection(db, collectionName) {
		log.Infof("deleting existing collection: %s", collectionName)
		err := rag.DeleteRAG(db, collectionName)
		if err != nil {
			log.Warnf("failed to delete existing collection: %v", err)
		}
	}

	// 直接导入（使用文件中的 HNSW 索引）
	log.Infof("importing RAG data from file: %s", aikbPath)
	err := rag.ImportRAG(aikbPath,
		rag.WithRAGCollectionName(collectionName),
		rag.WithDB(db),
		rag.WithExportOverwriteExisting(true),
		// 不重建索引，直接使用文件中的索引
	)

	// 验证导入结果
	assert.NoError(t, err, "import should succeed")

	if err == nil {
		// 验证集合存在
		exists := vectorstore.HasCollection(db, collectionName)
		assert.True(t, exists, "collection should exist after import")

		// 尝试加载 RAG 系统
		ragSystem, loadErr := rag.Get(collectionName, rag.WithDB(db), rag.WithLazyLoadEmbeddingClient(true))
		assert.NoError(t, loadErr, "should be able to load RAG system after import")
		assert.NotNil(t, ragSystem, "RAG system should not be nil")

		if ragSystem != nil {
			// 验证可以查询
			results, queryErr := ragSystem.QueryTopN("Yaklang中如何发送HTTP请求？", 5, 0.3)
			assert.NoError(t, queryErr, "query should succeed")
			log.Infof("query returned %d results", len(results))
		}

		log.Infof("=== Direct Import Test PASSED ===")

		// 清理测试数据
		log.Infof("cleaning up test data")
		rag.DeleteRAG(db, collectionName)
	}
}
