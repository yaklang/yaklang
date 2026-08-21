package yak

import (
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
)

// _collection2DPoints 获取指定集合所有文档向量的二维投影（PCA 降维）
//
// 输入集合名称，读取该集合已持久化的全部向量，统一降维为二维点后返回。
// 只读取数据库中已存储的向量，不会调用远端/本地 embedding 服务。
//
// 参数:
//   - collectionName: 集合名称
//   - opts: 可选查询选项（如 rag.dbQueryDB 指定数据库连接）
//
// 返回值:
//   - 二维点列表，每个点包含 documentID / type / x / y / contentPreview 字段
//   - 错误信息（集合不存在时返回错误）
//
// Example:
// ```
//
//	points = rag.Collection2DPoints("my-collection")~
//	for _, p in points {
//	    println(p.documentID, p.x, p.y, p.contentPreview)
//	}
//
//	// 指定数据库连接
//	points = rag.Collection2DPoints("my-collection", rag.dbQueryDB(db))~
//
// ```
func _collection2DPoints(collectionName string, opts ...DBQueryOption) ([]*vectorstore.Embedding2DPoint, error) {
	config := NewDBQueryConfig(opts...)
	return vectorstore.Collection2DPoints(config.db, collectionName)
}
