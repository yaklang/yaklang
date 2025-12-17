package rag

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
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
	Version    uint32
}
type ExportVectorStoreDocument struct {
	DocumentID      string                 `json:"document_id"`
	Metadata        map[string]interface{} `json:"metadata"`
	Embedding       []float32              `json:"embedding"`
	PQCode          []byte                 `json:"pq_code"`
	Content         string                 `json:"content"`
	DocumentType    string                 `json:"document_type"`
	EntityID        string                 `json:"entity_id"`
	RelatedEntities string                 `json:"related_entities"`
}

func ExportRAG(collectionName string, fileName string, opts ...RAGSystemConfigOption) error {
	reader, err := ExportRAGToBinary(collectionName, opts...)
	if err != nil {
		return err
	}
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, reader)
	if err != nil {
		return err
	}
	return nil
}

// ExportRAGToBinary 导出RAG数据为二进制格式
func ExportRAGToBinary(collectionName string, opts ...RAGSystemConfigOption) (io.Reader, error) {
	cfg := NewRAGSystemConfig(opts...)
	buf := new(bytes.Buffer)
	db := cfg.db

	ragSystem, err := LoadRAGSystem(collectionName, append(opts, WithLazyLoadEmbeddingClient(true), WithDisableEmbedCollectionInfo(true))...)
	if err != nil {
		return nil, utils.Wrap(err, "failed to load rag system")
	}

	ragID := ragSystem.RAGID

	// 进度回调辅助函数
	reportProgress := func(percent float64, message string, messageType string) {
		if cfg.progressHandler != nil {
			cfg.progressHandler(percent, message, messageType)
		}
	}

	reportProgress(0, "开始导出向量库数据", "info")

	// 查询集合信息
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return nil, utils.Errorf("failed to get collection %s: %v", collectionName, err)
	}
	if collection == nil {
		return nil, utils.Errorf("collection %s not found", collectionName)
	}

	reportProgress(10, "正在写入集合信息", "info")

	// 确保Collection有UUID（如果没有就生成一个）
	if collection.UUID == "" {
		return nil, utils.Errorf("collection %s has empty UUID", collectionName)
	}

	// 写入魔数头和版本号
	if _, err := buf.WriteString("YAKRAG"); err != nil {
		return nil, utils.Wrap(err, "failed to write magic header")
	}
	if err := pbWriteUint32(buf, 2); err != nil {
		return nil, utils.Wrap(err, "failed to write version")
	}

	if err := pbWriteBytes(buf, []byte(uuid.NewString())); err != nil {
		return nil, utils.Wrap(err, "failed to write serialVersionUID")
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

	reportProgress(20, fmt.Sprintf("开始导出 %d 个向量文档", totalDocs), "info")

	// 分页导出文档数据
	const pageSize = 100
	page := 1
	processedDocs := int64(0)

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
			if cfg.documentHandler != nil {
				newDoc, err := cfg.documentHandler(*doc)
				if err != nil {
					return nil, utils.Wrapf(err, "failed to handle document %s", doc.DocumentID)
				}
				doc = &newDoc
			}

			if err := writeDocumentToBinary(buf, doc, ragID, cfg); err != nil {
				return nil, utils.Wrapf(err, "failed to write document %s", doc.DocumentID)
			}

			processedDocs++

			// 每处理10个文档或最后一个文档报告进度
			if processedDocs%10 == 0 || processedDocs == totalDocs {
				progress := 20 + (float64(processedDocs)/float64(totalDocs))*60 // 20-80%用于文档导出
				reportProgress(progress, fmt.Sprintf("已导出 %d/%d 个向量文档", processedDocs, totalDocs), "info")
			}
		}

		// 如果当前页数据少于pageSize，说明已经是最后一页
		if len(documents) < pageSize {
			break
		}

		page++
	}

	reportProgress(80, "向量文档导出完成，开始导出HNSW索引", "info")

	// 导出并写入HNSW索引
	if !cfg.noHNSWGraph {
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
		reportProgress(95, "HNSW索引导出完成", "info")
	} else {
		// 不包含HNSW索引，写入0长度
		if err := pbWriteVarint(buf, 0); err != nil {
			return nil, utils.Wrap(err, "failed to write empty hnsw index length")
		}
		reportProgress(95, "跳过HNSW索引导出", "info")
	}

	reportProgress(100, "向量库数据导出完成", "success")
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
func writeDocumentToBinary(writer io.Writer, doc *schema.VectorStoreDocument, ragID string, cfg *RAGSystemConfig) error {
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

	// PQCode
	if err := pbWriteBytes(writer, doc.PQCode); err != nil {
		return utils.Wrap(err, "write pq code")
	}

	// Content
	if err := pbWriteBytes(writer, []byte(doc.Content)); err != nil {
		return utils.Wrap(err, "write content")
	}

	// DocumentType
	if err := pbWriteBytes(writer, []byte(doc.DocumentType)); err != nil {
		return utils.Wrap(err, "write document type")
	}

	// EntityID
	if err := pbWriteBytes(writer, []byte(doc.EntityID)); err != nil {
		return utils.Wrap(err, "write entity id")
	}

	// RelatedEntities
	if err := pbWriteBytes(writer, []byte(doc.RelatedEntities)); err != nil {
		return utils.Wrap(err, "write related entities")
	}

	db := cfg.db
	knowledgeBuffer := new(bytes.Buffer)
	entityBuffer := new(bytes.Buffer)
	if uuid, ok := doc.Metadata.GetDataUUID(); ok && uuid != "" {
		var knowledgeEntry schema.KnowledgeBaseEntry
		err := db.Model(&schema.KnowledgeBaseEntry{}).Where("hidden_index = ?", uuid).First(&knowledgeEntry).Error
		if err == nil && knowledgeEntry.ID != 0 {
			err = writeKnowledgeEntryToBinary(knowledgeBuffer, &knowledgeEntry)
			if err != nil {
				return utils.Wrap(err, "write knowledge entry")
			}
		}
		var entity schema.ERModelEntity
		err = db.Model(&schema.ERModelEntity{}).Where("uuid = ?", doc.EntityID).First(&entity).Error
		if err == nil && entity.ID != 0 {
			err = writeEntityToBinary(entityBuffer, &entity)
			if err != nil {
				return utils.Wrap(err, "write entity")
			}
		}
	}

	pbWriteVarint(writer, 2)
	err = writeExtraDataToBinary("knowledge_entry", writer, knowledgeBuffer)
	if err != nil {
		return utils.Wrap(err, "write knowledge entry")
	}
	err = writeExtraDataToBinary("entity", writer, entityBuffer)
	if err != nil {
		return utils.Wrap(err, "write entity")
	}
	return nil
}

func writeExtraDataToBinary(typeName string, writer io.Writer, data *bytes.Buffer) error {
	if err := pbWriteBytes(writer, []byte(typeName)); err != nil {
		return utils.Wrap(err, "write type name")
	}
	if err := pbWriteBytes(writer, data.Bytes()); err != nil {
		return utils.Wrap(err, "write extra data")
	}
	return nil
}

func writeKnowledgeEntryToBinary(writer io.Writer, entry *schema.KnowledgeBaseEntry) error {
	if err := pbWriteBytes(writer, []byte(entry.RelatedEntityUUIDS)); err != nil {
		return utils.Wrap(err, "write related entity uuid s")
	}
	if err := pbWriteBytes(writer, []byte(entry.KnowledgeTitle)); err != nil {
		return utils.Wrap(err, "write knowledge title")
	}
	if err := pbWriteBytes(writer, []byte(entry.KnowledgeType)); err != nil {
		return utils.Wrap(err, "write knowledge type")
	}
	if err := pbWriteUint32(writer, uint32(entry.ImportanceScore)); err != nil {
		return utils.Wrap(err, "write importance score")
	}
	if err := pbWriteUint32(writer, uint32(len(entry.Keywords))); err != nil {
		return utils.Wrap(err, "write keywords length")
	}
	for _, val := range entry.Keywords {
		if err := pbWriteBytes(writer, []byte(val)); err != nil {
			return utils.Wrap(err, "write keywords")
		}
	}
	if err := pbWriteBytes(writer, []byte(entry.KnowledgeDetails)); err != nil {
		return utils.Wrap(err, "write knowledge details")
	}
	if err := pbWriteBytes(writer, []byte(entry.Summary)); err != nil {
		return utils.Wrap(err, "write summary")
	}
	if err := pbWriteUint32(writer, uint32(entry.SourcePage)); err != nil {
		return utils.Wrap(err, "write source page")
	}
	if err := pbWriteUint32(writer, uint32(len(entry.PotentialQuestions))); err != nil {
		return utils.Wrap(err, "write potential questions length")
	}
	for _, val := range entry.PotentialQuestions {
		if err := pbWriteBytes(writer, []byte(val)); err != nil {
			return utils.Wrap(err, "write potential questions")
		}
	}
	if err := pbWriteUint32(writer, uint32(len(entry.PotentialQuestionsVector))); err != nil {
		return utils.Wrap(err, "write potential questions vector")
	}
	for _, val := range entry.PotentialQuestionsVector {
		if err := pbWriteFloat32(writer, val); err != nil {
			return utils.Wrap(err, "write potential questions vector")
		}
	}

	return nil
}

func writeEntityToBinary(writer io.Writer, entity *schema.ERModelEntity) error {
	if err := pbWriteBytes(writer, []byte(entity.EntityName)); err != nil {
		return utils.Wrap(err, "write entity name")
	}
	// if err := pbWriteBytes(writer, []byte(entity.Uuid)); err != nil {
	// 	return utils.Wrap(err, "write entity uuid")
	// }
	if err := pbWriteBytes(writer, []byte("")); err != nil {
		return utils.Wrap(err, "write entity uuid")
	}
	if err := pbWriteBytes(writer, []byte(entity.Description)); err != nil {
		return utils.Wrap(err, "write description")
	}
	if err := pbWriteBytes(writer, []byte(entity.EntityType)); err != nil {
		return utils.Wrap(err, "write entity type")
	}
	if err := pbWriteBytes(writer, []byte(entity.EntityTypeVerbose)); err != nil {
		return utils.Wrap(err, "write entity type verbose")
	}

	// 序列化属性
	attrBytes, err := json.Marshal(entity.Attributes)
	if err != nil {
		return utils.Wrap(err, "marshal attributes")
	}
	if err := pbWriteBytes(writer, attrBytes); err != nil {
		return utils.Wrap(err, "write attributes")
	}

	return nil
}

func LoadRAGFileHeader(reader io.Reader) (*RAGBinaryData, error) {
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
	var serialVersionUID string
	if version == 2 {
		serialVersionUIDBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read serialVersionUID")
		}
		serialVersionUID = string(serialVersionUIDBytes)
	}

	// 读取集合信息
	collection, err := readCollectionFromStream(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read collection")
	}

	collection.SerialVersionUID = serialVersionUID
	return &RAGBinaryData{
		Version:    version,
		Collection: collection,
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
func readDocumentsFromStream(reader io.Reader, extDataHandle func(doc *ExportVectorStoreDocument, typeName string, reader io.Reader) error) ([]*ExportVectorStoreDocument, error) {
	// 文档数量
	docCount, err := consumeVarint(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read documents count")
	}

	documents := make([]*ExportVectorStoreDocument, docCount)

	for i := uint64(0); i < docCount; i++ {
		doc, err := readDocumentFromStream(reader, extDataHandle)
		if err != nil {
			return nil, utils.Wrap(err, "read document")
		}
		documents[i] = doc
	}

	return documents, nil
}

func readDocumentFromStream(reader io.Reader, extDataHandle func(doc *ExportVectorStoreDocument, typeName string, reader io.Reader) error) (*ExportVectorStoreDocument, error) {
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

	// PQCode
	pqCodeBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read pq code")
	}
	doc.PQCode = pqCodeBytes

	// Content
	contentBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read content")
	}
	doc.Content = string(contentBytes)

	// DocumentType
	documentTypeBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read document type")
	}
	doc.DocumentType = string(documentTypeBytes)

	// EntityID
	entityIDBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read entity id")
	}
	doc.EntityID = string(entityIDBytes)

	// RelatedEntities
	relatedEntitiesBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read related entities")
	}
	doc.RelatedEntities = string(relatedEntitiesBytes)

	extraDataCount, err := consumeVarint(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read extra data count")
	}
	for i := uint64(0); i < extraDataCount; i++ {
		// 读取类型名称
		typeNameBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read extra data type name")
		}
		dataBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read extra data")
		}
		if len(dataBytes) == 0 {
			continue
		}
		extReader := bytes.NewReader(dataBytes)
		err = extDataHandle(doc, string(typeNameBytes), extReader)
		if err != nil {
			return nil, utils.Wrap(err, "handle extra data")
		}
	}

	return doc, nil
}

// readEntityFromBinary 从二进制数据读取实体
func readEntityFromStream(reader io.Reader) (*schema.ERModelEntity, error) {
	entity := &schema.ERModelEntity{}

	// 读取 EntityName
	entityName, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read entity name")
	}
	entity.EntityName = string(entityName)

	// 读取 Uuid
	uuid, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read entity uuid")
	}
	entity.Uuid = string(uuid)

	// 读取 Description
	description, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read description")
	}
	entity.Description = string(description)

	// 读取 EntityType
	entityType, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read entity type")
	}
	entity.EntityType = string(entityType)

	// 读取 EntityTypeVerbose
	typeVerbose, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read entity type verbose")
	}
	entity.EntityTypeVerbose = string(typeVerbose)

	// 读取 Attributes
	attrBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read attributes")
	}
	if len(attrBytes) > 0 {
		if err := json.Unmarshal(attrBytes, &entity.Attributes); err != nil {
			return nil, utils.Wrap(err, "unmarshal attributes")
		}
	}

	return entity, nil
}

// readEntryFromBinary 从二进制数据读取知识库条目
func readKnowledgeEntryFromStream(reader io.Reader) (*schema.KnowledgeBaseEntry, error) {
	entry := &schema.KnowledgeBaseEntry{}

	// 读取 RelatedEntityUUIDS
	relatedEntityBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read related entity uuids")
	}
	entry.RelatedEntityUUIDS = string(relatedEntityBytes)

	// 读取 KnowledgeTitle
	titleBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read knowledge title")
	}
	entry.KnowledgeTitle = string(titleBytes)

	// 读取 KnowledgeType
	typeBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read knowledge type")
	}
	entry.KnowledgeType = string(typeBytes)

	// 读取 ImportanceScore
	importanceScore, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read importance score")
	}
	entry.ImportanceScore = int(importanceScore)

	// 读取 Keywords
	keywordsLength, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read keywords length")
	}
	keywords := make([]string, keywordsLength)
	for i := uint32(0); i < keywordsLength; i++ {
		keywordBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read keyword")
		}
		keywords[i] = string(keywordBytes)
	}
	entry.Keywords = schema.StringArray(keywords)

	// 读取 KnowledgeDetails
	detailsBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read knowledge details")
	}
	entry.KnowledgeDetails = string(detailsBytes)

	// 读取 Summary
	summaryBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read summary")
	}
	entry.Summary = string(summaryBytes)

	// 读取 SourcePage
	sourcePage, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read source page")
	}
	entry.SourcePage = int(sourcePage)

	// 读取 PotentialQuestions
	questionsLength, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read potential questions length")
	}
	questions := make([]string, questionsLength)
	for i := uint32(0); i < questionsLength; i++ {
		questionBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read potential question")
		}
		questions[i] = string(questionBytes)
	}
	entry.PotentialQuestions = schema.StringArray(questions)

	// 读取 PotentialQuestionsVector
	vectorLength, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read potential questions vector length")
	}
	vector := make([]float32, vectorLength)
	for i := uint32(0); i < vectorLength; i++ {
		value, err := consumeFloat32(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read potential questions vector value")
		}
		vector[i] = value
	}
	entry.PotentialQuestionsVector = schema.FloatArray(vector)
	return entry, nil
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

func ImportRAG(inputPath string, optFuncs ...RAGSystemConfigOption) error {
	// 读取二进制文件
	file, err := os.Open(inputPath)
	if err != nil {
		return utils.Wrap(err, "failed to open input file")
	}
	defer file.Close()

	ragData, err := LoadRAGFileHeader(file)
	if err != nil {
		return utils.Wrap(err, "load rag file header")
	}

	ragSystemConfig := NewRAGSystemConfig(optFuncs...)
	if !ragSystemConfig.overwriteExisting && CollectionIsExists(ragSystemConfig.db, ragSystemConfig.Name) {
		return utils.Errorf("collection %s already exists", ragSystemConfig.Name)
	}

	if ragSystemConfig.Name != "" {
		ragData.Collection.Name = ragSystemConfig.Name
	}

	DeleteRAG(ragSystemConfig.db, ragData.Collection.Name)

	// 创建集合
	ragID := uuid.NewString()
	// 兼容旧版本导出文件（无 SerialVersionUID），导入时仍标记为“已导入”
	if ragData.Collection.SerialVersionUID == "" {
		ragData.Collection.SerialVersionUID = uuid.NewString()
	}
	ragData.Collection.RAGID = ragID
	collection, err := vectorstore.CreateCollectionRecord(ragSystemConfig.db, ragSystemConfig.Name, ragSystemConfig.description, ragSystemConfig.ConvertToVectorStoreOptions()...)
	if err != nil {
		return utils.Wrap(err, "create collection record")
	}
	err = ragSystemConfig.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).Update("rag_id", ragID).Error
	if err != nil {
		return utils.Wrap(err, "update collection rag id")
	}

	// 创建知识库
	knowledgeBase, err := knowledgebase.NewKnowledgeBaseWithVectorStore(ragSystemConfig.db, ragSystemConfig.Name, ragSystemConfig.description, ragSystemConfig.knowledgeBaseType, ragSystemConfig.tags, nil)
	if err != nil {
		return utils.Wrap(err, "create knowledge base")
	}
	err = ragSystemConfig.db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", knowledgeBase.GetID()).Updates(map[string]interface{}{
		"rag_id":             ragID,
		"serial_version_uid": ragData.Collection.SerialVersionUID,
	}).Error
	if err != nil {
		return utils.Wrap(err, "update knowledge base rag id")
	}
	knowledgeBaseInfo := knowledgeBase.GetKnowledgeBaseInfo()

	entityBaseInfo := &schema.EntityRepository{
		EntityBaseName: ragSystemConfig.Name,
		Description:    ragSystemConfig.description,
		Uuid:           uuid.NewString(),
	}
	err = yakit.CreateEntityBaseInfo(ragSystemConfig.db, entityBaseInfo)
	if err != nil {
		return utils.Wrap(err, "create entity base info")
	}

	// 读取文档数据
	documents, err := readDocumentsFromStream(file, func(doc *ExportVectorStoreDocument, typeName string, extReader io.Reader) error {
		switch typeName {
		case "knowledge_entry":
			knowledgeEntry, err := readKnowledgeEntryFromStream(extReader)
			if err != nil {
				return utils.Wrap(err, "read knowledge entry")
			}
			knowledgeEntry.HiddenIndex = uuid.NewString()
			knowledgeEntry.KnowledgeBaseID = int64(knowledgeBaseInfo.ID)
			err = yakit.CreateKnowledgeBaseEntry(ragSystemConfig.db, knowledgeEntry)
			if err != nil {
				return utils.Wrap(err, "create knowledge base entry")
			}
			doc.Metadata[schema.META_Data_UUID] = knowledgeEntry.HiddenIndex
		case "entity":
			entity, err := readEntityFromStream(extReader)
			if err != nil {
				return utils.Wrap(err, "read entity")
			}
			// 强制使用新uuid
			entity.Uuid = uuid.NewString()
			entity.RepositoryUUID = entityBaseInfo.Uuid
			err = yakit.CreateEntity(ragSystemConfig.db, entity)
			if err != nil {
				return utils.Wrap(err, "create entity")
			}
			doc.Metadata[schema.META_Data_UUID] = entity.Uuid
		}
		return nil
	})
	if err != nil {
		return utils.Wrap(err, "read documents")
	}

	// 读取HNSW索引
	hnswIndex, err := readHNSWIndexFromStream(file)
	if err != nil {
		return utils.Wrap(err, "read hnsw index")
	}
	ragData.Collection.GraphBinary = hnswIndex
	ragData.Documents = documents

	// 执行导入
	return importRAGDataToDB(ragData, optFuncs...)
}

// importRAGDataToDB 将RAG数据导入到数据库
func importRAGDataToDB(ragData *RAGBinaryData, optFuncs ...RAGSystemConfigOption) error {
	opts := NewRAGSystemConfig(optFuncs...)
	db := opts.db
	if ragData.Collection == nil {
		return utils.Error("collection data is missing")
	}

	// 进度回调辅助函数
	reportProgress := func(percent float64, message string, messageType string) {
		if opts.progressHandler != nil {
			opts.progressHandler(percent, message, messageType)
		}
	}

	reportProgress(0, "开始导入向量库数据", "info")

	collectionIns := *ragData.Collection
	collection := &collectionIns
	if opts.rebuildHNSWIndex {
		collection.GraphBinary = nil
	}
	// 保存原始集合名称（用于计算 UID）
	// originalCollectionName := collection.Name
	collectionName := collection.Name

	// 如果指定了集合名称，使用指定的名称
	if opts.Name != "" {
		collectionName = opts.Name
		collection.Name = collectionName
	}

	reportProgress(10, "正在处理集合信息", "info")

	// 检查集合是否已存在
	existingCollection, _ := yakit.QueryRAGCollectionByName(db, collectionName)
	var collectionID uint
	if existingCollection != nil {
		// 删除现有数据
		collectionID = existingCollection.ID
		err := db.Unscoped().Where("collection_id = ?", collectionID).Delete(&schema.VectorStoreDocument{}).Error
		if err != nil {
			return utils.Wrap(err, "failed to delete existing documents")
		}

		// 更新集合信息
		err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionID).Updates(map[string]interface{}{
			"description":        collection.Description,
			"model_name":         collection.ModelName,
			"dimension":          collection.Dimension,
			"serial_version_uid": collection.SerialVersionUID,
			"m":                  collection.M,
			"ml":                 collection.Ml,
			"ef_search":          collection.EfSearch,
			"ef_construct":       collection.EfConstruct,
			"distance_func_type": collection.DistanceFuncType,
			"uuid":               collection.UUID,
			"rag_id":             collection.RAGID,
		}).Error
		if err != nil {
			return utils.Wrap(err, "failed to update collection")
		}
	} else {
		// 创建新集合
		collection.ID = 0 // 重置ID，让数据库自动分配
		// 确保UUID不为空
		if collection.UUID == "" {
			collection.UUID = uuid.NewString()
		}
		collection.RAGID = opts.ragID
		err := db.Create(collection).Error
		if err != nil {
			return utils.Wrap(err, "failed to create collection")
		}
		collectionID = collection.ID
	}

	reportProgress(20, "集合信息处理完成，开始导入向量文档", "info")

	// 导入文档数据
	var documents []*schema.VectorStoreDocument
	if len(ragData.Documents) > 0 {
		totalDocs := len(ragData.Documents)
		documents = make([]*schema.VectorStoreDocument, totalDocs)

		reportProgress(30, fmt.Sprintf("正在处理 %d 个向量文档", totalDocs), "info")

		for i, exportDoc := range ragData.Documents {
			documents[i] = &schema.VectorStoreDocument{
				DocumentID:   exportDoc.DocumentID,
				Metadata:     exportDoc.Metadata,
				Embedding:    exportDoc.Embedding,
				PQCode:       exportDoc.PQCode,
				Content:      exportDoc.Content,
				DocumentType: schema.RAGDocumentType(exportDoc.DocumentType),
				// 使用原始集合名称计算 UID，确保与 HNSW 图中的 UID 匹配
				// UID:             vectorstore.GetLazyNodeUIDByMd5(originalCollectionName, exportDoc.DocumentID),
				EntityID:        exportDoc.EntityID,
				RelatedEntities: exportDoc.RelatedEntities,
				CollectionID:    collectionID,
				CollectionUUID:  collection.UUID,
			}

			// 应用文档处理器
			if opts.documentHandler != nil {
				newDoc, err := opts.documentHandler(*documents[i])
				if err != nil {
					return utils.Wrapf(err, "failed to handle document %s", exportDoc.DocumentID)
				}
				documents[i] = &newDoc
			}

			// 每处理100个文档报告一次进度
			if (i+1)%100 == 0 || i+1 == totalDocs {
				progress := 30 + (float64(i+1)/float64(totalDocs))*40 // 30-70%用于文档处理
				reportProgress(progress, fmt.Sprintf("已处理 %d/%d 个向量文档", i+1, totalDocs), "info")
			}
		}

		reportProgress(70, "开始将向量文档插入数据库", "info")

		// 逐个插入文档，避免批量创建的问题
		for i, doc := range documents {
			err := db.Create(doc).Error
			if err != nil {
				log.Errorf("failed to create document %d (ID: %s): %v", i, doc.DocumentID, err)
				continue
			}

			// 每插入100个文档报告一次进度
			if (i+1)%100 == 0 || i+1 == len(documents) {
				progress := 70 + (float64(i+1)/float64(len(documents)))*20 // 70-90%用于文档插入
				reportProgress(progress, fmt.Sprintf("已插入 %d/%d 个向量文档到数据库", i+1, len(documents)), "info")
			}
		}
	}

	reportProgress(90, "向量文档导入完成，开始导入HNSW索引", "info")

	// 导入HNSW索引（如果存在）
	if len(ragData.Collection.GraphBinary) > 0 && !opts.rebuildHNSWIndex {
		err := db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionID).Update("graph_binary", ragData.Collection.GraphBinary).Error
		if err != nil {
			// HNSW索引导入失败不应该影响整个导入过程
			log.Warnf("failed to import HNSW index: %v", err)
		}
		reportProgress(95, "HNSW索引导入完成", "info")
	} else if len(documents) > 0 {
		reportProgress(92, "HNSW索引重建开始", "info")
		// 确保使用正确的集合 ID（MigrateHNSWGraph 需要正确的 ID 来查询文档）
		collection.ID = collectionID
		err := vectorstore.MigrateHNSWGraph(db, collection)
		if err != nil {
			if errors.Is(err, vectorstore.ErrGraphNodesIsEmpty) {
				log.Warnf("HNSW graph is empty, skip migration")
			} else {
				return utils.Wrap(err, "failed to migrate HNSW graph")
			}
		}
		reportProgress(95, "HNSW索引重建完成", "info")
	} else {
		log.Info("No documents to migrate, skip HNSW index rebuild")
		reportProgress(95, "无文档需要重建HNSW索引", "info")
	}

	reportProgress(100, "向量库数据导入完成", "success")
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

func VerifyImportFile(importFile string) error {
	if importFile == "" {
		return utils.Error("import file is empty")
	}
	if !utils.FileExists(importFile) {
		return utils.Error("import file not found")
	}
	file, err := os.Open(importFile)
	if err != nil {
		return utils.Wrap(err, "open import file")
	}
	defer file.Close()
	header, err := LoadRAGFileHeader(file)
	if err != nil {
		return utils.Wrap(err, "load rag file header")
	}

	// 读取文档数据
	documents, err := readDocumentsFromStream(file, nil)
	if err != nil {
		return utils.Wrap(err, "read documents")
	}

	// 所有 docUID 列表
	collectionUUID := header.Collection.UUID
	docUIDs := make([]string, 0)
	for _, doc := range documents {
		docUID := md5.Sum([]byte(collectionUUID + doc.DocumentID))
		docUIDStr, err := uuid.FromBytes(docUID[:])
		if err != nil {
			return utils.Wrap(err, "convert doc uid to string")
		}
		docUIDs = append(docUIDs, docUIDStr.String())
	}

	// 解析图
	pers, err := hnsw.LoadBinary[string](bytes.NewReader(header.Collection.GraphBinary))
	if err != nil {
		return utils.Wrap(err, "load hnsw graph")
	}

	// 遍历图节点 key ，验证节点数据是否存在
	for _, node := range pers.OffsetToKey[1:] {
		code := node.Code
		bytesCode, ok := code.([]byte)
		if !ok {
			return utils.Error("hnsw graph node code is not []byte")
		}
		uuidStr, err := uuid.FromBytes(bytesCode)
		if err != nil {
			return utils.Wrap(err, "convert doc uid to string")
		}
		if !slices.Contains(docUIDs, uuidStr.String()) {
			return utils.Errorf("hnsw graph node data not found: %s", uuidStr.String())
		}
	}
	return nil
}
