package consts

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	SQLiteExtend = "sqlite3_extended"
	MySQL        = "mysql"
	Postgres     = "postgres"
	SQLite       = "sqlite3"

	DEFAULT_DRIVER = SQLite
)

var RegisterDriverOnce = new(sync.Once)

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

// OpenDatabaseByDriver opens a database with gorm v2 using the same DSN
// formatting and optimization settings used throughout the project.
func OpenDatabaseByDriver(driver string, source string) (*gorm.DB, error) {
	return createAndConfigDatabase(source, driver)
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
	if path == "" {
		return nil, utils.Errorf("database path is empty")
	}
	// register sql-extend driver (custom md5/regexp/sleep functions)
	RegisterDriverOnce.Do(registerDriver)

	driver := DEFAULT_DRIVER
	if len(drivers) > 0 {
		driver = drivers[0]
	}

	purePath := path
	switch driver {
	case SQLiteExtend, SQLite:
		path = fmt.Sprintf("%s?cache=shared&mode=rwc", path)
	case MySQL:
		path = fmt.Sprintf("%s?charset=utf8mb4&parseTime=True&loc=Local", path)
	default:
	}

	var dialector gorm.Dialector
	switch driver {
	case SQLiteExtend:
		// Use the custom database/sql driver that registers md5/regexp/sleep.
		dialector = sqlite.New(sqlite.Config{DriverName: SQLiteExtend, DSN: path})
	case SQLite:
		dialector = sqlite.Open(path)
	case MySQL:
		dialector = mysql.Open(path)
	case Postgres:
		dialector = postgres.Open(path)
	default:
		return nil, utils.Errorf("unsupported database driver: %s", driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil && (driver == SQLite || driver == SQLiteExtend) {
		log.Warnf("open database[%s] with driver[%s] failed: %s, try to check and fix it", purePath, driver, err)
		err = checkAndTryFixDatabase(purePath)
		if err != nil {
			return nil, err
		}
		db, err = gorm.Open(dialector, &gorm.Config{})
	}
	if err != nil {
		return nil, err
	}
	configureAndOptimizeDB(driver, db)
	return db, nil
}

// CloseGormDB closes the underlying *sql.DB of a *gorm.DB.
// gorm v2 no longer exposes db.Close() directly.
func CloseGormDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func configureAndOptimizeDB(drive string, db *gorm.DB) {
	// reference: https://stackoverflow.com/questions/35804884/sqlite-concurrent-writing-performance
	sqlDB, err := db.DB()
	if err != nil {
		log.Warnf("get underlying sql.DB failed: %s", err)
		return
	}
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetMaxIdleConns(10)
	// SQLite must keep a single writer connection to avoid "database is locked" under concurrent writes.
	// For server databases (MySQL/Postgres), allow a small pool for throughput.
	switch drive {
	case SQLiteExtend, SQLite:
		sqlDB.SetMaxOpenConns(1)
	default:
		sqlDB.SetMaxOpenConns(20)
	}

	if drive == SQLiteExtend || drive == SQLite {
		db.Exec("PRAGMA synchronous = OFF;")
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
