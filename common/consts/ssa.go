package consts

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	_ "github.com/jinzhu/gorm/dialects/mysql"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
)

type Language string

const EmbedSfBuildInRuleKey = "e18179b8cbbea727589cd210c8204306"
const (
	Yak     Language = "yak"
	JS      Language = "js"
	PHP     Language = "php"
	JAVA    Language = "java"
	GO      Language = "golang"
	TS      Language = "ts"
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
	YAK_SSA_PROJECT_DB_PATH = "default-yakssa.db"
	ssaDatabase             *gorm.DB
)

const (
	YAK_SSA_PROJECT_DB_DEFAULT = "default-yakssa.db"
)

func GetSSADataBasePathDefault(base string) string {
	if filepath.IsAbs(YAK_SSA_PROJECT_DB_PATH) {
		return YAK_SSA_PROJECT_DB_PATH
	}
	return filepath.Join(base, YAK_SSA_PROJECT_DB_PATH)
}

func SetGormSSAProjectDatabaseByPath(path string) error {
	db, err := CreateSSAProjectDatabase(path)
	if err != nil {
		return err
	}
	ssaDatabase = db
	return nil
}

func SetGormSSAProjectDatabaseByDB(db *gorm.DB) {
	ssaDatabase = db
}

func SetSSAProjectDatabasePath(path string) {
	if path == "" {
		return
	}
	YAK_SSA_PROJECT_DB_PATH = path
}

func CreateSSAProjectDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path, SQLiteExtend)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_SSA_DATABASE)
	return db, nil
}

func GetTempSSADataBase() (*gorm.DB, error) {
	path := filepath.Join(GetDefaultYakitBaseTempDir(), fmt.Sprintf("temp-yakssa-%s.db", uuid.NewString()))
	return CreateSSAProjectDatabase(path)
}

func GetGormDefaultSSADataBase() *gorm.DB {
	initYakitDatabase()
	return ssaDatabase
}
