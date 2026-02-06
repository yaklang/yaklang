package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func AIYakToolVTableName() string {
	return (&schema.AIYakTool{}).TableName() + "_fts"
}

type AIYakToolFilter struct {
	ToolNames     []string
	ToolPaths     []string
	OnlyFavorites bool
	Keywords      string
}

func FilterAIYakTools(db *gorm.DB, filter *AIYakToolFilter) *gorm.DB {
	db = db.Model(&schema.AIYakTool{})
	if filter == nil {
		return db
	}

	db = bizhelper.ExactQueryStringArrayOr(db, "name", filter.ToolNames)
	db = bizhelper.ExactQueryStringArrayOr(db, "path", filter.ToolPaths)
	if filter.OnlyFavorites {
		db = db.Where("is_favorite = ?", true)
	}
	db = bizhelper.FuzzSearchEx(db, []string{"name", "keywords", "description", "path"}, filter.Keywords, false)
	return db
}

var defaultAIYakToolFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel: &schema.AIYakTool{},
	FTSTable:  AIYakToolVTableName(),
	Columns:   []string{"name", "verbose_name", "description", "keywords", "path"},
	// Use external content mode to keep the FTS index consistent and avoid FTS5 maintenance commands
	// that are not supported by all SQLite builds for contentful FTS tables.
	ContentTable: "ai_yak_tools",
	Tokenize:     "trigram",
}

//func init() {
//	// Ensure AIYakTool has a proper FTS index in SQLite profile DB to accelerate searching.
//	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_PROFILE_DATABASE, func(db *gorm.DB) {
//		if db == nil {
//			return
//		}
//		if !schema.IsSQLite(db) {
//			return
//		}
//		baseTable := (&schema.AIYakTool{}).TableName()
//		if !db.HasTable(baseTable) {
//			// Base table is gone, but the FTS virtual table may remain; clean it up.
//			if err := bizhelper.SQLiteFTS5Drop(db, defaultAIYakToolFTS5); err != nil {
//				log.Warnf("failed to drop orphan ai_yak_tools fts5 index: %v", err)
//			}
//			return
//		}
//		if err := EnsureAIYakToolFTS5(db); err != nil {
//			log.Warnf("failed to setup ai_yak_tools fts5 index: %v", err)
//		}
//	})
//}

func EnsureAIYakToolFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	if err := bizhelper.SQLiteFTS5Setup(db, defaultAIYakToolFTS5); err != nil {
		// Some sqlite builds might not include FTS5 (e.g. custom builds).
		// Treat it as non-fatal and let the caller decide how to handle.
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

// SearchAIYakToolBM25 uses SQLite FTS5 BM25 ranking to search AIYakTool.
// It follows the same pattern as SearchVectorStoreDocumentBM25:
// - Extract match from filter.Keywords
// - For short keywords (<3), fall back to LIKE-based search
// - For longer keywords, clear filter.Keywords and apply FTS (to avoid double filtering)
func SearchAIYakToolBM25(db *gorm.DB, filter *AIYakToolFilter, limit, offset int) ([]*schema.AIYakTool, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}

	var match string
	if filter != nil {
		match = strings.TrimSpace(filter.Keywords)
	}
	if match == "" {
		return []*schema.AIYakTool{}, nil
	}

	var res = make([]*schema.AIYakTool, 0)
	if len(match) < 3 || !schema.IsSQLite(db) || !db.HasTable(defaultAIYakToolFTS5.FTSTable) {
		if err := FilterAIYakTools(db, filter).Limit(limit).Offset(offset).Find(&res).Error; err != nil {
			return nil, err
		}
		return res, nil
	}

	if filter != nil {
		filter.Keywords = ""
	}

	return bizhelper.SQLiteFTS5BM25Match[*schema.AIYakTool](FilterAIYakTools(db, filter), defaultAIYakToolFTS5, match, limit, offset)
}
