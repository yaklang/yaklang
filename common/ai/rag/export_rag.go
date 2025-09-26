package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"google.golang.org/protobuf/encoding/protowire"
)

// RAGBinaryData 简化的RAG二进制数据结构（仅用于导入）
type RAGBinaryData struct {
	Collection *schema.VectorStoreCollection
	Documents  []*ExportVectorStoreDocument
}

// RAGExportConfig 导出选项
type RAGExportConfig struct {
	IncludeHNSWIndex bool // 是否包含HNSW索引
	OnlyPQCode       bool // 是否只导出PQ编码
	NoMetadata       bool // 是否不导出元数据
}

// RAGImportConfig 导入选项
type RAGImportConfig struct {
	OverwriteExisting bool   // 是否覆盖现有数据
	CollectionName    string // 指定集合名称（可选）
}

type ExportOptionFunc func(*RAGExportConfig)

func WithExportNoMetadata(b bool) ExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.NoMetadata = b
	}
}

func WithExportOnlyPQCode(b bool) ExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.OnlyPQCode = b
	}
}

func WithExportIncludeHNSWIndex(b bool) ExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.IncludeHNSWIndex = b
	}
}

func NewExportOptions(opts ...ExportOptionFunc) *RAGExportConfig {
	config := &RAGExportConfig{
		IncludeHNSWIndex: true,
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// ExportRAGToBinary 导出RAG数据为二进制格式
func ExportRAGToBinary(ctx context.Context, db *gorm.DB, collectionName string, opts ...ExportOptionFunc) (io.Reader, error) {
	cfg := NewExportOptions(opts...)
	buf := new(bytes.Buffer)
	// 查询集合信息
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return nil, utils.Errorf("failed to get collection %s: %v", collectionName, err)
	}
	if collection == nil {
		return nil, utils.Errorf("collection %s not found", collectionName)
	}

	// 写入魔数头和版本号
	if _, err := buf.WriteString("YAKRAG"); err != nil {
		return nil, utils.Wrap(err, "failed to write magic header")
	}
	if err := pbWriteUint32(buf, 1); err != nil {
		return nil, utils.Wrap(err, "failed to write version")
	}

	// 写入集合信息
	if err := writeCollectionToBinary(buf, collection); err != nil {
		return nil, utils.Wrap(err, "failed to write collection")
	}

	// 统计文档总数
	var totalDocs int64
	err = db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Count(&totalDocs).Error
	if err != nil {
		return nil, utils.Wrap(err, "failed to count documents")
	}

	// 写入文档总数
	if err := pbWriteVarint(buf, uint64(totalDocs)); err != nil {
		return nil, utils.Wrap(err, "failed to write documents count")
	}

	// 分页导出文档数据
	const pageSize = 100
	page := 1

	for {
		var documents []*schema.VectorStoreDocument

		// 使用bizhelper.Paging分页查询
		_, paginatedDB := bizhelper.Paging(
			db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID),
			page, pageSize, &documents,
		)

		if paginatedDB.Error != nil {
			return nil, utils.Errorf("failed to query documents page %d: %v", page, paginatedDB.Error)
		}

		// 如果没有更多数据，跳出循环
		if len(documents) == 0 {
			break
		}

		// 逐个写入文档数据
		for _, doc := range documents {
			if err := writeDocumentToBinary(buf, doc); err != nil {
				return nil, utils.Wrapf(err, "failed to write document %s", doc.DocumentID)
			}
		}

		// 如果当前页数据少于pageSize，说明已经是最后一页
		if len(documents) < pageSize {
			break
		}

		page++
	}

	// 导出并写入HNSW索引
	if cfg.IncludeHNSWIndex {
		hnswGraphBinary := collection.GraphBinary
		// 写入HNSW索引长度
		if err := pbWriteVarint(buf, uint64(len(hnswGraphBinary))); err != nil {
			return nil, utils.Wrap(err, "failed to write hnsw index length")
		}
		// 写入HNSW索引数据
		if len(hnswGraphBinary) > 0 {
			if _, err := buf.Write(hnswGraphBinary); err != nil {
				return nil, utils.Wrap(err, "failed to write hnsw index data")
			}
		}
	} else {
		// 不包含HNSW索引，写入0长度
		if err := pbWriteVarint(buf, 0); err != nil {
			return nil, utils.Wrap(err, "failed to write empty hnsw index length")
		}
	}

	return buf, nil
}

// writeCollectionToBinary 写入集合信息到二进制文件
func writeCollectionToBinary(writer io.Writer, collection *schema.VectorStoreCollection) error {
	// 集合名称
	if err := pbWriteBytes(writer, []byte(collection.Name)); err != nil {
		return utils.Wrap(err, "write collection name")
	}

	// 集合描述
	if err := pbWriteBytes(writer, []byte(collection.Description)); err != nil {
		return utils.Wrap(err, "write collection description")
	}

	// 模型名称
	if err := pbWriteBytes(writer, []byte(collection.ModelName)); err != nil {
		return utils.Wrap(err, "write model name")
	}

	// 维度
	if err := pbWriteUint32(writer, uint32(collection.Dimension)); err != nil {
		return utils.Wrap(err, "write dimension")
	}

	// HNSW参数
	if err := pbWriteUint32(writer, uint32(collection.M)); err != nil {
		return utils.Wrap(err, "write m")
	}

	if err := pbWriteFloat32(writer, float32(collection.Ml)); err != nil {
		return utils.Wrap(err, "write ml")
	}

	if err := pbWriteUint32(writer, uint32(collection.EfSearch)); err != nil {
		return utils.Wrap(err, "write ef_search")
	}

	if err := pbWriteUint32(writer, uint32(collection.EfConstruct)); err != nil {
		return utils.Wrap(err, "write ef_construct")
	}

	// 距离函数类型
	if err := pbWriteBytes(writer, []byte(collection.DistanceFuncType)); err != nil {
		return utils.Wrap(err, "write distance func type")
	}

	// EnablePQMode
	if err := pbWriteBool(writer, collection.EnablePQMode); err != nil {
		return utils.Wrap(err, "write enable pq mode")
	}

	// CodeBookBinary
	if err := pbWriteBytes(writer, collection.CodeBookBinary); err != nil {
		return utils.Wrap(err, "write code book binary")
	}

	// UUID
	if err := pbWriteBytes(writer, []byte(collection.UUID)); err != nil {
		return utils.Wrap(err, "write uuid")
	}

	// GraphBinary
	if err := pbWriteBytes(writer, collection.GraphBinary); err != nil {
		return utils.Wrap(err, "write graph binary")
	}

	// PQ编码
	if err := pbWriteBytes(writer, collection.CodeBookBinary); err != nil {
		return utils.Wrap(err, "write code book binary")
	}

	return nil
}

// writeDocumentToBinary 写入单个文档到二进制文件
func writeDocumentToBinary(writer io.Writer, doc *schema.VectorStoreDocument) error {
	// 文档ID
	if err := pbWriteBytes(writer, []byte(doc.DocumentID)); err != nil {
		return utils.Wrap(err, "write document id")
	}

	// 元数据 (JSON序列化)
	metadataBytes, err := json.Marshal(doc.Metadata)
	if err != nil {
		return utils.Wrap(err, "marshal metadata")
	}
	if err := pbWriteBytes(writer, metadataBytes); err != nil {
		return utils.Wrap(err, "write metadata")
	}

	// 向量数据
	if err := pbWriteVarint(writer, uint64(len(doc.Embedding))); err != nil {
		return utils.Wrap(err, "write embedding length")
	}
	for _, val := range doc.Embedding {
		if err := pbWriteFloat32(writer, val); err != nil {
			return utils.Wrap(err, "write embedding value")
		}
	}

	return nil
}

// LoadRAGFromBinary 从二进制数据流式加载RAG格式
func LoadRAGFromBinary(reader io.Reader) (*RAGBinaryData, error) {
	// 读取魔数头
	magic := make([]byte, 6)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return nil, utils.Wrap(err, "read magic header")
	}
	if string(magic) != "YAKRAG" {
		return nil, utils.Error("invalid magic header")
	}

	// 读取版本号
	version, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read version")
	}
	if version != 1 {
		return nil, utils.Errorf("unsupported version: %d", version)
	}

	// 读取集合信息
	collection, err := readCollectionFromStream(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read collection")
	}

	// 读取文档数据
	documents, err := readDocumentsFromStream(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read documents")
	}

	// 读取HNSW索引
	hnswIndex, err := readHNSWIndexFromStream(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read hnsw index")
	}
	collection.GraphBinary = hnswIndex
	return &RAGBinaryData{
		Collection: collection,
		Documents:  documents,
	}, nil
}

// readCollectionFromStream 从流中读取集合信息
func readCollectionFromStream(reader io.Reader) (*schema.VectorStoreCollection, error) {
	collection := &schema.VectorStoreCollection{}

	// 集合名称
	nameBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read collection name")
	}
	collection.Name = string(nameBytes)

	// 集合描述
	descBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read collection description")
	}
	collection.Description = string(descBytes)

	// 模型名称
	modelBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read model name")
	}
	collection.ModelName = string(modelBytes)

	// 维度
	dimension, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read dimension")
	}
	collection.Dimension = int(dimension)

	// HNSW参数
	m, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read m")
	}
	collection.M = int(m)

	ml, err := consumeFloat32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read ml")
	}
	collection.Ml = float64(ml)

	efSearch, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read ef_search")
	}
	collection.EfSearch = int(efSearch)

	efConstruct, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read ef_construct")
	}
	collection.EfConstruct = int(efConstruct)

	// 距离函数类型
	distanceBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read distance func type")
	}
	collection.DistanceFuncType = string(distanceBytes)

	// EnablePQMode
	pqMode, err := consumeBool(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read enable pq mode")
	}
	collection.EnablePQMode = pqMode

	// CodeBookBinary
	codeBookBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read code book binary")
	}
	collection.CodeBookBinary = codeBookBytes

	// UUID
	uuidBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read uuid")
	}
	collection.UUID = string(uuidBytes)

	// GraphBinary
	graphBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read graph binary")
	}
	collection.GraphBinary = graphBytes

	// PQ编码
	pqCodeBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read pq code")
	}
	collection.CodeBookBinary = pqCodeBytes

	return collection, nil
}

// readDocumentsFromStream 从流中读取文档数据
func readDocumentsFromStream(reader io.Reader) ([]*ExportVectorStoreDocument, error) {
	// 文档数量
	docCount, err := consumeVarint(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read documents count")
	}

	documents := make([]*ExportVectorStoreDocument, docCount)

	for i := uint64(0); i < docCount; i++ {
		doc := &ExportVectorStoreDocument{}

		// 文档ID
		docIDBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read document id")
		}
		doc.DocumentID = string(docIDBytes)

		// 元数据
		metadataBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read metadata")
		}
		if len(metadataBytes) > 0 {
			err = json.Unmarshal(metadataBytes, &doc.Metadata)
			if err != nil {
				return nil, utils.Wrap(err, "unmarshal metadata")
			}
		}

		// 向量数据
		embeddingLen, err := consumeVarint(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read embedding length")
		}

		doc.Embedding = make([]float32, embeddingLen)
		for j := uint64(0); j < embeddingLen; j++ {
			val, err := consumeFloat32(reader)
			if err != nil {
				return nil, utils.Wrap(err, "read embedding value")
			}
			doc.Embedding[j] = val
		}

		documents[i] = doc
	}

	return documents, nil
}

// readHNSWIndexFromStream 从流中读取HNSW索引
func readHNSWIndexFromStream(reader io.Reader) ([]byte, error) {
	// HNSW索引长度
	indexLen, err := consumeVarint(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read hnsw index length")
	}

	if indexLen == 0 {
		return []byte{}, nil
	}

	// HNSW索引数据
	hnswIndex := make([]byte, indexLen)
	if _, err := io.ReadFull(reader, hnswIndex); err != nil {
		return nil, utils.Wrap(err, "read hnsw index data")
	}

	return hnswIndex, nil
}

// ImportRAGFromBinary 从二进制文件导入RAG数据
func ImportRAGFromBinary(ctx context.Context, db *gorm.DB, inputPath string, opts *RAGImportConfig) error {
	if opts == nil {
		opts = &RAGImportConfig{
			OverwriteExisting: false,
		}
	}

	// 读取二进制文件
	file, err := os.Open(inputPath)
	if err != nil {
		return utils.Wrap(err, "failed to open input file")
	}
	defer file.Close()

	// 加载RAG数据
	ragData, err := LoadRAGFromBinary(file)
	if err != nil {
		return utils.Wrap(err, "failed to load RAG binary data")
	}

	// 执行导入
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		return importRAGDataToDB(ctx, tx, ragData, opts)
	})
}

// importRAGDataToDB 将RAG数据导入到数据库
func importRAGDataToDB(ctx context.Context, db *gorm.DB, ragData *RAGBinaryData, opts *RAGImportConfig) error {
	if ragData.Collection == nil {
		return utils.Error("collection data is missing")
	}

	collection := ragData.Collection
	collectionName := collection.Name

	// 如果指定了集合名称，使用指定的名称
	if opts.CollectionName != "" {
		collectionName = opts.CollectionName
		collection.Name = collectionName
	}

	// 检查集合是否已存在
	existingCollection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return utils.Wrap(err, "failed to query existing collection")
	}

	var collectionID uint
	if existingCollection != nil {
		if !opts.OverwriteExisting {
			return utils.Errorf("collection %s already exists", collectionName)
		}

		// 删除现有数据
		collectionID = existingCollection.ID
		err = db.Unscoped().Where("collection_id = ?", collectionID).Delete(&schema.VectorStoreDocument{}).Error
		if err != nil {
			return utils.Wrap(err, "failed to delete existing documents")
		}

		// 更新集合信息
		err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionID).Updates(map[string]interface{}{
			"description":        collection.Description,
			"model_name":         collection.ModelName,
			"dimension":          collection.Dimension,
			"m":                  collection.M,
			"ml":                 collection.Ml,
			"ef_search":          collection.EfSearch,
			"ef_construct":       collection.EfConstruct,
			"distance_func_type": collection.DistanceFuncType,
		}).Error
		if err != nil {
			return utils.Wrap(err, "failed to update collection")
		}
	} else {
		// 创建新集合
		collection.ID = 0 // 重置ID，让数据库自动分配
		err = db.Create(collection).Error
		if err != nil {
			return utils.Wrap(err, "failed to create collection")
		}
		collectionID = collection.ID
	}

	// 导入文档数据
	if len(ragData.Documents) > 0 {
		documents := make([]*schema.VectorStoreDocument, len(ragData.Documents))
		for i, exportDoc := range ragData.Documents {
			documents[i] = &schema.VectorStoreDocument{
				DocumentID:   exportDoc.DocumentID,
				Metadata:     exportDoc.Metadata,
				Embedding:    exportDoc.Embedding,
				CollectionID: collectionID,
			}
		}

		// 批量插入文档
		batchSize := 1000
		for i := 0; i < len(documents); i += batchSize {
			end := i + batchSize
			if end > len(documents) {
				end = len(documents)
			}

			err = db.Create(documents[i:end]).Error
			if err != nil {
				return utils.Wrapf(err, "failed to create documents batch %d-%d", i, end)
			}
		}
	}

	// 导入HNSW索引（如果存在）
	if len(ragData.Collection.GraphBinary) > 0 {
		err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionID).Update("graph_binary", ragData.Collection.GraphBinary).Error
		if err != nil {
			// HNSW索引导入失败不应该影响整个导入过程
			log.Warnf("failed to import HNSW index: %v", err)
		}
	}

	return nil
}

// 创建流式读取辅助函数
func consumeUint32(reader io.Reader) (uint32, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return 0, utils.Wrap(err, "read uint32")
	}
	v, n := protowire.ConsumeFixed32(buf)
	if n < 0 {
		return 0, utils.Errorf("consume fixed32: %d", n)
	}
	return v, nil
}

func consumeFloat32(reader io.Reader) (float32, error) {
	v, err := consumeUint32(reader)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func consumeVarint(reader io.Reader) (uint64, error) {
	// 变长编码最多10字节
	buf := make([]byte, 10)
	for i := 0; i < 10; i++ {
		if _, err := io.ReadFull(reader, buf[i:i+1]); err != nil {
			return 0, utils.Wrap(err, "read varint byte")
		}

		// 检查是否完整
		v, n := protowire.ConsumeVarint(buf[:i+1])
		if n > 0 {
			return v, nil
		}
	}
	return 0, utils.Error("invalid varint encoding")
}

func consumeBytes(reader io.Reader) ([]byte, error) {
	length, err := consumeVarint(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read bytes length")
	}
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, utils.Wrap(err, "read bytes data")
	}
	return buf, nil
}

func consumeBool(reader io.Reader) (bool, error) {
	b, err := consumeVarint(reader)
	if err != nil {
		return false, utils.Wrap(err, "read bool")
	}
	return b != 0, nil
}

// protowire辅助函数
func pbWriteVarint(w io.Writer, value uint64) error {
	var i []byte
	i = protowire.AppendVarint(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteBytes(w io.Writer, value []byte) error {
	var i []byte
	i = protowire.AppendBytes(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteUint32(w io.Writer, value uint32) error {
	var i []byte
	i = protowire.AppendFixed32(i, value)
	_, err := w.Write(i)
	return err
}

func pbWriteFloat32(w io.Writer, value float32) error {
	var i []byte
	i = protowire.AppendFixed32(i, math.Float32bits(value))
	_, err := w.Write(i)
	return err
}

func pbWriteBool(w io.Writer, value bool) error {
	var i []byte
	if value {
		i = protowire.AppendVarint(i, uint64(1))
	} else {
		i = protowire.AppendVarint(i, uint64(0))
	}
	_, err := w.Write(i)
	return err
}
