package consts

import (
	"path/filepath"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

var (
	YAK_SSA_PROJECT_DB_NAME = "default-yakssa.db"
)

const (
	YAK_SSA_PROJECT_DB_NAME_DEFAULT = "default-yakssa.db"
)

func SetSSADataBaseName(name string) {
	if name == "" {
		YAK_SSA_PROJECT_DB_NAME = YAK_SSA_PROJECT_DB_NAME_DEFAULT
		return
	}
	YAK_SSA_PROJECT_DB_NAME = name
}

func GetDefaultSSADataBase() string {
	if filepath.IsAbs(YAK_SSA_PROJECT_DB_NAME) {
		return YAK_SSA_PROJECT_DB_NAME
	}
	return filepath.Join(GetDefaultYakitBaseDir(), YAK_SSA_PROJECT_DB_NAME)
}

func GetGormDefaultSSADataBase() *gorm.DB {
	db, err := createAndConfigDatabase(GetDefaultSSADataBase(), SQLiteExtend)
	if err != nil {
		log.Errorf("create ssa database err: %v", err)
	}
	return db
}
