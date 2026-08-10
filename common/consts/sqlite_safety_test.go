package consts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSQLiteSynchronousNotOff proves that the SQLite synchronous PRAGMA
// is not set to OFF (which risks database corruption on power loss).
// It should be NORMAL (safe with WAL mode) or FULL.
func TestSQLiteSynchronousNotOff(t *testing.T) {
	// Create a temp SQLite DB via CreateSSAProjectDatabase
	// and check the synchronous PRAGMA value
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test-synchronous.db"
	db, err := CreateSSAProjectDatabase(SQLiteExtend, dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Query the synchronous PRAGMA
	var syncMode string
	err = db.Raw("PRAGMA synchronous").Row().Scan(&syncMode)
	require.NoError(t, err)
	t.Logf("PRAGMA synchronous = %s", syncMode)

	// Must not be OFF (0) — should be NORMAL (1) or FULL (2)
	require.NotEqual(t, "0", syncMode,
		"PRAGMA synchronous must not be OFF (risks corruption)")
	require.NotEqual(t, "off", syncMode,
		"PRAGMA synchronous must not be off")
}
