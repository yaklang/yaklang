package yakit

import (
	"crypto/md5"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func selectRAGCollectionCoreFields(db *gorm.DB) *gorm.DB {
	return db.Model(&schema.VectorStoreCollection{}).Select("id, name, description, model_name, dimension, m, ml, ef_search, ef_construct, distance_func_type, enable_pq_mode, graph_binary, code_book_binary, uuid")
}

func QueryRAGCollectionByName(db *gorm.DB, name string) (*schema.VectorStoreCollection, error) {
	var collection schema.VectorStoreCollection
	db = selectRAGCollectionCoreFields(db).Where("name = ?", name).First(&collection)
	if db.Error != nil {
		return nil, db.Error
	}
	return &collection, nil
}

func QueryRAGCollectionByID(db *gorm.DB, id int64) (*schema.VectorStoreCollection, error) {
	var collection schema.VectorStoreCollection
	db = selectRAGCollectionCoreFields(db).Where("id = ?", id).First(&collection)
	if db.Error != nil {
		return nil, db.Error
	}
	return &collection, nil
}

func GetAllRAGCollectionNames(db *gorm.DB) ([]string, error) {
	var collections []*schema.VectorStoreCollection
	db = db.Model(&schema.VectorStoreCollection{}).Select("name").Find(&collections)
	if db.Error != nil {
		return nil, db.Error
	}
	names := make([]string, 0, len(collections))
	for _, collection := range collections {
		names = append(names, collection.Name)
	}
	return names, nil
}

func GetAllRAGCollectionInfos(db *gorm.DB) ([]*schema.VectorStoreCollection, error) {
	collections := []*schema.VectorStoreCollection{}
	db = selectRAGCollectionCoreFields(db).Find(&collections)
	if db.Error != nil {
		return nil, db.Error
	}
	return collections, nil
}

func GetRAGCollectionInfoByName(db *gorm.DB, name string) (*schema.VectorStoreCollection, error) {
	var collection schema.VectorStoreCollection
	db = selectRAGCollectionCoreFields(db).Where("name = ?", name).First(&collection)
	if db.Error != nil {
		return nil, db.Error
	}
	return &collection, nil
}

func GetRAGCollectionInfoByID(db *gorm.DB, id int64) (*schema.VectorStoreCollection, error) {
	var collection schema.VectorStoreCollection
	db = selectRAGCollectionCoreFields(db).Where("id = ?", id).First(&collection)
	if db.Error != nil {
		return nil, db.Error
	}
	return &collection, nil
}

func GetRAGDocumentByID(db *gorm.DB, name string, id string) (*schema.VectorStoreDocument, error) {
	var doc schema.VectorStoreDocument
	db = db.Where("document_id = ?", id).First(&doc)
	if db.Error != nil {
		return nil, db.Error
	}
	return &doc, nil
}

func GetRAGDocumentsByCollectionNameAnd(db *gorm.DB, name string) ([]*schema.VectorStoreDocument, error) {
	collection, err := GetRAGCollectionInfoByName(db, name)
	if err != nil {
		return nil, err
	}
	var docs []*schema.VectorStoreDocument
	if err := db.Where("collection_id = ?", collection.ID).Find(&docs).Error; err != nil {
		return nil, err
	}
	return docs, nil
}

func GetRAGDocumentByCollectionIDAndKey(db *gorm.DB, collectionID uint, name string) (*schema.VectorStoreDocument, error) {
	var doc schema.VectorStoreDocument
	db = db.Where("document_id = ? and collection_id = ?", name, collectionID).First(&doc)
	if db.Error != nil {
		return nil, db.Error
	}
	return &doc, nil
}

// FilterRAGDocument 过滤 RAG 文档
func FilterRAGDocument(db *gorm.DB, docFilter *ypb.ListVectorStoreEntriesFilter) *gorm.DB {
	if docFilter == nil {
		return db
	}
	db = db.Model(&schema.VectorStoreDocument{})

	// 精确匹配集合ID
	if docFilter.CollectionID > 0 {
		db = bizhelper.ExactQueryInt64(db, "collection_id", docFilter.CollectionID)
	}

	// 通过集合名称查找集合ID
	if docFilter.CollectionName != "" && docFilter.CollectionID == 0 {
		var collection schema.VectorStoreCollection
		if err := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", docFilter.CollectionName).First(&collection).Error; err == nil {
			db = bizhelper.ExactQueryInt64(db, "collection_id", int64(collection.ID))
		}
	}

	// 多字段模糊搜索
	if docFilter.Keyword != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"document_id", "content"}, docFilter.Keyword, false)
	}

	return db
}

// QueryRAGDocumentPaging 分页查询 RAG 文档
func QueryRAGDocumentPaging(db *gorm.DB, filter *ypb.ListVectorStoreEntriesFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.VectorStoreDocument, error) {
	// 1. 设置查询的数据模型
	db = db.Model(&schema.VectorStoreDocument{})

	// 2. 应用过滤条件
	db = FilterRAGDocument(db, filter)

	// 3. 执行分页查询
	ret := make([]*schema.VectorStoreDocument, 0)
	pag, db := bizhelper.YakitPagingQuery(db, paging, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return pag, ret, nil
}

// GetRAGDocumentByFilter 根据过滤条件获取 RAG 文档（兼容旧接口）
func GetRAGDocumentByFilter(db *gorm.DB, filter *ypb.Paging) (*bizhelper.Paginator, []*schema.VectorStoreDocument, error) {
	return QueryRAGDocumentPaging(db, nil, filter)
}

func UpdateRAGDocument(db *gorm.DB, doc *schema.VectorStoreDocument) error {
	return db.Save(doc).Error
}

const (
	uidTypeMd5        = "md5"
	uidTypeID         = "id"
	uidTypeDocumentID = "document_id"
)

func getLazyNodeUID(uidType string, collectionName string, data any) hnswspec.LazyNodeID {
	switch uidType {
	case uidTypeMd5:
		key, ok := data.(string)
		if !ok {
			log.Errorf("expected string for key, got %T", data)
			return nil
		}
		m := md5.Sum([]byte(collectionName + key))
		return m[:]
	case uidTypeID:
		key := utils.InterfaceToInt(data)
		return hnswspec.LazyNodeID(key)
	case uidTypeDocumentID:
		key, ok := data.(string)
		if !ok {
			log.Errorf("expected string for key, got %T", data)
			return nil
		}
		return hnswspec.LazyNodeID(key)
	}
	return nil
}
