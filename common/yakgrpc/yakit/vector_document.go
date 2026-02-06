package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"strings"
)

type VectorDocumentFilter struct {
	DocumentTypes  []string
	DocumentIDs    []string
	Keywords       string
	RuntimeID      []string
	CollectionUUID string
}

func VectorDocumentVTableName() string {
	return (&schema.VectorStoreDocument{}).TableName() + "_fts"
}

var defaultVectorStoreDocumentFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel: &schema.VectorStoreDocument{},
	FTSTable:  VectorDocumentVTableName(),
	Columns:   []string{"content", "metadata"},
	Tokenize:  "trigram",
}

//func init() {
//	// Setup FTS5 index for rag_vector_document_v1 in SQLite profile DB to accelerate searching.
//	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_PROFILE_DATABASE, func(db *gorm.DB) {
//		if db == nil {
//			return
//		}
//		if !schema.IsSQLite(db) {
//			return
//		}
//		baseTable := (&schema.VectorStoreDocument{}).TableName()
//		if !db.HasTable(baseTable) {
//			// Base table is gone, but the FTS virtual table may remain; clean it up.
//			if err := bizhelper.SQLiteFTS5Drop(db, defaultVectorStoreDocumentFTS5); err != nil {
//				log.Warnf("failed to drop orphan %s fts5 index: %v", baseTable, err)
//			}
//			return
//		}
//		if err := EnsureVectorStoreDocumentFTS5(db); err != nil {
//			log.Warnf("failed to setup %s fts5 index: %v", (&schema.VectorStoreDocument{}).TableName(), err)
//		}
//	})
//}

func EnsureVectorStoreDocumentFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	if err := bizhelper.SQLiteFTS5Setup(db, defaultVectorStoreDocumentFTS5); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

func SearchVectorStoreDocumentBM25(db *gorm.DB, filter *VectorDocumentFilter, limit, offset int) ([]*schema.VectorStoreDocument, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}
	var match string
	if filter != nil {
		match = strings.TrimSpace(filter.Keywords)
	}
	var res = make([]*schema.VectorStoreDocument, 0)
	if len(match) < 3 || !schema.IsSQLite(db) || !db.HasTable(defaultVectorStoreDocumentFTS5.FTSTable) {
		if err := FilterVectorDocuments(db, filter).Limit(limit).Offset(offset).Find(&res).Error; err != nil {
			return nil, err
		}
		return res, nil
	}
	filter.Keywords = "" // if use FTS5, clear keywords in filter to avoid double filtering
	return bizhelper.SQLiteFTS5BM25Match[*schema.VectorStoreDocument](FilterVectorDocuments(db, filter), defaultVectorStoreDocumentFTS5, match, limit, offset)
}

func SearchVectorStoreDocumentBM25Yield(ctx context.Context, db *gorm.DB, filter *VectorDocumentFilter, options ...bizhelper.YieldModelOpts) chan *schema.VectorStoreDocument {
	if db == nil {
		return nil
	}
	var match string
	if filter != nil {
		match = strings.TrimSpace(filter.Keywords)
	}
	if len(match) < 3 || !schema.IsSQLite(db) || !db.HasTable(defaultVectorStoreDocumentFTS5.FTSTable) {
		return YieldVectorDocument(ctx, db, filter, options...)
	}
	filter.Keywords = "" // if use FTS5, clear keywords in filter to avoid double filtering
	return bizhelper.SQLiteFTS5BM25MatchYield[*schema.VectorStoreDocument](ctx, FilterVectorDocuments(db, filter), defaultVectorStoreDocumentFTS5, match, options...)
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
