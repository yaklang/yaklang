package rag

import (
	"context"
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// IsReadyCollection 检查集合是否存在
func IsReadyCollection(db *gorm.DB, collectionName string) bool {
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return false
	}
	return collection != nil
}

type ExportVectorStoreDocument struct {
	DocumentID string                 `json:"document_id"`
	Metadata   map[string]interface{} `json:"metadata"`
	Embedding  []float32              `json:"embedding"`
}

func ImportVectorData(db *gorm.DB, filepath string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		var collectionId uint
		unmarshalFunc := func(b []byte) (*schema.VectorStoreDocument, error) {
			var v ExportVectorStoreDocument
			if err := json.Unmarshal(b, &v); err != nil {
				return nil, err
			}
			collection := &schema.VectorStoreDocument{
				DocumentID:   v.DocumentID,
				Metadata:     v.Metadata,
				Embedding:    v.Embedding,
				CollectionID: collectionId,
			}
			return collection, nil
		}
		return bizhelper.ImportTableZipWithMarshalFunc(context.Background(), tx, filepath, unmarshalFunc, bizhelper.WithMetaDataHandler(func(metaData bizhelper.MetaData) error {
			collectionName := metaData["collection_name"].(string)
			collectionDescription := metaData["collection_description"].(string)
			collectionModelName := metaData["collection_model_name"].(string)
			collectionDimension := metaData["collection_dimension"].(float64)

			err := tx.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Assign(&schema.VectorStoreCollection{
				Name:        collectionName,
				Description: collectionDescription,
				ModelName:   collectionModelName,
				Dimension:   int(collectionDimension),
			}).FirstOrCreate(&schema.VectorStoreCollection{}).Error

			if err != nil {
				return err
			}

			collection, err := yakit.QueryRAGCollectionByName(tx, collectionName)
			if err != nil {
				return err
			}

			if collection == nil {
				return utils.Errorf("save collection %s failed, collection not found", collectionName)
			}
			collectionId = collection.ID
			return nil
		}), bizhelper.WithImportUniqueIndexField("DocumentID"), bizhelper.WithImportAllowOverwrite(true))
	})

}

func ExportVectorData(db *gorm.DB, collectionName string, filepath string) error {
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return utils.Errorf("failed to get %s collection: %v", collectionName, err)
	}

	if collection == nil {
		return utils.Errorf("collection %s not found", collectionName)
	}

	metaData := make(bizhelper.MetaData)
	metaData["collection_name"] = collection.Name
	metaData["collection_description"] = collection.Description
	metaData["collection_model_name"] = collection.ModelName
	metaData["collection_dimension"] = collection.Dimension

	exportDB := db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID)
	opts := []bizhelper.ExportOption{
		bizhelper.WithExportMetadata(metaData),
	}
	marshalFunc := func(v *schema.VectorStoreDocument) ([]byte, error) {
		return json.Marshal(&ExportVectorStoreDocument{
			DocumentID: v.DocumentID,
			Metadata:   v.Metadata,
			Embedding:  v.Embedding,
		})
	}
	return bizhelper.ExportTableZipWithMarshalFunc(context.Background(), exportDB, filepath, marshalFunc, opts...)
}

func ImportVectorDataFullUpdate(db *gorm.DB, filepath string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		var collectionId uint
		unmarshalFunc := func(b []byte) (*schema.VectorStoreDocument, error) {
			var v ExportVectorStoreDocument
			if err := json.Unmarshal(b, &v); err != nil {
				return nil, err
			}
			collection := &schema.VectorStoreDocument{
				DocumentID:   v.DocumentID,
				Metadata:     v.Metadata,
				Embedding:    v.Embedding,
				CollectionID: collectionId,
			}
			return collection, nil
		}
		return bizhelper.ImportTableZipWithMarshalFunc(context.Background(), tx, filepath, unmarshalFunc, bizhelper.WithMetaDataHandler(func(metaData bizhelper.MetaData) error {
			collectionName := metaData["collection_name"].(string)
			collectionDescription := metaData["collection_description"].(string)
			collectionModelName := metaData["collection_model_name"].(string)
			collectionDimension := metaData["collection_dimension"].(float64)

			collection, err := yakit.QueryRAGCollectionByName(tx, collectionName)
			if err != nil {
				return err
			}

			if collection == nil {
				return utils.Errorf("save collection %s failed, collection not found", collectionName)
			}
			if collection != nil {
				collectionId = collection.ID
				err = tx.Unscoped().Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collectionId).Delete(&schema.VectorStoreDocument{}).Error
				if err != nil {
					return err
				}
				err = tx.Unscoped().Model(&schema.VectorStoreCollection{}).Where("id = ?", collectionId).Delete(&schema.VectorStoreCollection{}).Error
				if err != nil {
					return err
				}
			}

			err = tx.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Assign(&schema.VectorStoreCollection{
				Name:        collectionName,
				Description: collectionDescription,
				ModelName:   collectionModelName,
				Dimension:   int(collectionDimension),
			}).FirstOrCreate(&schema.VectorStoreCollection{}).Error

			if err != nil {
				return err
			}

			collection, err = yakit.QueryRAGCollectionByName(tx, collectionName)
			if err != nil {
				return err
			}

			if collection == nil {
				return utils.Errorf("save collection %s failed, collection not found", collectionName)
			}
			collectionId = collection.ID
			return nil
		}), bizhelper.WithImportUniqueIndexField("DocumentID"), bizhelper.WithImportAllowOverwrite(true))
	})
}
