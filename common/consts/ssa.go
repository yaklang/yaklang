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

const (
	CONST_SSA_DATABASE_RAW = "SSA_DATABASE_RAW"
)

var (
	SSA_PROJECT_YAKIT_DB_RAW = "default-yakitssa.db"
	SSA_PROJECT_DB_RAW       = "default-yakssa.db"
	SSA_PROJECT_DB_DIALECT   = SQLiteExtend
	ssaDatabase              *gorm.DB
)

const (
	YAK_SSA_PROJECT_DB_DEFAULT = "default-yakssa.db"
	YAK_SSA_PROJECT_DB_DIALECT = SQLiteExtend
)

func GetSSADatabaseInfoFromEnv(isIRify bool) string {
	raw := os.Getenv(CONST_SSA_DATABASE_RAW)
	if raw == "" {
		if isIRify {
			raw = SSA_PROJECT_DB_RAW
		} else {
			raw = SSA_PROJECT_YAKIT_DB_RAW
		}
	}
	return raw
}

func GetSSADataBaseInfo() (string, string) {
	if !filepath.IsAbs(SSA_PROJECT_YAKIT_DB_RAW) {
		SSA_PROJECT_YAKIT_DB_RAW = filepath.Join(GetDefaultYakitBaseDir(), SSA_PROJECT_YAKIT_DB_RAW)
	}
	return SSA_PROJECT_DB_DIALECT, SSA_PROJECT_YAKIT_DB_RAW
}

func parseDatabaseURL(raw string) (string, string) {
	// mysql://root:password@tcp()/<dbname>?charset=utf8&parseTime=True&loc=Local
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) == 2 {
		dialect := strings.ToLower(parts[0])
		connectionDetails := parts[1]
		switch dialect {
		case "sqlite", "sqlite3":
			// Assuming SQLiteExtend is a defined constant for the dialect string
			return SQLiteExtend, connectionDetails
		case "mysql":
			// Assuming MySQL is a defined constant for the dialect string
			return MySQL, connectionDetails
		// Add other supported dialects here
		// case "postgres", "postgresql":
		// 	return PostgreSQL, connectionDetails // Assuming PostgreSQL constant exists
		default:
			// Default case for unknown schemes: treat as SQLite with the full raw string as path
			return SQLiteExtend, raw
		}
	} else {
		// Assume raw is a file path for SQLite if no scheme is provided
		return SQLiteExtend, raw
	}
}

func SetSSADatabaseInfo(raw string) {
	if raw == "" {
		return
	}

	dialect, connectionDetails := parseDatabaseURL(raw)
	SSA_PROJECT_DB_DIALECT = dialect
	SSA_PROJECT_YAKIT_DB_RAW = connectionDetails
}

func SetGormSSAProjectDatabaseByInfo(raw string) error {
	dialect, path := parseDatabaseURL(raw)
	db, err := CreateSSAProjectDatabase(dialect, path)
	if err != nil {
		return err
	}
	ssaDatabase = db
	return nil
}

func CreateSSAProjectDatabase(dialect, path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path, dialect)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_SSA_DATABASE)
	configureAndOptimizeDB(dialect, db)
	return db, nil
}

func GetTempSSADataBase() (*gorm.DB, error) {
	path := filepath.Join(GetDefaultYakitBaseTempDir(), fmt.Sprintf("temp-yakssa-%s.db", uuid.NewString()))
	return CreateSSAProjectDatabase(SQLiteExtend, path)
}

func SetGormSSAProjectDatabase(db *gorm.DB) {
	ssaDatabase = db
}

func GetGormDefaultSSADataBase() *gorm.DB {
	if ssaDatabase == nil {
		initYakitDatabase()
	}
	return ssaDatabase
}
