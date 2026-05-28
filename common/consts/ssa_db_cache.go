package consts

import (
	"path/filepath"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

var (
	ssaDBCacheMu sync.RWMutex
	ssaDBCache   = make(map[string]*gorm.DB)
)

func normalizeSSADBCacheKey(raw string) string {
	if raw == "" {
		return ""
	}
	dialect, path := parseDatabaseURL(raw)
	if dialect == SQLiteExtend || dialect == SQLite {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return dialect + "://" + path
}

// GetOrOpenSSADB returns a cached SSA IR database for raw, opening it when missing.
// Other cached paths stay open (multi-database read).
func GetOrOpenSSADB(raw string) (*gorm.DB, error) {
	if raw == "" {
		return nil, nil
	}
	key := normalizeSSADBCacheKey(raw)
	if key == "" {
		return nil, nil
	}

	ssaDBCacheMu.RLock()
	if db, ok := ssaDBCache[key]; ok && isSSADBUsable(db) {
		ssaDBCacheMu.RUnlock()
		return db, nil
	}
	ssaDBCacheMu.RUnlock()

	ssaDBCacheMu.Lock()
	defer ssaDBCacheMu.Unlock()

	if db, ok := ssaDBCache[key]; ok && isSSADBUsable(db) {
		return db, nil
	}
	if db, ok := ssaDBCache[key]; ok && db != nil {
		_ = db.Close()
		delete(ssaDBCache, key)
	}

	db, err := CreateSSAProjectDatabaseRaw(raw)
	if err != nil {
		return nil, err
	}
	ssaDBCache[key] = db
	return db, nil
}

func isSSADBUsable(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	sqlDB := db.DB()
	if sqlDB == nil {
		return false
	}
	return sqlDB.Ping() == nil
}

// CloseSSADBPath closes and removes a cached SSA database by connection raw/path.
func CloseSSADBPath(raw string) error {
	key := normalizeSSADBCacheKey(raw)
	if key == "" {
		return nil
	}
	ssaDBCacheMu.Lock()
	defer ssaDBCacheMu.Unlock()

	db, ok := ssaDBCache[key]
	if !ok {
		return nil
	}
	delete(ssaDBCache, key)
	if db == nil {
		return nil
	}
	err := db.Close()
	if ssaDatabase == db {
		ssaDatabase = nil
		schema.SetDefaultSSADatabase(nil)
	}
	return err
}

// setActiveSSADatabase sets the process write/read default handle without closing other cache entries.
func setActiveSSADatabase(db *gorm.DB, raw string) {
	if raw != "" {
		SetSSADatabaseInfo(raw)
		registerSSADBInCache(db, raw)
	}
	ssaDatabase = db
	schema.SetDefaultSSADatabase(db)
}

func registerSSADBInCache(db *gorm.DB, raw string) {
	if db == nil || raw == "" {
		return
	}
	key := normalizeSSADBCacheKey(raw)
	if key == "" {
		return
	}
	ssaDBCacheMu.Lock()
	ssaDBCache[key] = db
	ssaDBCacheMu.Unlock()
}
