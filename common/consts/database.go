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

	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	"github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	SQLiteExtend = "sqlite3_extended"
	MySQL        = "mysql"
	SQLite       = "sqlite3"

	DEFAULT_DRIVER = SQLite
)

type DatabasePostInitTag string

const (
	POST_INIT_SYNC_AI_TOOLS           DatabasePostInitTag = "sync-ai-tools"
	POST_INIT_SYNC_BUILDIN_AI_FORGES  DatabasePostInitTag = "sync-buildin-ai-forge"
	POST_INIT_SYNC_YAKIT_CORE_PLUGINS DatabasePostInitTag = "sync-core-plugin-for-yakit"
)

var (
	databaseInitBarrier = utils.NewCondBarrier()
	barriers            = map[DatabasePostInitTag]*utils.Barrier{
		POST_INIT_SYNC_AI_TOOLS:           databaseInitBarrier.CreateBarrier(string(POST_INIT_SYNC_AI_TOOLS)),
		POST_INIT_SYNC_BUILDIN_AI_FORGES:  databaseInitBarrier.CreateBarrier(string(POST_INIT_SYNC_BUILDIN_AI_FORGES)),
		POST_INIT_SYNC_YAKIT_CORE_PLUGINS: databaseInitBarrier.CreateBarrier(string(POST_INIT_SYNC_YAKIT_CORE_PLUGINS)),
	}
)

func DatabaseInitDone(i string) {
	if b, ok := barriers[DatabasePostInitTag(i)]; ok {
		b.Done()
	}
}

func WaitDatabasePostInitAITools() {
	err := databaseInitBarrier.Wait(string(POST_INIT_SYNC_AI_TOOLS))
	if err != nil {
		return
	}
}

func WaitDatabasePostInitBuildinAIForages() {
	err := databaseInitBarrier.Wait(string(POST_INIT_SYNC_BUILDIN_AI_FORGES))
	if err != nil {
		return
	}
}

func WaitAIDatabasePostInit() {
	WaitDatabasePostInitAITools()
	WaitDatabasePostInitBuildinAIForages()
}

var waitCorePluginOnce = utils.NewOnce()

func WaitDatabasePostInitYakitCorePlugins() {
	waitCorePluginOnce.Do(func() {
		err := databaseInitBarrier.Wait(string(POST_INIT_SYNC_YAKIT_CORE_PLUGINS))
		if err != nil {
			return
		}
	})
}

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
	if driver == SQLiteExtend || driver == SQLite {
		path = fmt.Sprintf("%s?cache=shared&mode=rwc", path)
	} else {
		path = fmt.Sprintf("%s?charset=utf8mb4&parseTime=True&loc=Local", path)
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
	configureAndOptimizeDB(driver, db)
	return db, nil
}

func configureAndOptimizeDB(drive string, db *gorm.DB) {
	// reference: https://stackoverflow.com/questions/35804884/sqlite-concurrent-writing-performance
	db.DB().SetConnMaxLifetime(time.Hour)
	db.DB().SetMaxIdleConns(10)
	// set MaxOpenConns to disable connections pool, for write speed and "database is locked" error
	db.DB().SetMaxOpenConns(1)

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
