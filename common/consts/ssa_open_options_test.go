package consts

import (
	"context"
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
	require.Equal(t, 60000, opt.sqliteBusyTimeoutMS)
	require.Equal(t, int64(64*1024*1024), opt.sqliteJournalSizeLimitBytes)
	require.Equal(t, int64(256*1024), opt.sqliteCacheKiB, "SSA compile writes need a 256MB page cache, not the 32MB default")
	require.Equal(t, int64(512*1024*1024), opt.sqliteMmapBytes)
	require.True(t, opt.sqliteWALAutocheckpointSet)
	require.Equal(t, 0, opt.sqliteWALAutocheckpoint, "automatic checkpoints rewrite the growing database; SSA folds the WAL once at the end")

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

	var busyTimeout int
	require.NoError(t, db.Raw("PRAGMA busy_timeout").Row().Scan(&busyTimeout))
	require.Equal(t, 60000, busyTimeout)

	// journal_size_limit is applied by the driver hook, so it has to hold on
	// every pooled connection, not just the one db.Exec happened to reach.
	ctx := context.Background()
	for i := 0; i < opt.sqliteMaxOpenConns; i++ {
		conn, err := db.DB().Conn(ctx)
		require.NoError(t, err)
		var limit int64
		err = conn.QueryRowContext(ctx, "PRAGMA journal_size_limit").Scan(&limit)
		require.NoError(t, err)
		require.Equal(t, int64(64*1024*1024), limit, "connection %d", i)
		var cacheKiB int64
		err = conn.QueryRowContext(ctx, "PRAGMA cache_size").Scan(&cacheKiB)
		require.NoError(t, err)
		require.Equal(t, int64(-256*1024), cacheKiB, "connection %d cache", i)
		var checkpoint int
		err = conn.QueryRowContext(ctx, "PRAGMA wal_autocheckpoint").Scan(&checkpoint)
		conn.Close()
		require.NoError(t, err)
		require.Equal(t, 0, checkpoint, "connection %d checkpoint", i)
	}
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
