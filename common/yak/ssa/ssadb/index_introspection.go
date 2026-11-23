package ssadb

import (
	"strings"

	"github.com/yaklang/gorm"
)

// namedIndexDefinition returns the stored CREATE INDEX text for the dialects
// whose catalog is readable. SQLite keeps it in sqlite_master; PostgreSQL
// exposes it through pg_indexes. MySQL and DM have no portable equivalent, so
// callers fall back to HasIndex and leave an existing index untouched.
func namedIndexDefinition(db *gorm.DB, table, index string) (string, bool) {
	if db == nil || db.Dialect() == nil {
		return "", false
	}
	switch dialectOf(db) {
	case sqlDialectSQLite:
		var ddl string
		err := db.Raw(
			`SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?`,
			table, index,
		).Row().Scan(&ddl)
		return ddl, err == nil && strings.TrimSpace(ddl) != ""
	case sqlDialectPostgres:
		var ddl string
		query := bindSQLPlaceholdersDB(db,
			`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename::text = ? AND indexname::text = ?`)
		err := db.Raw(query, table, index).Row().Scan(&ddl)
		return ddl, err == nil && strings.TrimSpace(ddl) != ""
	default:
		return "", false
	}
}
