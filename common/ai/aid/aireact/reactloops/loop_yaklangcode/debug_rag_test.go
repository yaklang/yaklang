package loop_yaklangcode

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// TestDebugRAGCollection 调试 RAG 集合状态
func TestDebugRAGCollection(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName

	var collection schema.VectorStoreCollection
	err := db.Where("name = ?", collectionName).First(&collection).Error
	if err != nil {
		log.Errorf("collection not found: %v", err)
		t.Fatalf("collection not found: %v", err)
	}

	log.Infof("=== Collection Debug Info ===")
	log.Infof("ID: %d", collection.ID)
	log.Infof("Name: %s", collection.Name)
	log.Infof("RAGID: %s", collection.RAGID)
	log.Infof("GraphBinary length: %d bytes", len(collection.GraphBinary))
	log.Infof("CodeBookBinary length: %d bytes", len(collection.CodeBookBinary))
	log.Infof("SerialVersionUID: %s", collection.SerialVersionUID)

	// 检查文档数量
	var docCount int64
	err = db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Count(&docCount).Error
	if err != nil {
		log.Errorf("failed to count documents: %v", err)
	} else {
		log.Infof("Document count: %d", docCount)
	}

	if len(collection.GraphBinary) == 0 {
		log.Warnf("⚠️  GraphBinary is empty! This is why loading fails.")
	} else {
		log.Infof("✅ GraphBinary exists")
	}
}
