package yakit

import (
	"fmt"
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

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// 1. Create virtual table
	createSQL := fmt.Sprintf(
		"CREATE VIRTUAL TABLE IF NOT EXISTS %q USING fts5(%s, content='yak_scripts', content_rowid='id', tokenize='trigram');",
		yakScriptAIFTSTable,
		quoteColumns(yakScriptAIFTSColumns),
	)
	if err := tx.Exec(createSQL).Error; err != nil {
		tx.Rollback()
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}

	// 2. Filtered data migration: only enable_for_ai=1 rows
	insertCols := "rowid, " + quoteColumns(yakScriptAIFTSColumns)
	selectCols := `"id", ` + quoteColumns(yakScriptAIFTSColumns)
	migrateSQL := fmt.Sprintf(
		"INSERT OR IGNORE INTO %q(%s) SELECT %s FROM \"yak_scripts\" WHERE \"enable_for_ai\" = 1;",
		yakScriptAIFTSTable, insertCols, selectCols,
	)
	if err := tx.Exec(migrateSQL).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 3. Conditional triggers
	if err := bindYakScriptAIFTSTriggers(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func quoteColumns(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = fmt.Sprintf("%q", c)
	}
	return strings.Join(quoted, ", ")
}

func bindYakScriptAIFTSTriggers(db *gorm.DB) error {
	fts := fmt.Sprintf("%q", yakScriptAIFTSTable)
	ftsCmdCol := fmt.Sprintf("%q", yakScriptAIFTSTable)

	colsCSV := quoteColumns(yakScriptAIFTSColumns)
	newVals := make([]string, len(yakScriptAIFTSColumns))
	oldVals := make([]string, len(yakScriptAIFTSColumns))
	for i, col := range yakScriptAIFTSColumns {
		newVals[i] = fmt.Sprintf("new.%q", col)
		oldVals[i] = fmt.Sprintf("old.%q", col)
	}
	newValsCSV := strings.Join(newVals, ", ")
	oldValsCSV := strings.Join(oldVals, ", ")

	insertCols := fmt.Sprintf("rowid, %s", colsCSV)
	newInsertVals := fmt.Sprintf("new.\"id\", %s", newValsCSV)
	deleteSuffix := fmt.Sprintf(", %s", colsCSV)
	oldDeleteSuffix := fmt.Sprintf(", %s", oldValsCSV)

	const triggerPrefix = "yak_scripts_ai_fts5"
	aiName := fmt.Sprintf("%q", triggerPrefix+"_ai")
	auDelName := fmt.Sprintf("%q", triggerPrefix+"_au_del")
	auInsName := fmt.Sprintf("%q", triggerPrefix+"_au_ins")
	adName := fmt.Sprintf("%q", triggerPrefix+"_ad")

	// Drop old triggers first (including legacy single _au trigger)
	legacyAU := fmt.Sprintf("%q", triggerPrefix+"_au")
	for _, name := range []string{aiName, auDelName, auInsName, adName, legacyAU} {
		if err := db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s;", name)).Error; err != nil {
			return err
		}
	}

	// AFTER INSERT: only when new row has enable_for_ai=1
	aiSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER INSERT ON "yak_scripts" WHEN NEW."enable_for_ai" = 1 BEGIN
  INSERT INTO %s(%s) VALUES (%s);
END;`, aiName, fts, insertCols, newInsertVals)

	// AFTER DELETE: only when old row had enable_for_ai=1
	adSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER DELETE ON "yak_scripts" WHEN OLD."enable_for_ai" = 1 BEGIN
  INSERT INTO %s(%s, rowid%s) VALUES ('delete', old."id"%s);
END;`, adName, fts, ftsCmdCol, deleteSuffix, oldDeleteSuffix)

	// AFTER UPDATE (delete phase): remove old FTS entry when old row was AI-enabled
	auDelSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER UPDATE ON "yak_scripts" WHEN OLD."enable_for_ai" = 1 BEGIN
  INSERT INTO %s(%s, rowid%s) VALUES ('delete', old."id"%s);
END;`, auDelName, fts, ftsCmdCol, deleteSuffix, oldDeleteSuffix)

	// AFTER UPDATE (insert phase): add new FTS entry when new row is AI-enabled
	auInsSQL := fmt.Sprintf(`CREATE TRIGGER %s AFTER UPDATE ON "yak_scripts" WHEN NEW."enable_for_ai" = 1 BEGIN
  INSERT INTO %s(%s) VALUES (%s);
END;`, auInsName, fts, insertCols, newInsertVals)

	for _, s := range []string{aiSQL, adSQL, auDelSQL, auInsSQL} {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
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

	fts := fmt.Sprintf("%q", yakScriptAIFTSTable)
	deleteAllSQL := fmt.Sprintf("INSERT INTO %s(%s) VALUES('delete-all');", fts, fts)
	if err := db.Exec(deleteAllSQL).Error; err != nil {
		return err
	}

	insertCols := "rowid, " + quoteColumns(yakScriptAIFTSColumns)
	selectCols := `"id", ` + quoteColumns(yakScriptAIFTSColumns)
	populateSQL := fmt.Sprintf(
		"INSERT INTO %s(%s) SELECT %s FROM \"yak_scripts\" WHERE \"enable_for_ai\" = 1;",
		fts, insertCols, selectCols,
	)
	return db.Exec(populateSQL).Error
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

	// Clear keywords from filter to avoid double-filtering (FTS handles matching)
	if filter != nil {
		filter.Keywords = nil
	}

	return bizhelper.SQLiteFTS5BM25Match[*schema.YakScript](
		FilterYakScriptForAI(db, filter), defaultYakScriptForAIFTS5, matches, limit, offset,
	)
}
