package vectorstore

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// embedding2DContentPreviewLen 限制 ContentPreview 的最大字符数，避免大文档拖垮返回体
const embedding2DContentPreviewLen = 100

// Embedding2DPoint 表示集合内一个文档向量降维后的二维坐标点，
// 携带文档标识、类型与内容预览，便于绘图侧做分组着色与悬停标注。
type Embedding2DPoint struct {
	// 文档 ID（集合内唯一）
	DocumentID string `json:"documentID"`
	// 文档类型（schema.RAGDocumentType）
	Type string `json:"type"`
	// PCA 降维后的横坐标
	X float32 `json:"x"`
	// PCA 降维后的纵坐标
	Y float32 `json:"y"`
	// 文档内容预览（换行折叠为空格并截断），可用作悬停提示
	ContentPreview string `json:"contentPreview"`
}

// Collection2DPoints 读取指定集合的全部文档向量，通过 PCA 统一降维为二维点。
//
// 只读取数据库中已持久化的向量，不初始化 embedding 客户端（不访问远端/本地
// embedding 服务），也不加载 HNSW 图，适合 embedding 分布可视化等离线分析场景。
// 输出顺序与内部文档查询顺序一致，坐标由固定随机种子的 PCA 计算，可复现。
func Collection2DPoints(db *gorm.DB, collectionName string) ([]*Embedding2DPoint, error) {
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, utils.Errorf("rag collection %s not found", collectionName)
		}
		return nil, utils.Wrap(err, "query rag collection")
	}
	if collection == nil {
		return nil, utils.Errorf("rag collection %s not found", collectionName)
	}

	var docs []schema.VectorStoreDocument
	if err := db.Where("collection_id = ?", collection.ID).
		Where("document_id <> ?", DocumentTypeCollectionInfo).
		Find(&docs).Error; err != nil {
		return nil, utils.Wrap(err, "query collection documents")
	}

	vectors := make([][]float32, 0, len(docs))
	kept := make([]*schema.VectorStoreDocument, 0, len(docs))
	for i := range docs {
		if len(docs[i].Embedding) == 0 {
			continue
		}
		vectors = append(vectors, []float32(docs[i].Embedding))
		kept = append(kept, &docs[i])
	}
	if len(vectors) == 0 {
		return []*Embedding2DPoint{}, nil
	}

	coords, err := embedding.ReduceTo2D(vectors)
	if err != nil {
		return nil, utils.Wrap(err, "reduce embeddings to 2d")
	}

	points := make([]*Embedding2DPoint, len(kept))
	for i, doc := range kept {
		points[i] = &Embedding2DPoint{
			DocumentID:     doc.DocumentID,
			Type:           string(doc.DocumentType),
			X:              coords[i][0],
			Y:              coords[i][1],
			ContentPreview: truncateContentPreview(doc.Content),
		}
	}
	return points, nil
}

// truncateContentPreview 折叠连续空白（含换行）并截断内容预览
func truncateContentPreview(content string) string {
	preview := strings.Join(strings.Fields(content), " ")
	runes := []rune(preview)
	if len(runes) > embedding2DContentPreviewLen {
		return string(runes[:embedding2DContentPreviewLen]) + "..."
	}
	return preview
}
