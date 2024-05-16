package consts

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/permutil"
)

var (
	projectDataBase *gorm.DB
	initOnce        = new(sync.Once)
	profileDatabase *gorm.DB
)

func SetGormProjectDatabase(d *gorm.DB) {
	log.Info("load gorm database connection")
	projectDataBase = d
}

func GetGormProfileDatabase() *gorm.DB {
	initDatabase()
	return profileDatabase
}

func GetGormProjectDatabase() *gorm.DB {
	initDatabase()
	return projectDataBase
}

func InitilizeDatabase(projectDatabase string, profileDBName string) {
	projectName := GetProjectDatabaseNameFromEnv()
	if projectDatabase != "" {
		projectName = projectDatabase
	}
	profileDatabase := GetProfileDatabaseNameFromEnv()
	if profileDBName != "" {
		profileDatabase = profileDBName
	}
	SetDefaultYakitProjectDatabaseName(projectName)
	SetDefaultYakitProfileDatabaseName(profileDatabase)
	initDatabase()
}
func initDatabase() {
	initOnce.Do(func() {
		log.Debug("start to loading gorm project/profile database")
		var (
			err                 error = nil
			baseDir                   = GetDefaultYakitBaseDir()
			projectDatabaseName       = GetDefaultYakitProjectDatabase(baseDir)
			profileDatabaseName       = GetDefaultYakitPluginDatabase(baseDir)
		)

		/* 先创建插件数据库 */
		profileDatabase, err = createAndConfigDatabase(profileDatabaseName)
		if err != nil {
			log.Errorf("init plugin-db[%v] failed: %s", profileDatabaseName, err)
		}
		/* 再创建项目数据库 */
		projectDataBase, err = createAndConfigDatabase(projectDatabaseName)
		if err != nil {
			log.Errorf("init plugin-db[%v] failed: %s", projectDatabaseName, err)
		}

		doDBPatch()
		doDBRiskPatch()
	})
}

func doDBPatch() {
	var err error
	if !projectDataBase.HasTable("http_flows") {
		return
	}
	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_source"
ON "http_flows" (
  "source_type" ASC
);`).Unscoped().Error
	if err != nil {
		log.Warnf("failed to add index on http_flows.source_type: %v", err)
	}

	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_tags"
ON "http_flows" (
  "tags" ASC
);`).Error
	if err != nil {
		log.Warnf("failed to add index on table: http_flows.tags: %v", err)
	}
}

func doDBRiskPatch() {
	if !projectDataBase.HasTable("risks") {
		return
	}
	err := projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_id ON risks(id);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.id: %v", err)
	}
	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_is_read ON risks(is_read);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.is_read: %v", err)
	}

	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_risk_type ON risks(risk_type);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.risk_type: %v", err)
	}

	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_ip ON risks(ip);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.ip: %v", err)
	}
}

func createAndConfigDatabase(path string) (*gorm.DB, error) {
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

	if utils.IsDir(path) {
		os.RemoveAll(path)
	}
	db, err := gorm.Open("sqlite3", fmt.Sprintf("%s?cache=shared&mode=rwc", path))
	if err != nil {
		return nil, err
	}
	configureAndOptimizeDB(db)
	err = os.Chmod(path, 0o666)
	if err != nil {
		log.Errorf("chmod +rw failed: %s", err)
	}
	return db, nil
}

func configureAndOptimizeDB(db *gorm.DB) {
	// reference: https://stackoverflow.com/questions/35804884/sqlite-concurrent-writing-performance
	db.DB().SetConnMaxLifetime(time.Hour)
	db.DB().SetMaxIdleConns(10)
	// set MaxOpenConns to disable connections pool, for write speed and "database is locked" error
	db.DB().SetMaxOpenConns(1)

	db.Exec("PRAGMA synchronous = OFF;")
	// db.Exec("PRAGMA locking_mode = EXCLUSIVE;")
	// set journal_mode for write speed
	db.Exec("PRAGMA journal_mode = WAL;")
	db.Exec("PRAGMA temp_store = MEMORY;")
	db.Exec("PRAGMA cache_size = 8000;")
	db.Exec("PRAGMA busy_timeout = 10000;")
}
