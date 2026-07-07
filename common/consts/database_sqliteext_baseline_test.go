package consts

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestSQLiteExtendCustomFunctionsBaseline 验证自定义 SQLiteExtend 方言
// （md5 / regexp / sleep）在当前 worktree 基线可用。迁移到 gorm v2 后应继续通过。
func TestSQLiteExtendCustomFunctionsBaseline(t *testing.T) {
	tmpDir := GetDefaultYakitBaseTempDir()
	_ = os.MkdirAll(tmpDir, 0o755)
	dbPath := filepath.Join(tmpDir, "sqliteext-baseline-"+uuid.NewString()+".db")
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

	db, err := createAndConfigDatabase(dbPath, SQLiteExtend)
	if err != nil {
		t.Fatalf("createAndConfigDatabase(SQLiteExtend) failed: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	var md5Res string
	row := db.Raw("SELECT md5(?)", "hello").Row()
	if err := row.Scan(&md5Res); err != nil {
		t.Fatalf("md5 scan failed: %v", err)
	}
	if md5Res == "" {
		t.Fatalf("md5 returned empty")
	}

	var regexpRes int
	row = db.Raw("SELECT regexp(?, ?)", "^he", "hello").Row()
	if err := row.Scan(&regexpRes); err != nil {
		t.Fatalf("regexp scan failed: %v", err)
	}
	if regexpRes != 1 {
		t.Fatalf("regexp match expected 1, got %d", regexpRes)
	}

	if err := db.Exec("SELECT sleep(?)", 0).Error; err != nil {
		t.Fatalf("sleep(0) failed: %v", err)
	}

	// 同时验证 AutoMigrate 和基础 CRUD 在 SQLiteExtend 上可用
	type baselineKV struct {
		ID    uint64 `gorm:"primaryKey"`
		Key   string `gorm:"uniqueIndex"`
		Value string `gorm:"type:text"`
	}
	if err := db.AutoMigrate(&baselineKV{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	if err := db.Create(&baselineKV{Key: "k1", Value: "v1"}).Error; err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	var got baselineKV
	if err := db.Where("key = ?", "k1").First(&got).Error; err != nil {
		t.Fatalf("First failed: %v", err)
	}
	if got.Value != "v1" {
		t.Fatalf("expected v1, got %s", got.Value)
	}
}

// TestTempTestDatabaseBaseline 验证 GetTempTestDatabase 使用 SQLiteExtend 基线行为。
func TestTempTestDatabaseBaseline(t *testing.T) {
	path, db, err := GetTempTestDatabase()
	if err != nil {
		t.Fatalf("GetTempTestDatabase failed: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	defer os.Remove(path)
	defer os.Remove(path + "-wal")
	defer os.Remove(path + "-shm")

	if db.Error != nil {
		t.Fatalf("db has initial error: %v", db.Error)
	}

	var one int
	if err := db.Raw("SELECT 1").Row().Scan(&one); err != nil || one != 1 {
		t.Fatalf("basic query failed: %v / %d", err, one)
	}
}

// Compile-time guard：确保 createAndConfigDatabase 返回 *gorm.DB。
var _ *gorm.DB

// Ensure sql.DB is available for close.
var _ *sql.DB
