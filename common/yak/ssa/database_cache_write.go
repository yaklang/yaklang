package ssa

import (
	"errors"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// SQLite permits one write transaction at a time. The cache's instruction,
// type, source, index and offset savers are independent goroutines; allowing
// them to race for an immediate write transaction can exhaust busy_timeout and
// permanently poison a saver. Serialize only their transactions, leaving the
// connection pool available for concurrent reads. Non-SQLite backends retain
// their normal write concurrency.
func cacheWriteTransaction(db *gorm.DB, work func(*gorm.DB) error) error {
	if db == nil {
		return errors.New("SSA cache write database is nil")
	}
	return withCacheWriteLock(db.Dialect().GetName(), func() error {
		return utils.GormTransaction(db, work)
	})
}

func withCacheWriteLock(dialect string, work func() error) error {
	return ssadb.WithSQLiteWriteLock(dialect, work)
}
