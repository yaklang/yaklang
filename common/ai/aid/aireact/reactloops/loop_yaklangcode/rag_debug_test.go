package loop_yaklangcode

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// TestDebugImportProcess 调试导入过程
func TestDebugImportProcess(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := "test_debug_import"
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"

	log.Infof("=== Debugging Import Process ===")

	// 先删除旧数据（如果存在）
	if vectorstore.HasCollection(db, collectionName) {
		log.Infof("deleting existing collection: %s", collectionName)
		err := rag.DeleteRAG(db, collectionName)
		if err != nil {
			log.Warnf("failed to delete existing collection: %v", err)
		}
	}

	// 读取文件头信息
	file, err := os.Open(aikbPath)
	assert.NoError(t, err, "should be able to open file")
	defer file.Close()

	header, err := rag.LoadRAGFileHeader(file)
	assert.NoError(t, err, "should be able to load header")
	log.Infof("RAG file header loaded:")
	log.Infof("  Collection Name: %s", header.Collection.Name)
	log.Infof("  Collection Dimension: %d", header.Collection.Dimension)
	log.Infof("  Collection Model: %s", header.Collection.ModelName)
	log.Infof("  Serial Version UID: %s", header.Collection.SerialVersionUID)

	// 使用自定义进度回调来监控导入过程
	progressHandler := func(percent float64, message string, messageType string) {
		log.Infof("[Progress %.0f%%] %s (%s)", percent, message, messageType)
	}

	// 导入数据（直接使用文件中的 HNSW 索引，不重建）
	log.Infof("starting import (using existing HNSW index from file)")
	err = rag.ImportRAG(aikbPath,
		rag.WithRAGCollectionName(collectionName),
		rag.WithDB(db),
		rag.WithExportOverwriteExisting(true),
		// 不使用 WithImportRebuildHNSWIndex，避免 "unsupported node code type" 错误
		rag.WithExportOnProgressHandler(progressHandler),
	)

	if err != nil {
		log.Errorf("import failed: %v", err)

		// 检查集合是否存在
		if vectorstore.HasCollection(db, collectionName) {
			log.Infof("collection exists, checking document count")
			var count int64
			db.Model(&schema.VectorStoreDocument{}).Where("collection_id IN (SELECT id FROM vector_store_collections WHERE name = ?)", collectionName).Count(&count)
			log.Infof("document count in database: %d", count)
		}
	}

	assert.NoError(t, err, "import should succeed")

	// 清理
	if vectorstore.HasCollection(db, collectionName) {
		rag.DeleteRAG(db, collectionName)
	}
}
