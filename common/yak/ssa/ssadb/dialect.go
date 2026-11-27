package ssadb

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
)

// sqlDialect is the small set of dialect differences the SSA store needs.
// Keep every dialect switch in this file: DDL, catalog checks, identifier
// quoting and regexp syntax otherwise drift apart across store files.
type sqlDialect string

const (
	sqlDialectSQLite   sqlDialect = "sqlite"
	sqlDialectPostgres sqlDialect = "postgres"
	sqlDialectMySQL    sqlDialect = "mysql"
	sqlDialectDM       sqlDialect = "dm"
)

func dialectOf(db *gorm.DB) sqlDialect {
	if db == nil || db.Dialect() == nil {
		return ""
	}
	name := strings.ToLower(strings.TrimSpace(db.Dialect().GetName()))
	switch name {
	case "sqlite", "sqlite3":
		return sqlDialectSQLite
	case "mysql":
		return sqlDialectMySQL
	case "postgres", "postgresql", "cloudsqlpostgres":
		return sqlDialectPostgres
	}
	if damenggorm.IsDM(db) {
		return sqlDialectDM
	}
	return sqlDialect(name)
}

// QuoteIdent quotes one identifier with the active dialect's quote character.
// Raw SQL in SSA must use this instead of hard-coded double quotes: MySQL only
// treats "..." as an identifier under ANSI_QUOTES, which SSA does not set.
func QuoteIdent(db *gorm.DB, ident string) string {
	if db != nil && db.Dialect() != nil {
		return db.Dialect().Quote(ident)
	}
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// RegexpOperator returns the infix regexp operator. PostgreSQL uses "~";
// SQLite and MySQL use "REGEXP". DM support is not verified yet, so it keeps
// the existing REGEXP spelling to avoid changing the DM path in this change.
func RegexpOperator(db *gorm.DB) string {
	switch dialectOf(db) {
	case sqlDialectPostgres:
		return "~"
	default:
		return "REGEXP"
	}
}

// RegexpPredicate builds "<column> <operator> ?" for the active dialect.
func RegexpPredicate(db *gorm.DB, column string) string {
	return column + " " + RegexpOperator(db) + " ?"
}

func supportsIfNotExists(d sqlDialect) bool {
	return d == sqlDialectSQLite || d == sqlDialectPostgres
}

func supportsPartialIndex(d sqlDialect) bool {
	return d == sqlDialectSQLite || d == sqlDialectPostgres
}

func supportsExpressionIndex(d sqlDialect) bool {
	return d == sqlDialectSQLite || d == sqlDialectPostgres || d == sqlDialectMySQL
}
