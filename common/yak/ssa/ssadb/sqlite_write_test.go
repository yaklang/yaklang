package ssadb

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func openConcurrentWriteDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sqlite-write-lock.db")
	db, err := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	dialect := db.Dialect().GetName()
	require.Contains(t, []string{"sqlite3", "sqlite3_extended"}, dialect)

	var journal string
	require.NoError(t, db.Raw("PRAGMA journal_mode").Row().Scan(&journal))
	require.Equal(t, "wal", strings.ToLower(journal))

	var busy int
	require.NoError(t, db.Raw("PRAGMA busy_timeout").Row().Scan(&busy))
	require.Equal(t, 10000, busy)

	sqlDB := db.DB()
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	var pooledBusy int
	require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&pooledBusy))
	require.Equal(t, 10000, pooledBusy, "a pooled connection must inherit _busy_timeout from the SSA DSN")
	var pooledJournal string
	require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&pooledJournal))
	require.Equal(t, "wal", strings.ToLower(pooledJournal))

	_, err = sqlDB.Exec(`CREATE TABLE concurrent_write_probe (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		worker INTEGER NOT NULL,
		n INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	return sqlDB, dialect
}

func insertProbeRow(sqlDB *sql.DB, dialect string, worker, n int) error {
	return WithSQLiteWriteLock(dialect, func() error {
		tx, err := sqlDB.Begin()
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO concurrent_write_probe(worker, n) VALUES(?, ?)`, worker, n)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	})
}

func requireNoSQLiteLockError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := strings.ToLower(err.Error())
	require.NotContains(t, msg, "sqlite_busy")
	require.NotContains(t, msg, "database is locked")
	require.NoError(t, err)
}

func TestConcurrentWritesSucceedWithoutBusy(t *testing.T) {
	sqlDB, dialect := openConcurrentWriteDB(t)
	const workers = 8
	const rows = 20

	var wg sync.WaitGroup
	errs := make([]error, workers)
	started := time.Now()
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; n < rows; n++ {
				if err := insertProbeRow(sqlDB, dialect, worker, n); err != nil {
					errs[worker] = err
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	elapsed := time.Since(started)
	t.Logf("SQLITE_WRITE_TIMING concurrent_inserts workers=%d rows=%d elapsed=%s", workers, rows, elapsed)

	for _, err := range errs {
		requireNoSQLiteLockError(t, err)
	}
	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT COUNT(*) FROM concurrent_write_probe`).Scan(&count))
	require.Equal(t, workers*rows, count)
}

func TestConcurrentWriteWaitsForLongTransaction(t *testing.T) {
	sqlDB, dialect := openConcurrentWriteDB(t)
	const waiters = 7
	const hold = 1500 * time.Millisecond

	started := make(chan struct{})
	var startOnce sync.Once
	var wg sync.WaitGroup
	errs := make([]error, waiters+1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs[0] = WithSQLiteWriteLock(dialect, func() error {
			tx, err := sqlDB.Begin()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(`INSERT INTO concurrent_write_probe(worker, n) VALUES(?, ?)`, 0, 0); err != nil {
				_ = tx.Rollback()
				return err
			}
			startOnce.Do(func() { close(started) })
			time.Sleep(hold)
			return tx.Commit()
		})
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("holder did not open its write transaction")
	}

	waitStarted := time.Now()
	for worker := 1; worker <= waiters; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			errs[worker] = insertProbeRow(sqlDB, dialect, worker, 0)
		}(worker)
	}
	wg.Wait()
	elapsed := time.Since(waitStarted)
	t.Logf("SQLITE_WRITE_TIMING long_transaction hold=%s waiters=%d elapsed=%s", hold, waiters, elapsed)

	for _, err := range errs {
		requireNoSQLiteLockError(t, err)
	}
	require.Less(t, elapsed, 9*time.Second, "writers must finish inside busy_timeout instead of failing after it")
	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT COUNT(*) FROM concurrent_write_probe`).Scan(&count))
	require.Equal(t, waiters+1, count)
}
