package ssadb

import (
	"strings"
	"sync"

	"github.com/yaklang/gorm"
)

// Index DDL is not fully portable, so the capability questions that change the
// statement are answered by the indexDialect implementations in
// index_dialect.go:
//
//   - partial indexes (CREATE INDEX ... WHERE ...): SQLite and PostgreSQL only.
//   - expression key parts: every backend accepts one, but MySQL needs it
//     wrapped in its own parentheses and only supports it from 8.0.13; MariaDB
//     has no equivalent syntax.
//
// This file only answers the MySQL version question the MySQL adapter needs.
// Everything else (identifier quoting, catalog lookups, error vocabulary) is
// already owned by the dialect.

// mysqlFunctionalIndexMinVersion is MySQL's first release with functional key
// parts, encoded as major*10000 + minor*100 + patch.
const mysqlFunctionalIndexMinVersion = 80013

// mysqlFunctionalIndexCache remembers the answer per connection pool; the
// server version cannot change for the life of a pool.
var mysqlFunctionalIndexCache sync.Map // *sql.DB -> bool

// mysqlFunctionalIndexSupported reports whether this MySQL server accepts
// "(expression)" as an index key part. An unreadable version is treated as
// unsupported: emitting a statement the server rejects on every open is worse
// than leaving the index out and saying so once.
func mysqlFunctionalIndexSupported(db *gorm.DB) bool {
	if db == nil || db.DB() == nil {
		return false
	}
	pool := db.DB()
	if cached, ok := mysqlFunctionalIndexCache.Load(pool); ok {
		return cached.(bool)
	}
	var version string
	supported := false
	if err := pool.QueryRow("SELECT VERSION()").Scan(&version); err == nil {
		supported = mysqlVersionSupportsFunctionalIndex(version)
	}
	mysqlFunctionalIndexCache.Store(pool, supported)
	return supported
}

// mysqlVersionSupportsFunctionalIndex parses values such as "8.0.36-log",
// "8.0.13" or "5.7.44". MariaDB reports its own version and has no functional
// key parts, so it never qualifies.
func mysqlVersionSupportsFunctionalIndex(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	if version == "" || strings.Contains(version, "mariadb") {
		return false
	}
	if dash := strings.IndexByte(version, '-'); dash >= 0 {
		version = version[:dash]
	}
	fields := strings.Split(version, ".")
	if len(fields) < 3 {
		return false
	}
	encoded := leadingInt(fields[0])*10000 + leadingInt(fields[1])*100 + leadingInt(fields[2])
	return encoded >= mysqlFunctionalIndexMinVersion
}

// leadingInt reads the leading decimal digits of s, or 0 when there are none.
func leadingInt(s string) int {
	value := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		value = value*10 + int(r-'0')
	}
	return value
}
