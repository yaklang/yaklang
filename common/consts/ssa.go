package consts

import (
	"fmt"
	"os"
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

const (
	CONST_SSA_DATABASE_DIALECT = "SSA_DATABASE_DIALECT"
	CONST_SSA_DATABASE_RAW     = "SSA_DATABASE_RAW"
)

var (
	SSA_PROJECT_DB_RAW     = "default-yakssa.db"
	SSA_PROJECT_DB_DIALECT = SQLiteExtend
	ssaDatabase            *gorm.DB
)

const (
	YAK_SSA_PROJECT_DB_DEFAULT = "default-yakssa.db"
	YAK_SSA_PROJECT_DB_DIALECT = SQLiteExtend
)

func GetSSADatabaseInfoFromEnv() (string, string) {
	raw := os.Getenv(CONST_SSA_DATABASE_RAW)
	dialect := os.Getenv(CONST_SSA_DATABASE_DIALECT)
	if raw == "" {
		raw = SSA_PROJECT_DB_RAW
	}
	if dialect == "" {
		dialect = SSA_PROJECT_DB_DIALECT
	}
	return raw, dialect
}

func GetSSADataBaseInfo() (string, string) {
	return SSA_PROJECT_DB_RAW, SSA_PROJECT_DB_DIALECT
}

func SetSSADatabaseInfo(dialect string, raw string) {
	if raw == "" || dialect == "" {
		return
	}
	SSA_PROJECT_DB_RAW = raw
	SSA_PROJECT_DB_DIALECT = dialect
}

func SetGormSSAProjectDatabaseByInfo(dialect string, raw string) error {
	db, err := CreateSSAProjectDatabase(raw, dialect)
	if err != nil {
		return err
	}
	ssaDatabase = db
	return nil
}

func CreateSSAProjectDatabase(path string, dialect string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path, dialect)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_SSA_DATABASE)
	configureAndOptimizeDB(db)
	return db, nil
}

func GetTempSSADataBase() (*gorm.DB, error) {
	path := filepath.Join(GetDefaultYakitBaseTempDir(), fmt.Sprintf("temp-yakssa-%s.db", uuid.NewString()))
	return CreateSSAProjectDatabase(path, SQLiteExtend)
}

func GetGormDefaultSSADataBase() *gorm.DB {
	if ssaDatabase == nil {
		initYakitDatabase()
	}
	return ssaDatabase
}
