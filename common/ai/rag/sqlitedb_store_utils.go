package rag

import (
	"context"
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

// IsReadyCollection 检查集合是否存在
func IsReadyCollection(db *gorm.DB, collectionName string) bool {
	var collections []*schema.VectorStoreCollection
	dbErr := db.Where("name = ?", collectionName).Find(&collections)
	if dbErr.Error != nil {
		return false
	}
	return len(collections) > 0
}

type ExportVectorStoreDocument struct {
	DocumentID string                 `json:"document_id"`
	Metadata   map[string]interface{} `json:"metadata"`
	Embedding  []float64              `json:"embedding"`
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

			cs := []*schema.VectorStoreCollection{}
			err = tx.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Find(&cs).Error
			if err != nil {
				return err
			}

			if len(cs) == 0 {
				return utils.Errorf("save collection %s failed, collection not found", collectionName)
			}
			collectionId = cs[0].ID
			return nil
		}))
	})

}

func ExportVectorData(db *gorm.DB, collectionName string, filepath string) error {
	var collections []*schema.VectorStoreCollection
	err := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Find(&collections).Error
	if err != nil {
		return utils.Errorf("failed to get %s collection: %v", collectionName, err)
	}

	if len(collections) == 0 {
		return utils.Errorf("collection %s not found", collectionName)
	}

	metaData := make(bizhelper.MetaData)
	metaData["collection_name"] = collections[0].Name
	metaData["collection_description"] = collections[0].Description
	metaData["collection_model_name"] = collections[0].ModelName
	metaData["collection_dimension"] = collections[0].Dimension

	exportDB := db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collections[0].ID)
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
