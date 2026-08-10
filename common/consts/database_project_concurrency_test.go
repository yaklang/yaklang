package consts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProjectDatabaseOpenOptions(t *testing.T) {
	t.Run("default remains serialized", func(t *testing.T) {
		t.Setenv(YakitSQLiteProjectMaxOpenConnsEnv, "")
		options, err := projectDatabaseOpenOptions()
		require.NoError(t, err)
		require.Equal(t, 1, options.sqliteMaxOpenConns)
		require.False(t, options.sqlitePrivateCache)
	})

	t.Run("concurrent mode uses private cache", func(t *testing.T) {
		t.Setenv(YakitSQLiteProjectMaxOpenConnsEnv, "2")
		options, err := projectDatabaseOpenOptions()
		require.NoError(t, err)
		require.Equal(t, 2, options.sqliteMaxOpenConns)
		require.True(t, options.sqlitePrivateCache)
	})

	t.Run("invalid values fail closed", func(t *testing.T) {
		for _, value := range []string{"invalid", "0", "9"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv(YakitSQLiteProjectMaxOpenConnsEnv, value)
				_, err := projectDatabaseOpenOptions()
				require.ErrorContains(t, err, YakitSQLiteProjectMaxOpenConnsEnv)
			})
		}
	})
}

func TestProjectDatabaseReadPoolOptions(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv(YakitSQLiteProjectReadPoolConnsEnv, "")
		conns, err := projectDatabaseReadPoolConns()
		require.NoError(t, err)
		require.Zero(t, conns)
	})

	t.Run("accepts a bounded read pool", func(t *testing.T) {
		t.Setenv(YakitSQLiteProjectReadPoolConnsEnv, "1")
		conns, err := projectDatabaseReadPoolConns()
		require.NoError(t, err)
		require.Equal(t, 1, conns)
	})

	t.Run("invalid values fail closed", func(t *testing.T) {
		for _, value := range []string{"invalid", "-1", "5"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv(YakitSQLiteProjectReadPoolConnsEnv, value)
				_, err := projectDatabaseReadPoolConns()
				require.ErrorContains(t, err, YakitSQLiteProjectReadPoolConnsEnv)
			})
		}
	})
}

func TestProjectSQLiteConcurrentReaderDoesNotBlockWriter(t *testing.T) {
	options := databaseOpenOptions{sqliteMaxOpenConns: 2, sqlitePrivateCache: true}
	db, err := createAndConfigDatabaseWithOptions(filepath.Join(t.TempDir(), "project.db"), options)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Exec("CREATE TABLE flow_probe (id INTEGER PRIMARY KEY, value TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO flow_probe(value) VALUES ('seed')").Error)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, err := db.DB().Conn(ctx)
	require.NoError(t, err)
	writer, err := db.DB().Conn(ctx)
	require.NoError(t, err)
	for _, conn := range []*sql.Conn{reader, writer} {
		var busyTimeout, synchronous, cacheSize int
		var journalMode string
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout))
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous))
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA cache_size").Scan(&cacheSize))
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode))
		require.Equal(t, 10000, busyTimeout)
		require.Equal(t, 1, synchronous, "SQLite must be opened with synchronous=NORMAL, not OFF")
		require.Equal(t, 8000, cacheSize)
		require.Equal(t, "wal", journalMode)
	}

	readTx, err := reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	defer readTx.Rollback()
	var count int
	require.NoError(t, readTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM flow_probe").Scan(&count))
	require.Equal(t, 1, count)

	_, err = writer.ExecContext(ctx, "INSERT INTO flow_probe(value) VALUES ('concurrent')")
	require.NoError(t, err)
	require.NoError(t, readTx.Commit())
	require.NoError(t, reader.Close())
	require.NoError(t, writer.Close())
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM flow_probe").Row().Scan(&count))
	require.Equal(t, 2, count)
}

func TestProjectSQLiteDedicatedReaderKeepsWriterSerialized(t *testing.T) {
	t.Setenv(YakitSQLiteProjectMaxOpenConnsEnv, "1")
	t.Setenv(YakitSQLiteProjectReadPoolConnsEnv, "1")
	path := filepath.Join(t.TempDir(), "project.db")
	writerDB, err := CreateProjectDatabase(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writerDB.Close() })
	readerDB, err := CreateProjectDatabaseReadOnly(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readerDB.Close() })
	require.Equal(t, 1, writerDB.DB().Stats().MaxOpenConnections)
	require.Equal(t, 1, readerDB.DB().Stats().MaxOpenConnections)

	var queryOnly int
	require.NoError(t, readerDB.Raw("PRAGMA query_only").Row().Scan(&queryOnly))
	require.Equal(t, 1, queryOnly)
	require.Error(t, readerDB.Exec("CREATE TABLE forbidden_write (id INTEGER)").Error)

	require.NoError(t, writerDB.Exec("CREATE TABLE flow_probe (id INTEGER PRIMARY KEY, value TEXT)").Error)
	require.NoError(t, writerDB.Exec("INSERT INTO flow_probe(value) VALUES ('seed')").Error)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, err := readerDB.DB().Conn(ctx)
	require.NoError(t, err)
	defer reader.Close()
	readTx, err := reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	defer readTx.Rollback()
	var count int
	require.NoError(t, readTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM flow_probe").Scan(&count))
	require.Equal(t, 1, count)

	require.NoError(t, writerDB.Exec("INSERT INTO flow_probe(value) VALUES ('concurrent')").Error)
	require.NoError(t, readTx.Commit())
	require.NoError(t, writerDB.Raw("SELECT COUNT(*) FROM flow_probe").Row().Scan(&count))
	require.Equal(t, 2, count)
}
