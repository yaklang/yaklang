package ssadb

import "sync"

// SQLite has one writer per database file. SSA cache savers and the name pool
// share the same file but enter through different packages; coordinate their
// write operations before opening a transaction. Reads do not take this lock.
var sqliteWriteMu sync.Mutex

func WithSQLiteWriteLock(dialect string, work func() error) error {
	switch dialect {
	case "sqlite3", "sqlite3_extended":
		sqliteWriteMu.Lock()
		defer sqliteWriteMu.Unlock()
	}
	return work()
}
