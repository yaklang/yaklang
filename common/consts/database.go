package consts

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	_ "github.com/go-sql-driver/mysql"
	"github.com/mattn/go-sqlite3"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	SQLiteExtend = "sqlite3_extended"
	MySQL        = "mysql"
	Postgres     = "postgres"
	SQLite       = "sqlite3"

	DEFAULT_DRIVER = SQLite
)

var RegisterDriverOnce = new(sync.Once)

type databaseOpenOptions struct {
	sqliteMaxOpenConns int
	sqlitePrivateCache bool
	// sqliteSynchronous is the SQLite PRAGMA/DSN synchronous mode.
	// Empty keeps the generic default (OFF).
	sqliteSynchronous string
	// sqliteTxLock is the SQLite DSN _txlock value.
	// Empty omits _txlock from the DSN.
	sqliteTxLock string
}

func defaultDatabaseOpenOptions() databaseOpenOptions {
	return databaseOpenOptions{sqliteMaxOpenConns: 1}
}

func DeleteDatabaseFile(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		return err
	}
	// delete wal log and shm file
	os.RemoveAll(path + "-wal")
	os.RemoveAll(path + "-shm")
	return nil
}

func registerDriver() {
	{
		sqlDialect, _ := gorm.GetDialect(SQLite)
		gorm.RegisterDialect(SQLiteExtend, sqlDialect)
	}

	regex := func(re, s string) (bool, error) {
		return regexp.MatchString(re, s)
	}
	sleep := func(s int) bool {
		time.Sleep(time.Duration(s) * time.Second)
		return true
	}
	sql.Register(SQLiteExtend,
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				err := conn.RegisterFunc("md5", func(s any) any {
					return codec.Md5(s)
				}, true)
				if err != nil {
					return err
				}

				err = conn.RegisterFunc("regexp", regex, true)
				if err != nil {
					return err
				}
				err = conn.RegisterFunc("sleep", sleep, true)
				if err != nil {
					return err
				}
				return nil
			},
		})
}

func GetTempTestDatabase() (string, *gorm.DB, error) {
	dbPath := filepath.Join(GetDefaultYakitBaseTempDir(), fmt.Sprintf("temp-yaktest-%s.db", uuid.NewString()))
	db, err := createAndConfigDatabase(dbPath, SQLiteExtend)
	if err != nil {
		return "", nil, err
	}
	return dbPath, db, nil
}

func createAndConfigDatabase(path string, drivers ...string) (*gorm.DB, error) {
	return createAndConfigDatabaseWithOptions(path, defaultDatabaseOpenOptions(), drivers...)
}

func createAndConfigDatabaseWithOptions(path string, options databaseOpenOptions, drivers ...string) (*gorm.DB, error) {
	if path == "" {
		return nil, utils.Errorf("database path is empty")
	}
	// register sql-extend driver
	RegisterDriverOnce.Do(registerDriver)

	driver := DEFAULT_DRIVER
	if len(drivers) > 0 {
		driver = drivers[0]
	} else {
	}

	purePath := path
	switch driver {
	case SQLiteExtend, SQLite:
		cacheMode := "shared"
		if options.sqlitePrivateCache {
			cacheMode = "private"
		}
		syncMode := "OFF"
		if options.sqliteSynchronous != "" {
			syncMode = options.sqliteSynchronous
		}
		params := url.Values{
			"mode":          []string{"rwc"},
			"cache":         []string{cacheMode},
			"_busy_timeout": []string{"10000"},
			"_synchronous":  []string{syncMode},
			"_cache_size":   []string{"8000"},
		}
		if options.sqliteTxLock != "" {
			params.Set("_txlock", options.sqliteTxLock)
		}
		path = fmt.Sprintf("%s?%s", path, params.Encode())
	case MySQL:
		path = fmt.Sprintf("%s?charset=utf8mb4&parseTime=True&loc=Local", path)
	default:
	}

	db, err := gorm.Open(driver, path)
	if err != nil && (driver == SQLite || driver == SQLiteExtend) {
		log.Warnf("open database[%s] with driver[%s] failed: %s, try to check and fix it", purePath, driver, err)
		err = checkAndTryFixDatabase(purePath)
		if err != nil {
			return nil, err
		}
		db, err = gorm.Open(driver, path)
	}
	if err != nil {
		return nil, err
	}
	configureAndOptimizeDBWithOptions(driver, db, options)
	return db, nil
}

func createSQLiteReadOnlyDatabase(path string, maxOpenConns int) (*gorm.DB, error) {
	if path == "" {
		return nil, utils.Errorf("database path is empty")
	}
	if maxOpenConns < 1 {
		return nil, utils.Errorf("read-only SQLite max open connections must be positive")
	}
	RegisterDriverOnce.Do(registerDriver)
	params := url.Values{
		"mode":          []string{"ro"},
		"cache":         []string{"private"},
		"_query_only":   []string{"1"},
		"_busy_timeout": []string{"10000"},
		"_cache_size":   []string{"8000"},
	}
	db, err := gorm.Open(SQLite, fmt.Sprintf("%s?%s", path, params.Encode()))
	if err != nil {
		return nil, err
	}
	db.DB().SetConnMaxLifetime(time.Hour)
	db.DB().SetMaxOpenConns(maxOpenConns)
	db.DB().SetMaxIdleConns(maxOpenConns)
	if err := db.DB().Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configureAndOptimizeDB(drive string, db *gorm.DB) {
	configureAndOptimizeDBWithOptions(drive, db, defaultDatabaseOpenOptions())
}

func configureAndOptimizeDBWithOptions(drive string, db *gorm.DB, options databaseOpenOptions) {
	// reference: https://stackoverflow.com/questions/35804884/sqlite-concurrent-writing-performance
	db.DB().SetConnMaxLifetime(time.Hour)
	// SQLite must keep a single writer connection to avoid "database is locked" under concurrent writes.
	// For server databases (MySQL/Postgres), allow a small pool for throughput.
	switch drive {
	case SQLiteExtend, SQLite:
		maxOpenConns := options.sqliteMaxOpenConns
		if maxOpenConns < 1 {
			maxOpenConns = 1
		}
		db.DB().SetMaxOpenConns(maxOpenConns)
		db.DB().SetMaxIdleConns(maxOpenConns)
	default:
		db.DB().SetMaxOpenConns(20)
		db.DB().SetMaxIdleConns(10)
	}

	if drive == SQLiteExtend || drive == SQLite {
		syncMode := "OFF"
		if options.sqliteSynchronous != "" {
			syncMode = options.sqliteSynchronous
		}
		db.Exec("PRAGMA synchronous = " + syncMode + ";")
		// db.Exec("PRAGMA locking_mode = EXCLUSIVE;")
		// set journal_mode for write speed
		db.Exec("PRAGMA journal_mode = WAL;")
		db.Exec("PRAGMA temp_store = MEMORY;")
		db.Exec("PRAGMA cache_size = 8000;")
		db.Exec("PRAGMA busy_timeout = 10000;")
	}
}

func checkAndTryFixDatabase(path string) error {
	baseDir := filepath.Dir(path)
	if exist, err := utils.PathExists(baseDir); err != nil {
		log.Errorf("check dir[%v] if exist failed: %s", baseDir, err)
	} else if !exist {
		err = os.MkdirAll(baseDir, 0o777)
		if err != nil {
			log.Errorf("make dir[%v] failed: %s", baseDir, err)
		}
	}

	if runtime.GOOS == "darwin" {
		if permutil.IsFileUnreadAndUnWritable(path) {
			log.Infof("打开数据库[%s]遇到权限问题，尝试自主修复数据库权限错误", path)
			if err := permutil.DarwinSudo(
				"chmod +rw "+strconv.Quote(path),
				permutil.WithVerbose(fmt.Sprintf("修复 Yakit 数据库[%s]权限", path)),
			); err != nil {
				log.Errorf("sudo chmod +rw %v failed: %v", strconv.Quote(path), err)
			}
			if permutil.IsFileUnreadAndUnWritable(path) {
				log.Errorf("No Permission for %v", path)
			}
		}
	}
	{
		if utils.IsDir(path) {
			if utils.IsSubPath(path, GetDefaultYakitBaseDir()) {
				os.RemoveAll(path)
			} else {
				path = fmt.Sprintf("%s-%s.db", path, utils.RandNumberStringBytes(5))
			}
		}
	}
	err := os.Chmod(path, 0o666)
	if err != nil {
		log.Errorf("chmod +rw failed: %s", err)
	}
	return nil
}
