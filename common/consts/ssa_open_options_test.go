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
}

func TestSSADatabaseMaxOpenConnsEnv(t *testing.T) {
	t.Setenv(ENV_SSA_SQLITE_MAX_OPEN_CONNS, "4")
	db, err := CreateSSAProjectDatabaseRaw(filepath.Join(t.TempDir(), "ssa-open-env.db"))
	require.NoError(t, err)
	defer db.Close()
	require.Equal(t, 4, db.DB().Stats().MaxOpenConnections,
		"SSA SQLite pool must honor YAK_SSA_SQLITE_MAX_OPEN_CONNS override")
}
