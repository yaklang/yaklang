package consts

import (
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type Language string

const (
	Yak  Language = "yak"
	JS   Language = "js"
	PHP  Language = "php"
	JAVA Language = "java"
	GO   Language = "go"
)

var (
	YAK_SSA_PROJECT_DB_PATH = ""
	ssaDatabase             *gorm.DB
	initSSADatabaseOnce     *sync.Once
)

func init() {
	resetSSADB()
}

func resetSSADB() {
	if ssaDatabase != nil {
		ssaDatabase.Close()
		ssaDatabase = nil
	}
	initSSADatabaseOnce = new(sync.Once)
}

func GetSSADataBasePathDefault() string {
	filename := "default-yakssa.db"
	return filepath.Join(GetDefaultYakitBaseDir(), filename)
}

func SetSSADataBasePath(path string) {
	YAK_SSA_PROJECT_DB_PATH = path
	resetSSADB()
}

func GetSSADataBasePath() string {
	if YAK_SSA_PROJECT_DB_PATH == "" {
		return GetSSADataBasePathDefault()
	}
	return YAK_SSA_PROJECT_DB_PATH
}

func initSSADatabase() {
	initSSADatabaseOnce.Do(func() {
		var err error
		ssaDatabase, err = createAndConfigDatabase(GetSSADataBasePath(), SQLiteExtend)
		if err != nil {
			log.Errorf("create ssa database err: %v", err)
		}
		log.Infof("init ssa database: %s", GetSSADataBasePath())
		schema.AutoMigrate(ssaDatabase, schema.KEY_SCHEMA_SSA_DATABASE)
	})
}

func GetGormDefaultSSADataBase() *gorm.DB {
	initSSADatabase()
	return ssaDatabase
}
