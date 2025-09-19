package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func QueryRAGCollectionByName(db *gorm.DB, name string) (*schema.VectorStoreCollection, error) {
	var collection *schema.VectorStoreCollection
	db = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Select("id, name, description, model_name, dimension, m, ml, ef_search, ef_construct, distance_func_type, enable_pq_mode, graph_binary, code_book_binary").First(&collection)
	return collection, db.Error
}

func QueryAllRAGCollectionInfosByName(db *gorm.DB, name string) ([]*schema.VectorStoreCollection, error) {
	var collections []*schema.VectorStoreCollection
	db = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Select("id, name, description, model_name, dimension, m, ml, ef_search, ef_construct, distance_func_type, enable_pq_mode").Find(&collections)
	return collections, db.Error
}

func GetAllRAGCollectionNames(db *gorm.DB) ([]string, error) {
	names := []string{}
	db = db.Model(&schema.VectorStoreCollection{}).Select("name").Find(&names)
	return names, db.Error
}
