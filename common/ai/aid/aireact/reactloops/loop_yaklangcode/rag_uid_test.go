package loop_yaklangcode

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// TestCheckDocumentUIDs 检查导入后的文档 UID
func TestCheckDocumentUIDs(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := "test_debug_import"
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"

	log.Infof("=== Checking Document UIDs ===")

	// 先读取文件头，获取原始集合名称
	file, err := os.Open(aikbPath)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	header, err := rag.LoadRAGFileHeader(file)
	if err != nil {
		t.Fatalf("failed to load header: %v", err)
	}

	log.Infof("Original collection name: %s", header.Collection.Name)
	log.Infof("Target collection name: %s", collectionName)

	// 查询数据库中的文档
	var docs []schema.VectorStoreDocument
	err = db.Model(&schema.VectorStoreDocument{}).
		Where("collection_id IN (SELECT id FROM vector_store_collections WHERE name = ?)", collectionName).
		Limit(5).
		Find(&docs).Error

	if err != nil {
		t.Fatalf("failed to query documents: %v", err)
	}

	log.Infof("Found %d documents in database", len(docs))
	for i, doc := range docs {
		log.Infof("Document %d:", i+1)
		log.Infof("  DocumentID: %s", doc.DocumentID)
		log.Infof("  UID: %x", doc.UID)
		log.Infof("  Content (first 50 chars): %s", truncate(doc.Content, 50))
	}

	// 查询集合信息
	var collection schema.VectorStoreCollection
	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).First(&collection).Error
	if err != nil {
		t.Fatalf("failed to query collection: %v", err)
	}

	log.Infof("Collection info:")
	log.Infof("  Name: %s", collection.Name)
	log.Infof("  GraphBinary length: %d", len(collection.GraphBinary))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

