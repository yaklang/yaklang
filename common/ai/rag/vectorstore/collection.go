package vectorstore

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func CreateCollection(db *gorm.DB, name string, description string, opts ...CollectionConfigFunc) (*SQLiteVectorStoreHNSW, error) {
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	// 创建RAG配置
	// 检查集合是否存在
	if HasCollection(db, name) {
		return nil, utils.Errorf("集合 %s 已存在", name)
	}

	collection, err := NewSQLiteVectorStoreHNSWEx(db, name, description, opts...)
	if err != nil {
		return nil, utils.Errorf("创建SQLite向量存储失败: %v", err)
	}

	collection.Add(&Document{
		ID:      DocumentTypeCollectionInfo,
		Content: fmt.Sprintf("collection_name: %s\ncollection_description: %s", name, description),
		Metadata: map[string]any{
			"collection_name": name,
			"collection_id":   collection.GetCollectionInfo().ID,
		},
	})

	return collection, nil
}

func LoadCollection(db *gorm.DB, name string, opts ...CollectionConfigFunc) (*SQLiteVectorStoreHNSW, error) {
	collection, err := LoadSQLiteVectorStoreHNSW(db, name, opts...)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

// HasCollection 检查知识库是否存在
func HasCollection(db *gorm.DB, name string) bool {
	col, err := yakit.QueryRAGCollectionByName(db, name)
	return col != nil && err == nil
}

func GetCollection(db *gorm.DB, collectionName string, opts ...CollectionConfigFunc) (*SQLiteVectorStoreHNSW, error) {
	if HasCollection(db, collectionName) {
		log.Infof("collection '%s' exists, loading it", collectionName)
		return LoadCollection(db, collectionName, opts...)
	} else {
		log.Infof("collection '%s' does not exist, creating it", collectionName)
		return CreateCollection(db, collectionName, "", opts...)
	}
}

func RemoveCollection(db *gorm.DB, collectionName string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		collection, err := yakit.QueryRAGCollectionByName(tx, collectionName)
		if err != nil {
			return err
		}
		if collection == nil {
			return utils.Errorf("集合 %s 不存在", collectionName)
		}

		if err := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreCollection{}).Error; err != nil {
			return err
		}
		return nil
	})
}
