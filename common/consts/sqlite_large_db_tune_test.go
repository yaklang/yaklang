package consts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

// TestTuneSQLiteByDatabaseFileSizeBelowThreshold verifies that a small
// database (<128MB) does not trigger tuning.
func TestTuneSQLiteByDatabaseFileSizeBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "small.db")
	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	db.Exec("CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY)")
	db.Exec("INSERT INTO test (id) VALUES (1)")

	result := TuneSQLiteByDatabaseFileSize(db, dbPath)
	require.False(t, result, "small DB should not trigger tuning")
}

// TestTuneSQLiteByDatabaseFileSizeAboveThreshold verifies that a large
// database (>=128MB) triggers tuning and applies PRAGMAs.
func TestTuneSQLiteByDatabaseFileSizeAboveThreshold(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "large.db")
	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Create a table and insert enough data to exceed 128MB
	db.Exec("CREATE TABLE IF NOT EXISTS big (id INTEGER PRIMARY KEY, data TEXT)")
	for i := 0; i < 130; i++ {
		largeData := make([]byte, 1024*1024) // 1MB
		db.Exec("INSERT INTO big (id, data) VALUES (?, ?)", i, string(largeData))
	}

	fi, err := os.Stat(dbPath)
	require.NoError(t, err)
	require.Greater(t, fi.Size(), int64(128*1024*1024), "DB should be >= 128MB")

	// Clear any previous tune state for this path
	sqliteLargeDBTuneLast.Delete(sqliteTunePathKey(dbPath))

	// First call should tune
	result := TuneSQLiteByDatabaseFileSize(db, dbPath)
	require.True(t, result, "large DB should trigger tuning on first call")

	// Verify PRAGMAs were applied using Row().Scan (not Raw().Scan)
	var cacheSize int
	row := db.Raw("PRAGMA cache_size").Row()
	require.NoError(t, row.Scan(&cacheSize), "PRAGMA cache_size should be readable")
	require.Negative(t, cacheSize, "cache_size should be negative (KiB mode) after tuning")

	// Second call at same tier should NOT re-tune
	result2 := TuneSQLiteByDatabaseFileSize(db, dbPath)
	require.False(t, result2, "same tier should not re-tune")
}

// TestTuneSQLiteByDatabaseFileSizeNilDB verifies nil safety.
func TestTuneSQLiteByDatabaseFileSizeNilDB(t *testing.T) {
	require.False(t, TuneSQLiteByDatabaseFileSize(nil, "/tmp/test.db"))
	require.False(t, TuneSQLiteByDatabaseFileSize(nil, ""))
}

// TestTuneSQLiteByDatabaseFileSizeEmptyPath verifies empty path safety.
func TestTuneSQLiteByDatabaseFileSizeEmptyPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")
	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()

	require.False(t, TuneSQLiteByDatabaseFileSize(db, ""))
}
