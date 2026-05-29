package consts

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// sqliteLargeDBTuneLevel is the PRAGMA cache_size tier (KiB, negated when applied).
// Each level maps to a fixed mmap / wal_autocheckpoint profile.
type sqliteLargeDBTuneLevel int64

const (
	sqliteLargeDBTuneNone sqliteLargeDBTuneLevel = 0
	sqliteLargeDBTune32M  sqliteLargeDBTuneLevel = 32768
	sqliteLargeDBTune64M  sqliteLargeDBTuneLevel = 65536
	sqliteLargeDBTune128M sqliteLargeDBTuneLevel = 131072
	sqliteLargeDBTune256M sqliteLargeDBTuneLevel = 262144
)

var sqliteLargeDBTuneLast sync.Map // abs db path -> sqliteLargeDBTuneLevel

func sqliteTunePathKey(pureDBPath string) string {
	abs, err := filepath.Abs(pureDBPath)
	if err != nil {
		return filepath.Clean(pureDBPath)
	}
	return abs
}

func sqliteDatabaseOnDiskBytes(pureDBPath string) int64 {
	if pureDBPath == "" {
		return 0
	}
	fi, err := os.Stat(pureDBPath)
	if err != nil {
		return 0
	}
	size := fi.Size()
	if wal, err := os.Stat(pureDBPath + "-wal"); err == nil {
		size += wal.Size()
	}
	return size
}

func sqliteLargeDBTuneLevelForSize(size int64) sqliteLargeDBTuneLevel {
	switch {
	case size >= 2*1024*1024*1024:
		return sqliteLargeDBTune256M
	case size >= 1024*1024*1024:
		return sqliteLargeDBTune128M
	case size >= 512*1024*1024:
		return sqliteLargeDBTune64M
	case size >= 128*1024*1024:
		return sqliteLargeDBTune32M
	default:
		return sqliteLargeDBTuneNone
	}
}

func (level sqliteLargeDBTuneLevel) mmapAndWAL() (mmapSize int64, walCheckpoint int) {
	switch level {
	case sqliteLargeDBTune256M:
		return 512 * 1024 * 1024, 20000
	case sqliteLargeDBTune128M:
		return 256 * 1024 * 1024, 15000
	case sqliteLargeDBTune64M:
		return 128 * 1024 * 1024, 10000
	case sqliteLargeDBTune32M:
		return 64 * 1024 * 1024, 8000
	default:
		return 0, 0
	}
}

func applySQLiteLargeDBTune(db *gorm.DB, level sqliteLargeDBTuneLevel) {
	mmapSize, walCheckpoint := level.mmapAndWAL()
	db.Exec(fmt.Sprintf("PRAGMA cache_size = %d;", -int64(level)))
	db.Exec(fmt.Sprintf("PRAGMA mmap_size = %d;", mmapSize))
	db.Exec(fmt.Sprintf("PRAGMA wal_autocheckpoint = %d;", walCheckpoint))
	db.Exec("PRAGMA journal_size_limit = 67108864;")
}

// TuneSQLiteByDatabaseFileSize raises cache/mmap for large project databases to reduce slow inserts.
// Returns true when PRAGMAs were applied and logged; false when skipped (below threshold or same tier as last time).
func TuneSQLiteByDatabaseFileSize(db *gorm.DB, pureDBPath string) bool {
	if db == nil || pureDBPath == "" {
		return false
	}
	size := sqliteDatabaseOnDiskBytes(pureDBPath)
	level := sqliteLargeDBTuneLevelForSize(size)
	if level == sqliteLargeDBTuneNone {
		return false
	}

	pathKey := sqliteTunePathKey(pureDBPath)
	if prev, ok := sqliteLargeDBTuneLast.Load(pathKey); ok && prev.(sqliteLargeDBTuneLevel) == level {
		return false
	}

	applySQLiteLargeDBTune(db, level)
	sqliteLargeDBTuneLast.Store(pathKey, level)
	mmapSize, walCheckpoint := level.mmapAndWAL()
	log.Infof(
		"sqlite tuned for large project db (%s): cache=%s mmap=%s wal_autocheckpoint=%d",
		utils.ByteSize(uint64(size)),
		utils.ByteSize(uint64(int64(level)*1024)),
		utils.ByteSize(uint64(mmapSize)),
		walCheckpoint,
	)
	return true
}
