package consts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSADatabaseOpenOptions(t *testing.T) {
	opt := ssaDatabaseOpenOptions()
	require.Equal(t, 8, ssaSQLiteMaxOpenConns)
	require.Equal(t, "NORMAL", ssaSQLiteSynchronous)
	require.Equal(t, "immediate", ssaSQLiteTxLock)
	require.Equal(t, ssaSQLiteMaxOpenConns, opt.sqliteMaxOpenConns)
	require.True(t, opt.sqlitePrivateCache)
	require.Equal(t, ssaSQLiteSynchronous, opt.sqliteSynchronous)
	require.Equal(t, ssaSQLiteTxLock, opt.sqliteTxLock)

	projectOpt := defaultDatabaseOpenOptions()
	require.Equal(t, 1, projectOpt.sqliteMaxOpenConns)
	require.False(t, projectOpt.sqlitePrivateCache)
	require.Empty(t, projectOpt.sqliteSynchronous)
	require.Empty(t, projectOpt.sqliteTxLock)

	db, err := CreateSSAProjectDatabaseRaw(filepath.Join(t.TempDir(), "ssa-open-default.db"))
	require.NoError(t, err)
	defer db.Close()
	require.Equal(t, opt.sqliteMaxOpenConns, db.DB().Stats().MaxOpenConnections,
		"SSA SQLite pool must allow concurrent readers (single writer via _txlock=immediate)")

	var syncMode string
	require.NoError(t, db.Raw("PRAGMA synchronous").Row().Scan(&syncMode))
	require.Equal(t, "1", syncMode, "SSA SQLite must use synchronous=NORMAL")

	var journalMode string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Row().Scan(&journalMode))
	require.Equal(t, "wal", journalMode)
}

func TestSSADatabaseSynchronousNotOff(t *testing.T) {
	db, err := CreateSSAProjectDatabase(SQLiteExtend, filepath.Join(t.TempDir(), "test-synchronous.db"))
	require.NoError(t, err)
	defer db.Close()

	var syncMode string
	require.NoError(t, db.Raw("PRAGMA synchronous").Row().Scan(&syncMode))
	require.NotEqual(t, "0", syncMode, "PRAGMA synchronous must not be OFF (risks corruption)")
	require.NotEqual(t, "off", syncMode, "PRAGMA synchronous must not be off")
}
