package consts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTuneSQLiteByDatabaseFileSize(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "large.db")

	db, err := createAndConfigDatabase(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.DB().Close())

	// Inflate WAL size so tuning kicks in without corrupting the main db file.
	wal, err := os.OpenFile(dbPath+"-wal", os.O_CREATE|os.O_WRONLY, 0o666)
	require.NoError(t, err)
	require.NoError(t, wal.Truncate(1536*1024*1024))
	require.NoError(t, wal.Close())

	db, err = createAndConfigDatabase(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DB().Close() })

	TuneSQLiteByDatabaseFileSize(db, dbPath)

	var cacheSize int64
	require.NoError(t, db.Raw("PRAGMA cache_size;").Row().Scan(&cacheSize))
	require.Less(t, cacheSize, int64(0), "large db should use negative KiB cache_size")
	require.LessOrEqual(t, cacheSize, int64(-131072))
}

func TestSqliteDatabaseOnDiskBytesIncludesWal(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "x.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("main"), 0o644))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("wal"), 0o644))

	require.Equal(t, int64(7), sqliteDatabaseOnDiskBytes(dbPath))
}
