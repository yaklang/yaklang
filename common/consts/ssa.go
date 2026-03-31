package consts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/jinzhu/gorm"
)

const EmbedSfBuildInRuleKey = "e18179b8cbbea727589cd210c8204306"

const (
	ENV_SSA_DATABASE_RAW = "SSA_DATABASE_RAW"
	// ENV_SSA_DB_SKIP_MIGRATE disables SSA DB AutoMigrate/patches for this process.
	// This is useful when using a read-only SSA-IR DB DSN on scan-only nodes.
	ENV_SSA_DB_SKIP_MIGRATE = "SSA_DB_SKIP_MIGRATE"
)

var (
	SSA_PROJECT_DB_RAW     = "default-yakssa.db"
	SSA_PROJECT_DB_DIALECT = SQLiteExtend
	ssaDatabase            *gorm.DB
)

const (
	SSA_PROJECT_YAKIT_DB_RAW       = "default-yakitssa.db" // 前端是yakit使用这个名字
	SSA_PROJECT_Default_DB_DEFAULT = "default-yakssa.db"   // yakit和命令行使用这个名字
)

func GetSSADatabaseInfoFromEnv() string {
	raw := os.Getenv(ENV_SSA_DATABASE_RAW)
	return raw
}

func GetSSADataBaseInfo() (string, string) {
	// Only SQLite uses a filesystem path. DSNs (MySQL/Postgres/...) must not be joined with base dir.
	if SSA_PROJECT_DB_DIALECT == SQLiteExtend || SSA_PROJECT_DB_DIALECT == SQLite {
		if !filepath.IsAbs(SSA_PROJECT_DB_RAW) {
			SSA_PROJECT_DB_RAW = filepath.Join(GetDefaultYakitBaseDir(), SSA_PROJECT_DB_RAW)
		}
	}
	return SSA_PROJECT_DB_DIALECT, SSA_PROJECT_DB_RAW
}

// GetSSADataBaseConnString returns the current SSA DB setting in a form that can
// be passed back into SetSSADatabaseInfo/CreateSSAProjectDatabaseRaw.
func GetSSADataBaseConnString() string {
	dialect, raw := GetSSADataBaseInfo()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return raw
	}
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "sqlite", "sqlite3", strings.ToLower(SQLiteExtend):
		return raw
	default:
		return fmt.Sprintf("%s://%s", dialect, raw)
	}
}

func parseDatabaseURL(raw string) (string, string) {
	// mysql://root:password@tcp()/<dbname>?charset=utf8&parseTime=True&loc=Local
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) == 2 {
		dialect := strings.ToLower(parts[0])
		connectionDetails := parts[1]
		switch dialect {
		case "sqlite", "sqlite3":
			// Assuming SQLiteExtend is a defined constant for the dialect string
			return SQLiteExtend, connectionDetails
		case "mysql":
			// Assuming MySQL is a defined constant for the dialect string
			return MySQL, connectionDetails
		case "postgres", "postgresql":
			// Keep the full raw string to preserve URL scheme.
			// lib/pq and gorm both accept URL DSNs like: postgres://user:pass@host:5432/db?sslmode=disable
			return Postgres, raw
		// Add other supported dialects here
		// case "postgres", "postgresql":
		// 	return PostgreSQL, connectionDetails // Assuming PostgreSQL constant exists
		default:
			// Default case for unknown schemes: treat as SQLite with the full raw string as path
			return SQLiteExtend, raw
		}
	} else {
		// Assume raw is a file path for SQLite if no scheme is provided
		return SQLiteExtend, raw
	}
}

func SetSSADatabaseInfo(raw string) {
	if raw == "" {
		return
	}

	dialect, connectionDetails := parseDatabaseURL(raw)
	SSA_PROJECT_DB_DIALECT = dialect
	SSA_PROJECT_DB_RAW = connectionDetails
}

func SetGormSSAProjectDatabaseByInfo(raw string) error {
	db, err := CreateSSAProjectDatabaseRaw(raw)
	if err != nil {
		return err
	}
	ssaDatabase = db
	// 同步更新 schema 包中的默认 SSA 数据库
	schema.SetDefaultSSADatabase(db)
	return nil
}

func CreateSSAProjectDatabaseRaw(raw string) (*gorm.DB, error) {
	dialect, path := parseDatabaseURL(raw)
	return CreateSSAProjectDatabase(dialect, path)
}

func CreateSSAProjectDatabase(dialect, path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path, dialect)
	if err != nil {
		return nil, err
	}
	// SSA-IR DB may be accessed with a read-only credential (scan-only nodes).
	// In that case, AutoMigrate would fail even if the schema already exists.
	if !utils.InterfaceToBoolean(os.Getenv(ENV_SSA_DB_SKIP_MIGRATE)) {
		schema.AutoMigrate(db, schema.KEY_SCHEMA_SSA_DATABASE)
		schema.ApplyPatches(db, schema.KEY_SCHEMA_SSA_DATABASE)
	}
	configureAndOptimizeDB(dialect, db)
	return db, nil
}

func GetTempSSADataBase() (*gorm.DB, error) {
	path := filepath.Join(GetDefaultYakitBaseTempDir(), fmt.Sprintf("temp-yakssa-%s.db", uuid.NewString()))
	return CreateSSAProjectDatabase(SQLiteExtend, path)
}

func SetGormSSAProjectDatabase(db *gorm.DB) {
	ssaDatabase = db
	// 同步更新 schema 包中的默认 SSA 数据库
	schema.SetDefaultSSADatabase(db)
}

func GetGormSSAProjectDataBase() *gorm.DB {
	if ssaDatabase == nil {
		initYakitDatabase()
	}
	return ssaDatabase
}
