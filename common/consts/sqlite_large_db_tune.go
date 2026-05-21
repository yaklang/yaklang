package consts

import (
	"fmt"
	"os"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// sqliteDatabaseOnDiskBytes returns main db + wal file size (best-effort).
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

// TuneSQLiteByDatabaseFileSize raises cache/mmap for large project databases to reduce slow inserts.
// Negative cache_size values are KiB units per SQLite semantics.
func TuneSQLiteByDatabaseFileSize(db *gorm.DB, pureDBPath string) {
	if db == nil || pureDBPath == "" {
		return
	}
	size := sqliteDatabaseOnDiskBytes(pureDBPath)
	if size <= 0 {
		return
	}

	var (
		cacheSizeKiB int64
		mmapSize     int64
		walCheckpoint int
	)
	switch {
	case size >= 2*1024*1024*1024:
		cacheSizeKiB = 262144 // 256 MiB
		mmapSize = 512 * 1024 * 1024
		walCheckpoint = 20000
	case size >= 1024*1024*1024:
		cacheSizeKiB = 131072 // 128 MiB
		mmapSize = 256 * 1024 * 1024
		walCheckpoint = 15000
	case size >= 512*1024*1024:
		cacheSizeKiB = 65536 // 64 MiB
		mmapSize = 128 * 1024 * 1024
		walCheckpoint = 10000
	case size >= 128*1024*1024:
		cacheSizeKiB = 32768 // 32 MiB
		mmapSize = 64 * 1024 * 1024
		walCheckpoint = 8000
	default:
		return
	}

	db.Exec(fmt.Sprintf("PRAGMA cache_size = %d;", -cacheSizeKiB))
	db.Exec(fmt.Sprintf("PRAGMA mmap_size = %d;", mmapSize))
	db.Exec(fmt.Sprintf("PRAGMA wal_autocheckpoint = %d;", walCheckpoint))
	// Cap WAL growth on disk during heavy scan/MITM writes (bytes, default 64MB if unsupported).
	db.Exec("PRAGMA journal_size_limit = 67108864;")
	log.Infof(
		"sqlite tuned for large project db (%s): cache=%s mmap=%s wal_autocheckpoint=%d",
		utils.ByteSize(uint64(size)),
		utils.ByteSize(uint64(cacheSizeKiB*1024)),
		utils.ByteSize(uint64(mmapSize)),
		walCheckpoint,
	)
}
