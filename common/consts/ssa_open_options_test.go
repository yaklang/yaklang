package consts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSADatabaseMaxOpenConnsDefault(t *testing.T) {
	t.Setenv(ENV_SSA_SQLITE_MAX_OPEN_CONNS, "")
	db, err := CreateSSAProjectDatabaseRaw(filepath.Join(t.TempDir(), "ssa-open-default.db"))
	require.NoError(t, err)
	defer db.Close()
	require.Equal(t, 8, db.DB().Stats().MaxOpenConnections,
		"default SSA SQLite pool must allow concurrent readers (single writer via _txlock=immediate)")

	var syncMode string
	require.NoError(t, db.Raw("PRAGMA synchronous").Row().Scan(&syncMode))
	require.Equal(t, "1", syncMode, "SSA SQLite must use synchronous=NORMAL")
}

func TestSSADatabaseMaxOpenConnsEnv(t *testing.T) {
	t.Setenv(ENV_SSA_SQLITE_MAX_OPEN_CONNS, "4")
	db, err := CreateSSAProjectDatabaseRaw(filepath.Join(t.TempDir(), "ssa-open-env.db"))
	require.NoError(t, err)
	defer db.Close()
	require.Equal(t, 4, db.DB().Stats().MaxOpenConnections,
		"SSA SQLite pool must honor YAK_SSA_SQLITE_MAX_OPEN_CONNS override")

	var syncMode string
	require.NoError(t, db.Raw("PRAGMA synchronous").Row().Scan(&syncMode))
	require.Equal(t, "1", syncMode, "SSA SQLite must keep synchronous=NORMAL under pool override")
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
