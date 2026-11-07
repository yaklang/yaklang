package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestBeginSQLiteBulkDeleteRestoresDurability(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)

	restore := BeginSQLiteBulkDelete(db)
	require.Equal(t, 0, sqlitePragmaInt(t, db, "synchronous"))
	require.Equal(t, 0, sqlitePragmaInt(t, db, "wal_autocheckpoint"))
	restore()
	require.Equal(t, 2, sqlitePragmaInt(t, db, "synchronous"))
	require.Equal(t, 20000, sqlitePragmaInt(t, db, "wal_autocheckpoint"))
}

func TestBeginSQLiteSingleConnPinsAndRestoresPool(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(8)

	restore := BeginSQLiteSingleConn(db)
	require.Equal(t, 1, db.DB().Stats().MaxOpenConnections)
	restore()
	require.Equal(t, 8, db.DB().Stats().MaxOpenConnections)
}

func TestBeginSQLiteBulkDeleteNilIsNoop(t *testing.T) {
	require.NotPanics(t, func() {
		BeginSQLiteBulkDelete(nil)()
		BeginSQLiteSingleConn(nil)()
	})
}

func sqlitePragmaInt(t *testing.T, db *gorm.DB, name string) int {
	t.Helper()
	var value int
	require.NoError(t, db.Raw("PRAGMA "+name).Row().Scan(&value))
	return value
}
