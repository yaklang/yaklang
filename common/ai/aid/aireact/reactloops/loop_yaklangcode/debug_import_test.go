package loop_yaklangcode

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TestDebugImportProcess 调试导入过程
func TestDebugImportProcess(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"

	log.Infof("=== Step 1: Delete existing collection ===")
	if vectorstore.HasCollection(db, collectionName) {
		err := vectorstore.DeleteCollection(db, collectionName)
		if err != nil {
			t.Fatalf("failed to delete collection: %v", err)
		}
		log.Infof("collection deleted")
	}

	log.Infof("=== Step 2: Import RAG data ===")
	err := rag.ImportRAG(aikbPath,
		rag.WithRAGCollectionName(collectionName),
		rag.WithDB(db),
		rag.WithExportOverwriteExisting(true),
	)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	log.Infof("import succeeded")

	log.Infof("=== Step 3: Check imported data ===")
	// 检查集合是否存在
	exists := vectorstore.HasCollection(db, collectionName)
	log.Infof("collection exists: %v", exists)

	// 获取集合信息
	collection, err := yakit.GetRAGCollectionInfoByName(db, collectionName)
	if err != nil {
		t.Fatalf("failed to get collection: %v", err)
	}
	log.Infof("collection ID: %d, UUID: %s", collection.ID, collection.UUID)
	log.Infof("GraphBinary length: %d bytes", len(collection.GraphBinary))

	// 检查文档数量
	var docCount int64
	db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Count(&docCount)
	log.Infof("document count: %d", docCount)

	// 检查前几个文档的 UID 和 embedding
	var docs []schema.VectorStoreDocument
	db.Where("collection_id = ?", collection.ID).Limit(5).Find(&docs)
	for i, doc := range docs {
		log.Infof("doc %d: ID=%d, DocumentID=%s, UID=%x (len=%d), Embedding len=%d",
			i, doc.ID, doc.DocumentID, doc.UID, len(doc.UID), len(doc.Embedding))
	}

	log.Infof("=== Step 4: Try to load RAG system ===")
	ragSystem, err := rag.Get(collectionName, rag.WithDB(db))
	if err != nil {
		log.Errorf("failed to load RAG system: %v", err)
		t.Fatalf("load failed: %v", err)
	}
	log.Infof("RAG system loaded successfully!")

	// 尝试查询
	results, err := ragSystem.QueryTopN("Yaklang中如何发送HTTP请求？", 5, 0.3)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	log.Infof("query returned %d results", len(results))
}
