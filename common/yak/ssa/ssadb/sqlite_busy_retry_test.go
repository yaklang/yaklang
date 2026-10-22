package ssadb

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/utils"
)

func TestGormTransactionRetriesPastSQLiteBusyTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	db, err := gorm.Open("sqlite3", path+"?cache=private&_busy_timeout=200&_txlock=immediate")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.DB().SetMaxOpenConns(2)
	require.NoError(t, db.Exec("PRAGMA journal_mode=WAL").Error)
	require.NoError(t, db.Exec("CREATE TABLE busy_probe (id INTEGER PRIMARY KEY, value TEXT)").Error)

	ctx := context.Background()
	holder, err := db.DB().Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	_, err = holder.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	release := make(chan struct{})
	go func() {
		time.Sleep(900 * time.Millisecond)
		_, _ = holder.ExecContext(ctx, "COMMIT")
		close(release)
	}()

	start := time.Now()
	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO busy_probe(value) VALUES ('ok')").Error
	})
	<-release
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(start), 700*time.Millisecond)

	var n int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM busy_probe").Row().Scan(&n))
	require.Equal(t, 1, n)
}
