package loop_yaklangcode

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TestDebugHNSWGraph 调试 HNSW 图中的节点 ID
func TestDebugHNSWGraph(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	collectionName := defaultYaklangAIKBRagCollectionName

	// 获取集合信息
	collection, err := yakit.GetRAGCollectionInfoByName(db, collectionName)
	if err != nil {
		t.Fatalf("failed to get collection: %v", err)
	}

	log.Infof("collection: %s, GraphBinary len: %d", collection.Name, len(collection.GraphBinary))

	// 解析 HNSW 图
	graphReader := bytes.NewReader(collection.GraphBinary)
	pers, err := hnsw.LoadBinary[string](graphReader)
	if err != nil {
		t.Fatalf("failed to load binary: %v", err)
	}

	log.Infof("HNSW graph: Total=%d, Dims=%d, ExportMode=%d", pers.Total, pers.Dims, pers.ExportMode)
	log.Infof("Layers: %d", len(pers.Layers))
	if len(pers.Layers) > 0 {
		log.Infof("Layer 0 nodes: %d", len(pers.Layers[0].Nodes))
	}
	log.Infof("OffsetToKey: %d", len(pers.OffsetToKey))

	// 检查前几个节点的 Key 和 Code（UID）
	for i := 1; i < 6 && i < len(pers.OffsetToKey); i++ {
		node := pers.OffsetToKey[i]
		log.Infof("node %d: Key=%s, Code type=%T", i, node.Key, node.Code)
		if uid, ok := node.Code.([]byte); ok {
			log.Infof("  UID=%x (len=%d)", uid, len(uid))

			// 计算期望的 UID
			expectedUID := vectorstore.GetLazyNodeUIDByMd5(collection.Name, node.Key)
			log.Infof("  Expected UID=%x (len=%d)", expectedUID, len(expectedUID))

			if !bytes.Equal(uid, expectedUID) {
				log.Errorf("  UID MISMATCH!")
			} else {
				log.Infof("  UID matches!")
			}
		}
	}
}
