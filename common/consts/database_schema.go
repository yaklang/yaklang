package consts

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
)

const (
	KEY_SCHEMA_YAKIT_DATABASE uint8 = iota
	KEY_SCHEMA_PROFILE_DATABASE
	KEY_SCHEMA_CVE_DATABASE
	KEY_SCHEMA_CVE_DESCRIPTION_DATABASE
	KEY_SCHEMA_VULINBOX_DATABASE
	KEY_SCHEMA_SSA_DATABASE
)

var databaseSchemas = map[uint8][]any{
	KEY_SCHEMA_YAKIT_DATABASE:           nil,
	KEY_SCHEMA_PROFILE_DATABASE:         nil,
	KEY_SCHEMA_CVE_DATABASE:             nil,
	KEY_SCHEMA_CVE_DESCRIPTION_DATABASE: nil,
	KEY_SCHEMA_VULINBOX_DATABASE:        nil,
	KEY_SCHEMA_SSA_DATABASE:             nil,
}

func RegisterDatabaseSchema(key uint8, schema ...any) {
	if _, ok := databaseSchemas[key]; !ok {
		panic("Database schema key invalid")
	}

	databaseSchemas[key] = lo.Uniq(append(databaseSchemas[key], schema...))
}

func AutoMigrate(db *gorm.DB, key uint8) {
	if schemas, ok := databaseSchemas[key]; ok {
		if len(schemas) == 0 {
			panic("Database schema is empty")
		}
		db.AutoMigrate(schemas...)
	} else {
		panic("Database schema key invalid")
	}
}
