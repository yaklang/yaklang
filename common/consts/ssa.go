package consts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	_ "github.com/yaklang/gorm/dialects/mysql"
	_ "github.com/yaklang/gorm/dialects/postgres"

	"github.com/yaklang/yaklang/common/log"
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

// parseDatabaseURL maps SSA_DATABASE_RAW to the gorm driver and the DSN that
// driver receives. A value without a scheme is a SQLite file path; a value
// with "://" must name a driver this build supports.
//
// An unknown scheme is an error. It used to fall back to SQLite with the whole
// string as a path, so a mistyped DSN silently created a local database and
// the process looked healthy while writing IR somewhere nobody expected.
func parseDatabaseURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) != 2 {
		// No scheme: a SQLite database file.
		return SQLiteExtend, raw, nil
	}

	scheme := strings.ToLower(strings.TrimSpace(parts[0]))
	connectionDetails := parts[1]
	switch scheme {
	case "sqlite", "sqlite3", "file":
		return SQLiteExtend, connectionDetails, nil
	case "mysql":
		return MySQL, connectionDetails, nil
	case "postgres", "postgresql":
		// Keep the full raw string to preserve the URL scheme. lib/pq and gorm
		// both accept URL DSNs like postgres://user:pass@host:5432/db?sslmode=disable
		return Postgres, raw, nil
	case "dm":
		return damenggorm.Dialect, raw, nil
	default:
		return "", "", fmt.Errorf(
			"unsupported SSA database URL scheme %q: use a file path or sqlite://, mysql://, postgres://, dm://",
			scheme,
		)
	}
}

// SetSSADatabaseInfo points this process at raw as its SSA database. On error
// the previous configuration is left untouched, so a bad value cannot quietly
// redirect IR to a default local file.
//
// Callers that ignore the returned error still compile; the error is what
// distyak and the CLI surface instead of failing later with an unrelated
// "failed to open" message.
func SetSSADatabaseInfo(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	dialect, connectionDetails, err := parseDatabaseURL(raw)
	if err != nil {
		// The DSN can carry a password, so only the error (scheme only) is logged.
		log.Errorf("invalid SSA database configuration: %v", err)
		return err
	}
	SSA_PROJECT_DB_DIALECT = dialect
	SSA_PROJECT_DB_RAW = connectionDetails
	return nil
}

func SetGormSSAProjectDatabaseByInfo(raw string) error {
	if err := SetSSADatabaseInfo(raw); err != nil {
		return err
	}
	db, err := CreateSSAProjectDatabaseRaw(raw)
	if err != nil {
		return err
	}
	SetGormSSAProjectDatabase(db)
	return nil
}

func CreateSSAProjectDatabaseRaw(raw string) (*gorm.DB, error) {
	dialect, path, err := parseDatabaseURL(raw)
	if err != nil {
		return nil, err
	}
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
		// A failed table creation must not be silent: a database opened without
		// ir_codes or the const pool used to report success and then fail
		// inside unrelated queries. It must not stop the process either -- a
		// read-only or partially migrated database is still usable for the
		// tables that do exist.
		if err := schema.AutoMigrateChecked(db, schema.KEY_SCHEMA_SSA_DATABASE); err != nil {
			log.Errorf("migrate SSA database %q failed, continuing with the tables that exist: %v", path, err)
		}
		schema.ApplyPatches(db, schema.KEY_SCHEMA_SSA_DATABASE)
		if db.Error != nil {
			log.Errorf("patch SSA database %q failed, continuing: %v", path, db.Error)
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
