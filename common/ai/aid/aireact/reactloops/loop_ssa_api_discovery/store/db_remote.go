package store

import (
	"strings"

	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	"github.com/yaklang/yaklang/common/utils"
)

// SessionDBConfig 可选会话库配置：默认 SQLite(workDir)；也可指定 PostgreSQL DSN。
type SessionDBConfig struct {
	Dialect string // sqlite3 | postgres
	DSN     string
	WorkDir string
}

// OpenSessionDBFromConfig 打开 discovery 会话库。未指定 postgres DSN 时行为与 OpenSessionDB 一致。
func OpenSessionDBFromConfig(cfg SessionDBConfig) (*gorm.DB, error) {
	dialect := strings.ToLower(strings.TrimSpace(cfg.Dialect))
	if dialect == "" || dialect == "sqlite" || dialect == "sqlite3" {
		return OpenSessionDB(cfg.WorkDir)
	}
	if dialect != "postgres" && dialect != "postgresql" {
		return nil, utils.Errorf("unsupported session db dialect: %q", cfg.Dialect)
	}
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, utils.Error("postgres session db requires DSN")
	}
	db, err := gorm.Open("postgres", dsn)
	if err != nil {
		return nil, utils.Wrapf(err, "open postgres session db")
	}
	sqlDB := db.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
	}
	if err := AutoMigrate(db); err != nil {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}
	return db, nil
}

// SessionDBRef 返回会话库引用字符串（sqlite 路径或 postgres DSN）。
func SessionDBRef(workDir, postgresDSN string) string {
	if strings.TrimSpace(postgresDSN) != "" {
		return strings.TrimSpace(postgresDSN)
	}
	return DBPath(workDir)
}
