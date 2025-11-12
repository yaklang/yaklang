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

// TestDebugVectorQuery 测试向量查询是否正常
func TestDebugVectorQuery(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName
	aikbPath := "/Users/v1ll4n/yakit-projects/projects/libs/yaklang-aikb.rag"
	
	log.Infof("=== Step 1: Import data (no rebuild) ===")
	
	// 删除现有集合
	if vectorstore.HasCollection(db, collectionName) {
		vectorstore.DeleteCollection(db, collectionName)
	}
	
	// 导入（不重建 HNSW）
	err := rag.ImportRAG(aikbPath,
		rag.WithRAGCollectionName(collectionName),
		rag.WithDB(db),
		rag.WithExportOverwriteExisting(true),
		rag.WithImportRebuildHNSWIndex(false), // 不重建
	)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	
	log.Infof("=== Step 2: Check documents ===")
	collection, err := yakit.GetRAGCollectionInfoByName(db, collectionName)
	if err != nil {
		t.Fatalf("failed to get collection: %v", err)
	}
	
	// 获取几个文档
	var docs []schema.VectorStoreDocument
	db.Where("collection_id = ?", collection.ID).Limit(5).Find(&docs)
	
	for i, doc := range docs {
		log.Infof("doc %d: DocumentID=%s", i, doc.DocumentID)
		log.Infof("  UID=%x (len=%d)", doc.UID, len(doc.UID))
		log.Infof("  Embedding len=%d", len(doc.Embedding))
		
		// 尝试通过 UID 查询
		var queryDoc schema.VectorStoreDocument
		err := db.Where("uid = ?", doc.UID).First(&queryDoc).Error
		if err != nil {
			log.Errorf("  Query by UID failed: %v", err)
		} else {
			log.Infof("  Query by UID succeeded: DocumentID=%s, Embedding len=%d", 
				queryDoc.DocumentID, len(queryDoc.Embedding))
		}
		
		// 尝试通过 DocumentID 查询
		var queryDoc2 schema.VectorStoreDocument
		err = db.Where("document_id = ?", doc.DocumentID).First(&queryDoc2).Error
		if err != nil {
			log.Errorf("  Query by DocumentID failed: %v", err)
		} else {
			log.Infof("  Query by DocumentID succeeded: UID=%x, Embedding len=%d", 
				queryDoc2.UID, len(queryDoc2.Embedding))
		}
	}
}

