package ssadb

import "github.com/yaklang/gorm"

// BeginSQLiteBulkDelete relaxes SQLite durability for a large DELETE and
// returns a function that restores the compile write profile.
//
// Deleting a multi-million-row program with journaling FULL writes every row
// image through the WAL before the compile can start. The delete is a
// destructive pre-step of a full recompile: the old rows are gone either way,
// and a crash leaves the program to be deleted again on the next attempt.
// Non-SQLite databases keep their defaults.
func BeginSQLiteBulkDelete(db *gorm.DB) func() {
	if !isSQLiteDB(db) || db.DB() == nil {
		return func() {}
	}
	db.Exec("PRAGMA synchronous = OFF;")
	db.Exec("PRAGMA wal_autocheckpoint = 0;")
	db.Exec("PRAGMA cache_size = -1048576;")
	return func() {
		db.Exec("PRAGMA synchronous = FULL;")
		db.Exec("PRAGMA wal_autocheckpoint = 20000;")
	}
}

// BeginSQLiteSingleConn pins the pool to one connection so PRAGMAs and the
// bulk DELETE run on the same connection. SQLite PRAGMAs are per-connection;
// the SSA pool otherwise has eight connections and the delete could land on
// one that still has FULL synchronous and WAL autocheckpoint enabled.
func BeginSQLiteSingleConn(db *gorm.DB) func() {
	if !isSQLiteDB(db) || db.DB() == nil {
		return func() {}
	}
	sqlDB := db.DB()
	original := sqlDB.Stats().MaxOpenConnections
	if original <= 1 {
		return func() {}
	}
	sqlDB.SetMaxOpenConns(1)
	return func() {
		sqlDB.SetMaxOpenConns(original)
	}
}
