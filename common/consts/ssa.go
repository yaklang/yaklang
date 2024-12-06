package consts

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	_ "github.com/jinzhu/gorm/dialects/mysql"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type Language string

const (
	Yak     Language = "yak"
	JS      Language = "js"
	PHP     Language = "php"
	JAVA    Language = "java"
	GO      Language = "golang"
	General Language = "general"
)

func GetAllSupportedLanguages() []Language {
	return []Language{Yak, JS, PHP, JAVA, GO}
}

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

	// use env to config ssa database
	dialect := os.Getenv("YAK_SSA_DATABASE_DIALECT") // dialect
	url := os.Getenv("YAK_SSA_DATABASE_URL")         // url
	if dialect != "" && url != "" {
		db, err := gorm.Open(dialect, url)
		if err != nil {
			log.Errorf("create ssa database err: %v", err)
		} else {
			ssaDatabase = db
			log.Infof("init ssa database:[%s]%s", dialect, url)
		}
	}
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
	initSSADatabaseOnce = new(sync.Once)
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
		// use default sqlite database
		if ssaDatabase == nil {
			if db, err := createAndConfigDatabase(GetSSADataBasePath(), SQLiteExtend); err != nil {
				log.Errorf("create ssa database err: %v", err)
			} else {
				ssaDatabase = db
				log.Infof("init ssa database: %s", GetSSADataBasePath())
			}
		}
		schema.AutoMigrate(ssaDatabase, schema.KEY_SCHEMA_SSA_DATABASE)
	})
}

func GetGormDefaultSSADataBase() *gorm.DB {
	initSSADatabase()
	return ssaDatabase
}
