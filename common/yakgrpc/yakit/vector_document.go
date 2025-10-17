package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type VectorDocumentFilter struct {
	DocumentTypes  []string
	DocumentIDs    []string
	Keywords       string
	RuntimeID      []string
	CollectionUUID string
}

func FilterVectorDocuments(db *gorm.DB, filter *VectorDocumentFilter) *gorm.DB {
	db = db.Model(&schema.VectorStoreDocument{})
	if filter == nil {
		return db
	}
	db = bizhelper.ExactQueryStringArrayOr(db, "document_type", filter.DocumentTypes)
	db = bizhelper.ExactQueryStringArrayOr(db, "document_id", filter.DocumentIDs)
	db = bizhelper.ExactQueryStringArrayOr(db, "runtime_id", filter.RuntimeID)
	db = bizhelper.ExactQueryString(db, "collection_uuid", filter.CollectionUUID)
	db = bizhelper.FuzzSearchEx(db, []string{"metadata", "content"}, filter.Keywords, false)
	return db
}

func YieldVectorDocument(ctx context.Context, db *gorm.DB, filter *VectorDocumentFilter, options ...bizhelper.YieldModelOpts) chan *schema.VectorStoreDocument {
	db = FilterVectorDocuments(db, filter)
	return bizhelper.YieldModel[*schema.VectorStoreDocument](ctx, db, options...)
}
