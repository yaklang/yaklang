package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

// AIForgeVTableName returns the FTS5 virtual table name for AIForge.
func AIForgeVTableName() string {
	return "ai_forges_fts"
}

// AIForgeSearchFilter is a lightweight filter for intent-oriented forge search.
// It does not depend on ypb.AIForgeFilter to avoid protobuf coupling in the intent layer.
type AIForgeSearchFilter struct {
	ForgeNames []string
	Keywords   []string
}

// aiForgesTable is the GORM table name for AIForge.
// Used to qualify column names when JOINing with the FTS5 table.
const aiForgesTable = "ai_forges"

// FilterAIForgeForSearch builds a GORM query from AIForgeSearchFilter.
// This applies keyword-based LIKE search across forge_name, forge_verbose_name,
// description, tool_keywords, and tags.
// Column names are qualified with the base table name to avoid ambiguity when
// JOINed with the FTS5 virtual table (which indexes some of the same columns).
func FilterAIForgeForSearch(db *gorm.DB, filter *AIForgeSearchFilter) *gorm.DB {
	db = db.Model(&schema.AIForge{})
	if filter == nil {
		return db
	}

	// Qualify column names with base table to avoid ambiguity during BM25 JOIN
	db = bizhelper.ExactQueryStringArrayOr(db, aiForgesTable+".forge_name", filter.ForgeNames)
	var keywords []string
	for _, kw := range filter.Keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		keywords = append(keywords, kw)
	}
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		aiForgesTable + ".forge_name",
		aiForgesTable + ".forge_verbose_name",
		aiForgesTable + ".description",
		aiForgesTable + ".tool_keywords",
		aiForgesTable + ".tags",
	}, keywords, false)
	return db
}

// defaultAIForgeFTS5 defines the FTS5 trigram index configuration for AIForge.
// Uses external content mode referencing the ai_forges table.
// Indexes: forge_name, forge_verbose_name, description, tool_keywords, tags
var defaultAIForgeFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel:    &schema.AIForge{},
	FTSTable:     AIForgeVTableName(),
	Columns:      []string{"forge_name", "forge_verbose_name", "description", "tool_keywords", "tags"},
	ContentTable: "ai_forges",
	Tokenize:     "trigram",
}

// EnsureAIForgeFTS5 creates or updates the FTS5 trigram index for AIForge.
// Safe to call multiple times; idempotent.
func EnsureAIForgeFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	if err := bizhelper.SQLiteFTS5Setup(db, defaultAIForgeFTS5); err != nil {
		// Some sqlite builds might not include FTS5 (e.g. custom builds).
		// Treat it as non-fatal and let the caller decide how to handle.
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

// SearchAIForgeBM25 uses SQLite FTS5 BM25 ranking to search AIForge.
// Follows the same dual-channel pattern as SearchAIYakToolBM25:
//   - For short keywords (<3 chars): fall back to LIKE-based search via FuzzSearchEx
//   - For longer keywords: use FTS5 BM25 trigram matching for ranked results
//   - If FTS5 table is not available: fall back to LIKE-based search
func SearchAIForgeBM25(db *gorm.DB, filter *AIForgeSearchFilter, limit, offset int) ([]*schema.AIForge, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}

	var matches []string
	if filter != nil {
		for _, m := range filter.Keywords {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return []*schema.AIForge{}, nil
	}

	var res = make([]*schema.AIForge, 0)
	maxLen := 0
	for _, m := range matches {
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}
	// Short keywords or non-SQLite or no FTS table: fall back to LIKE search
	if maxLen < 3 || !schema.IsSQLite(db) || !db.HasTable(defaultAIForgeFTS5.FTSTable) {
		if err := FilterAIForgeForSearch(db, filter).Limit(limit).Offset(offset).Find(&res).Error; err != nil {
			return nil, err
		}
		return res, nil
	}

	// BM25 path: clear Keywords to avoid double-filtering, then use FTS5
	if filter != nil {
		filter.Keywords = nil
	}

	return bizhelper.SQLiteFTS5BM25Match[*schema.AIForge](FilterAIForgeForSearch(db, filter), defaultAIForgeFTS5, matches, limit, offset)
}
