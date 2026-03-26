package yakit

import (
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

const yakScriptAIFTSTable = "yak_scripts_ai_fts"

var yakScriptFTSOnce sync.Once

var yakScriptAIFTSColumns = []string{"script_name", "help", "tags", "ai_desc", "ai_keywords"}

var defaultYakScriptForAIFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel:    &schema.YakScript{},
	FTSTable:     yakScriptAIFTSTable,
	Columns:      yakScriptAIFTSColumns,
	ContentTable: "yak_scripts",
	Tokenize:     "trigram",
	IndexedRows: &bizhelper.SQLiteFTS5IndexedRows{
		Column: "enable_for_ai",
		Value:  true,
	},
}

// EnsureYakScriptForAIFTS5 creates the FTS5 trigram index that only covers
// enable_for_ai=true rows in the yak_scripts table.
// It uses conditional triggers (WHEN enable_for_ai=1) so that the thousands
// of non-AI plugins are never touched by the FTS engine.
func EnsureYakScriptForAIFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	if err := bizhelper.SQLiteFTS5Setup(db, defaultYakScriptForAIFTS5); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

type YakScriptForAIFilter struct {
	Keywords []string
}

func FilterYakScriptForAI(db *gorm.DB, filter *YakScriptForAIFilter) *gorm.DB {
	db = db.Model(&schema.YakScript{}).
		Where("\"enable_for_ai\" = ?", true).
		Where("\"type\" IN (?)", []string{"yak", "mitm", "port-scan"})
	if filter == nil {
		return db
	}

	var keywords []string
	for _, kw := range filter.Keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"script_name", "help", "tags", "ai_desc", "ai_keywords",
	}, keywords, false)
	return db
}

// RebuildYakScriptAIFTS performs a filtered rebuild of the FTS5 index,
// re-indexing only enable_for_ai=true rows. Unlike FTS5's built-in 'rebuild'
// command (which reads ALL rows from the content table), this preserves the
// selective indexing design.
func RebuildYakScriptAIFTS(db *gorm.DB) error {
	if db == nil || !schema.IsSQLite(db) {
		return nil
	}
	if !db.HasTable(yakScriptAIFTSTable) {
		return nil
	}
	return bizhelper.SQLiteFTS5Rebuild(db, defaultYakScriptForAIFTS5)
}

// ensureYakScriptFTSLazy initializes the FTS5 index on first use.
func ensureYakScriptFTSLazy(db *gorm.DB) {
	if db == nil || !schema.IsSQLite(db) {
		return
	}
	yakScriptFTSOnce.Do(func() {
		if err := EnsureYakScriptForAIFTS5(db); err != nil {
			log.Warnf("lazy init yak_scripts_ai_fts failed: %v", err)
			return
		}
		if err := RebuildYakScriptAIFTS(db); err != nil {
			log.Warnf("lazy rebuild yak_scripts_ai_fts failed: %v", err)
		}
	})
}

// SearchYakScriptForAIBM25 searches enable_for_ai=true YakScripts using
// BM25 ranking when FTS5 is available. Falls back to LIKE for short keywords,
// non-SQLite databases, or when the FTS table is missing.
func SearchYakScriptForAIBM25(db *gorm.DB, filter *YakScriptForAIFilter, limit, offset int) ([]*schema.YakScript, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}

	ensureYakScriptFTSLazy(db)

	var matches []string
	if filter != nil {
		for _, m := range filter.Keywords {
			m = strings.TrimSpace(m)
			if m != "" {
				matches = append(matches, m)
			}
		}
	}
	if len(matches) == 0 {
		return []*schema.YakScript{}, nil
	}

	maxLen := 0
	for _, m := range matches {
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}

	if maxLen < 3 || !schema.IsSQLite(db) || !db.HasTable(yakScriptAIFTSTable) {
		var res []*schema.YakScript
		if err := FilterYakScriptForAI(db, filter).Limit(limit).Offset(offset).Find(&res).Error; err != nil {
			return nil, err
		}
		return res, nil
	}

	return bizhelper.SQLiteFTS5BM25Match[*schema.YakScript](
		FilterYakScriptForAI(db, cloneYakScriptForAIFilterWithKeywords(filter, nil)), defaultYakScriptForAIFTS5, matches, limit, offset,
	)
}

func cloneYakScriptForAIFilterWithKeywords(filter *YakScriptForAIFilter, keywords []string) *YakScriptForAIFilter {
	if filter == nil {
		if keywords == nil {
			return nil
		}
		return &YakScriptForAIFilter{Keywords: append([]string(nil), keywords...)}
	}
	cloned := *filter
	if keywords == nil {
		cloned.Keywords = nil
	} else {
		cloned.Keywords = append([]string(nil), keywords...)
	}
	return &cloned
}
