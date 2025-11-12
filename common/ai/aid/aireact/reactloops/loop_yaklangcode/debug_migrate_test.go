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

// TestDebugMigrateHNSW 调试 HNSW 图迁移过程
func TestDebugMigrateHNSW(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"
	
	log.Infof("=== Step 1: Delete and reimport with rebuild ===")
	
	// 删除现有集合
	if vectorstore.HasCollection(db, collectionName) {
		err := vectorstore.DeleteCollection(db, collectionName)
		if err != nil {
			t.Fatalf("failed to delete collection: %v", err)
		}
	}
	
	// 导入并重建 HNSW 索引
	log.Infof("importing with rebuild HNSW index")
	err := rag.ImportRAG(aikbPath,
		rag.WithRAGCollectionName(collectionName),
		rag.WithDB(db),
		rag.WithExportOverwriteExisting(true),
		rag.WithImportRebuildHNSWIndex(true),
	)
	
	if err != nil {
		log.Errorf("import failed: %v", err)
		
		// 检查导入后的状态
		log.Infof("=== Checking import status ===")
		if vectorstore.HasCollection(db, collectionName) {
			collection, err := yakit.GetRAGCollectionInfoByName(db, collectionName)
			if err != nil {
				t.Fatalf("failed to get collection: %v", err)
			}
			
			var docCount int64
			db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Count(&docCount)
			log.Infof("collection exists, documents: %d, GraphBinary len: %d", docCount, len(collection.GraphBinary))
			
			// 检查前几个文档
			var docs []schema.VectorStoreDocument
			db.Where("collection_id = ?", collection.ID).Limit(3).Find(&docs)
			for i, doc := range docs {
				log.Infof("doc %d: DocumentID=%s, UID=%x, Embedding len=%d", 
					i, doc.DocumentID, doc.UID, len(doc.Embedding))
			}
		}
		
		t.Fatalf("import failed: %v", err)
	}
	
	log.Infof("import succeeded!")
	
	// 验证可以加载
	log.Infof("=== Step 2: Try to load RAG system ===")
	ragSystem, err := rag.Get(collectionName, rag.WithDB(db))
	if err != nil {
		t.Fatalf("failed to load RAG system: %v", err)
	}
	
	log.Infof("RAG system loaded successfully!")
	
	// 测试查询
	results, err := ragSystem.QueryTopN("Yaklang中如何发送HTTP请求？", 5, 0.3)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	log.Infof("query returned %d results", len(results))
}

