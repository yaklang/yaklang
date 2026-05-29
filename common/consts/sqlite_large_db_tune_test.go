package consts

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTuneSQLiteByDatabaseFileSize(t *testing.T) {
	sqliteLargeDBTuneLast = sync.Map{}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "large.db")

	db, err := createAndConfigDatabase(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.DB().Close())

	wal, err := os.OpenFile(dbPath+"-wal", os.O_CREATE|os.O_WRONLY, 0o666)
	require.NoError(t, err)
	require.NoError(t, wal.Truncate(1536*1024*1024))
	require.NoError(t, wal.Close())

	db, err = createAndConfigDatabase(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DB().Close() })

	require.True(t, TuneSQLiteByDatabaseFileSize(db, dbPath))
	require.False(t, TuneSQLiteByDatabaseFileSize(db, dbPath))

	var cacheSize int64
	require.NoError(t, db.Raw("PRAGMA cache_size;").Row().Scan(&cacheSize))
	require.Equal(t, int64(-sqliteLargeDBTune128M), cacheSize)
}

func TestSqliteDatabaseOnDiskBytesIncludesWal(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "x.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("main"), 0o644))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("wal"), 0o644))

	require.Equal(t, int64(7), sqliteDatabaseOnDiskBytes(dbPath))
}

func TestSqliteLargeDBTuneLevelForSize(t *testing.T) {
	require.Equal(t, sqliteLargeDBTuneNone, sqliteLargeDBTuneLevelForSize(100*1024*1024))
	require.Equal(t, sqliteLargeDBTune32M, sqliteLargeDBTuneLevelForSize(128*1024*1024))
	require.Equal(t, sqliteLargeDBTune256M, sqliteLargeDBTuneLevelForSize(3*1024*1024*1024))
}
