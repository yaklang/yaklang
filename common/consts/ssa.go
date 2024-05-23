package consts

import (
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

var (
	YAK_SSA_PROJECT_DB_NAME = ""
	ssaDatabase             *gorm.DB
	initSSADatabaseOnce     = new(sync.Once)
)

func GetSSAProjectDBNameDefault() string {
	filename := "default-yakssa.db"
	return filepath.Join(GetDefaultYakitBaseDir(), filename)
}

func SetSSADataBaseName(name string) {
	YAK_SSA_PROJECT_DB_NAME = name
}

func GetDefaultSSADataBase() string {
	if YAK_SSA_PROJECT_DB_NAME == "" {
		return GetSSAProjectDBNameDefault()
	}
	return YAK_SSA_PROJECT_DB_NAME
}

func initSSADatabase() {
	initSSADatabaseOnce.Do(func() {
		var err error
		log.Infof("init ssa database: %s", GetDefaultSSADataBase())
		ssaDatabase, err = createAndConfigDatabase(GetDefaultSSADataBase(), SQLiteExtend)
		if err != nil {
			log.Errorf("create ssa database err: %v", err)
		}
		schema.AutoMigrate(ssaDatabase, schema.KEY_SCHEMA_SSA_DATABASE)
	})
}

func GetGormDefaultSSADataBase() *gorm.DB {
	initSSADatabase()
	return ssaDatabase
}
