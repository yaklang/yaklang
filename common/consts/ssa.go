package consts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"

	"github.com/yaklang/yaklang/common/log"
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

// GetCanonicalDefaultSSADatabasePath returns the configured legacy/default SSA IR database path.
// Unlike GetSSADataBaseInfo, this is stable while dedicated project databases are opened.
func GetCanonicalDefaultSSADatabasePath() string {
	raw := GetSSADatabaseInfoFromEnv()
	if raw == "" {
		return filepath.Join(GetDefaultYakitBaseDir(), SSA_PROJECT_Default_DB_DEFAULT)
	}
	dialect, path := parseDatabaseURL(raw)
	if dialect == SQLiteExtend || dialect == SQLite {
		if !filepath.IsAbs(path) {
			path = filepath.Join(GetDefaultYakitBaseDir(), path)
		}
	}
	return path
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
	if raw == "" {
		return utils.Errorf("set SSA database failed: path is empty")
	}
	db, err := GetOrOpenSSADB(raw)
	if err != nil {
		return err
	}
	setActiveSSADatabase(db, raw)
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
	setActiveSSADatabase(db, "")
}

func isGormSSAProjectDatabaseUsable() bool {
	if ssaDatabase == nil {
		return false
	}
	sqlDB := ssaDatabase.DB()
	if sqlDB == nil {
		return false
	}
	return sqlDB.Ping() == nil
}

// CloseGormSSAProjectDatabase clears the active SSA IR handle without closing cached databases.
func CloseGormSSAProjectDatabase() error {
	ssaDatabase = nil
	schema.SetDefaultSSADatabase(nil)
	return nil
}

// GetActiveSSADatabaseRawPath returns the connection target of the current SSA database.
func GetActiveSSADatabaseRawPath() string {
	_, path := GetSSADataBaseInfo()
	return path
}

// IsGormSSAProjectDatabaseOpen reports whether the global SSA IR database handle is open.
func IsGormSSAProjectDatabaseOpen() bool {
	return ssaDatabase != nil
}

func ensureGormSSAProjectDatabase() {
	if isGormSSAProjectDatabaseUsable() {
		return
	}
	ssaDatabase = nil
	schema.SetDefaultSSADatabase(nil)
	initYakitDatabase()
	if isGormSSAProjectDatabaseUsable() {
		return
	}
	path := GetCanonicalDefaultSSADatabasePath()
	if err := SetGormSSAProjectDatabaseByInfo(path); err != nil {
		log.Errorf("reopen default SSA database failed: %s", err)
	}
}

func GetGormSSAProjectDataBase() *gorm.DB {
	ensureGormSSAProjectDatabase()
	return ssaDatabase
}

// RestoreDefaultSSAProjectDatabase reopens the process default SSA IR database.
func RestoreDefaultSSAProjectDatabase() error {
	return SetGormSSAProjectDatabaseByInfo(GetCanonicalDefaultSSADatabasePath())
}
