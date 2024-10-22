package consts

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"

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
	GO   Language = "golang"
)

func ValidateLanguage(language string) (Language, error) {
	switch strings.TrimSpace(strings.ToLower(language)) {
	case "yak", "yaklang":
		return Yak, nil
	case "java":
		return JAVA, nil
	case "php":
		return PHP, nil
	case "js", "es", "javascript", "ecmascript", "nodejs", "node", "node.js":
		return JS, nil
	case "go", "golang":
		return GO, nil
	}
	return "", errors.Errorf("unsupported language: %s", language)
}

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

func SetSSADB(db *gorm.DB) {
	ssaDatabase = db
}

func SetSSADataBasePath(path string) {
	if path == "" {
		return
	}
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
		if ssaDatabase == nil {
			var err error
			// if os.Getenv("SSA_DATABASE_PATH") != "" && os.Getenv("SSA_DATABASE_DRIVER") != "" {
			// ssaDatabase, err = createAndConfigDatabase(os.Getenv("SSA_DATABASE_PATH"), os.Getenv("SSA_DATABASE_DRIVER"))

			// connectStr := "host=127.0.0.1 user=postgres password=password dbname=postgres port=5432 sslmode=disable TimeZone=Asia/Shanghai"
			// ssaDatabase, err = gorm.Open("postgres", connectStr)

			connectStr := "root:password@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=True&loc=Local"
			ssaDatabase, err = gorm.Open("mysql", connectStr)

			// ssaDatabase, err = createAndConfigDatabase()
			// } else {
			// 	ssaDatabase, err = createAndConfigDatabase(GetSSADataBasePath(), SQLiteExtend)
			// }
			if err != nil {
				log.Errorf("create ssa database err: %v", err)
			}
		}
		log.Infof("init ssa database: %s", GetSSADataBasePath())
		schema.AutoMigrate(ssaDatabase, schema.KEY_SCHEMA_SSA_DATABASE)
	})
}

func GetGormDefaultSSADataBase() *gorm.DB {
	initSSADatabase()
	return ssaDatabase
}
