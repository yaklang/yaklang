package rag

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func autoAutomigrateVectorStoreDocument(db *gorm.DB) error {
	db.Model(&schema.VectorStoreDocument{}).Exec(fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", "uix_vector_store_documents_document_id"))
	return nil
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		autoAutomigrateVectorStoreDocument(consts.GetGormProfileDatabase())
		return nil
	})
}
