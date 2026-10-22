package consts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	_ "github.com/yaklang/gorm/dialects/mysql"
	_ "github.com/yaklang/gorm/dialects/postgres"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/damenggorm"

	"github.com/yaklang/gorm"
)

const EmbedSfBuildInRuleKey = "e18179b8cbbea727589cd210c8204306"

const (
	ENV_SSA_DATABASE_RAW = "SSA_DATABASE_RAW"
	// ENV_SSA_DB_SKIP_MIGRATE disables SSA DB AutoMigrate/patches for this process.
	// This is useful when using a read-only SSA-IR DB DSN on scan-only nodes.
	ENV_SSA_DB_SKIP_MIGRATE = "SSA_DB_SKIP_MIGRATE"
)

const (
	ssaSQLiteMaxOpenConns = 8
	ssaSQLiteSynchronous  = "NORMAL"
	ssaSQLiteTxLock       = "immediate"
	// Instruction batches can hold the write lock for well over the generic
	// 10s busy_timeout. SSA waits 60s so the other store retries inside SQLite
	// instead of failing with "database is locked".
	ssaSQLiteBusyTimeoutMS = 60000
	// Without a journal_size_limit the WAL file keeps its high-water size
	// forever. A checkpoint then has to walk that whole file on every pass,
	// which is what pinned scans when the disk was already busy. 64MiB matches
	// the cap the project-database tuner uses.
	ssaSQLiteJournalSizeLimitBytes int64 = 64 * 1024 * 1024
	// The project-database tuner only reaches a 256MB cache after the file
	// is already 2GB, and SSA never called that tuner. Hue's insert working
	// set does not fit in the 8000-page (32MB) default, so every connection
	// starts at the large-database tier.
	ssaSQLiteCacheKiB  int64 = 256 * 1024
	ssaSQLiteMmapBytes int64 = 512 * 1024 * 1024
	// 100000 pages is about 400MB. hue-cache-01 checkpointed every ~80MB and
	// rewrote the growing file (2127s). Turning the checkpoint off entirely
	// (hue-wal-01) let the WAL reach 13GB and made compile reads slower, so the
	// run was already behind at review 100%. 400MB is the middle: fewer copies
	// than 80MB, without a multi-gigabyte WAL for readers to scan.
	ssaSQLiteWALAutocheckpoint = 100000
)

var (
	SSA_PROJECT_DB_RAW     = "default-yakssa.db"
	SSA_PROJECT_DB_DIALECT = SQLiteExtend
	// Plain pointer, same as profileDatabase. initYakitDatabaseOnce is the
	// only guard: the handle is published when it is still unset, and later
	// SetGormSSAProjectDatabase calls are explicit rebinds.
	ssaDatabase *gorm.DB
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
		case "dm":
			return damenggorm.Dialect, raw
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
	SetSSADatabaseInfo(raw)
	db, err := CreateSSAProjectDatabaseRaw(raw)
	if err != nil {
		return err
	}
	SetGormSSAProjectDatabase(db)
	return nil
}

func CreateSSAProjectDatabaseRaw(raw string) (*gorm.DB, error) {
	dialect, path := parseDatabaseURL(raw)
	return CreateSSAProjectDatabase(dialect, path)
}

func CreateSSAProjectDatabase(dialect, path string) (*gorm.DB, error) {
	options := ssaDatabaseOpenOptions()
	// Connection PRAGMAs are applied by the driver hook, which is installed
	// once, so the profile has to be published before the pool opens.
	if dialect == SQLiteExtend || dialect == SQLite {
		SetSQLiteConnectionPragmas(options)
	}
	db, err := createAndConfigDatabaseWithOptions(path, options, dialect)
	if err != nil {
		return nil, err
	}
	// SSA-IR DB may be accessed with a read-only credential (scan-only nodes).
	// In that case, AutoMigrate would fail even if the schema already exists.
	if !utils.InterfaceToBoolean(os.Getenv(ENV_SSA_DB_SKIP_MIGRATE)) {
		if dialect == damenggorm.Dialect {
			if err := schema.AutoMigrateChecked(db, schema.KEY_SCHEMA_SSA_DATABASE); err != nil {
				_ = db.Close()
				return nil, err
			}
		} else {
			schema.AutoMigrate(db, schema.KEY_SCHEMA_SSA_DATABASE)
		}
		schema.ApplyPatches(db, schema.KEY_SCHEMA_SSA_DATABASE)
		if dialect == damenggorm.Dialect && db.Error != nil {
			_ = db.Close()
			return nil, db.Error
		}
	}
	configureAndOptimizeDBWithOptions(dialect, db, options)
	return db, nil
}

func ssaDatabaseOpenOptions() databaseOpenOptions {
	// SSA-only SQLite open settings. Project/general DBs keep
	// defaultDatabaseOpenOptions() (single connection, synchronous=OFF,
	// no _txlock). Injected via databaseOpenOptions so database.go stays generic.
	return databaseOpenOptions{
		sqliteMaxOpenConns:          ssaSQLiteMaxOpenConns,
		sqlitePrivateCache:          true,
		sqliteSynchronous:           ssaSQLiteSynchronous,
		sqliteTxLock:                ssaSQLiteTxLock,
		sqliteBusyTimeoutMS:         ssaSQLiteBusyTimeoutMS,
		sqliteJournalSizeLimitBytes: ssaSQLiteJournalSizeLimitBytes,
		sqliteCacheKiB:              ssaSQLiteCacheKiB,
		sqliteMmapBytes:             ssaSQLiteMmapBytes,
		sqliteWALAutocheckpoint:     ssaSQLiteWALAutocheckpoint,
		sqliteWALAutocheckpointSet:  true,
	}
}

func GetTempSSADataBase() (*gorm.DB, error) {
	path := filepath.Join(GetDefaultYakitBaseTempDir(), fmt.Sprintf("temp-yakssa-%s.db", uuid.NewString()))
	return CreateSSAProjectDatabase(SQLiteExtend, path)
}

func SetGormSSAProjectDatabase(db *gorm.DB) {
	// schema 里的句柄只是这里的镜像，一起赋值。
	schema.SetDefaultSSADatabase(db)
	ssaDatabase = db
}

func GetGormSSAProjectDataBase() *gorm.DB {
	if ssaDatabase == nil {
		initYakitDatabase()
	}
	return ssaDatabase
}
