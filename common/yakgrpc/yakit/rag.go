package yakit

import (
	"crypto/md5"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func selectRAGCollectionCoreFields(db *gorm.DB) *gorm.DB {
	return db.Model(&schema.VectorStoreCollection{}).Select("id, name, description, model_name, dimension, m, ml, ef_search, ef_construct, distance_func_type, enable_pq_mode, graph_binary, code_book_binary")
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
	db = db.Where("collection_id = ?", collection.ID).Find(&docs)
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
